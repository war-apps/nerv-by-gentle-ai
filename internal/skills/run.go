package skills

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
)

// Options configures one Run invocation.
type Options struct {
	// Only restricts processing to these skill names. Empty means every
	// skill in the manifest.
	Only []string
	// DryRun computes the install plan (and every status) without
	// actually running any "npx skills add" install or "gentle-ai sync".
	DryRun bool
	// ConfirmSync is asked, with the missing gentle-ai skill names, before the
	// gentle-ai sync runs; it also rewrites gentle-ai's managed files. Nil or a
	// false answer means the sync never runs and the remedies stay: callers
	// without a terminal leave it nil, interactive callers prompt.
	ConfirmSync func(skills []string) bool
}

// Report is Run's result: the computed status of every processed skill,
// the derived install plan, and (unless DryRun) the outcome of actually
// running it. Result and Sync are zero when DryRun is true. After a sync,
// the gentle-ai Statuses and Plan.Remedies reflect only what is still missing.
type Report struct {
	Statuses []SkillStatus
	Plan     Plan
	Result   InstallResult
	Sync     SyncResult
}

// ErrManifest wraps a failure to load or read the skills manifest, so
// Run's caller can tell it apart from an --only rejection (FilterOnly's
// plain error) — install/configure callers treat every Run error the same
// way (warn and skip), but "nerv skills" reports the two differently
// (exit 2 for an unreadable manifest, exit 1 for an unknown --only name).
type ErrManifest struct{ Err error }

func (e *ErrManifest) Error() string { return e.Err.Error() }
func (e *ErrManifest) Unwrap() error { return e.Err }

// Run is the whole "install/verify skills" use case shared by "nerv
// install", the setup wizard's required-skills offer, and "nerv skills":
// load the manifest, optionally narrow it to opts.Only, compute every
// entry's status against skillsDir, derive the install plan, and (unless
// opts.DryRun) actually run it: the npx installs, then, when opts.ConfirmSync
// agrees, one gentle-ai sync for the missing gentle-ai skills, whose remedies
// survive only for the skills still missing afterwards. Every caller renders Report into its own
// output shape.
func Run(ctx context.Context, runner env.Runner, fsys fs.FS, skillsDir string, opts Options) (Report, error) {
	manifest, err := LoadManifestFS(fsys)
	if err != nil {
		return Report{}, &ErrManifest{Err: err}
	}

	if len(opts.Only) > 0 {
		entries, err := FilterOnly(manifest, opts.Only)
		if err != nil {
			return Report{}, err
		}
		manifest = &Manifest{Schema: manifest.Schema, Skills: entries}
	}

	statuses := Status(manifest, skillsDir)
	plan := InstallPlan(statuses)

	report := Report{Statuses: statuses, Plan: plan}
	if opts.DryRun {
		return report, nil
	}

	report.Result = Install(ctx, runner, plan)
	if plan.Sync != nil && opts.ConfirmSync != nil {
		if opts.ConfirmSync(plan.Sync.Skills) {
			report.Sync = Sync(ctx, runner, plan.Sync)
		} else {
			report.Sync = SyncResult{Declined: true}
		}
	}
	if report.Sync.Ran {
		refreshGentleAI(&report, Status(manifest, skillsDir))
	}
	return report, nil
}

// refreshGentleAI updates the gentle-ai entries of report.Statuses from fresh
// (a Status recomputed after the sync) and recomputes Plan.Remedies, so the
// report never shows a synced skill as missing. External entries keep their
// pre-install status. It relies on Status returning the same order and length
// for the same (possibly --only narrowed) manifest, so fresh[i] describes
// report.Statuses[i]; both come from the one manifest Run holds.
func refreshGentleAI(report *Report, fresh []SkillStatus) {
	for i, s := range report.Statuses {
		if s.Kind == KindGentleAI {
			report.Statuses[i] = fresh[i]
		}
	}
	report.Plan.Remedies = InstallPlan(fresh).Remedies
}

// SyncPrompt is the confirmation question for the gentle-ai sync, shared by
// every interactive caller. The side effect is spelled out because gentle-ai
// has no skills-only command.
func SyncPrompt(names []string) string {
	return fmt.Sprintf("Run 'gentle-ai sync' to provide the missing gentle-ai skills (%s)? "+
		"It also rewrites gentle-ai's managed files (e.g. ~/.claude/CLAUDE.md)", strings.Join(names, ", "))
}

// RenderSync writes the sync outcome of report to w, the one rendering shared
// by "nerv install", the wizard and "nerv skills": the sync line, plus a
// warning with its indented diagnostic when it failed, or one skipped line
// when the user declined it. Nothing is written when no sync was planned or
// no confirmation was asked (non-interactive runs just keep the remedies).
func RenderSync(w io.Writer, report Report) {
	switch {
	case report.Sync.Ran:
		fmt.Fprintf(w, "-> gentle-ai %s\n", strings.Join(report.Plan.Sync.Args, " "))
		if report.Sync.Failed {
			fmt.Fprintln(w, "Warning: gentle-ai sync failed; the gentle-ai skills may still be missing.")
			if report.Sync.Output != "" {
				fmt.Fprintf(w, "  %s\n", strings.ReplaceAll(report.Sync.Output, "\n", "\n  "))
			}
		}
	case report.Sync.Declined:
		fmt.Fprintln(w, "gentle-ai sync skipped: declined; the remedies below still apply.")
	}
}
