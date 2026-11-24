package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/ecosystem"
	"github.com/war-apps/nerv-by-gentle-ai/internal/install"
	"github.com/war-apps/nerv-by-gentle-ai/internal/wizard"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
)

var installUsage = `Usage: nerv install [flags]

Before anything else, when gentle-ai is on PATH and already 4.x, it
upgrades gentle-ai to its newest 4.x release (gentle-ai upgrade gentle-ai,
then gentle-ai sync), so NERV's own files below are laid over the newest
gentle-ai. A newer major version is reported and left alone; a failed
release lookup or upgrade is only a warning, and install continues with
the gentle-ai it found.

First offers the NERV-ecosystem tools (nerv-task, nerv-release): on a
terminal with neither --tools nor --no-tools, each tool's state (not
installed, its version, or outdated against its latest release) is shown
and confirmed one by one; a confirmed tool's official install script then
runs (install.sh on Linux/macOS, install.ps1 on Windows).

Then materializes the embedded NERV plugin under <home>/.nerv/marketplace,
registers it in Claude Code's settings.json, refreshes the plugin cache,
applies model/effort assignments to the cached agents, ensures the Engram
"nerv" knowledge base, and installs skills. Old /task:* copies in
<home>/.claude/commands/task, which shadow the /task:* commands, are
reported, and removed when you confirm on a terminal.

The extension plugins nerv no longer ships (task and task-teamwork,
replaced by nerv-task; release, its providers and the older deploy ones,
replaced by nerv-release) are disabled: removed from settings.json and
uninstalled from the plugin cache. When the tool that replaces them is
not installed (declined, or not asked without a terminal), install says
which were disabled and how to install the tool (nerv install --tools
<tool>): their /task:* or /release:* commands are unavailable until then.

Then, when such a tool is installed and its settings are still in the
user config.yaml, it offers nerv migrate --user: on a terminal it shows
the steps and asks [y/N] (nothing is written without a y); otherwise it
prints a hint naming the command.

Unless --no-configure is given,
it then runs the interactive setup wizard when stdin is a terminal, or
prints the "run nerv configure" hint otherwise.

Flags:
  --require-gentle-ai   Fail (exit 1) when gentle-ai is missing or not 4.x
  --no-skills           Skip installing skills
  --no-configure        Skip the closing wizard/hint entirely
` + ecosystemToolsFlagsUsage("install")

const uninstallUsage = `Usage: nerv uninstall [flags]

Removes NERV's settings.json registration, uninstalls the cached plugin and
every other enabled plugin of the nerv marketplace, and removes the
materialized marketplace directory.
`

const applyModelsUsage = `Usage: nerv apply-models [flags]

Applies the models: overrides from the user-scope config.yaml (merged over
the plugin's own committed defaults) to the cached agent frontmatter.
`

