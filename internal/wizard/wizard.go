// Package wizard implements the interactive half of "nerv configure": the
// terminal setup wizard, driven over an injected io.Reader/io.Writer so it
// can be scripted in tests exactly like plugin/tools/configure.ps1's own
// -AnswersFile mode. It is the interactive port of configure.ps1's
// prerequisites/required-skills/user-config/models/repos/slash-commands/
// apply-and-refresh sections (see tests/configure.test.ps1's end-to-end
// -AnswersFile scenarios, this package's executable specification), with
// configure-models.ps1's interactive models editor folded in directly
// instead of shelling out to a second script.
//
// Every nerv.yaml write goes through configure.Store, config.SetManagedValue
// (via configure.Set), or config.SetModelsBlock — the same primitives
// "nerv configure --set"/"--set-model" already use — so there is exactly
// one place in the whole program that ever writes nerv.yaml text. Repo
// initialization and the Teamwork commands install likewise reuse
// configure.InitRepo and configure.InstallCommands rather than
// reimplementing their file writes.
package wizard

import (
	"context"
	"fmt"
	"io"

	"github.com/war-apps/nerv-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-gentle-ai/internal/gentleai"
	"github.com/war-apps/nerv-gentle-ai/internal/install"
)

// Deps is the wizard's external effects — the same seam every other
// configure use case shares. See configure.Deps's own doc comment.
type Deps = configure.Deps

// Options are the wizard's skip flags (configure.ps1's
// -SkipSkills/-SkipModels/-SkipRepos/-SkipCommands/-NoRefresh) plus the
// resolved Paths the section functions read and write.
type Options struct {
	// Paths is the resolved set of filesystem locations the wizard reads
	// and writes (see configure.ResolvePaths).
	Paths configure.Paths
	// SettingsPath overrides the Claude Code settings.json path used by
	// the closing "apply and refresh" step's install.Deps. Empty means
	// the default "<Home>/.claude/settings.json".
	SettingsPath string
	SkipSkills   bool
	SkipModels   bool
	SkipRepos    bool
	SkipCommands bool
	NoRefresh    bool
}

// Summary is Run's report of what happened, used to decide whether to
// print the closing restart reminder.
type Summary struct {
	// Changed is true when any section actually wrote something (user
	// config, the models block, a repo's .nerv/nerv.yaml, or the
	// Teamwork commands).
	Changed bool
}

// Run drives the interactive NERV setup wizard end to end: prerequisites,
// the optional required-skills install, user-scope config (git, tasks,
// skills stacks, critical paths, artifacts), the optional per-role models
// editor, the optional per-repo .nerv/nerv.yaml init loop, the optional
// Teamwork /task:* commands install, and the closing apply/refresh step.
// Mirrors plugin/tools/configure.ps1's interactive body (~1451-2089) plus
// plugin/tools/configure-models.ps1's interactive body (~390-690, Section
// 2's model editor, folded in directly instead of shelling out).
//
// Deviation from the port brief: configure.ps1 always prints its closing
// "Restart Claude Code..." line unconditionally; Run prints it only when
// Summary.Changed is true (this package's Goal explicitly calls for that
// narrower condition).
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

	if !opts.SkipModels {
		changed, err := runModelsSection(deps, opts.Paths, s, out)
		if err != nil {
			return summary, err
		}
		summary.Changed = summary.Changed || changed
	}

	if !opts.SkipRepos {
		changed, err := runReposSection(deps, opts.Paths, s, out, uc.BaseBranch, uc.Provider)
		if err != nil {
			return summary, err
		}
		summary.Changed = summary.Changed || changed
	}

	if !opts.SkipCommands {
		changed, err := runCommandsSection(deps, opts.Paths, s, out)
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
		fmt.Fprintf(out, "gentle-ai      : %s (WARNING: NERV requires major version 3)\n", prereq.GentleAI.Version)
	default:
		fmt.Fprintln(out, "gentle-ai      : NOT FOUND on PATH (NERV requires gentle-ai 3.x)")
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

// runApplyAndRefreshSection is Section 5: optionally applies the resolved
// models: block to the plugin cache (install.ApplyModels) and refreshes
// the "claude plugin" cache (install.RefreshCache) — the same tail
// install.Install itself runs after registering settings.json. Mirrors
// configure.ps1's "-- Section 5: Apply and refresh --" (~2077-2086), which
// calls Invoke-NervApplyModels then `install.ps1 -RefreshCache`.
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
