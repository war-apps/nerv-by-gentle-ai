package shipconfig_test

import (
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// dotnetProject is a release: section with one dotnet project; extra is
// added under dotnet: and project is added at the project level.
func dotnetProject(dotnet, project string) string {
	return "release:\n  projects:\n    - name: api\n      path: .\n      presets: [dotnet]\n" + project +
		"      dotnet:\n        project_file: Api.csproj\n" + dotnet
}

// The scalar form keeps its meaning: the line floor only.
func TestLoad_CoverageThresholdScalarIsTheLineFloor(t *testing.T) {
	rel := mustLoad(t, dotnetProject("        coverage_threshold: 80\n", ""))
	if got, want := rel.Projects[0].Dotnet.CoverageThreshold, (shipconfig.CoverageThreshold{Line: 80}); got != want {
		t.Errorf("coverage_threshold = %+v, want %+v", got, want)
	}
	if rel.Projects[0].Dotnet.CoverageThreshold.Branch != 0 || rel.Projects[0].Dotnet.CoverageThreshold.Method != 0 {
		t.Error("the scalar form must not set branch or method floors")
	}
}

func TestLoad_CoverageThresholdMapForm(t *testing.T) {
	rel := mustLoad(t, dotnetProject("        coverage_threshold: { line: 80, branch: 70.5, method: 85 }\n", ""))
	if got, want := rel.Projects[0].Dotnet.CoverageThreshold, (shipconfig.CoverageThreshold{Line: 80, Branch: 70.5, Method: 85}); got != want {
		t.Errorf("coverage_threshold = %+v, want %+v", got, want)
	}
	rel = mustLoad(t, dotnetProject("        coverage_threshold:\n          branch: 60\n", ""))
	if got, want := rel.Projects[0].Dotnet.CoverageThreshold, (shipconfig.CoverageThreshold{Branch: 60}); got != want {
		t.Errorf("coverage_threshold = %+v, want %+v", got, want)
	}
}

func TestLoad_SharedHostSdkAndSubmodules(t *testing.T) {
	rel := mustLoad(t, dotnetProject("        shared_host: true\n        sdk: 8.0.100\n", "      submodules: true\n"))
	p := rel.Projects[0]
	if !p.Dotnet.SharedHost || p.Dotnet.SDK != "8.0.100" || !p.Submodules {
		t.Errorf("project = submodules %v, dotnet %+v; want submodules, shared_host and sdk 8.0.100", p.Submodules, *p.Dotnet)
	}
	// Unset, they keep today's plan: nothing is added.
	rel = mustLoad(t, dotnetProject("", ""))
	p = rel.Projects[0]
	if p.Dotnet.SharedHost || p.Dotnet.SDK != "" || p.Submodules {
		t.Errorf("defaults = submodules %v, dotnet %+v; want all off", p.Submodules, *p.Dotnet)
	}
}

func TestLoad_SharedHostCoverageAndSdkErrors(t *testing.T) {
	tests := []struct {
		name, dotnet, want string
	}{
		{"map floor out of range", "        coverage_threshold: { line: 80, branch: 120 }\n", "release.projects[0].dotnet.coverage_threshold.branch: 120 must be between 0 and 100"},
		{"map floor negative", "        coverage_threshold: { method: -1 }\n", "release.projects[0].dotnet.coverage_threshold.method: -1 must be between 0 and 100"},
		{"unknown floor", "        coverage_threshold: { lines: 80 }\n", "release.projects[0].dotnet.coverage_threshold.lines: unknown field"},
		{"floor not a number", "        coverage_threshold: { line: high }\n", "release: line"},
		{"sdk not a version", "        sdk: \"8\"\n", `release.projects[0].dotnet.sdk: "8" is not a .NET SDK version (e.g. 8.0.100)`},
		{"sdk with a quote", "        sdk: '8.0.100\"'\n", `release.projects[0].dotnet.sdk: "8.0.100\"" is not a .NET SDK version (e.g. 8.0.100)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := shipconfig.Load(writeConfig(t, dotnetProject(tt.dotnet, "")))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}