func runInstall(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	requireGentleAI := fs.Bool("require-gentle-ai", false, "")
	noSkills := fs.Bool("no-skills", false, "")
	noConfigure := fs.Bool("no-configure", false, "")
	tools := fs.String("tools", "", "")
	noTools := fs.Bool("no-tools", false, "")
	// skipGentleAIUpgrade is internal: only nerv upgrade's own install run
	// sets it, since upgradeGentleAI already upgraded gentle-ai once for
	// that upgrade (S5) and this run must not upgrade it again.
	skipGentleAIUpgrade := fs.Bool("skip-gentle-ai-upgrade", false, "")
	// skipEcosystemTools is internal: only nerv upgrade's own install run
	// sets it, since it offers the ecosystem tools itself afterwards, with
	// action "upgrade" instead of "install".
	skipEcosystemTools := fs.Bool("skip-ecosystem-tools", false, "")
	// skipMigrateOffer is internal: only nerv upgrade's own install run sets
	// it, since the upgrade offers nerv migrate itself afterwards. It is
	// separate from --skip-ecosystem-tools because an older nerv's upgrade
	// passes that flag alone, and its first install of this version must
	// still offer the migration.
	skipMigrateOffer := fs.Bool("skip-migrate-offer", false, "")
	// skipOrphanCleanup is internal: only nerv upgrade's own install run
	// sets it, since the upgrade disables the orphaned extension plugins
	// itself, after its own ecosystem-tools step. An older nerv's upgrade
	// does not pass it, so its first install of this version disables them.
	skipOrphanCleanup := fs.Bool("skip-orphan-cleanup", false, "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprint(stderr, installUsage)
		return 2
	}

	home, err := configure.ResolveHome(opts.Home)
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return 2
	}
	workflowconfig.PrintHints(stderr, home, "")
	if refusedPreviousUser(stderr, "install", home) {
		return 1
	}

	// S4: upgrade gentle-ai to its newest 4.x release before laying NERV's
	// own files over it (install.Install, below), since gentle-ai sync
	// rewrites gentle-ai's managed files. A failed release lookup or
	// upgrade is only a warning: install continues with the gentle-ai it
	// found, and this does not change the exit code.
	if !*skipGentleAIUpgrade {
		if _, gentleErr := upgradeGentleAI(context.Background(), stdout, stderr, opts); gentleErr != nil {
			fmt.Fprintf(stdout, "Warning: gentle-ai: %v\n", gentleErr)
		}
	}

	// S11: the ecosystem tools are offered first, so a tool chosen in
	// this run (not only one already installed on this machine) counts as
	// installed when the extension plugins it replaces are disabled (S6).
	var selectedTools []ecosystem.Tool
	if !*skipEcosystemTools {
		var ecoErr error
		selectedTools, ecoErr = offerEcosystemTools(context.Background(), stdout, opts, home, "install", *tools, *noTools)
		if ecoErr != nil {
			fmt.Fprintf(stdout, "nerv install: %v\n", ecoErr)
			return exitCodeFor(ecoErr)
		}
	}

	deps := install.Deps{
		Home:                        home,
		SettingsPath:                opts.Settings,
		FS:                          opts.PluginFS,
		Runner:                      opts.Runner,
		Now:                         opts.Now,
		LookPath:                    opts.LookPath,
		ConfirmSync:                 syncConfirmer(opts, stdout),
		ConfirmRemoveLegacyCommands: legacyCommandsConfirmer(opts, stdout),
		Stdout:                      stdout,
	}

	installOpts := install.Options{RequireGentleAI: *requireGentleAI, NoSkills: *noSkills}
	if !*skipOrphanCleanup {
		installOpts.Orphans = orphanFamilies(context.Background(), opts, home, selectedTools)
	}
	err = install.Install(context.Background(), deps, installOpts)
	if code := mapInstallError(stdout, err); code != 0 {
		return code
	}

	// S3: a tool installed (before or in this run) whose settings are
	// still in the user config.yaml gets nerv migrate --user offered.
	// nerv upgrade's own install run (--skip-migrate-offer) leaves it to
	// the upgrade, which offers it once after its own tools step.
	if !*skipMigrateOffer {
		offerToolMigration(context.Background(), stdout, opts, home)
	}

	if *noConfigure {
		return 0
	}
	if !opts.IsTerminal() {
		fmt.Fprintln(stdout, "\nRun `nerv configure` to set up NERV.")
		return 0
	}

	wizDeps := configure.Deps{Home: home, FS: opts.PluginFS, Runner: opts.Runner, Now: opts.Now, LookPath: opts.LookPath, Getenv: opts.Getenv}
	wizPaths := configure.ResolvePaths(home, "")
	wizOpts := wizard.Options{Paths: wizPaths, SettingsPath: opts.Settings}
	if _, err := wizard.Run(wizDeps, opts.Stdin, stdout, wizOpts); err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		if errors.Is(err, wizard.ErrInputClosed) {
			return 1
		}
		return exitCodeFor(err)
	}
	return 0
}

func runUninstall(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if err := fs.Parse(args); err != nil {
		fmt.Fprint(stderr, uninstallUsage)
		return 2
	}

	home, err := configure.ResolveHome(opts.Home)
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return 2
	}

	deps := install.Deps{
		Home:         home,
		SettingsPath: opts.Settings,
		FS:           opts.PluginFS,
		Runner:       opts.Runner,
		Now:          opts.Now,
		LookPath:     opts.LookPath,
		Stdout:       stdout,
	}

	err = install.Uninstall(context.Background(), deps)
	return mapInstallError(stdout, err)
}

func runApplyModels(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("apply-models", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if err := fs.Parse(args); err != nil {
		fmt.Fprint(stderr, applyModelsUsage)
		return 2
	}

	home, err := configure.ResolveHome(opts.Home)
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return 2
	}
	workflowconfig.PrintHints(stderr, home, "")
	if refusedPreviousUser(stderr, "apply-models", home) {
		return 1
	}

	deps := install.Deps{
		Home:     home,
		FS:       opts.PluginFS,
		Runner:   opts.Runner,
		Now:      opts.Now,
		LookPath: opts.LookPath,
		Stdout:   stdout,
	}

	err = install.ApplyModels(context.Background(), deps)
	return mapInstallError(stdout, err)
}

// mapInstallError formats an Install/Uninstall/ApplyModels error to
// stdout and returns the process exit code: 0 on success, 1 on a
// *install.RefusalError, 2 on any other error, via exitCodeFor.
func mapInstallError(stdout io.Writer, err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintf(stdout, "nerv: %v\n", err)
	return exitCodeFor(err)
}

// splitList splits a comma-separated flag value, dropping blanks.
func splitList(value string) []string {
	var names []string
	for _, name := range strings.Split(value, ",") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}
