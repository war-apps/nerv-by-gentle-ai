package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

const pipelineUsage = `Usage: nerv pipeline <step> [flags]

Runs one step of the Docker pipeline configured in the pipeline: section
of the repository's local .nerv/nerv.yaml (see docs/pipeline.md), a
standalone copy of dim.comprobantes.api's CI/CD.

Steps:
  validate   dotnet restore, then dotnet format --verify-no-changes
  test       dotnet restore, build (-warnaserror unless warnings_as_errors
             is false), dotnet test with coverlet's collector, the
             runsettings and a trx logger, then the runsettings'
             coverage thresholds; results in
             .nerv/artifacts/pipeline/test (git-ignored)

  release    semantic-release on origin's state, as nerv ship releases it
             but without building or publishing: version files
             (pipeline.release.version_files), release commit, CHANGELOG,
             tag, GitHub release; outputs in
             .nerv/artifacts/pipeline/release.json

  docker     build, tag and push every pipeline.images entry from the
             released commit (release.json), in a clone of origin at the
             release tag; result in .nerv/artifacts/pipeline/docker.json

  sync       merge the released branch back as dim's sync jobs: main or
             a pipeline.sync.release branch into pipeline.sync.develop,
             and main into every pipeline.sync.release branch on origin
             (--no-ff, "chore(sync): merge <source> into <branch> [skip
             ci]"), committed as you and pushed with your credentials; a
             conflict is aborted, the other branches still sync, and the
             step fails at the end; result in
             .nerv/artifacts/pipeline/sync.json

validate and test run in your working tree, as CI runs on a checkout: they
check the code as it is, uncommitted changes included. release releases
what is on origin: your local branch must equal origin's.

Flags:
  --cwd <dir>   A directory inside the repository (default ".")
  --json        Print the result as one JSON document

release also takes:
  --branch <name>               Branch to release (default: the current one)
  --dry-run                     Compute the release only; never release
  --yes                         Release without asking (required without a
                                terminal)
  --expect <project>=<version>  Release only this approved version
  --fetch-timeout <dur>         Bound for each read of origin (default 60s)

docker also takes:
  --dry-run       Build the images only; never log in or push (required
                  after a dry-run release)
  --no-login      Use docker's own credentials for the registry instead of
                  logging in with pipeline.registry's username_env and
                  token_env (the token is given on stdin only)
  --fetch-timeout <dur>  Bound for each read of origin (default 60s)

sync also takes:
  --dry-run       Preview the merges (conflicts included); never merge or
                  push (required after a dry-run release)
  --yes           Merge and push without asking (required without a
                  terminal)
  --fetch-timeout <dur>  Bound for each read of origin (default 60s)

Progress goes to stderr (each command's output prefixed
"[pipeline:<step>] "), the result to stdout. Exit codes: 0 passed, 1
refused (no or invalid pipeline: section, not a git repository), 2 a
command failed or the environment is missing a tool, 128 + the signal when
interrupted.
`

// pipelineStepNames are the implemented steps, in pipeline order.
var pipelineStepNames = []string{"validate", "test", "release", "docker", "sync"}

// pipelineRefused is the status of a step that did not start.
const pipelineRefused = "refused"

