package shipplan

import (
	"slices"
	"strings"
	"testing"
)

func TestDotnetPreset_NoBuildServers(t *testing.T) {
	p := dimProject()
	p.Presets = append(p.Presets, "nuget")
	plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, st := range plan.Stages {
		for _, s := range st.Steps {
			if !strings.HasPrefix(s.Name, "dotnet:") || s.Func != nil {
				continue
			}
			seen[s.Exec[1]] = true
			for k, v := range DotnetBuildServerEnv() {
				if s.Command.Env[k] != v {
					t.Errorf("%s: env %s=%q, want %q", s.Name, k, s.Command.Env[k], v)
				}
			}
			wantFlag := s.Exec[1] != "format"
			if got := slices.Contains(s.Exec, DisableBuildServers); got != wantFlag {
				t.Errorf("%s %q: --disable-build-servers %v, want %v (dotnet format rejects it)", s.Name, s.Exec, got, wantFlag)
			}
		}
	}
	for _, cmd := range []string{"restore", "format", "build", "test", "pack"} {
		if !seen[cmd] {
			t.Errorf("no dotnet %s step in the plan", cmd)
		}
	}
}

func TestDotnetBuildServerEnv(t *testing.T) {
	env := DotnetBuildServerEnv()
	want := map[string]string{"MSBUILDDISABLENODEREUSE": "1", "UseSharedCompilation": "false", "DOTNET_CLI_USE_MSBUILD_SERVER": "0"}
	if len(env) != len(want) {
		t.Errorf("env %v", env)
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s=%q, want %q", k, env[k], v)
		}
	}
	env["X"] = "y"
	if _, ok := DotnetBuildServerEnv()["X"]; ok {
		t.Error("DotnetBuildServerEnv must return a fresh map")
	}
}
