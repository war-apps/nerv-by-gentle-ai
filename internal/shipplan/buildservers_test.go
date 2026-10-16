package shipplan

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
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

func TestSupportsDisableBuildServers(t *testing.T) {
	for _, tt := range []struct {
		out     string
		want    bool
		wantErr bool
	}{
		{"10.0.112\n", true, false},
		{"8.0.404\n", true, false},
		{"7.0.100", true, false},
		{"7.0.101", true, false},
		{"6.0.428\n", false, false},
		{"7.0.100-preview.1.22110.4", false, false},
		{"7.0.200-preview.1", true, false},
		{"8.0.100+abc", true, false},
		{"Welcome to .NET!\n\n8.0.404\n", true, false},
		{"garbage", false, true},
		{"", false, true},
		{"8.0", false, true},
		{"v8.0.404", false, true},
	} {
		got, err := SupportsDisableBuildServers(tt.out)
		if got != tt.want || (err != nil) != tt.wantErr {
			t.Errorf("SupportsDisableBuildServers(%q) = %v, %v; want %v, error %v", tt.out, got, err, tt.want, tt.wantErr)
		}
	}
}

// captureless runs steps but cannot capture their output.
type captureless struct{ fakeRunner }

func (c *captureless) Capture() {}

func TestExecutor_BuildServersFlagFollowsTheSDK(t *testing.T) {
	for _, tt := range []struct {
		name     string
		sdk      string
		sdkErr   error
		noCap    bool
		wantFlag bool
		wantNote bool
	}{
		{"SDK 10", "10.0.112\n", nil, false, true, false},
		{"SDK 7.0.100", "7.0.100\n", nil, false, true, false},
		{"SDK 6", "6.0.428\n", nil, false, false, false},
		{"SDK 7 preview", "7.0.100-preview.1\n", nil, false, false, false},
		{"detection fails", "", errors.New("dotnet: not found"), false, false, true},
		{"unparsable version", "garbage\n", nil, false, false, true},
		{"runner cannot capture", "", nil, true, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := dimProject()
			p.Presets = append(p.Presets, "nuget")
			plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
			if err != nil {
				t.Fatal(err)
			}
			plan.Stages = plan.Pipeline()
			plan.Stages = slices.DeleteFunc(plan.Stages, func(s Stage) bool { return s.Name == "coverage" })
			f := &fakeRunner{sdk: tt.sdk, sdkErr: tt.sdkErr}
			var steps StepRunner = f
			if tt.noCap {
				c := &captureless{}
				steps, f = c, &c.fakeRunner
			}
			var out bytes.Buffer
			if err := (Executor{Steps: steps, Output: &out}).Run(context.Background(), plan, shiprun.Vars{Project: "api"}); err != nil {
				t.Fatal(err)
			}
			var dotnet int
			for _, s := range f.steps {
				if len(s.Exec) < 2 || s.Exec[0] != "dotnet" {
					continue
				}
				dotnet++
				wantFlag := tt.wantFlag && s.Exec[1] != "format"
				if got := slices.Contains(s.Exec, DisableBuildServers); got != wantFlag {
					t.Errorf("%s %q: --disable-build-servers %v, want %v", s.Name, s.Exec, got, wantFlag)
				}
				if s.Command.Env["MSBUILDDISABLENODEREUSE"] != "1" {
					t.Errorf("%s: the env must apply whatever the SDK, got %v", s.Name, s.Command.Env)
				}
			}
			if dotnet != 5 {
				t.Errorf("ran %d dotnet commands (%q), want restore, format, build, test and pack", dotnet, f.names())
			}
			if !tt.noCap {
				if len(f.probes) != 1 || !slices.Equal(f.probes[0].Exec, []string{"dotnet", "--version"}) || f.probes[0].Command.Workdir != "" {
					t.Errorf("probes %+v, want one dotnet --version in the repository root", f.probes)
				}
				if f.probes[0].Command.Env["MSBUILDDISABLENODEREUSE"] != "1" {
					t.Errorf("the probe must get the commands' env, got %v", f.probes[0].Command.Env)
				}
			}
			notes := strings.Count(out.String(), DisableBuildServers)
			if tt.wantNote && (notes != 1 || !strings.HasPrefix(out.String(), "[api:dotnet:restore] ")) {
				t.Errorf("output %q: want one prefixed note about the flag", out.String())
			}
			if !tt.wantNote && notes != 0 {
				t.Errorf("output %q: no note expected", out.String())
			}
		})
	}
}