func runPipeline(args []string, stdout, stderr io.Writer, opts options) int {
	if len(args) == 0 {
		return pipelineUsageError(stderr, "a step is required")
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprint(stdout, pipelineUsage)
		return 0
	}
	step := args[0]
	switch step {
	case "release":
		return runPipelineRelease(args[1:], stdout, stderr, opts)
	case "docker":
		return runPipelineDocker(args[1:], stdout, stderr, opts)
	case "sync":
		return runPipelineSync(args[1:], stdout, stderr, opts)
	}
	if !slices.Contains(pipelineStepNames, step) {
		return pipelineUsageError(stderr, fmt.Sprintf("unknown step %q (want one of %s)", step, strings.Join(pipelineStepNames, ", ")))
	}
	fs := flag.NewFlagSet("pipeline "+step, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cwd := fs.String("cwd", ".", "")
	jsonOut := fs.Bool("json", false, "")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, pipelineUsage)
			return 0
		}
		return pipelineUsageError(stderr, err.Error())
	}
	if fs.NArg() > 0 {
		return pipelineUsageError(stderr, fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	signalCtx := opts.SignalContext
	if signalCtx == nil {
		signalCtx = signalContext
	}
	ctx, stop, received := signalCtx(context.Background())
	defer stop()

	res, code := runPipelineStep(ctx, step, *cwd, stderr, opts)
	if res.Status != pipelineRefused && ctx.Err() != nil {
		res.Status = pipeline.StatusInterrupted
		code = interruptedExitCode(received())
		fmt.Fprintf(stderr, "nerv pipeline %s: interrupted\n", step)
	}
	if err := writePipelineResult(stdout, *jsonOut, res); err != nil {
		fmt.Fprintf(stderr, "nerv pipeline %s: writing the result: %v\n", step, err)
		if code == 0 {
			code = 2
		}
	}
	return code
}

// runPipelineStep loads the configuration and runs step; it returns the
// result and the exit code (0 passed, 1 refused, 2 failed).
func runPipelineStep(ctx context.Context, step, cwd string, stderr io.Writer, opts options) (pipeline.Result, int) {
	res := pipeline.Result{Step: step, Commands: []pipeline.CommandResult{}}
	refused := func(err error) (pipeline.Result, int) {
		res.Status, res.Error = pipelineRefused, err.Error()
		fmt.Fprintf(stderr, "nerv pipeline %s: %v\n", step, err)
		return res, 1
	}
	top, err := gitTopLevel(ctx, opts.Runner, cwd)
	if err != nil {
		if refusal.Is(err) {
			return refused(err)
		}
		res.Status, res.Error = pipeline.StatusFailed, err.Error()
		fmt.Fprintf(stderr, "nerv pipeline %s: %v\n", step, err)
		return res, 2
	}
	cfg, err := pipelineconfig.Load(top)
	switch {
	case errors.Is(err, pipelineconfig.ErrNotConfigured):
		return refused(errors.New("the repository has no pipeline: section in .nerv/nerv.yaml; see docs/pipeline.md"))
	case err != nil:
		return refused(err)
	}
	newSteps := opts.PipelineSteps
	if newSteps == nil {
		newSteps = func(root string, out io.Writer) pipeline.StepRunner {
			return &shiprun.Runner{RepoRoot: root, Output: out}
		}
	}
	steps := pipeline.Steps{Repo: top, Config: cfg, Run: newSteps(top, stderr)}
	fmt.Fprintf(stderr, "nerv pipeline %s: in the working tree %s\n", step, top)
	switch step {
	case "validate":
		res = steps.Validate(ctx)
	case "test":
		res = steps.Test(ctx)
	}

	if res.Status != pipeline.StatusPassed {
		fmt.Fprintf(stderr, "nerv pipeline %s: %s\n", step, res.Error)
		return res, 2
	}
	return res, 0
}

func pipelineUsageError(stderr io.Writer, problem string) int {
	fmt.Fprintf(stderr, "nerv pipeline: %s\n\n%s", problem, pipelineUsage)
	return 2
}

// writePipelineResult writes the result in one write: the JSON document or
// a short text summary.
func writePipelineResult(w io.Writer, asJSON bool, res pipeline.Result) error {
	var b bytes.Buffer
	if asJSON {
		data, err := json.Marshal(res)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
	} else {
		fmt.Fprintf(&b, "%s: %s\n", res.Step, res.Status)
		for _, c := range res.Commands {
			fmt.Fprintf(&b, "  exit %d  %s\n", c.Exit, strings.Join(c.Argv, " "))
		}
		if a := res.Artifacts; a.ResultsDir != "" {
			fmt.Fprintf(&b, "  results: %s\n", a.ResultsDir)
			for _, c := range a.Coverage {
				fmt.Fprintf(&b, "  coverage: %s\n", c)
			}
		}
		for _, n := range res.Notes {
			fmt.Fprintf(&b, "  %s\n", n)
		}
		if res.Error != "" {
			fmt.Fprintf(&b, "  error: %s\n", res.Error)
		}
	}
	if _, err := w.Write(b.Bytes()); err != nil {
		return err
	}
	if f, ok := w.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}
