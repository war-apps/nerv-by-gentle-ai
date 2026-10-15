package pipeline

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

// smokeRepo generates a tiny classlib with an xunit project in a solution,
// and a runsettings declaring dim's 100/80/100 coverage threshold.
func smokeRepo(t *testing.T) (string, *pipelineconfig.Pipeline) {
	t.Helper()
	if testing.Short() {
		t.Skip("smoke test: runs dotnet")
	}
	if _, err := exec.LookPath("dotnet"); err != nil {
		t.Skip("dotnet not available")
	}
	root := t.TempDir()
	tool := func(args ...string) {
		t.Helper()
		cmd := exec.Command("dotnet", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "DOTNET_NOLOGO=true", "DOTNET_CLI_TELEMETRY_OPTOUT=true")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("dotnet %v: %v\n%s", args, err, out)
		}
	}
	tool("new", "classlib", "-n", "Calc", "-o", "Calc")
	tool("new", "xunit", "-n", "Calc.Tests", "-o", "Calc.Tests")
	tool("new", "sln", "-n", "Smoke")
	sln := "Smoke.slnx"
	if _, err := os.Stat(filepath.Join(root, sln)); err != nil {
		sln = "Smoke.sln"
	}
	tool("sln", sln, "add", "Calc/Calc.csproj", "Calc.Tests/Calc.Tests.csproj")
	tool("add", "Calc.Tests/Calc.Tests.csproj", "reference", "Calc/Calc.csproj")
	os.Remove(filepath.Join(root, "Calc", "Class1.cs"))
	os.Remove(filepath.Join(root, "Calc.Tests", "UnitTest1.cs"))
	write(t, root, "Calc/Ops.cs", "namespace Calc;\n\npublic static class Ops\n{\n    public static int Add(int a, int b) => a + b;\n}\n")
	write(t, root, "Calc.Tests/OpsTests.cs", "namespace Calc.Tests;\n\npublic class OpsTests\n{\n    [Fact]\n    public void Adds() => Assert.Equal(3, Calc.Ops.Add(1, 2));\n}\n")
	write(t, root, "coverlet.runsettings", `<?xml version="1.0" encoding="utf-8"?>
<RunSettings>
  <DataCollectionRunSettings>
    <DataCollectors>
      <DataCollector friendlyName="XPlat code coverage">
        <Configuration>
          <Format>cobertura</Format>
          <Threshold>100,80,100</Threshold>
          <ThresholdType>line,branch,method</ThresholdType>
          <ThresholdStat>total</ThresholdStat>
        </Configuration>
      </DataCollector>
    </DataCollectors>
  </DataCollectionRunSettings>
</RunSettings>
`)
	yes := true
	return root, &pipelineconfig.Pipeline{Dotnet: pipelineconfig.Dotnet{Solution: sln, Configuration: "Release", Runsettings: "coverlet.runsettings", WarningsAsErrors: &yes}}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func smokeSteps(root string, cfg *pipelineconfig.Pipeline, out *strings.Builder) Steps {
	return Steps{Repo: root, Config: cfg, Run: &shiprun.Runner{RepoRoot: root, Output: out}}
}

func TestSmoke_ValidateAndTest(t *testing.T) {
	root, cfg := smokeRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var out strings.Builder

	start := time.Now()
	v := smokeSteps(root, cfg, &out).Validate(ctx)
	t.Logf("validate: %s in %s", v.Status, time.Since(start).Round(time.Millisecond))
	if v.Status != StatusPassed || len(v.Commands) != 2 {
		t.Fatalf("validate %+v\n%s", v, out.String())
	}
	if !strings.Contains(out.String(), "[pipeline:validate] ") {
		t.Errorf("output not prefixed:\n%s", out.String())
	}

	out.Reset()
	start = time.Now()
	r := smokeSteps(root, cfg, &out).Test(ctx)
	t.Logf("test: %s in %s, coverage %v", r.Status, time.Since(start).Round(time.Millisecond), r.Artifacts.Coverage)
	if r.Status != StatusPassed || len(r.Commands) != 3 || len(r.Artifacts.Coverage) != 1 {
		t.Fatalf("test %+v\n%s", r, out.String())
	}
	if len(r.Notes) != 1 || r.Notes[0] != "coverage (total): line 100% >= 100%, branch 100% >= 80%, method 100% >= 100%" {
		t.Errorf("coverage note %v", r.Notes)
	}
	if _, err := os.Stat(r.Artifacts.Trx); err != nil {
		t.Errorf("trx: %v", err)
	}
	if status, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err == nil {
		if st, _ := exec.Command("git", "-C", root, "status", "--porcelain", "--", ".nerv").Output(); len(st) != 0 {
			t.Errorf("artifacts are not ignored: %q", st)
		}
	} else {
		t.Logf("git init: %v %s", err, status)
	}

	// An uncovered method breaches the runsettings' 100% thresholds:
	// dotnet test passes (coverlet's collector ignores <Threshold>), and
	// nerv's coverage check fails the step.
	write(t, root, "Calc/More.cs", "namespace Calc;\n\npublic static class More\n{\n    public static int Sub(int a, int b) => a - b;\n}\n")
	out.Reset()
	r = smokeSteps(root, cfg, &out).Test(ctx)
	want := "coverage below the thresholds of coverlet.runsettings (total): line 50% < 100%, branch 100% >= 80%, method 50% < 100%"
	if r.Status != StatusFailed || len(r.Commands) != 3 || r.Commands[2].Exit != 0 || r.Error != want {
		t.Errorf("uncovered method: %+v\nwant error %q\n%s", r, want, out.String())
	}
	if data, err := os.ReadFile(r.Artifacts.Coverage[0]); err != nil || !strings.Contains(string(data), `line-rate="0.5"`) {
		t.Errorf("coverage report: %v\n%.400s", err, data)
	}

	// A failing test fails the step at dotnet test.
	write(t, root, "Calc.Tests/FailTests.cs", "namespace Calc.Tests;\n\npublic class FailTests\n{\n    [Fact]\n    public void Fails() => Assert.Equal(1, 2);\n}\n")
	out.Reset()
	r = smokeSteps(root, cfg, &out).Test(ctx)
	if r.Status != StatusFailed || len(r.Commands) != 3 || r.Commands[2].Exit != 1 || !strings.Contains(r.Error, "dotnet test") {
		t.Errorf("failing test: %+v", r)
	}
	os.Remove(filepath.Join(root, "Calc.Tests", "FailTests.cs"))

	// Misformatted code fails validate at dotnet format.
	write(t, root, "Calc/Ugly.cs", "namespace Calc;\npublic static class Ugly{public static int One()=>1;}\n")
	out.Reset()
	v = smokeSteps(root, cfg, &out).Validate(ctx)
	if v.Status != StatusFailed || len(v.Commands) != 2 || v.Commands[1].Exit == 0 || !strings.Contains(v.Error, "dotnet format") {
		t.Errorf("misformatted: %+v", v)
	}
}
