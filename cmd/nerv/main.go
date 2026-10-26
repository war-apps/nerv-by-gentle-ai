// Command nerv is the Nerv by Gentle-AI CLI: it materializes the embedded
// plugin tree, configures NERV, and reports version information.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"time"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseclone"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseplan"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

const usage = `Usage: nerv <command> [flags]

Commands:
  version [--json]     Print the binary and plugin versions
  version next [flags] Compute the next release version with the native
                       engine (see "nerv version next --help")
  release [flags]      Build, test and package origin's code into a local
                       artifact, no git writes (see "nerv release --help")
  release setup --import [--dry-run]
                       Import a semantic-release configuration into the
                       release: section (see "nerv release setup --help")
  deploy --artifact <dir> [flags]
                       Release and deploy a release artifact: release commit,
                       tag, push, targets, git host release
                       (see "nerv deploy --help")
  migrate [flags]      Move a repository (and, with --user, the user
                       configuration) from nerv 4.2.0 or 5.0.0-alpha.1
                       (see "nerv migrate --help")
  doctor [flags]       Check that NERV is in order on this machine and in
                       the repository, and list the steps to take
                       (see "nerv doctor --help")
  upgrade [--alpha | --rc]
                       Replace this binary with the newest release, then
                       run its install (see "nerv upgrade --help")
  configure [flags]    Configure NERV (non-interactive; see "nerv configure --help")
  install [flags]      Install NERV (see "nerv install --help")
  uninstall [flags]    Uninstall NERV (see "nerv uninstall --help")
  apply-models [flags] Apply model/effort assignments to the cached agents
  skills [flags]       Install or verify skills (see "nerv skills --help")

Maintainer commands:
  maintainer-release <preview|apply|guard> [flags]
                       Release tooling for this repository (see "nerv maintainer-release --help")
`

// options bundles nerv's external effects (the embedded plugin tree, the
// process-launching/PATH-lookup seam, and the clock) so run can be
// exercised end to end against fakes: main fills the real ones, tests fill
// scripted ones.
type options struct {
	PluginFS fs.FS
	// ExtensionsFS holds the optional extension plugins (the task plugin
	// and its providers). Nil ships none.
	ExtensionsFS fs.FS
	Runner       env.Runner
	Now          func() time.Time
	LookPath     func(string) (string, error)
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
	// ComputeVersions is the `nerv version next` engine (releaseclone.Compute);
	// tests inject a recorded fake. Nil means releaseclone.Compute.
	ComputeVersions func(context.Context, releaseclone.ComputeOptions) ([]versioning.Result, error)
	// SignalContext derives the context a long-running command runs under,
	// cancelled on SIGINT, SIGTERM and (on Unix) SIGHUP, with a stop function
	// and the signal received (nil if none). Tests inject a controllable one.
	// Nil means signalContext.
	SignalContext func(context.Context) (context.Context, context.CancelFunc, func() os.Signal)
	// OpenReleaseWorkspace creates the release clone of nerv release; tests inject
	// a fake. Nil means releaseclone.New.
	OpenReleaseWorkspace func(context.Context, releaseclone.Options) (releaseWorkspace, error)
	// ReleaseSteps returns the runner of nerv release's process steps for the
	// release clone at repoRoot, streaming to output; tests inject a
	// recorder. Nil means a releaserun.Runner.
	ReleaseSteps func(repoRoot string, output io.Writer) releaseplan.StepRunner
	// DeployProc runs nerv deploy's provider commands and the native
	// deploy-ssh-compose operations' infisical, ssh and sshpass, and
	// DeploySleep waits between the health checks; tests inject fakes. Nil
	// means proc.ExecRunner and a real wait.
	DeployProc  proc.Runner
	DeploySleep func(context.Context, time.Duration) error
	// ReleaseProc runs the release extension's provider steps (the
	// version engine's and the build providers' `nerv provider
	// release-<name> <operation>`); tests may inject a fake. Nil means
	// proc.ExecRunner.
	ReleaseProc proc.Runner
	// HTTPClient, ReleaseAPIBase and ReleaseDownloadBase are where nerv
	// upgrade looks up and downloads releases; tests point them at an
	// httptest.Server. Nil or empty means a client with a timeout,
	// upgrade.DefaultAPIBase and upgrade.DefaultDownloadBase.
	HTTPClient          *http.Client
	ReleaseAPIBase      string
	ReleaseDownloadBase string
	// GentleAIAPIBase is where nerv upgrade looks up gentle-ai's newest
	// release; empty means upgrade.GentleAIAPIBase.
	GentleAIAPIBase string
	// Executable locates the running nerv binary nerv upgrade replaces
	// (symlinks are resolved after it); nil means os.Executable.
	Executable func() (string, error)
	// GOOS and GOARCH are the platform nerv upgrade downloads for; empty
	// means runtime.GOOS and runtime.GOARCH.
	GOOS, GOARCH string
	// RunBinary runs the new binary's install after nerv upgrade, with this
	// process's stdin, streaming to stdout and stderr, and returns its exit
	// code; tests inject a recorder. Nil means execBinary.
	RunBinary func(ctx context.Context, bin string, args []string, stdout, stderr io.Writer) (int, error)
}

