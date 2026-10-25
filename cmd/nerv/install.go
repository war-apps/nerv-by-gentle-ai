package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/install"
	"github.com/war-apps/nerv-by-gentle-ai/internal/wizard"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
)

const installUsage = `Usage: nerv install [flags]

Materializes the embedded NERV plugin under <home>/.nerv/marketplace,
registers it in Claude Code's settings.json, refreshes the plugin cache,
applies model/effort assignments to the cached agents, ensures the Engram
"nerv" knowledge base, and installs skills. It also installs the optional
extensions and providers named by --with, keeps the ones already enabled up
to date, and, when stdin is a terminal, offers the others, in this order:
  1. the task extension (the /task:* commands);
  2. its providers (task-<name>), only once task is installed;
  3. the release extension (nerv release, the version engine);
  4. the deploy extension (nerv deploy), only once release is installed.
An extension whose extension.yaml says providers: bundled (release and
deploy) installs all its providers with it, without asking: release brings
release-conventional-commits, release-docker-compose and release-bash, and
deploy brings deploy-bash, deploy-github and deploy-ssh-compose. task's
providers are suggested: each is offered after task.
Requirements: deploy requires release, and a provider requires its
extension; each is offered only once what it requires is installed or
chosen. An extension or provider named in --with brings what it requires
with it; a requirement this nerv does not ship is refused.
Old /task:* copies in <home>/.claude/commands/task, which shadow the task
extension's commands, are reported, and removed when you confirm on a terminal.
Unless --no-configure is given,
it then runs the interactive setup wizard when stdin is a terminal, or
prints the "run nerv configure" hint otherwise.

Flags:
  --require-gentle-ai   Fail (exit 1) when gentle-ai is missing or not 4.x
  --no-skills           Skip installing skills
  --no-configure        Skip the closing wizard/hint entirely
  --with <names>        Comma-separated extensions and providers to install, such as
                        task,task-<provider>,release,deploy,deploy-<provider>
                        (each brings what it requires)
`

const uninstallUsage = `Usage: nerv uninstall [flags]

Removes NERV's settings.json registration, uninstalls the cached plugin and
every enabled extension and provider,
and removes the materialized marketplace directory.
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
	with := fs.String("with", "", "")

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

	deps := install.Deps{
		Home:                        home,
		SettingsPath:                opts.Settings,
		FS:                          opts.PluginFS,
		ExtensionsFS:                opts.ExtensionsFS,
		Runner:                      opts.Runner,
		Now:                         opts.Now,
		LookPath:                    opts.LookPath,
		ConfirmSync:                 syncConfirmer(opts, stdout),
		ConfirmExtension:            extensionConfirmer(opts, stdout),
		ConfirmRemoveLegacyCommands: legacyCommandsConfirmer(opts, stdout),
		Stdout:                      stdout,
	}

	err = install.Install(context.Background(), deps, install.Options{
		RequireGentleAI: *requireGentleAI,
		NoSkills:        *noSkills,
		Extensions:      splitList(*with),
	})
	if code := mapInstallError(stdout, err); code != 0 {
		return code
	}

	if *noConfigure {
		return 0
	}
	if !opts.IsTerminal() {
		fmt.Fprintln(stdout, "\nRun `nerv configure` to set up NERV.")
		return 0
	}

	wizDeps := configure.Deps{Home: home, FS: opts.PluginFS, ExtensionsFS: opts.ExtensionsFS, Runner: opts.Runner, Now: opts.Now, LookPath: opts.LookPath, Getenv: opts.Getenv}
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
		ExtensionsFS: opts.ExtensionsFS,
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
