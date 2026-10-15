package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"context"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/semrel"
)

const pipelineReleaseYAML = `pipeline:
  dotnet:
    solution: App.slnx
  images:
    - name: ghcr.io/acme/app
  release:
    version_files: [src/Api/Api.csproj]
`

// newPipelineReleaseFixture is a ship fixture for a repository with a
// pipeline: section and no release: section (one project named "repo"),
// whose clone holds the version file.
func newPipelineReleaseFixture(t *testing.T, yaml string) *shipFixture {
	t.Helper()
	f := newShipFixture(t, yaml)
	f.ws.results = []semrel.ProjectResult{shipResult("repo", "1.1.0-alpha.1", "v1.1.0-alpha.1")}
	f.ws.results[0].NewReleaseChannel = "alpha"
	f.ws.results[0].SemanticReleaseConfig = semrel.ConfigBuiltIn
	csproj := filepath.Join(f.ws.dir, "src", "Api", "Api.csproj")
	if err := os.MkdirAll(filepath.Dir(csproj), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(csproj, []byte(apiCsproj), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *shipFixture) runPipeline(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(append([]string{"pipeline"}, args...), &out, &errOut, f.options(t))
	return code, out.String(), errOut.String()
}

func readOutputs(t *testing.T, top string) (pipeline.ReleaseOutputs, bool) {
	t.Helper()
	data, err := os.ReadFile(pipeline.ReleaseOutputsPath(top))
	if errors.Is(err, os.ErrNotExist) {
		return pipeline.ReleaseOutputs{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	var o pipeline.ReleaseOutputs
	if err := json.Unmarshal(data, &o); err != nil {
		t.Fatal(err)
	}
	return o, true
}

func TestPipelineRelease_ReleasesWithoutBuildingOrPublishing(t *testing.T) {
	f := newPipelineReleaseFixture(t, pipelineReleaseYAML)
	code, stdout, stderr := f.runPipeline(t, "release", "--yes", "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if want := []string{"open", "compute", "check-push", "release repo", "close"}; !slices.Equal(f.events, want) {
		t.Errorf("events %v, want %v (no build, publish or hooks)", f.events, want)
	}
	if req := f.ws.requests[0]; !slices.Equal(req.ExtraAssets, []string{"src/Api/Api.csproj"}) || req.Expected.NewReleaseVersion != "1.1.0-alpha.1" {
		t.Errorf("release request %+v", req)
	}
	data, _ := os.ReadFile(filepath.Join(f.ws.dir, "src", "Api", "Api.csproj"))
	if !strings.Contains(string(data), "<Version>1.1.0-alpha.1</Version>") || !strings.Contains(string(data), "https://github.com/acme/widgets/releases/tag/v1.1.0-alpha.1") {
		t.Errorf("version file in the clone:\n%s", data)
	}
	o, ok := readOutputs(t, f.top)
	if !ok || o.Status != "passed" || !o.Published || o.Version != "1.1.0-alpha.1" || o.Channel != "alpha" || o.GitTag != "v1.1.0-alpha.1" ||
		o.GitHead != "release-commit-repo" || o.DryRun || o.SemanticReleaseConfig != "default" || o.Outputs["new_release_version"] != "1.1.0-alpha.1" {
		t.Errorf("release.json %+v", o)
	}
	var doc pipeline.ReleaseOutputs
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil || doc.Version != o.Version || doc.GitHead != o.GitHead {
		t.Errorf("stdout %q (%v), want the release.json document", stdout, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
		if !strings.HasPrefix(line, "[pipeline:release] ") {
			t.Errorf("stderr line not prefixed: %q", line)
		}
	}
	if strings.Contains(stderr, "nerv ship") || !strings.Contains(stderr, "nerv pipeline release") {
		t.Errorf("messages name the wrong command:\n%s", stderr)
	}
}

func TestPipelineRelease_DryRunComputesOnly(t *testing.T) {
	f := newPipelineReleaseFixture(t, pipelineReleaseYAML)
	code, stdout, stderr := f.runPipeline(t, "release", "--dry-run")
	if code != 0 || slices.Contains(f.events, "check-push") || len(f.ws.requests) != 0 {
		t.Fatalf("exit %d, events %v\n%s", code, f.events, stderr)
	}
	data, _ := os.ReadFile(filepath.Join(f.ws.dir, "src", "Api", "Api.csproj"))
	if string(data) != apiCsproj {
		t.Errorf("version file written in a dry run")
	}
	o, ok := readOutputs(t, f.top)
	if !ok || !o.DryRun || !o.Published || o.Version != "1.1.0-alpha.1" || o.GitHead != localHead {
		t.Errorf("release.json %+v", o)
	}
	if !strings.Contains(stdout, "release: passed") || !strings.Contains(stdout, "dry run") {
		t.Errorf("stdout %q", stdout)
	}
}

func TestPipelineRelease_NoNewRelease(t *testing.T) {
	f := newPipelineReleaseFixture(t, pipelineReleaseYAML)
	f.ws.results = []semrel.ProjectResult{{Project: "repo", Branch: "main", LastReleaseVersion: "1.0.0", LastReleaseGitTag: "v1.0.0"}}
	code, _, _ := f.runPipeline(t, "release", "--yes")
	o, ok := readOutputs(t, f.top)
	if code != 0 || !ok || o.Published || o.Version != "" || o.LastVersion != "1.0.0" || len(f.ws.requests) != 0 {
		t.Errorf("exit %d, release.json %+v", code, o)
	}
}

func TestPipelineRelease_Refusals(t *testing.T) {
	monorepo := pipelineReleaseYAML + "release:\n  projects:\n    - { name: api, path: api }\n    - { name: web, path: web }\n"
	tests := []struct {
		name, yaml string
		args       []string
		setup      func(f *shipFixture)
		want       string
	}{
		{"no pipeline section", "enabled: true\n", nil, nil, "docs/pipeline.md"},
		{"several projects", monorepo, nil, nil, "one project"},
		{"version file not on origin", pipelineReleaseYAML, nil, func(f *shipFixture) {
			os.RemoveAll(filepath.Join(f.ws.dir, "src"))
		}, `"src/Api/Api.csproj"`},
		{"expect mismatch", pipelineReleaseYAML, []string{"--expect", "repo=9.9.9"}, nil, "expected 9.9.9"},
		{"invalid pipeline section", "pipeline:\n  images:\n    - name: ghcr.io/acme/app\n", nil, nil, "pipeline.dotnet.solution"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPipelineReleaseFixture(t, tt.yaml)
			if tt.setup != nil {
				tt.setup(f)
			}
			code, stdout, _ := f.runPipeline(t, append([]string{"release", "--yes", "--json"}, tt.args...)...)
			var doc pipeline.ReleaseOutputs
			_ = json.Unmarshal([]byte(stdout), &doc)
			if code != 1 || doc.Status != "refused" || !strings.Contains(doc.Error, tt.want) || len(f.ws.requests) != 0 {
				t.Errorf("exit %d, %+v", code, doc)
			}
			if _, ok := readOutputs(t, f.top); ok {
				t.Error("release.json written for a refused run")
			}
		})
	}
}

func TestPipelineRelease_NeedsYesWithoutATerminal(t *testing.T) {
	f := newPipelineReleaseFixture(t, pipelineReleaseYAML)
	code, _, stderr := f.runPipeline(t, "release")
	if code != 1 || !strings.Contains(stderr, "--yes") || len(f.ws.requests) != 0 {
		t.Errorf("exit %d\n%s", code, stderr)
	}
}

func TestPipelineRelease_FailureLeavesNoOutputs(t *testing.T) {
	f := newPipelineReleaseFixture(t, pipelineReleaseYAML)
	if _, err := pipeline.WriteReleaseOutputs(f.top, pipeline.ReleaseOutputs{Step: "release", Status: "passed", Published: true, Version: "0.9.0"}); err != nil {
		t.Fatal(err)
	}
	f.ws.release = func(context.Context, semrel.ReleaseRequest) (semrel.ProjectResult, error) {
		return semrel.ProjectResult{}, errors.New("semantic-release: EGITNOPERMISSION")
	}
	code, stdout, _ := f.runPipeline(t, "release", "--yes", "--json")
	var doc pipeline.ReleaseOutputs
	_ = json.Unmarshal([]byte(stdout), &doc)
	if code != 2 || doc.Status != "failed" || doc.Action == "" || strings.Contains(doc.Action, "nerv ship") {
		t.Errorf("exit %d, %+v", code, doc)
	}
	if _, ok := readOutputs(t, f.top); ok {
		t.Error("a stale release.json survived a failed release")
	}
}

func TestPipelineRelease_Usage(t *testing.T) {
	f := newPipelineReleaseFixture(t, pipelineReleaseYAML)
	for _, args := range [][]string{{"release", "extra"}, {"release", "--expect", "bad"}, {"release", "--fetch-timeout", "0s"}} {
		if code, _, stderr := f.runPipeline(t, args...); code != 2 || !strings.Contains(stderr, "Usage: nerv pipeline") {
			t.Errorf("%v: exit %d %q", args, code, stderr)
		}
	}
}