func defaultOptions() options {
	return options{
		PluginFS:        pluginFS(),
		ExtensionsFS:    nerv.ExtensionsFS(),
		Runner:          env.ExecRunner{},
		Now:             time.Now,
		LookPath:        env.LookPath,
		Getenv:          os.Getenv,
		Stdin:           os.Stdin,
		IsTerminal:      stdinIsTerminal,
		ComputeVersions: releaseclone.Compute,
		SignalContext:   signalContext,
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
	case "deploy":
		return runDeploy(args[1:], stdout, stderr, opts)
	case "migrate":
		return runMigrate(args[1:], stdout, stderr, opts)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr, opts)
	case "upgrade":
		return runUpgrade(args[1:], stdout, stderr, opts)
	case "provider":
		// Hidden: the operations of nerv's native providers, run by their
		// extension (nerv deploy runs deploy-ssh-compose's, nerv release
		// the release-* ones).
		return runProvider(args[1:], stdout, stderr, opts)
	case "pipeline":
		fmt.Fprintln(stderr, pipelineRemoved)
		return 2
	case "maintainer-release":
		return runMaintainerRelease(args[1:], stdout, stderr, opts)
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
}

// runVersion prints the binary and plugin versions, or, as `nerv version
// next`, computes the next release version (runVersionNext). Any other
// argument is a usage error.
func runVersion(args []string, stdout, stderr io.Writer, opts options) int {
	if len(args) > 0 && args[0] == "next" {
		return runVersionNext(args[1:], stdout, stderr, opts)
	}
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

// pipelineRemoved answers the removed nerv pipeline command with where each
// of its steps went.
const pipelineRemoved = `nerv pipeline was removed. Its steps moved:
  validate and test: nerv release (stages of each project in the release: section)
  docker and release: nerv release, then nerv deploy
  deploy: the deploy-ssh-compose target of the deploy: section (nerv deploy)
  sync: not ported yet
Move the pipeline: section of .ai-workflow/config.yaml with nerv migrate, or by hand (/release:setup and /deploy:setup).`

func pluginFS() fs.FS {
	return nerv.PluginFS()
}

// releaseProviders calls the release extension's providers from the
// extensions tree (the embedded one when opts has none: the release
// extension always ships its own) merged with the project extensions and
// providers of the repository at top, their stderr streamed to output. An
// invalid project extension or provider is a refusal.
func releaseProviders(opts options, top string, output io.Writer) (releaseplan.Providers, error) {
	fsys := opts.ExtensionsFS
	if fsys == nil {
		fsys = nerv.ExtensionsFS()
	}
	fsys, err := projectTree(fsys, top)
	if err != nil {
		return releaseplan.Providers{}, err
	}
	return releaseplan.Providers{FS: fsys, Runner: opts.ReleaseProc, Output: output}, nil
}

// projectTree is fsys merged with the project extensions and providers of
// the repository at top (extension.WithProject); an invalid one is a
// refusal.
func projectTree(fsys fs.FS, top string) (fs.FS, error) {
	merged, err := extension.WithProject(fsys, top)
	if err != nil {
		return nil, &refusal.Error{Err: err}
	}
	return merged, nil
}
