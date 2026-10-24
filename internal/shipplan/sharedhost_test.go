package shipplan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// overridesLikeDim is the dotnet project of both dim release repos: every
// dotnet stage overridden to add flags, plus a coverage override.
func overridesLikeDim(sharedHost bool) shipconfig.Project {
	p := project("api", ".", "dotnet")
	p.Dotnet = &shipconfig.Dotnet{ProjectFile: "Api.csproj", Solution: "Api.slnx", Configuration: "Release", SharedHost: sharedHost}
	p.Stages = map[string]shipconfig.Command{
		"restore":  {Run: "dotnet restore Api.slnx", Shell: "bash"},
		"lint":     {Run: "dotnet format Api.slnx --verify-no-changes", Shell: "bash", Env: map[string]string{"UseSharedCompilation": "true"}},
		"build":    {Run: "dotnet build Api.slnx -warnaserror", Shell: "bash"},
		"test":     {Run: "dotnet test Api.slnx --no-build", Shell: "bash"},
		"coverage": {Run: "bash scripts/coverage-gate.sh", Shell: "bash"},
		"package":  {Run: "dotnet pack Api.csproj", Shell: "bash"},
	}
	return p
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

// Previous behaviour: without shared_host a stage override runs only with
// its own env (plus the artifacts directory).
func TestBuild_OverridesWithoutSharedHostKeepTheirOwnEnv(t *testing.T) {
	plan, err := Build(t.TempDir(), overridesLikeDim(false), ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"restore", "lint", "build", "test", "coverage", "package"} {
		env := stepOf(t, plan, name).Command.Env
		for k := range DotnetBuildServerEnv() {
			if _, ok := env[k]; ok && !(name == "lint" && k == "UseSharedCompilation") {
				t.Errorf("stages.%s got %s without shared_host: %v", name, k, env)
			}
		}
	}
}

func TestBuild_SharedHostGivesDotnetStageOverridesTheBuildServerEnv(t *testing.T) {
	plan, err := Build(t.TempDir(), overridesLikeDim(true), ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"restore", "lint", "build", "test", "package"} {
		env := stepOf(t, plan, name).Command.Env
		for k, v := range DotnetBuildServerEnv() {
			if name == "lint" && k == "UseSharedCompilation" {
				v = "true" // the override's own env wins
			}
			if env[k] != v {
				t.Errorf("stages.%s env %s = %q, want %q (env %v)", name, k, env[k], v, env)
			}
		}
		if env[ArtifactsEnv] == "" {
			t.Errorf("stages.%s lost %s", name, ArtifactsEnv)
		}
	}
	// The coverage override is not a dotnet command: it is left alone.
	if env := stepOf(t, plan, "coverage").Command.Env; env["MSBUILDDISABLENODEREUSE"] != "" {
		t.Errorf("stages.coverage got the build-server env: %v", env)
	}
	// The run lines are never rewritten.
	if got := stepOf(t, plan, "build").Command.Run; got != "dotnet build Api.slnx -warnaserror" {
		t.Errorf("stages.build run = %q", got)
	}
}

func TestBuild_SubmodulesAndSdkPinRunFirstInRestore(t *testing.T) {
	root := t.TempDir()
	p := overridesLikeDim(false)
	p.Submodules = true
	p.Dotnet.SDK = "8.0.100"
	p.Hooks = map[string][]shipconfig.Command{"before_restore": {{Run: "echo hook", Shell: "bash"}}}
	plan, err := Build(root, p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	assertLines(t, lines(plan)[:4], []string{
		"restore|git:submodules|git submodule update --init --recursive",
		"restore|dotnet:sdk|write global.json pinning the .NET SDK to 8.0.100 (rollForward latestFeature)",
		"before_restore|before_restore|bash: echo hook",
		"restore|restore|bash: dotnet restore Api.slnx",
	})
	sdk := stepOf(t, plan, "dotnet:sdk")
	summary, err := sdk.Func(context.Background(), os.LookupEnv)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "global.json"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"sdk\": {\n    \"version\": \"8.0.100\",\n    \"rollForward\": \"latestFeature\"\n  }\n}\n"; string(data) != want {
		t.Errorf("global.json = %q, want %q", data, want)
	}
	if !strings.Contains(summary, "8.0.100") {
		t.Errorf("summary = %q", summary)
	}
}

// Submodules also apply to a project with no restore command (docker only):
// the restore stage then holds only the setup step.
func TestBuild_SubmodulesWithoutARestoreCommand(t *testing.T) {
	p := project("web", ".", "docker")
	p.Submodules = true
	p.Docker = &shipconfig.Docker{Images: []shipconfig.DockerImage{{Name: "ghcr.io/org/web", Dockerfile: "Dockerfile", Context: ".", Tags: []string{"version"}, Push: push(false)}}}
	plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	if got := lines(plan)[0]; got != "restore|git:submodules|git submodule update --init --recursive" {
		t.Errorf("first step = %q", got)
	}
}
