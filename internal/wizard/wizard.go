// Package wizard implements the interactive half of "nerv configure": the
// terminal setup wizard, driven over an injected io.Reader/io.Writer so it
// can be scripted in tests (and via a real --answers file) exactly the
// same way. It walks prerequisites, required-skills, user-config, task
// providers, models, repos, and apply-and-refresh sections in order, with
// the per-role models editor folded directly into the models section
// instead of shelling out to a separate command.
//
// Every nerv.yaml write goes through configure.Store, config.SetManagedValue
// (via configure.Set), or config.SetModelsBlock — the same primitives
// "nerv configure --set"/"--set-model" already use — so there is exactly
// one place in the whole program that ever writes nerv.yaml text. Task
// provider settings go through configure.SetProviderSettings, the same
// format-preserving editor, and repo initialization reuses
// configure.InitRepo rather than reimplementing its file write.
package wizard

import (
	"context"
	"fmt"
	"io"

	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/gentleai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/install"
)

// Deps is the wizard's external effects — the same seam every other
// configure use case shares. See configure.Deps's own doc comment.
type Deps = configure.Deps

// Options are the wizard's --skip-skills/--skip-models/--skip-repos/
// --no-refresh flags plus the resolved Paths the section functions read
// and write.
type Options struct {
	// Paths is the resolved set of filesystem locations the wizard reads
	// and writes (see configure.ResolvePaths).
	Paths configure.Paths
	// SettingsPath overrides the Claude Code settings.json path the Task
	// providers section reads (which extension plugins are enabled) and
	// the closing "apply and refresh" step's install.Deps uses. Empty means
	// the default "<Home>/.claude/settings.json".
	SettingsPath string
	SkipSkills   bool
	SkipModels   bool
	SkipRepos    bool
	NoRefresh    bool
}

// Summary is Run's report of what happened, used to decide whether to
// print the closing restart reminder.
type Summary struct {
	// Changed is true when any section actually wrote something (user
	// config, task provider settings, the models block, or a repo's
	// .nerv/nerv.yaml).
	Changed bool
}

// Run drives the interactive NERV setup wizard end to end: prerequisites,
// the optional required-skills install, user-scope config (git, skills
// stacks, critical paths, artifacts), the task provider settings when the
// task plugin is enabled, the optional per-role models editor, the
// optional per-repo .nerv/nerv.yaml init loop, and the closing
// apply/refresh step.
//
// Run prints its closing "Restart Claude Code..." line only when
// Summary.Changed is true, rather than unconditionally — nothing worth
// restarting for happened otherwise.
func Run(deps Deps, in io.Reader, out io.Writer, opts Options) (Summary, error) {
	s := newSession(in, out)
	summary := Summary{}
	ctx := context.Background()

	fmt.Fprintln(out, "=== NERV Setup Wizard ===")
	fmt.Fprintln(out)

	printPrerequisites(ctx, deps, out)

	if !opts.SkipSkills {
		if err := offerSkillsInstall(ctx, deps, opts.Paths, s, out); err != nil {
			return summary, err
		}
	}

	uc, err := runUserConfigSection(deps, opts.Paths, s, out)
	if err != nil {
		return summary, err
	}
	summary.Changed = summary.Changed || uc.Changed

	taskEnabled, providers := taskPlugins(deps, opts, out)
	if taskEnabled {
		changed, err := runTaskProvidersSection(deps, opts.Paths, providers, s, out)
		if err != nil {
			return summary, err
		}
		summary.Changed = summary.Changed || changed
	}

	if !opts.SkipModels {
		changed, err := runModelsSection(deps, opts.Paths, s, out)
		if err != nil {
			return summary, err
		}
		summary.Changed = summary.Changed || changed
	}

	if !opts.SkipRepos {
		changed, err := runReposSection(deps, s, out, uc.BaseBranch, providers)
		if err != nil {
			return summary, err
		}
		summary.Changed = summary.Changed || changed
	}

	if !opts.NoRefresh {
		if err := runApplyAndRefreshSection(ctx, deps, opts, s, out); err != nil {
			return summary, err
		}
	}

	if summary.Changed {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Restart Claude Code for the change to take effect.")
	}

	return summary, nil
}

// printPrerequisites prints Section 0's informational gentle-ai/engram/
// claude checks. Purely informational: it never fails the wizard. Reuses
// gentleai.Prerequisites, the same call configure.Print makes for -Print's
// own prerequisites block.
//
// Deviation from the port brief: gentleai.Preflight does not retain
// gentle-ai --version's raw first output line, only its parsed
// MAJOR.MINOR.PATCH token (or that none was found); the unparseable-output
// case (found, but no parseable version at all) is folded into the "NOT
// FOUND" line rather than echoing raw output, since that text isn't
// available here.
func printPrerequisites(ctx context.Context, deps Deps, out io.Writer) {
	fmt.Fprintln(out, "--- Prerequisites ---")

	prereq := gentleai.Prerequisites(ctx, deps.Runner, deps.LookPath)
	switch {
	case prereq.GentleAI.Found && prereq.GentleAI.OK:
		fmt.Fprintf(out, "gentle-ai      : %s (OK)\n", prereq.GentleAI.Version)
	case prereq.GentleAI.Found && prereq.GentleAI.Version != "":
		fmt.Fprintf(out, "gentle-ai      : %s (WARNING: NERV requires gentle-ai 4.x)\n", prereq.GentleAI.Version)
	default:
		fmt.Fprintln(out, "gentle-ai      : NOT FOUND on PATH (NERV requires gentle-ai 4.x)")
	}

	fmt.Fprintf(out, "engram         : %s\n", toolLine(prereq.Engram.Found, "found on PATH (optional)", "not found on PATH (optional)"))
	fmt.Fprintf(out, "claude         : %s\n", toolLine(prereq.Claude.Found, "found on PATH", "NOT FOUND on PATH (required for the refresh step)"))
}

func toolLine(found bool, yes, no string) string {
	if found {
		return yes
	}
	return no
}

// runApplyAndRefreshSection is the closing section: optionally applies the
// resolved models: block to the plugin cache (install.ApplyModels) and
// refreshes the "claude plugin" cache (install.RefreshCache, which also
// refreshes the enabled extension plugins) — the same tail install.Install
// itself runs after registering settings.json.
func runApplyAndRefreshSection(ctx context.Context, deps Deps, opts Options, s *session, out io.Writer) error {
	fmt.Fprintln(out)
	apply, err := s.yesNo("Apply models to the plugin cache and refresh it now?", true)
	if err != nil {
		return err
	}
	if !apply {
		return nil
	}

	installDeps := install.Deps{
		Home:         deps.Home,
		SettingsPath: opts.SettingsPath,
		FS:           deps.FS,
		ExtensionsFS: deps.ExtensionsFS,
		Runner:       deps.Runner,
		Now:          deps.Now,
		LookPath:     deps.LookPath,
		Stdout:       out,
	}

	if err := install.ApplyModels(ctx, installDeps); err != nil {
		return err
	}
	return install.RefreshCache(ctx, installDeps)
}
