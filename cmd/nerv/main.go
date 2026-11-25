// Command nerv is the Nerv by Gentle-AI CLI: it materializes the embedded
// plugin tree, configures NERV, and reports version information.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"time"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
	"github.com/war-apps/nerv-kit-by-gentle-ai/runner"
)

const usage = `Usage: nerv <command> [flags]

Commands:
  version [--json]     Print the binary and plugin versions
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
	Runner   runner.Runner
	Now      func() time.Time
	LookPath func(string) (string, error)
	// Getenv resolves an environment variable (the wizard's herdr
	// detection, for %APPDATA% on Windows).
	Getenv func(string) string
	// Stdin is the wizard's interactive input source; tests inject a
	// scripted reader instead of the real os.Stdin.
	Stdin io.Reader
	// Home overrides the resolved home directory. Empty means
	// runner.HomeDir() decides, exactly as configure.ResolveHome documents.
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
	// EnableColor reports whether stdout shows ANSI colors (turning them on
	// on a Windows console). Nil means plain output; tests leave it nil.
	EnableColor func() bool
	// SignalContext derives the context a long-running command runs under,
	// cancelled on SIGINT, SIGTERM and (on Unix) SIGHUP, with a stop function
	// and the signal received (nil if none). Tests inject a controllable one.
	// Nil means signalContext.
	SignalContext func(context.Context) (context.Context, context.CancelFunc, func() os.Signal)
	// HTTPClient, ReleaseAPIBase and ReleaseDownloadBase are where nerv
	// upgrade looks up and downloads releases; tests point them at an
	// httptest.Server. Nil or empty means a client with a timeout,
	// defaultAPIBase and defaultDownloadBase.
	HTTPClient          *http.Client
	ReleaseAPIBase      string
	ReleaseDownloadBase string
	// GentleAIAPIBase is where nerv upgrade looks up gentle-ai's newest
	// release; empty means gentleAIAPIBase.
	GentleAIAPIBase string
	// EcosystemAPIBase overrides the GitHub API base used to resolve an
	// ecosystem tool's latest release, keyed by ecosystem.Tool.Name;
	// empty or missing means "https://api.github.com/repos/<tool.Repo>".
	// Tests only.
	EcosystemAPIBase map[string]string
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
		PluginFS:      pluginFS(),
		Runner:        runner.ExecRunner{},
		Now:           time.Now,
		LookPath:      runner.LookPath,
		Getenv:        os.Getenv,
		Stdin:         os.Stdin,
		IsTerminal:    stdinIsTerminal,
		EnableColor:   stdoutColor,
		SignalContext: signalContext,
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
	case "release", "deploy":
		fmt.Fprintln(stderr, movedToNervRelease(args))
		return 2
	case "migrate":
		return runMigrate(args[1:], stdout, stderr, opts)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr, opts)
	case "upgrade":
		return runUpgrade(args[1:], stdout, stderr, opts)
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

// runVersion prints the binary and plugin versions; `nerv version next`
// moved to nerv-release. Any other argument is a usage error.
// versionUsage documents "nerv version".
const versionUsage = `Usage: nerv version [--json]

Print the nerv binary version and the version of the NERV plugin it ships.
With --json, print {"binary":"...","plugin":"..."}.
`

func runVersion(args []string, stdout, stderr io.Writer, opts options) int {
	if len(args) > 0 && args[0] == "next" {
		fmt.Fprintln(stderr, movedToNervRelease([]string{"version", "next"}))
		return 2
	}
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
			continue
		case "-h", "--help":
			fmt.Fprint(stdout, versionUsage)
			return 0
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
const pipelineRemoved = `nerv pipeline was removed. Its steps moved to the nerv-release tool (nerv install --tools nerv-release):
  validate and test: nerv-release build (stages of each project in the release: section)
  docker and release: nerv-release build, then nerv-release deploy
  deploy: the release-ssh-compose target of the deploy: section (nerv-release deploy)
  sync: not ported yet
Move the pipeline: section of .ai-workflow/config.yaml by hand (/release:setup and /release:deploy-setup).`

// movedToNervRelease answers a release command that moved to the
// nerv-release tool (nerv release, nerv release setup, nerv deploy, nerv
// version next; args as given to nerv) with the command that replaces it.
func movedToNervRelease(args []string) string {
	name, replacement := "nerv "+args[0], "nerv-release "+args[0]
	switch {
	case args[0] == "release" && len(args) > 1 && args[1] == "setup":
		name, replacement = "nerv release setup", "nerv-release setup --import"
	case args[0] == "release":
		replacement = "nerv-release build"
	case args[0] == "version":
		name, replacement = "nerv version next", "nerv-release version next"
	}
	return fmt.Sprintf("%s moved to the nerv-release tool: install it with nerv install --tools nerv-release, then run %s.", name, replacement)
}

func pluginFS() fs.FS {
	return nerv.PluginFS()
}

// signalNotify and signalStop are signal.Notify and signal.Stop; tests
// replace them to observe the handler's lifetime without real signals.
var (
	signalNotify = signal.Notify
	signalStop   = signal.Stop
)

// signalContext returns a context cancelled by the first of cancelSignals,
// a stop function (call it when the command returns), and received, which
// reports that signal (nil when none arrived).
//
// The signals stop being caught as soon as the context is done (by a signal,
// the parent or stop), before the cancellation is visible: a second Ctrl-C
// then gets the default behaviour and ends the process at once instead of
// being swallowed while the command winds down.
func signalContext(parent context.Context) (ctx context.Context, stop context.CancelFunc, received func() os.Signal) {
	ctx, cancel := context.WithCancel(parent)
	ch := make(chan os.Signal, 1)
	signalNotify(ch, cancelSignals...)
	var (
		mu     sync.Mutex
		got    os.Signal
		once   sync.Once
		closed = make(chan struct{})
	)
	release := func() { once.Do(func() { signalStop(ch); close(closed) }) }
	go func() {
		select {
		case sig := <-ch:
			mu.Lock()
			got = sig
			mu.Unlock()
			release()
			cancel()
		case <-ctx.Done():
			release()
		}
	}()
	stop = func() {
		cancel()
		<-closed
	}
	received = func() os.Signal {
		mu.Lock()
		defer mu.Unlock()
		return got
	}
	return ctx, stop, received
}

// gitTopLevel resolves dir to the top level of its git repository. A
// directory outside any repository is a refusal; a git that cannot run is
// an environment error.
func gitTopLevel(ctx context.Context, runner runner.Runner, dir string) (string, error) {
	out, errOut, code, err := runner.Run(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", errors.New("git is required (git was not found in PATH)")
		}
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	if code != 0 {
		detail := strings.TrimSpace(errOut)
		if detail != "" {
			detail = " (" + detail + ")"
		}
		return "", &refusal.Error{Err: fmt.Errorf("%s is not inside a git repository%s", dir, detail)}
	}
	return strings.TrimSpace(out), nil
}
