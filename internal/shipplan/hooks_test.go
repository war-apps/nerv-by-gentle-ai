package shipplan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

func TestExecutor_RunHooksRunsOneLifecycleKeyInOrder(t *testing.T) {
	p := project("api", ".")
	p.Hooks = map[string][]shipconfig.Command{
		"prepare": {{Run: "echo one", Shell: "bash"}, {Run: "echo two", Shell: "bash"}},
		"success": {{Run: "echo done", Shell: "bash"}},
	}
	plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{}
	if err := (Executor{Steps: r}).RunHooks(context.Background(), plan, "prepare", shiprun.Vars{Project: "api"}); err != nil {
		t.Fatal(err)
	}
	if len(r.steps) != 2 || r.steps[0].Command.Run != "echo one" || r.steps[1].Command.Run != "echo two" || r.steps[0].Stage != "prepare" {
		t.Errorf("ran %+v", r.steps)
	}
	if r.steps[0].Command.Env[ArtifactsEnv] == "" {
		t.Errorf("hook lacks %s", ArtifactsEnv)
	}

	r = &fakeRunner{}
	if err := (Executor{Steps: r}).RunHooks(context.Background(), plan, "fail", shiprun.Vars{Project: "api"}); err != nil || len(r.steps) != 0 {
		t.Errorf("no fail hooks: err %v, ran %v", err, r.names())
	}

	stepErr := errors.New("exit code 3")
	r = &fakeRunner{}
	failSecond := &countingRunner{inner: r, failAt: 2, err: stepErr}
	err = (Executor{Steps: failSecond}).RunHooks(context.Background(), plan, "prepare", shiprun.Vars{Project: "api"})
	var f *StepFailure
	if !errors.As(err, &f) || f.Stage != "prepare" || f.Step != "prepare" || f.Index != 1 || !errors.Is(err, stepErr) {
		t.Fatalf("RunHooks = %v (%+v), want a StepFailure at prepare index 1", err, f)
	}
}

// countingRunner fails the failAt-th step (1-based).
type countingRunner struct {
	inner  *fakeRunner
	failAt int
	err    error
}

func (c *countingRunner) Capture(ctx context.Context, s shiprun.Step, v shiprun.Vars) (string, error) {
	return c.inner.Capture(ctx, s, v)
}

func (c *countingRunner) Run(ctx context.Context, s shiprun.Step, v shiprun.Vars) error {
	c.inner.Run(ctx, s, v)
	if len(c.inner.steps) == c.failAt {
		return c.err
	}
	return nil
}

func TestStepFailure_IndexIsThePositionInItsStage(t *testing.T) {
	p := dimProject()
	p.Docker.Images = p.Docker.Images[:1]
	p.Hooks = map[string][]shipconfig.Command{"before_publish": {{Run: "echo login", Shell: "bash"}}}
	plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	pushLatest := "docker:push ghcr.io/org/dim.comprobantes.api:latest"
	r := &fakeRunner{fail: map[string]error{pushLatest: errors.New("denied")}}
	err = (Executor{Steps: r}).RunPublish(context.Background(), plan, shiprun.Vars{Project: "api"})
	var f *StepFailure
	if !errors.As(err, &f) || f.Stage != PublishStage || f.Step != pushLatest || f.Index != 2 {
		t.Fatalf("RunPublish = %v (%+v), want index 2 (before_publish, the version push, then latest)", err, f)
	}
	if done := plan.Publish().Steps[:f.Index]; done[0].Name != "before_publish" || !strings.HasSuffix(done[1].Name, ":1.0.0") {
		t.Errorf("steps before the failure: %v", done)
	}
}

func TestWriteCsprojVersion_KeepsTheReleaseNotes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Api.csproj")
	orig := "<Project Sdk=\"Microsoft.NET.Sdk\">\r\n  <PropertyGroup>\r\n    <Version>1.0.0</Version>\r\n    <PackageReleaseNotes>old notes</PackageReleaseNotes>\r\n  </PropertyGroup>\r\n</Project>\r\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteCsprojVersion(path, "1.1.0-alpha.1"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	want := strings.Replace(orig, "<Version>1.0.0</Version>", "<Version>1.1.0-alpha.1</Version>", 1)
	if string(data) != want {
		t.Errorf("csproj =\n%q\nwant\n%q", data, want)
	}
	if err := WriteCsprojVersion(path, "1.0.0;rm"); err == nil {
		t.Error("an invalid version was written")
	}
}
