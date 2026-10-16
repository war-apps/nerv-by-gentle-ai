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
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipplan"
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

// CoverageFile is the Cobertura report coverlet's collector writes per test
// project; CoverageJSONFile its JSON report, requested when a coverage
// threshold is checked.
const (
	CoverageFile     = "coverage.cobertura.xml"
	CoverageJSONFile = "coverage.json"
)

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
	// CoverageJSON are coverlet's JSON reports, written when a coverage
	// threshold is checked.
	CoverageJSON []string `json:"coverage_json,omitempty"`
}

// Result is what a step did.
type Result struct {
	Step      string          `json:"step"`
	Status    string          `json:"status"`
	Commands  []CommandResult `json:"commands"`
	Artifacts Artifacts       `json:"artifacts"`
	// Notes say what was checked beyond the commands (the coverage gate).
	Notes []string `json:"notes,omitempty"`
	// Coverage is the test step's coverage gate: mode, thresholds checked
	// and measured values.
	Coverage *CoverageResult `json:"coverage,omitempty"`
	Error    string          `json:"error,omitempty"`
	// Err is the failure, for callers (not serialised).
	Err error `json:"-"`
}

// dotnetDefaultEnv is the environment of every dotnet command: dim's
// workflow environment plus shipplan's no-build-server defaults;
// pipeline.dotnet.env overrides any of them.
func dotnetDefaultEnv() map[string]string {
	env := shipplan.DotnetBuildServerEnv()
	env["DOTNET_NOLOGO"], env["DOTNET_CLI_TELEMETRY_OPTOUT"] = "true", "true"
	return env
}

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
		[]string{"dotnet", "restore", sln, shipplan.DisableBuildServers},
		[]string{"dotnet", "format", sln, "--verify-no-changes"})
	return *r
}

// Test is dim's test job: restore, the single build (with -warnaserror
// unless warnings_as_errors is false), then dotnet test --no-build with
// coverlet's collector, the runsettings when set and a trx logger. The
// results go to ResultsDir (dim uploads them as an artifact). Then the
// coverage thresholds of the runsettings are enforced on the reports
// (checkCoverage): coverlet's collector ignores them.
func (s Steps) Test(ctx context.Context) Result {
	d := s.Config.Dotnet
	r := &Result{Step: "test", Status: StatusPassed, Commands: []CommandResult{}}
	results := ResultsDir(s.Repo)
	if err := resetResults(s.Repo, results); err != nil {
		r.fail(err)
		return *r
	}
	r.Artifacts = Artifacts{ResultsDir: results, Trx: filepath.Join(results, TrxFile)}
	var g *gate
	if d.Runsettings != "" {
		data, err := os.ReadFile(repoPath(s.Repo, d.Runsettings))
		if err != nil {
			r.fail(fmt.Errorf("reading %s: %w", d.Runsettings, err))
			return *r
		}
		if g, err = readGate(data); err != nil {
			r.fail(err)
			return *r
		}
	}
	build := []string{"dotnet", "build", d.Solution, "--configuration", d.Configuration, "--no-restore", shipplan.DisableBuildServers}
	if d.TreatWarningsAsErrors() {
		build = append(build, "-warnaserror")
	}
	// The thresholds checked: the runsettings' ones, overridden key by key
	// by pipeline.dotnet.coverage.thresholds; none with mode off.
	mode := d.Coverage.Mode
	if mode == "" {
		mode = pipelineconfig.DefaultCoverageMode
	}
	g = effectiveGate(g, d.Coverage, mode)
	r.Coverage = &CoverageResult{Mode: mode, Met: true}
	test := []string{"dotnet", "test", d.Solution, "--configuration", d.Configuration, "--no-build", shipplan.DisableBuildServers, "--collect", g.collectArg()}
	if d.Runsettings != "" {
		test = append(test, "--settings", d.Runsettings)
	}
	test = append(test, "--logger", "trx;LogFileName="+TrxFile, "--results-directory", results)
	s.run(ctx, r, []string{"dotnet", "restore", d.Solution, shipplan.DisableBuildServers}, build, test)
	// Reported even after a failure: a failing test still leaves the
	// reports (dim uploads them always).
	r.Artifacts.Coverage = reportsNamed(results, CoverageFile)
	r.Artifacts.CoverageJSON = reportsNamed(results, CoverageJSONFile)
	if r.Status == StatusPassed && g != nil && g.th != nil {
		gateCoverage(r, d.Runsettings, mode, g.th)
	}
	return *r
}

