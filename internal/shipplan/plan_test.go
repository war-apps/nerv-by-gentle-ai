package shipplan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/artifact"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

// artifacts is the artifacts directory the tests plan with.
var artifacts = filepath.Join(string(filepath.Separator)+"tmp", "nerv-ship")

func ctxFor(version, channel string) Context {
	return Context{Version: version, Channel: channel, Tag: "v" + version, GitHead: "abc123", ArtifactsDir: artifacts}
}

// line renders a step for comparisons: "<stage>|<name>|<command>".
func line(s Step) string {
	cmd := ""
	switch {
	case s.Func != nil:
		cmd = s.Describe
	case len(s.Exec) > 0:
		cmd = strings.Join(s.Exec, " ")
	default:
		cmd = s.Command.Shell + ": " + s.Command.Run
	}
	return s.Stage + "|" + s.Name + "|" + cmd
}

func lines(p *Plan) []string {
	var out []string
	for _, st := range p.Stages {
		for _, s := range st.Steps {
			out = append(out, line(s))
		}
	}
	return out
}

func assertLines(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("steps:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func bash(run string) shipconfig.Command { return shipconfig.Command{Run: run, Shell: "bash"} }

// scriptProject is a script-mode project with every stage.
func scriptProject() shipconfig.Project {
	return shipconfig.Project{Name: "api", Path: ".", Build: &shipconfig.Build{Mode: shipconfig.ModeScript},
		Stages: map[string]shipconfig.Command{
			"restore":  bash("dotnet restore"),
			"lint":     bash("dotnet format --verify-no-changes"),
			"build":    bash("dotnet build -c Release"),
			"test":     bash("dotnet test --no-build"),
			"coverage": bash("reportgenerator"),
			"package":  bash(`dotnet pack -o "$NERV_RELEASE_OUTPUTS_DIR"`),
		}}
}

// composeProject is a compose-mode project with a test service and two
// images.
func composeProject() shipconfig.Project {
	return shipconfig.Project{Name: "api", Path: "api", Build: &shipconfig.Build{
		Mode: shipconfig.ModeCompose, File: "api/docker-compose.yml", TestService: "tests",
		Images: []string{"ghcr.io/acme/api:${version}", "ghcr.io/acme/worker:${channel}-${tag}"},
	}}
}

func stepOf(t *testing.T, plan *Plan, name string) Step {
	t.Helper()
	for _, st := range plan.Stages {
		for _, s := range st.Steps {
			if s.Name == name {
				return s
			}
		}
	}
	t.Fatalf("no step %q in %v", name, lines(plan))
	return Step{}
}

func TestBuild_ScriptModeRunsTheProjectsStagesInOrder(t *testing.T) {
	p := scriptProject()
	p.Submodules = true
	delete(p.Stages, "lint")
	p.Hooks = map[string][]shipconfig.Command{
		"before_test":    {bash("echo one"), {Run: "echo two", Shell: "pwsh"}},
		"after_test":     {{Run: "node report.mjs", Shell: "node"}},
		"before_lint":    {bash("echo never")}, // no lint command: skipped
		"after_build":    {bash("echo built")},
		"prepare":        {bash("./bump.sh")},
		"fail":           {bash("echo fail")},
		"before_restore": nil,
	}
	plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != shipconfig.ModeScript {
		t.Errorf("mode %q", plan.Mode)
	}
	assertLines(t, lines(plan), []string{
		"restore|git:submodules|git submodule update --init --recursive",
		"restore|restore|bash: dotnet restore",
		"build|build|bash: dotnet build -c Release",
		"after_build|after_build|bash: echo built",
		"before_test|before_test|bash: echo one",
		"before_test|before_test|pwsh: echo two",
		"test|test|bash: dotnet test --no-build",
		"after_test|after_test|node: node report.mjs",
		"coverage|coverage|bash: reportgenerator",
		`package|package|bash: dotnet pack -o "$NERV_RELEASE_OUTPUTS_DIR"`,
	})
	for _, key := range []string{"prepare", "fail"} {
		if len(plan.Hooks[key]) != 1 || plan.Hooks[key][0].Stage != key {
			t.Errorf("lifecycle hook %s = %+v", key, plan.Hooks[key])
		}
	}
	if len(plan.Images) != 0 {
		t.Errorf("script mode images %v", plan.Images)
	}
}

func TestBuild_ScriptModeStageKeepsItsSettings(t *testing.T) {
	p := scriptProject()
	p.Stages["build"] = shipconfig.Command{Run: "make all", Shell: "bash", Workdir: "src", Timeout: "10m"}
	plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	if s := stepOf(t, plan, "build"); s.Command.Workdir != "src" || s.Command.Timeout != "10m" || s.Command.Run != "make all" {
		t.Errorf("build step %+v, want the stage as configured", s.Command)
	}
}

// Without shared_host a stage runs only with its own env (plus NERV's
// directories); with it, the dotnet stages get the build-server env under
// their own.
func TestBuild_ScriptSharedHost(t *testing.T) {
	for _, shared := range []bool{false, true} {
		p := scriptProject()
		p.Build.SharedHost = shared
		p.Stages["lint"] = shipconfig.Command{Run: "dotnet format", Shell: "bash", Env: map[string]string{"UseSharedCompilation": "true"}}
		plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"restore", "lint", "build", "test", "coverage", "package"} {
			env := stepOf(t, plan, name).Command.Env
			for k, v := range DotnetBuildServerEnv() {
				want := shared && name != "coverage"
				if name == "lint" && k == "UseSharedCompilation" {
					if env[k] != "true" {
						t.Errorf("shared_host %v: lint's own %s lost: %v", shared, k, env)
					}
					continue
				}
				if got := env[k] == v; got != want {
					t.Errorf("shared_host %v: stages.%s has %s=%q, want present=%v", shared, name, k, env[k], want)
				}
			}
		}
		if p.Stages["lint"].Env["MSBUILDDISABLENODEREUSE"] != "" {
			t.Error("Build changed the configuration's env map")
		}
	}
}

func TestBuild_ComposeMode(t *testing.T) {
	p := composeProject()
	p.Stages = map[string]shipconfig.Command{"lint": bash("hadolint api/Dockerfile")}
	p.Hooks = map[string][]shipconfig.Command{"after_build": {bash("echo built")}}
	plan, err := Build(t.TempDir(), p, ctxFor("1.4.0-rc.1", "beta"))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(artifacts, "api", "outputs")
	assertLines(t, lines(plan), []string{
		"lint|lint|bash: hadolint api/Dockerfile",
		"build|compose:build|docker compose -f api/docker-compose.yml build",
		"after_build|after_build|bash: echo built",
		"test|compose:test|docker compose -f api/docker-compose.yml run --rm tests",
		"package|compose:save ghcr.io/acme/api:1.4.0-rc.1|docker save -o " + filepath.Join(out, "ghcr.io_acme_api_1.4.0-rc.1.tar") + " ghcr.io/acme/api:1.4.0-rc.1",
		"package|compose:save ghcr.io/acme/worker:beta-v1.4.0-rc.1|docker save -o " + filepath.Join(out, "ghcr.io_acme_worker_beta-v1.4.0-rc.1.tar") + " ghcr.io/acme/worker:beta-v1.4.0-rc.1",
	})
	if want := []Image{
		{Ref: "ghcr.io/acme/api:1.4.0-rc.1", Tar: "ghcr.io_acme_api_1.4.0-rc.1.tar"},
		{Ref: "ghcr.io/acme/worker:beta-v1.4.0-rc.1", Tar: "ghcr.io_acme_worker_beta-v1.4.0-rc.1.tar"},
	}; !slices.Equal(plan.Images, want) {
		t.Errorf("images %+v, want %+v", plan.Images, want)
	}
	// No test service: no test stage; the default channel has no channel.
	p.Build.TestService = ""
	if plan, err = Build(t.TempDir(), p, ctxFor("1.4.0", "")); err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(plan.Stages, func(s Stage) bool { return s.Name == "test" }) {
		t.Errorf("a test stage without test_service: %v", lines(plan))
	}
	if plan.Images[1].Ref != "ghcr.io/acme/worker:-v1.4.0" {
		t.Errorf("worker ref %q on the default channel", plan.Images[1].Ref)
	}
}

func TestBuild_ComposeImageRefMustBeValid(t *testing.T) {
	p := composeProject()
	p.Build.Images = []string{"ghcr.io/acme/api:${channel}"}
	if _, err := Build(t.TempDir(), p, ctxFor("1.0.0", "a b")); err == nil || !strings.Contains(err.Error(), "image") {
		t.Errorf("Build = %v, want a refusal of the expanded reference", err)
	}
}

func TestBuild_RequiresABuildMode(t *testing.T) {
	for _, b := range []*shipconfig.Build{nil, {}, {Mode: "make"}} {
		p := shipconfig.Project{Name: "api", Path: ".", Build: b}
		if _, err := Build(t.TempDir(), p, ctxFor("1.0.0", "")); err == nil || !strings.Contains(err.Error(), "build.mode") {
			t.Errorf("build %+v: Build = %v, want a refusal naming build.mode", b, err)
		}
	}
}

func TestBuild_DirectoriesAreGivenToEveryStep(t *testing.T) {
	for _, p := range []shipconfig.Project{scriptProject(), composeProject()} {
		if p.Stages == nil {
			p.Stages = map[string]shipconfig.Command{}
		}
		p.Stages["lint"] = shipconfig.Command{Run: "true", Shell: "bash", Env: map[string]string{"A": "1"}}
		p.Hooks = map[string][]shipconfig.Command{"after_test": {bash("true")}, "prepare": {bash("true")}}
		plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(artifacts, "api")
		all := plan.Hooks["prepare"]
		for _, st := range plan.Stages {
			all = append(all, st.Steps...)
		}
		for _, s := range all {
			if s.Func != nil {
				continue
			}
			if s.Command.Env[ArtifactsEnv] != dir || s.Command.Env[OutputsEnv] != filepath.Join(dir, "outputs") {
				t.Errorf("%s %s: env %v", p.Build.Mode, s.Name, s.Command.Env)
			}
		}
		if p.Stages["lint"].Env[ArtifactsEnv] != "" {
			t.Error("Build changed the configuration's env map")
		}
		if plan.OutputsDir != filepath.Join(dir, "outputs") || !slices.Contains(plan.ResetDirs, plan.OutputsDir) {
			t.Errorf("outputs dir %s, reset %v", plan.OutputsDir, plan.ResetDirs)
		}
	}
}

func TestBuild_ContextIsChecked(t *testing.T) {
	for _, c := range []Context{
		{ArtifactsDir: artifacts},
		{Version: "1.0.0"},
		{Version: "1.0.0", ArtifactsDir: "relative/dir"},
	} {
		if _, err := Build(t.TempDir(), scriptProject(), c); err == nil {
			t.Errorf("Build(%+v) = nil error", c)
		}
	}
}

// fakeRunner records the steps it runs and fails those named in fail. Its
// Capture answers the SDK probe (dotnet --version) with sdk (10.0.112 when
// empty) or sdkErr, recording the probes apart from the steps.
type fakeRunner struct {
	steps  []shiprun.Step
	fail   map[string]error
	sdk    string
	sdkErr error
	probes []shiprun.Step
}

func (f *fakeRunner) Capture(_ context.Context, s shiprun.Step, _ shiprun.Vars) (string, error) {
	f.probes = append(f.probes, s)
	if f.sdkErr != nil {
		return "", f.sdkErr
	}
	if f.sdk == "" {
		return "10.0.112\n", nil
	}
	return f.sdk, nil
}

func (f *fakeRunner) Run(_ context.Context, s shiprun.Step, _ shiprun.Vars) error {
	f.steps = append(f.steps, s)
	return f.fail[s.Name]
}

func (f *fakeRunner) names() []string {
	var n []string
	for _, s := range f.steps {
		n = append(n, s.Name)
	}
	return n
}

func TestExecutor_RunsStagesInOrderAndStopsAtTheFirstFailure(t *testing.T) {
	p := composeProject()
	p.Hooks = map[string][]shipconfig.Command{"after_test": {bash("echo after")}}
	plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{}
	if err := (Executor{Steps: r}).Run(context.Background(), plan, shiprun.Vars{Project: "api"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"compose:build", "compose:test", "after_test", "compose:save ghcr.io/acme/api:1.0.0", "compose:save ghcr.io/acme/worker:-v1.0.0"}; !slices.Equal(r.names(), want) {
		t.Errorf("ran %v, want %v", r.names(), want)
	}
	if r.steps[0].Stage != "build" || r.steps[2].Stage != "after_test" || r.steps[0].Exec[0] != "docker" {
		t.Errorf("shiprun steps lack their stage or argv: %+v", r.steps[:1])
	}

	// The test service's exit code is the test result.
	stepErr := errors.New("exit code 1")
	r = &fakeRunner{fail: map[string]error{"compose:test": stepErr}}
	err = (Executor{Steps: r}).Run(context.Background(), plan, shiprun.Vars{Project: "api"})
	var f *StepFailure
	if !errors.As(err, &f) || f.Stage != "test" || f.Step != "compose:test" || !errors.Is(err, stepErr) {
		t.Fatalf("Run = %v, want a StepFailure at test/compose:test", err)
	}
	if want := []string{"compose:build", "compose:test"}; !slices.Equal(r.names(), want) {
		t.Errorf("ran %v after the failure, want %v", r.names(), want)
	}
}

// writingRunner is a fakeRunner that writes files when a step runs:
// step name -> (root-relative path -> content).
type writingRunner struct {
	fakeRunner
	root   string
	writes map[string]map[string]string
	t      *testing.T
}

func (w *writingRunner) Run(ctx context.Context, s shiprun.Step, v shiprun.Vars) error {
	for rel, content := range w.writes[s.Name] {
		writeFile(w.t, w.root, rel, content)
	}
	return w.fakeRunner.Run(ctx, s, v)
}

func TestExecutor_CoverageCheckRunsInProcess(t *testing.T) {
	dir := t.TempDir()
	p := scriptProject()
	p.Build.Coverage = &shipconfig.Coverage{Line: 90}
	plan, err := Build(t.TempDir(), p, Context{Version: "1.0.0", ArtifactsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	cov := filepath.Join(dir, "api", "coverage")
	if s := stepOf(t, plan, "test"); s.Command.Env[CoverageEnv] != cov {
		t.Errorf("test step %s = %q, want %s", CoverageEnv, s.Command.Env[CoverageEnv], cov)
	}
	if got, want := line(stepOf(t, plan, "coverage:check")), "coverage|coverage:check|check line coverage >= 90% in "+cov; got != want {
		t.Errorf("check step %q, want %q", got, want)
	}
	// The test step writes the report, as dotnet test does.
	report := func(covered int) *writingRunner {
		return &writingRunner{root: dir, t: t, writes: map[string]map[string]string{
			"test": {"api/coverage/0a/coverage.cobertura.xml": cobertura(covered, 100)}}}
	}
	var out strings.Builder
	r := report(85)
	err = (Executor{Steps: r, Output: &out}).Run(context.Background(), plan, shiprun.Vars{Project: "api"})
	var f *StepFailure
	if !errors.As(err, &f) || f.Stage != "coverage" || f.Step != "coverage:check" || !strings.Contains(err.Error(), "85.00%") {
		t.Fatalf("Run = %v, want the coverage check failure", err)
	}
	if slices.Contains(r.names(), "coverage:check") {
		t.Error("the check was sent to the process runner")
	}
	out.Reset()
	if err := (Executor{Steps: report(95), Output: &out}).Run(context.Background(), plan, shiprun.Vars{Project: "api"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[api:coverage:check] line coverage 95.00% (95 of 100 lines, 1 report), threshold 90%") {
		t.Errorf("output = %q", out.String())
	}
}

// A configured coverage dir is searched in the release clone (a compose
// test service writes its reports through a bind mount) and not reset; a
// project without a coverage command still gets the check.
func TestBuild_CoverageDirInTheClone(t *testing.T) {
	root := t.TempDir()
	p := composeProject()
	p.Build.Coverage = &shipconfig.Coverage{Dir: "api/coverage", Line: 80, Method: 85}
	plan, err := Build(root, p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	cov := filepath.Join(root, "api", "coverage")
	if got, want := line(stepOf(t, plan, "coverage:check")), "coverage|coverage:check|check line coverage >= 80%, method coverage >= 85% in "+cov; got != want {
		t.Errorf("check step %q, want %q", got, want)
	}
	if slices.Contains(plan.ResetDirs, cov) {
		t.Errorf("the clone's coverage dir is reset: %v", plan.ResetDirs)
	}
}

func TestPlan_Render(t *testing.T) {
	p := composeProject()
	p.Build.Images = p.Build.Images[:1]
	p.Build.Coverage = &shipconfig.Coverage{Line: 80}
	p.Hooks = map[string][]shipconfig.Command{
		"after_test": {{Run: "scripts/report.ps1", Shell: "pwsh", Workdir: "tools"}},
		"prepare":    {bash("echo a\necho b")},
	}
	plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(artifacts, "api")
	want := `api 1.0.0 (default channel, tag v1.0.0, build mode compose)
  build
    compose:build  docker compose -f api/docker-compose.yml build
  test
    compose:test  docker compose -f api/docker-compose.yml run --rm tests
    after_test  [pwsh in tools] scripts/report.ps1
  coverage
    coverage:check  check line coverage >= 80% in ` + filepath.Join(dir, "coverage") + `
  package
    compose:save ghcr.io/acme/api:1.0.0  docker save -o ` + filepath.Join(dir, "outputs", "ghcr.io_acme_api_1.0.0.tar") + ` ghcr.io/acme/api:1.0.0
  hooks (prepare: after the pipeline, before the artifact is written; fail: when a stage or hook fails)
    prepare  [bash] echo a
        echo b
`
	if got := plan.Render(); got != want {
		t.Errorf("Render =\n%s\nwant\n%s", got, want)
	}
}

func TestExecutor_LeftoversOfAnEarlierRunAreNotUsed(t *testing.T) {
	p := scriptProject()
	p.Build.Coverage = &shipconfig.Coverage{Line: 90}
	c := ctxFor("3.1.0", "")
	c.ArtifactsDir = t.TempDir()
	plan, err := Build(t.TempDir(), p, c)
	if err != nil {
		t.Fatal(err)
	}
	// An earlier run left a report with no coverage and an older package.
	writeFile(t, c.ArtifactsDir, "api/coverage/old/coverage.cobertura.xml", cobertura(0, 1000))
	writeFile(t, c.ArtifactsDir, "api/outputs/Api.3.0.0.nupkg", "old")
	writeFile(t, c.ArtifactsDir, "other-project/keep.txt", "not ours")
	r := &writingRunner{root: c.ArtifactsDir, t: t, writes: map[string]map[string]string{
		"test":    {"api/coverage/new/coverage.cobertura.xml": cobertura(95, 100)},
		"package": {"api/outputs/Api.3.1.0.nupkg": "new"},
	}}
	if err := (Executor{Steps: r}).Run(context.Background(), plan, shiprun.Vars{Project: "api"}); err != nil {
		t.Fatalf("Run = %v (the old report must not count)", err)
	}
	outputs, err := plan.Outputs()
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) != 1 || outputs[0].Path != "Api.3.1.0.nupkg" {
		t.Errorf("outputs %+v, want only the new package", outputs)
	}
	if _, err := os.Stat(filepath.Join(c.ArtifactsDir, "other-project", "keep.txt")); err != nil {
		t.Errorf("another project's artifacts were removed: %v", err)
	}
}

func TestExecutor_ResetsOnlyInsideTheArtifactsRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "precious.txt", "x")
	for _, dir := range []string{outside, root, filepath.Join(root, "..", filepath.Base(outside))} {
		plan := &Plan{Project: "x", Context: Context{Version: "1.0.0", ArtifactsDir: root}, ResetDirs: []string{dir}}
		err := (Executor{Steps: &fakeRunner{}}).Run(context.Background(), plan, shiprun.Vars{})
		if err == nil || !strings.Contains(err.Error(), "not inside the artifacts directory") {
			t.Errorf("ResetDirs %q: Run = %v, want a refusal", dir, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "precious.txt")); err != nil {
		t.Errorf("a directory outside the artifacts root was touched: %v", err)
	}
}

func TestPlan_OutputsListsWhatThePackageStageLeft(t *testing.T) {
	c := ctxFor("1.0.0", "")
	c.ArtifactsDir = t.TempDir()
	script, err := Build(t.TempDir(), scriptProject(), c)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := script.Outputs(); err != nil || len(got) != 0 {
		t.Errorf("script mode before any run: %+v %v", got, err)
	}
	p := composeProject()
	p.Build.Images = p.Build.Images[:1]
	plan, err := Build(t.TempDir(), p, c)
	if err != nil {
		t.Fatal(err)
	}
	out := plan.OutputsDir
	writeFile(t, out, "ghcr.io_acme_api_1.0.0.tar", "image")
	writeFile(t, out, "site/index.html", "<html>")
	writeFile(t, out, "notes.txt", "x")
	got, err := plan.Outputs()
	if err != nil {
		t.Fatal(err)
	}
	want := []artifact.OutputSource{
		{Path: "ghcr.io_acme_api_1.0.0.tar", Kind: artifact.KindImageTar, From: filepath.Join(out, "ghcr.io_acme_api_1.0.0.tar")},
		{Path: "notes.txt", Kind: artifact.KindFile, From: filepath.Join(out, "notes.txt")},
		{Path: "site", Kind: artifact.KindDir, From: filepath.Join(out, "site")},
	}
	if !slices.Equal(got, want) {
		t.Errorf("outputs\n %+v\nwant\n %+v", got, want)
	}
	// A saved image missing from the outputs is an error.
	if err := os.Remove(filepath.Join(out, "ghcr.io_acme_api_1.0.0.tar")); err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Outputs(); err == nil || !strings.Contains(err.Error(), "ghcr.io/acme/api:1.0.0") {
		t.Errorf("missing image tar: %v", err)
	}
}
