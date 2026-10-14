package shipplan

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

// The smoke tests run real dotnet and docker through the plan. They are
// skipped under -short and when the tool is missing; the dotnet one needs
// nuget.org for the test project's packages. Nothing is ever pushed.

// only keeps the named stages of a plan.
func only(p *Plan, names ...string) *Plan {
	out := *p
	out.Stages = nil
	for _, st := range p.Stages {
		for _, n := range names {
			if st.Name == n {
				out.Stages = append(out.Stages, st)
			}
		}
	}
	return &out
}

func runTool(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "DOTNET_NOLOGO=1", "DOTNET_CLI_TELEMETRY_OPTOUT=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func TestSmoke_DotnetPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("smoke test: runs dotnet")
	}
	if _, err := exec.LookPath("dotnet"); err != nil {
		t.Skip("dotnet not on PATH")
	}
	start := time.Now()
	root := t.TempDir()
	runTool(t, root, "dotnet", "new", "classlib", "-n", "Calc", "-o", "Calc")
	runTool(t, root, "dotnet", "new", "xunit", "-n", "Calc.Tests", "-o", "Calc.Tests")
	runTool(t, root, "dotnet", "add", "Calc.Tests/Calc.Tests.csproj", "reference", "Calc/Calc.csproj")
	runTool(t, root, "dotnet", "new", "sln", "-n", "Smoke")
	sln := "Smoke.slnx"
	if _, err := os.Stat(filepath.Join(root, sln)); err != nil {
		sln = "Smoke.sln"
	}
	runTool(t, root, "dotnet", "sln", sln, "add", "Calc/Calc.csproj", "Calc.Tests/Calc.Tests.csproj")
	writeJSON(t, root, "Calc/Class1.cs", "namespace Calc;\n\npublic static class Adder\n{\n    public static int Add(int a, int b) => a + b;\n}\n")
	writeJSON(t, root, "Calc.Tests/UnitTest1.cs", "namespace Calc.Tests;\n\npublic class AdderTests\n{\n    [Fact]\n    public void Adds() => Assert.Equal(3, Calc.Adder.Add(1, 2));\n}\n")
	// The release-time csproj write, with notes MSBuild would otherwise
	// expand ($(...)) or unescape (%).
	notes := "* fix <b> & $(Foo) at 50%"
	if err := WriteCsproj(filepath.Join(root, "Calc", "Calc.csproj"), "1.2.3-smoke.1", notes); err != nil {
		t.Fatal(err)
	}
	t.Logf("setup: %s", time.Since(start).Round(time.Millisecond))

	p := shipconfig.Project{Name: "calc", Path: ".", Presets: []string{"dotnet", "nuget"},
		Dotnet: &shipconfig.Dotnet{ProjectFile: "Calc/Calc.csproj", Solution: sln, Configuration: "Release", CoverageThreshold: 90},
		NuGet:  &shipconfig.NuGet{Source: "https://nuget.invalid/v3/index.json", APIKeyEnv: "NERV_SMOKE_NO_KEY"}}
	artifacts := t.TempDir()
	plan, err := Build(root, p, Context{Version: "1.2.3-smoke.1", Tag: "v1.2.3-smoke.1", ArtifactsDir: artifacts, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	runner := &shiprun.Runner{RepoRoot: root, Output: &out}
	start = time.Now()
	err = Executor{Steps: runner, Output: &out}.Run(context.Background(), only(plan, "restore", "lint", "build", "test", "coverage", "package"), shiprun.Vars{Project: "calc", Version: "1.2.3-smoke.1", DryRun: true})
	t.Logf("restore, lint, build, test, coverage, pack: %s", time.Since(start).Round(time.Millisecond))
	if err != nil {
		t.Fatalf("Run = %v\noutput:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "[calc:dotnet:coverage] line coverage 100.00%") {
		t.Errorf("no coverage summary in the output:\n%s", out.String())
	}

	nupkg := filepath.Join(artifacts, "calc", "nuget", "Calc.1.2.3-smoke.1.nupkg")
	nuspec := readZipEntry(t, nupkg, "Calc.nuspec")
	for _, want := range []string{"<version>1.2.3-smoke.1</version>", "<releaseNotes>* fix &lt;b&gt; &amp; $(Foo) at 50%</releaseNotes>"} {
		if !strings.Contains(nuspec, want) {
			t.Errorf("Calc.nuspec lacks %s:\n%s", want, nuspec)
		}
	}
}

func readZipEntry(t *testing.T, path, name string) string {
	t.Helper()
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Name == name {
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			b, err := io.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
	}
	t.Fatalf("%s has no %s", path, name)
	return ""
}

func TestSmoke_DockerBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("smoke test: runs docker")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon not reachable")
	}
	root := t.TempDir()
	writeJSON(t, root, "app/hello.txt", "hello\n")
	writeJSON(t, root, "app/Dockerfile", "FROM scratch\nARG APP_VERSION\nLABEL app.version=$APP_VERSION\nCOPY hello.txt /hello.txt\n")
	name := fmt.Sprintf("nerv-shipplan-smoke/app-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_ = exec.Command("docker", "image", "rm", "-f", name+":1.0.0", name+":latest").Run()
	})
	p := shipconfig.Project{Name: "app", Path: "app", Presets: []string{"docker"},
		Docker: &shipconfig.Docker{Images: []shipconfig.DockerImage{{Name: name, Dockerfile: "app/Dockerfile", Context: "app",
			BuildArgs: map[string]string{"APP_VERSION": "${version}"}, Tags: []string{"version", "channel", "latest"}, Push: push(true)}}}}
	plan, err := Build(root, p, Context{Version: "1.0.0", GitHead: "0123abc", ArtifactsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	start := time.Now()
	// Run never runs the publish stage: nothing is pushed.
	err = Executor{Steps: &shiprun.Runner{RepoRoot: root, Output: &out}}.Run(context.Background(), plan, shiprun.Vars{Project: "app"})
	t.Logf("docker build: %s", time.Since(start).Round(time.Millisecond))
	if err != nil {
		t.Fatalf("Run = %v\noutput:\n%s", err, out.String())
	}
	labels, err := exec.Command("docker", "image", "inspect", "-f", "{{index .Config.Labels \"app.version\"}} {{index .Config.Labels \"org.opencontainers.image.version\"}} {{index .Config.Labels \"org.opencontainers.image.revision\"}}", name+":latest").Output()
	if err != nil {
		t.Fatalf("inspect %s:latest: %v", name, err)
	}
	if got := strings.TrimSpace(string(labels)); got != "1.0.0 1.0.0 0123abc" {
		t.Errorf("labels = %q, want the build arg and OCI labels", got)
	}
	if err := exec.Command("docker", "image", "inspect", name+":1.0.0").Run(); err != nil {
		t.Errorf("no %s:1.0.0 tag: %v", name, err)
	}
}
