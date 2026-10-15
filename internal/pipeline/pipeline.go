// Package pipeline runs the steps of `nerv pipeline`, a standalone copy of
// dim.comprobantes.api's GitHub Actions CI/CD (.github/workflows/ci-cd.yml)
// for repositories that deploy a Docker image. Each step is one job of that
// workflow, run from the pipeline: section of the repository's local
// .nerv/nerv.yaml (internal/pipelineconfig).
//
// The validate and test steps run in the user's working tree, as CI runs on
// a checkout: they verify the code as it is, uncommitted changes included.
// Every command runs through internal/shiprun as an argument list (no
// shell), its output streamed with the prefix "[pipeline:<step>] ", and the
// whole process tree killed on cancellation (internal/proc).
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

// Step statuses.
const (
	StatusPassed      = "passed"
	StatusFailed      = "failed"
	StatusInterrupted = "interrupted"
)

// TrxFile is the test results file the test step asks dotnet test for (the
// logger dim's workflow configures).
const TrxFile = "test-results.trx"

// CoverageFile is the report coverlet's collector writes per test project.
const CoverageFile = "coverage.cobertura.xml"

// StepRunner runs one process step; *shiprun.Runner is one.
type StepRunner interface {
	Run(ctx context.Context, step shiprun.Step, vars shiprun.Vars) error
}

// Steps runs pipeline steps for the repository at Repo.
type Steps struct {
	// Repo is the repository's absolute top level (the working tree).
	Repo string
	// Config is the loaded pipeline: section.
	Config *pipelineconfig.Pipeline
	// Run runs the commands.
	Run StepRunner
}

// CommandResult is one command a step ran: its argument list and exit code
// (-1 when it did not start or was killed).
type CommandResult struct {
	Argv []string `json:"argv"`
	Exit int      `json:"exit"`
}

// Artifacts are the files a step produced (the test step's results).
type Artifacts struct {
	// ResultsDir holds the test step's trx and coverage reports.
	ResultsDir string `json:"results_dir,omitempty"`
	// Trx is the test results file.
	Trx string `json:"trx,omitempty"`
	// Coverage are the coverlet cobertura reports, one per test project.
	Coverage []string `json:"coverage,omitempty"`
}

// Result is what a step did.
type Result struct {
	Step      string          `json:"step"`
	Status    string          `json:"status"`
	Commands  []CommandResult `json:"commands"`
	Artifacts Artifacts       `json:"artifacts"`
	Warnings  []string        `json:"warnings,omitempty"`
	Error     string          `json:"error,omitempty"`
	// Err is the failure, for callers (not serialised).
	Err error `json:"-"`
}

// dotnetDefaultEnv is dim's workflow environment for every dotnet command;
// pipeline.dotnet.env overrides it.
var dotnetDefaultEnv = map[string]string{"DOTNET_NOLOGO": "true", "DOTNET_CLI_TELEMETRY_OPTOUT": "true"}

// ResultsDir is where the test step writes its trx and coverage reports:
// <repo>/.nerv/artifacts/pipeline/test, emptied at the start of every run.
// <repo>/.nerv/artifacts holds a .gitignore of "*", so it is never
// committed, without touching the repository's own .gitignore.
func ResultsDir(repo string) string {
	return filepath.Join(repo, ".nerv", "artifacts", "pipeline", "test")
}

// Validate is dim's validate job: dotnet restore <solution>, then dotnet
// format <solution> --verify-no-changes.
func (s Steps) Validate(ctx context.Context) Result {
	sln := s.Config.Dotnet.Solution
	r := &Result{Step: "validate", Status: StatusPassed, Commands: []CommandResult{}}
	s.run(ctx, r,
		[]string{"dotnet", "restore", sln},
		[]string{"dotnet", "format", sln, "--verify-no-changes"})
	return *r
}

