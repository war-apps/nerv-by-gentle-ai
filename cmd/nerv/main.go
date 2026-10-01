// Command nerv is the Nerv by Gentle-AI CLI: it materializes the embedded
// plugin tree, configures NERV, and reports version information.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	nerv "github.com/war-apps/nerv-gentle-ai"
	"github.com/war-apps/nerv-gentle-ai/internal/env"
	"github.com/war-apps/nerv-gentle-ai/internal/version"
)

const usage = `Usage: nerv <command> [flags]

Commands:
  version [--json]     Print the binary and plugin versions
  configure [flags]    Configure NERV (non-interactive; see "nerv configure --help")
  install [flags]      Install NERV (see "nerv install --help")
  uninstall [flags]    Uninstall NERV (see "nerv uninstall --help")
  apply-models [flags] Apply model/effort assignments to the cached agents
  skills [flags]       Install or verify skills (see "nerv skills --help")

Maintainer commands:
  release <preview|apply|guard> [flags]
                       Release tooling for this repository (see "nerv release --help")
`

// options bundles nerv's external effects (the embedded plugin tree, the
// process-launching/PATH-lookup seam, and the clock) so run can be
// exercised end to end against fakes: main fills the real ones, tests fill
// scripted ones.
type options struct {
	PluginFS fs.FS
	Runner   env.Runner
	Now      func() time.Time
	LookPath func(string) (string, error)
	// Getenv resolves an environment variable (the wizard's herdr
	// detection, for %APPDATA% on Windows).
	Getenv func(string) string
	// Stdin is the wizard's interactive input source; tests inject a
	// scripted reader instead of the real os.Stdin.
	Stdin io.Reader
	// Home overrides the resolved home directory. Empty means
	// env.HomeDir() decides, exactly as configure.ResolveHome documents.
	//
	// There is deliberately no public "--home" flag: a fake home never
	// sandboxes the global commands every subcommand can still reach
	// through it (`claude plugin ...`, `npx ... -g`, gentle-ai's own
	// state). A 2026-09-29 incident ran `nerv configure --home <tmp>`
	// expecting that tmp dir to contain the blast radius, but the
	// wizard's `claude plugin uninstall/install` and `npx skills add -g`
	// calls ran against the real machine regardless of --home — the flag
	// looked like a sandbox and was not one. Home is settable only here,
	// in options, so only tests (which also fake Runner/LookPath) can
	// override it.
	Home string
	// Settings overrides the resolved Claude Code settings.json path.
	// Empty means "<Home>/.claude/settings.json" (install.Deps's own
	// default). No public "--settings" flag either, for the same reason
	// as Home above: settable only here, only for tests.
	Settings string
	// IsTerminal reports whether stdin is attached to a real interactive
	// terminal. Tests inject a fixed true/false; defaultOptions wires the
	// real golang.org/x/term check, which a unit test cannot exercise
	// directly (it depends on a real OS file descriptor/console).
	IsTerminal func() bool
}

func defaultOptions() options {
	return options{
		PluginFS:   pluginFS(),
		Runner:     env.ExecRunner{},
		Now:        time.Now,
		LookPath:   env.LookPath,
		Getenv:     os.Getenv,
		Stdin:      os.Stdin,
		IsTerminal: stdinIsTerminal,
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, defaultOptions()))
}

// run dispatches the given command-line arguments and returns the process
// exit code. It never touches package-level state so it can be exercised
// directly from tests, with opts carrying every external effect a
// subcommand might need.
func run(args []string, stdout, stderr io.Writer, opts options) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return 0
	}

	switch args[0] {
	case "version":
		return runVersion(args[1:], stdout, stderr, opts)
	case "configure":
		return runConfigure(args[1:], stdout, stderr, opts)
	case "install":
		return runInstall(args[1:], stdout, stderr, opts)
	case "uninstall":
		return runUninstall(args[1:], stdout, stderr, opts)
	case "apply-models":
		return runApplyModels(args[1:], stdout, stderr, opts)
	case "skills":
		return runSkills(args[1:], stdout, stderr, opts)
	case "release":
		return runRelease(args[1:], stdout, stderr, opts)
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
}

func runVersion(args []string, stdout, stderr io.Writer, opts options) int {
	jsonOutput := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		fmt.Fprint(stderr, usage)
		return 2
	}

	pluginVersion, err := version.PluginVersion(opts.PluginFS)
	if err != nil {
		fmt.Fprintf(stderr, "nerv: %v\n", err)
		return 2
	}

	if jsonOutput {
		info := version.Info{Binary: version.Binary, Plugin: pluginVersion}
		encoded, err := json.Marshal(info)
		if err != nil {
			fmt.Fprintf(stderr, "nerv: %v\n", err)
			return 2
		}
		fmt.Fprintf(stdout, "%s\n", encoded)
		return 0
	}

	fmt.Fprintf(stdout, "nerv %s (plugin %s)\n", version.Binary, pluginVersion)
	return 0
}

func pluginFS() fs.FS {
	return nerv.PluginFS()
}
