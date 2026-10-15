package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

// fakeSteps records every step and fails the ones named in fail.
type fakeSteps struct {
	steps []shiprun.Step
	vars  []shiprun.Vars
	fail  map[string]error
}

func (f *fakeSteps) Run(_ context.Context, s shiprun.Step, v shiprun.Vars) error {
	f.steps = append(f.steps, s)
	f.vars = append(f.vars, v)
	return f.fail[strings.Join(s.Exec[:2], " ")]
}

func (f *fakeSteps) argvs() [][]string {
	var out [][]string
	for _, s := range f.steps {
		out = append(out, s.Exec)
	}
	return out
}

func config(mod func(*pipelineconfig.Pipeline)) *pipelineconfig.Pipeline {
	yes := true
	p := &pipelineconfig.Pipeline{Dotnet: pipelineconfig.Dotnet{Solution: "App.slnx", Configuration: "Release", WarningsAsErrors: &yes}}
	if mod != nil {
		mod(p)
	}
	return p
}

func TestValidate_RestoresThenChecksTheFormat(t *testing.T) {
	f := &fakeSteps{}
	repo := t.TempDir()
	cfg := config(func(p *pipelineconfig.Pipeline) { p.Dotnet.Env = map[string]string{"NUGET_PACKAGES": "/cache/nuget"} })
	r := Steps{Repo: repo, Config: cfg, Run: f}.Validate(context.Background())
	want := [][]string{
		{"dotnet", "restore", "App.slnx"},
		{"dotnet", "format", "App.slnx", "--verify-no-changes"},
	}
	if !slices.EqualFunc(f.argvs(), want, slices.Equal) {
		t.Errorf("argv %v, want %v", f.argvs(), want)
	}
	if r.Step != "validate" || r.Status != StatusPassed || len(r.Commands) != 2 || r.Commands[1].Exit != 0 || r.Error != "" {
		t.Errorf("result %+v", r)
	}
	for _, s := range f.steps {
		if s.Command.Env["NUGET_PACKAGES"] != "/cache/nuget" || s.Command.Env["DOTNET_NOLOGO"] != "true" || s.Command.Env["DOTNET_CLI_TELEMETRY_OPTOUT"] != "true" {
			t.Errorf("env %v", s.Command.Env)
		}
		if s.Name != "validate" || s.Command.Workdir != "" {
			t.Errorf("step %+v", s)
		}
	}
	if f.vars[0].Project != "pipeline" {
		t.Errorf("output prefix project %q, want pipeline", f.vars[0].Project)
	}
}

func TestValidate_ConfiguredEnvOverridesTheDefaults(t *testing.T) {
	f := &fakeSteps{}
	cfg := config(func(p *pipelineconfig.Pipeline) { p.Dotnet.Env = map[string]string{"DOTNET_NOLOGO": "false"} })
	Steps{Repo: t.TempDir(), Config: cfg, Run: f}.Validate(context.Background())
	if f.steps[0].Command.Env["DOTNET_NOLOGO"] != "false" {
		t.Errorf("env %v", f.steps[0].Command.Env)
	}
}

func TestTest_Commands(t *testing.T) {
	no := false
	tests := []struct {
		name  string
		mod   func(*pipelineconfig.Pipeline)
		build []string
		test  []string
	}{
		{"defaults: warnings as errors, no runsettings", nil,
			[]string{"dotnet", "build", "App.slnx", "--configuration", "Release", "--no-restore", "-warnaserror"},
			[]string{"dotnet", "test", "App.slnx", "--configuration", "Release", "--no-build", "--collect", "XPlat Code Coverage", "--logger", "trx;LogFileName=test-results.trx", "--results-directory", "RESULTS"}},
		{"warnings allowed, runsettings, Debug", func(p *pipelineconfig.Pipeline) {
			p.Dotnet.WarningsAsErrors, p.Dotnet.Runsettings, p.Dotnet.Configuration = &no, "coverlet.runsettings", "Debug"
		},
			[]string{"dotnet", "build", "App.slnx", "--configuration", "Debug", "--no-restore"},
			[]string{"dotnet", "test", "App.slnx", "--configuration", "Debug", "--no-build", "--collect", "XPlat Code Coverage", "--settings", "coverlet.runsettings", "--logger", "trx;LogFileName=test-results.trx", "--results-directory", "RESULTS"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeSteps{}
			repo := t.TempDir()
			// A runsettings without a threshold: nothing to enforce.
			if err := os.WriteFile(filepath.Join(repo, "coverlet.runsettings"), []byte("<RunSettings />"), 0o644); err != nil {
				t.Fatal(err)
			}
			r := Steps{Repo: repo, Config: config(tt.mod), Run: f}.Test(context.Background())
			results := filepath.Join(repo, ".nerv", "artifacts", "pipeline", "test")
			for i := range tt.test {
				if tt.test[i] == "RESULTS" {
					tt.test[i] = results
				}
			}
			want := [][]string{{"dotnet", "restore", "App.slnx"}, tt.build, tt.test}
			if !slices.EqualFunc(f.argvs(), want, slices.Equal) {
				t.Errorf("argv\n %v\nwant\n %v", f.argvs(), want)
			}
			if r.Status != StatusPassed || r.Artifacts.ResultsDir != results || r.Artifacts.Trx != filepath.Join(results, "test-results.trx") {
				t.Errorf("result %+v", r)
			}
		})
	}
}