// Test is dim's test job: restore, the single build (with -warnaserror
// unless warnings_as_errors is false), then dotnet test --no-build with
// coverlet's collector, the runsettings when set and a trx logger. The
// results go to ResultsDir (dim uploads them as an artifact); the coverage
// thresholds are whatever the runsettings and coverlet do: NERV does not
// check them itself.
func (s Steps) Test(ctx context.Context) Result {
	d := s.Config.Dotnet
	r := &Result{Step: "test", Status: StatusPassed, Commands: []CommandResult{}}
	results := ResultsDir(s.Repo)
	if err := resetResults(s.Repo, results); err != nil {
		r.fail(err)
		return *r
	}
	r.Artifacts = Artifacts{ResultsDir: results, Trx: filepath.Join(results, TrxFile)}
	if w := thresholdWarning(s.Repo, d.Runsettings); w != "" {
		r.Warnings = append(r.Warnings, w)
	}
	build := []string{"dotnet", "build", d.Solution, "--configuration", d.Configuration, "--no-restore"}
	if d.TreatWarningsAsErrors() {
		build = append(build, "-warnaserror")
	}
	test := []string{"dotnet", "test", d.Solution, "--configuration", d.Configuration, "--no-build", "--collect", "XPlat Code Coverage"}
	if d.Runsettings != "" {
		test = append(test, "--settings", d.Runsettings)
	}
	test = append(test, "--logger", "trx;LogFileName="+TrxFile, "--results-directory", results)
	s.run(ctx, r, []string{"dotnet", "restore", d.Solution}, build, test)
	// Reported even after a failure: a breached coverage threshold or a
	// failing test still leaves the reports (dim uploads them always).
	r.Artifacts.Coverage = coverageReports(results)
	return *r
}

// run runs each command in order, stopping at the first failure, and
// records it in r.
func (s Steps) run(ctx context.Context, r *Result, cmds ...[]string) {
	env := maps.Clone(dotnetDefaultEnv)
	maps.Copy(env, s.Config.Dotnet.Env)
	for _, argv := range cmds {
		step := shiprun.Step{Name: r.Step, Stage: r.Step, Command: shipconfig.Command{Env: env}, Exec: argv}
		err := s.Run.Run(ctx, step, shiprun.Vars{Project: "pipeline"})
		cr := CommandResult{Argv: argv, Exit: 0}
		if err != nil {
			cr.Exit = -1
			var se *shiprun.StepError
			if errors.As(err, &se) && se.ExitCode >= 0 {
				cr.Exit = se.ExitCode
			}
		}
		r.Commands = append(r.Commands, cr)
		if err != nil {
			r.fail(fmt.Errorf("%s: %w", strings.Join(argv[:2], " "), err))
			if ctx.Err() != nil {
				r.Status = StatusInterrupted
			}
			return
		}
	}
}

func (r *Result) fail(err error) {
	r.Status, r.Error, r.Err = StatusFailed, err.Error(), err
}

// resetResults empties the results directory and makes .nerv/artifacts
// ignore itself.
func resetResults(repo, results string) error {
	artifacts := filepath.Join(repo, ".nerv", "artifacts")
	if rel, err := filepath.Rel(artifacts, results); err != nil || !filepath.IsLocal(rel) {
		return fmt.Errorf("results directory %s is not inside %s", results, artifacts)
	}
	if err := os.RemoveAll(results); err != nil {
		return fmt.Errorf("emptying %s: %w", results, err)
	}
	if err := os.MkdirAll(results, 0o755); err != nil {
		return err
	}
	ignore := filepath.Join(artifacts, ".gitignore")
	if _, err := os.Stat(ignore); errors.Is(err, os.ErrNotExist) {
		return os.WriteFile(ignore, []byte("*\n"), 0o644)
	}
	return nil
}

// coverageReports lists <results>/<run>/coverage.cobertura.xml, one per test
// project (the copies dotnet test attaches under the trx's In/ directory
// are left out), sorted.
func coverageReports(results string) []string {
	entries, err := os.ReadDir(results)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		p := filepath.Join(results, e.Name(), CoverageFile)
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

var thresholdRe = regexp.MustCompile(`<Threshold>\s*[^<\s]`)

// thresholdWarning says, when the runsettings declares a coverlet
// <Threshold>, that it is not enforced: coverlet's collector ("XPlat Code
// Coverage") ignores it (observed with coverlet.collector 6.0.4: 50% line
// coverage against a 100% threshold, dotnet test exit 0), and NERV does
// not check it either.
func thresholdWarning(repo, runsettings string) string {
	if runsettings == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(runsettings)))
	if err != nil || !thresholdRe.Match(data) {
		return ""
	}
	return fmt.Sprintf("%s declares a coverage <Threshold>, which is not enforced: coverlet's collector (XPlat Code Coverage) ignores it, and nerv does not check it", runsettings)
}
