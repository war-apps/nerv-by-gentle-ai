package skills

import (
	"context"
	"io/fs"

	"github.com/war-apps/nerv-gentle-ai/internal/env"
)

// Options configures one Run invocation.
type Options struct {
	// Only restricts processing to these skill names. Empty means every
	// skill in the manifest.
	Only []string
	// DryRun computes the install plan (and every status) without
	// actually running any "npx skills add" install.
	DryRun bool
}

// Report is Run's result: the computed status of every processed skill,
// the derived install plan, and (unless DryRun) the outcome of actually
// running it. Result is the zero InstallResult when DryRun is true.
type Report struct {
	Statuses []SkillStatus
	Plan     Plan
	Result   InstallResult
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
// opts.DryRun) actually run it. Every caller renders Report into its own
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
	return report, nil
}
