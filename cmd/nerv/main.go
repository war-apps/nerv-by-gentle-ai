// Command nerv is the Nerv by Gentle-AI CLI: it materializes the embedded
// plugin tree, configures NERV, and reports version information.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/semrel"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipplan"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
)

const usage = `Usage: nerv <command> [flags]

Commands:
  version [--json]     Print the binary and plugin versions
  version next [flags] Compute the next release version with semantic-release
                       (see "nerv version next --help")
  ship [flags]         Release the repository from origin: pipeline,
                       semantic-release, publish (see "nerv ship --help")
  pipeline <step>      Run a step of the Docker pipeline: validate, test
                       (see "nerv pipeline --help")
  configure [flags]    Configure NERV (non-interactive; see "nerv configure --help")
  install [flags]      Install NERV (see "nerv install --help")
  uninstall [flags]    Uninstall NERV (see "nerv uninstall --help")
  apply-models [flags] Apply model/effort assignments to the cached agents
  skills [flags]       Install or verify skills (see "nerv skills --help")
  spec-compose [flags] Merge a delta spec into a canonical spec (see "nerv spec-compose --help")

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
	// ComputeVersions is the `nerv version next` engine (semrel.Compute);
	// tests inject a recorded fake. Nil means semrel.Compute.
	ComputeVersions func(context.Context, semrel.Options) ([]semrel.ProjectResult, error)
	// SignalContext derives the context a long-running command runs under,
	// cancelled on SIGINT, SIGTERM and (on Unix) SIGHUP, with a stop function
	// and the signal received (nil if none). Tests inject a controllable one.
	// Nil means signalContext.
	SignalContext func(context.Context) (context.Context, context.CancelFunc, func() os.Signal)
	// OpenShipWorkspace creates nerv ship's release clone; tests inject a
	// fake. Nil means semrel.NewWorkspace.
	OpenShipWorkspace func(context.Context, semrel.WorkspaceOptions) (shipWorkspace, error)
	// ShipSteps returns the runner of nerv ship's process steps for the
	// release clone at repoRoot, streaming to output; tests inject a
	// recorder. Nil means a shiprun.Runner.
	ShipSteps func(repoRoot string, output io.Writer) shipplan.StepRunner
	// GitHubToken returns a token for semantic-release when the environment
	// has no GH_TOKEN or GITHUB_TOKEN; tests inject a fixed one so no real
	// credential is read. Nil means `gh auth token` through Runner.
	GitHubToken func(context.Context) string
	// PipelineSteps returns the runner of nerv pipeline's commands for the
	// repository at repoRoot, streaming to output; tests inject a recorder.
	// Nil means a shiprun.Runner.
	PipelineSteps func(repoRoot string, output io.Writer) pipeline.StepRunner
	// WriteReleaseOutputs writes nerv pipeline release's release.json; tests
	// inject a failure. Nil means pipeline.WriteReleaseOutputs.
	WriteReleaseOutputs func(repo string, o pipeline.ReleaseOutputs) (string, error)
	// OpenDockerWorkspace, DockerProc and DockerArgs are nerv pipeline
	// docker's build clone, docker runner and computed-argument runner;
	// tests inject fakes. Nil means semrel.NewWorkspace, proc.ExecRunner and
	// a shiprun.Runner.
	OpenDockerWorkspace func(context.Context, semrel.WorkspaceOptions) (dockerWorkspace, error)
	DockerProc          proc.Runner
	DockerArgs          func(repoRoot string, output io.Writer) pipeline.ArgRunner
	// OpenSyncWorkspace is nerv pipeline sync's clone; tests inject a fake.
	// Nil means semrel.NewWorkspace.
	OpenSyncWorkspace func(context.Context, semrel.WorkspaceOptions) (syncWorkspace, error)
	// DeployProc runs nerv pipeline deploy's infisical, ssh and sshpass
	// commands and DeploySleep waits between its health checks; tests
	// inject fakes. Nil means proc.ExecRunner and a real wait.
	DeployProc  proc.Runner
	DeploySleep func(context.Context, time.Duration) error
}

func defaultOptions() options {
	return options{
		PluginFS:        pluginFS(),
		Runner:          env.ExecRunner{},
		Now:             time.Now,
		LookPath:        env.LookPath,
		Getenv:          os.Getenv,
		Stdin:           os.Stdin,
		IsTerminal:      stdinIsTerminal,
		ComputeVersions: semrel.Compute,
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
	case "spec-compose":
		return runSpecCompose(args[1:], stdout, stderr)
	case "ship":
		return runShip(args[1:], stdout, stderr, opts)
	case "pipeline":
		return runPipeline(args[1:], stdout, stderr, opts)
	case "release":
		return runRelease(args[1:], stdout, stderr, opts)
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

func pluginFS() fs.FS {
	return nerv.PluginFS()
}