func TestTest_ArtifactsAreResetSelfIgnoredAndReported(t *testing.T) {
	repo := t.TempDir()
	results := filepath.Join(repo, ".nerv", "artifacts", "pipeline", "test")
	stale := filepath.Join(results, "old", "coverage.cobertura.xml")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fakeSteps{}
	run := &writingSteps{inner: f, write: func() {
		// What dotnet test writes: a trx and one coverage report per test project.
		for _, p := range []string{"test-results.trx", "a1/coverage.cobertura.xml", "b2/coverage.cobertura.xml"} {
			full := filepath.Join(results, filepath.FromSlash(p))
			os.MkdirAll(filepath.Dir(full), 0o755)
			os.WriteFile(full, []byte("x"), 0o644)
		}
	}}
	r := Steps{Repo: repo, Config: config(nil), Run: run}.Test(context.Background())
	if r.Status != StatusPassed {
		t.Fatalf("result %+v", r)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale report kept: %v", err)
	}
	want := []string{filepath.Join(results, "a1", "coverage.cobertura.xml"), filepath.Join(results, "b2", "coverage.cobertura.xml")}
	if !slices.Equal(r.Artifacts.Coverage, want) {
		t.Errorf("coverage %v, want %v", r.Artifacts.Coverage, want)
	}
	data, err := os.ReadFile(filepath.Join(repo, ".nerv", "artifacts", ".gitignore"))
	if err != nil || strings.TrimSpace(string(data)) != "*" {
		t.Errorf("artifacts .gitignore: %q %v", data, err)
	}
}

// writingSteps runs write when the dotnet test command runs.
type writingSteps struct {
	inner *fakeSteps
	write func()
}

func (w *writingSteps) Run(ctx context.Context, s shiprun.Step, v shiprun.Vars) error {
	if s.Exec[1] == "test" {
		w.write()
	}
	return w.inner.Run(ctx, s, v)
}

func TestSteps_StopAtTheFirstFailingCommand(t *testing.T) {
	stepErr := &shiprun.StepError{Project: "pipeline", Step: "test", Shell: "exec", Script: "dotnet", ExitCode: 1, Reason: "exit code 1"}
	f := &fakeSteps{fail: map[string]error{"dotnet build": stepErr}}
	r := Steps{Repo: t.TempDir(), Config: config(nil), Run: f}.Test(context.Background())
	if len(f.steps) != 2 {
		t.Errorf("ran %v after the failure", f.argvs())
	}
	if r.Status != StatusFailed || len(r.Commands) != 2 || r.Commands[1].Exit != 1 || !strings.Contains(r.Error, "dotnet build") {
		t.Errorf("result %+v", r)
	}
	if !errors.Is(r.Err, stepErr) {
		t.Errorf("Err %v", r.Err)
	}
}

func TestSteps_InterruptedOrUnstartable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &fakeSteps{fail: map[string]error{"dotnet restore": &shiprun.StepError{ExitCode: -1, Reason: "interrupted", Err: context.Canceled}}}
	r := Steps{Repo: t.TempDir(), Config: config(nil), Run: f}.Validate(ctx)
	if r.Status != StatusInterrupted || r.Commands[0].Exit != -1 {
		t.Errorf("result %+v", r)
	}
}

func TestTest_ResultsBehindASymlinkOutsideTheArtifactsAreRefused(t *testing.T) {
	repo, outside := t.TempDir(), t.TempDir()
	keep := filepath.Join(outside, "test", "keep.txt")
	if err := os.MkdirAll(filepath.Dir(keep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".nerv", "artifacts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, ".nerv", "artifacts", "pipeline")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	f := &fakeSteps{}
	r := Steps{Repo: repo, Config: config(nil), Run: f}.Test(context.Background())
	if r.Status != StatusFailed || !strings.Contains(r.Error, "outside") || len(f.steps) != 0 {
		t.Errorf("result %+v, ran %v", r, f.argvs())
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("a file outside the repository was removed: %v", err)
	}
}