// run runs each command in order, stopping at the first failure, and
// records it in r.
func (s Steps) run(ctx context.Context, r *Result, cmds ...[]string) {
	env := dotnetDefaultEnv()
	maps.Copy(env, s.Config.Dotnet.Env)
	vars := shiprun.Vars{Project: "pipeline"}
	probe := shipplan.NewBuildServersProbe(s.Run)
	for _, argv := range cmds {
		step := shiprun.Step{Name: r.Step, Stage: r.Step, Command: shipconfig.Command{Env: env}, Exec: argv}
		var note string
		step.Exec, note = probe.Argv(ctx, step, vars)
		if note != "" {
			r.Notes = append(r.Notes, note)
		}
		argv = step.Exec
		err := s.Run.Run(ctx, step, vars)
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
// ignore itself. Paths are checked after resolving symlinks: a
// .nerv/artifacts (or anything below it) linked outside the repository is
// refused before anything is removed.
func resetResults(repo, results string) error {
	artifacts := filepath.Join(repo, ".nerv", "artifacts")
	if rel, err := filepath.Rel(artifacts, results); err != nil || !filepath.IsLocal(rel) {
		return fmt.Errorf("results directory %s is not inside %s", results, artifacts)
	}
	// Before creating anything: what exists of .nerv/artifacts (a symlinked
	// .nerv or .nerv/artifacts included) must resolve inside the repository.
	if err := checkAncestorInside(repo, artifacts); err != nil {
		return err
	}
	if err := os.MkdirAll(artifacts, 0o755); err != nil {
		return err
	}
	if err := checkRealInside(repo, artifacts, results); err != nil {
		return err
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

// checkRealInside refuses when, symlinks resolved, artifacts is not inside
// the repository or results (or its deepest existing ancestor) is not
// inside artifacts.
func checkRealInside(repo, artifacts, results string) error {
	realRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return err
	}
	realArtifacts, err := filepath.EvalSymlinks(artifacts)
	if err != nil {
		return err
	}
	if !within(realRepo, realArtifacts, false) {
		return fmt.Errorf("%s resolves to %s, outside the repository: refusing to empty anything there", artifacts, realArtifacts)
	}
	existing := deepestExisting(results)
	realExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return err
	}
	if !within(realArtifacts, realExisting, existing == artifacts) {
		return fmt.Errorf("%s resolves to %s, outside %s: refusing to empty it", existing, realExisting, realArtifacts)
	}
	return nil
}

// checkAncestorInside refuses when the deepest existing part of path,
// symlinks resolved, is outside the repository (the repository itself is
// fine: what is missing below it is created there).
func checkAncestorInside(repo, path string) error {
	realRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return err
	}
	existing := deepestExisting(path)
	realExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return err
	}
	if !within(realRepo, realExisting, existing == repo) {
		return fmt.Errorf("%s resolves to %s, outside the repository: refusing to create or empty anything there", existing, realExisting)
	}
	return nil
}

// deepestExisting is path or its deepest ancestor that exists (as a file,
// directory or symlink).
func deepestExisting(path string) string {
	for {
		if _, err := os.Lstat(path); err == nil {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path
		}
		path = parent
	}
}

// within reports whether path is inside dir (or equal to it when same is
// allowed).
func within(dir, path string, same bool) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return same
	}
	return filepath.IsLocal(rel)
}

// reportsNamed lists <results>/<run>/<name>, one per test project (the
// copies dotnet test attaches under the trx's In/ directory are left out),
// sorted.
func reportsNamed(results, name string) []string {
	entries, err := os.ReadDir(results)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		p := filepath.Join(results, e.Name(), name)
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

// repoPath is a repository-relative path (forward slashes) under repo.
func repoPath(repo, rel string) string { return filepath.Join(repo, filepath.FromSlash(rel)) }
