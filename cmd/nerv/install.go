package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/war-apps/nerv-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-gentle-ai/internal/install"
	"github.com/war-apps/nerv-gentle-ai/internal/wizard"
)

const installUsage = `Usage: nerv install [flags]

Materializes the embedded NERV plugin under <home>/.nerv/marketplace,
registers it in Claude Code's settings.json, refreshes the plugin cache,
applies model/effort assignments to the cached agents, ensures the Engram
"nerv" knowledge base, and installs skills. Unless --no-configure is given,
it then runs the interactive setup wizard when stdin is a terminal, or
prints the "run nerv configure" hint otherwise.

Flags:
  --require-gentle-ai   Fail (exit 1) when gentle-ai is missing or not 3.x
  --no-skills           Skip installing skills
  --no-configure        Skip the closing wizard/hint entirely
  --home <dir>          Override the resolved home directory
  --settings <path>     Override the settings.json path
`

const uninstallUsage = `Usage: nerv uninstall [flags]

Removes NERV's settings.json registration, uninstalls the cached plugin,
and removes the materialized marketplace directory.

Flags:
  --home <dir>          Override the resolved home directory
  --settings <path>     Override the settings.json path
`

const applyModelsUsage = `Usage: nerv apply-models [flags]

Applies the models: overrides from the user-scope nerv.yaml (merged over
the plugin's own committed defaults) to the cached agent frontmatter.

Flags:
  --home <dir>          Override the resolved home directory
`

func runInstall(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	requireGentleAI := fs.Bool("require-gentle-ai", false, "")
	noSkills := fs.Bool("no-skills", false, "")
	noConfigure := fs.Bool("no-configure", false, "")
	homeOverride := fs.String("home", "", "")
	settingsOverride := fs.String("settings", "", "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprint(stderr, installUsage)
		return 2
	}

	home, err := configure.ResolveHome(*homeOverride)
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return 2
	}

	deps := install.Deps{
		Home:         home,
		SettingsPath: *settingsOverride,
		FS:           opts.PluginFS,
		Runner:       opts.Runner,
		Now:          opts.Now,
		LookPath:     opts.LookPath,
		Stdout:       stdout,
	}

	err = install.Install(context.Background(), deps, install.Options{
		RequireGentleAI: *requireGentleAI,
		NoSkills:        *noSkills,
		// cmd owns the hint-vs-wizard decision below, so the hint
		// install.Install would otherwise print unconditionally is always
		// suppressed here.
		NoConfigure: true,
	})
	if code := mapInstallError(stdout, err); code != 0 {
		return code
	}

	if *noConfigure {
		return 0
	}
	if !stdinIsTerminal() {
		fmt.Fprintln(stdout, "\nRun `nerv configure` to set up NERV.")
		return 0
	}

	wizDeps := configure.Deps{Home: home, FS: opts.PluginFS, Runner: opts.Runner, Now: opts.Now, LookPath: opts.LookPath}
	wizPaths := configure.ResolvePaths(home, "")
	wizOpts := wizard.Options{Paths: wizPaths, SettingsPath: *settingsOverride}
	if _, err := wizard.Run(wizDeps, opts.Stdin, stdout, wizOpts); err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		var refusal *configure.RefusalError
		if errors.As(err, &refusal) {
			return 1
		}
		return 2
	}
	return 0
}

func runUninstall(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	homeOverride := fs.String("home", "", "")
	settingsOverride := fs.String("settings", "", "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprint(stderr, uninstallUsage)
		return 2
	}

	home, err := configure.ResolveHome(*homeOverride)
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return 2
	}

	deps := install.Deps{
		Home:         home,
		SettingsPath: *settingsOverride,
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

	homeOverride := fs.String("home", "", "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprint(stderr, applyModelsUsage)
		return 2
	}

	home, err := configure.ResolveHome(*homeOverride)
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return 2
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
// *install.RefusalError, 2 on any other error. Mirrors
// cmd/nerv/configure.go's emitMutation error handling.
func mapInstallError(stdout io.Writer, err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintf(stdout, "nerv: %v\n", err)
	var refusal *install.RefusalError
	if errors.As(err, &refusal) {
		return 1
	}
	return 2
}
