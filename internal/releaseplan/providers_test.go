package releaseplan

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
)

// recordingRunner records every provider step it is asked to run, with
// its request, and answers them with answer (ok with no data when nil).
type recordingRunner struct {
	argv     [][]string
	requests []extension.Request
	answer   func(req extension.Request) extension.Response
}

func (r *recordingRunner) Run(_ context.Context, c proc.Command) (proc.Result, error) {
	r.argv = append(r.argv, append([]string{c.Name}, c.Args...))
	var req extension.Request
	if err := json.Unmarshal(c.Stdin, &req); err != nil {
		return proc.Result{}, err
	}
	r.requests = append(r.requests, req)
	resp := extension.Response{Status: extension.StatusOK}
	if r.answer != nil {
		resp = r.answer(req)
	}
	out, _ := json.Marshal(resp)
	return proc.Result{Stdout: out}, nil
}

func testProviders(r proc.Runner) Providers {
	return Providers{FS: nerv.ExtensionsFS(), Runner: r, Nerv: "/opt/nerv"}
}

func invalidProblems(t *testing.T, err error) string {
	t.Helper()
	var invalid *releaseconfig.ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want a *releaseconfig.ValidationError", err)
	}
	return strings.Join(invalid.Problems, "\n")
}

// S4, S6: the version engine is the version slot's provider, called with
// the common envelope: release.version names it (conventional-commits by
// default), its keys are release.providers.<provider>, the payload carries
// the repository, the branch and the projects.
func TestVersionEngine_CallsTheSelectedProviderWithTheEnvelope(t *testing.T) {
	r := &recordingRunner{answer: func(extension.Request) extension.Response {
		data, _ := json.Marshal(NextData{Results: nil})
		return extension.Response{Status: extension.StatusOK, Data: data}
	}}
	rel := &releaseconfig.Release{
		Providers: map[string]map[string]any{"conventional-commits": {"branches": []any{"main"}}},
		Projects:  []releaseconfig.Project{{Name: "api", Path: "api", Scopes: []string{"api"}}},
	}
	engine, err := NewVersionEngine(testProviders(r), rel)
	if err != nil {
		t.Fatal(err)
	}
	if engine.Name() != "conventional-commits" {
		t.Errorf("engine %q, want the slot's default conventional-commits", engine.Name())
	}
	if _, err := engine.Next(context.Background(), "/clone", "develop", []string{"api"}, "1.2.0"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"/opt/nerv", "provider", "release-conventional-commits", "next"}; len(r.argv) != 1 || !slices.Equal(r.argv[0], want) {
		t.Fatalf("argv = %q, want %q", r.argv, want)
	}
	req := r.requests[0]
	if req.Contract != extension.ContractVersion || req.Extension != "release" || req.Slot != "version" || req.Provider != "conventional-commits" || req.Operation != "next" {
		t.Errorf("request = %+v", req)
	}
	if got, ok := req.Config["branches"].([]any); !ok || len(got) != 1 || got[0] != "main" {
		t.Errorf("config = %v, want release.providers.conventional-commits", req.Config)
	}
	if _, set := req.Config["tag_format"]; !set {
		t.Errorf("config = %v, want the declared default tag_format", req.Config)
	}
	var payload VersionPayload
	if err := req.DecodePayload(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Dir != "/clone" || payload.Branch != "develop" || !slices.Equal(payload.Only, []string{"api"}) || payload.Version != "1.2.0" ||
		len(payload.Projects) != 1 || payload.Projects[0].Name != "api" || payload.Projects[0].Path != "api" {
		t.Errorf("payload = %+v", payload)
	}
}

// S4: an unknown provider, or one of the other slot, is refused naming the
// key that selected it and the slot's providers.
func TestProviders_UnknownProviderIsRefused(t *testing.T) {
	p := testProviders(&recordingRunner{})
	_, err := NewVersionEngine(p, &releaseconfig.Release{Version: "semver"})
	if got := invalidProblems(t, err); got != `release.version: unknown version provider "semver" (want conventional-commits)` {
		t.Errorf("problems = %s", got)
	}
	_, err = NewVersionEngine(p, &releaseconfig.Release{Version: "bash"})
	if got := invalidProblems(t, err); got != `release.version: "bash" is a build provider, not a version one (want conventional-commits)` {
		t.Errorf("problems = %s", got)
	}
	rel := &releaseconfig.Release{Build: "make", Projects: []releaseconfig.Project{{Name: "api", Path: "."}, {Name: "web", Path: "web", Build: "gradle"}}}
	_, err = NewBuildProvider(p, rel, 0)
	if got := invalidProblems(t, err); got != `release.build: unknown build provider "make" (want bash, docker-compose)` {
		t.Errorf("problems = %s", got)
	}
	_, err = NewBuildProvider(p, rel, 1)
	if got := invalidProblems(t, err); got != `release.projects[1].build: unknown build provider "gradle" (want bash, docker-compose)` {
		t.Errorf("problems = %s", got)
	}
	_, err = NewBuildProvider(p, &releaseconfig.Release{Projects: []releaseconfig.Project{{Name: "api", Path: "."}}}, 0)
	if got := invalidProblems(t, err); !strings.Contains(got, "release.projects[0].build: required by nerv release (the build provider: bash, docker-compose") {
		t.Errorf("problems = %s", got)
	}
}

// S7: a provider's keys are the ones its provider.yaml declares, of their
// type; a release.providers entry names a shipped provider.
func TestProviders_KeysAreCheckedAgainstTheProvider(t *testing.T) {
	p := testProviders(&recordingRunner{})
	rel := &releaseconfig.Release{
		Build: "bash",
		Providers: map[string]map[string]any{
			"bash":                 {"shared_host": "yes", "sdk": "8"},
			"conventional-commits": {"branches": "main"},
			"maven":                {},
		},
		Projects: []releaseconfig.Project{{Name: "api", Path: ".", Providers: map[string]map[string]any{"bash": {"shell": "pwsh"}}}},
	}
	_, err := NewVersionEngine(p, rel)
	got := invalidProblems(t, err)
	for _, want := range []string{
		"release.providers.maven: unknown release provider (want bash, conventional-commits, docker-compose)",
		"release.providers.conventional-commits.branches: must be a list",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("version engine problems lack %q:\n%s", want, got)
		}
	}
	_, err = NewBuildProvider(p, rel, 0)
	got = invalidProblems(t, err)
	for _, want := range []string{
		"release.providers.bash.sdk: unknown key (want shared_host)",
		"release.providers.bash.shared_host: must be a bool",
		"release.projects[0].providers.bash.shell: unknown key (want shared_host)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("build problems lack %q:\n%s", want, got)
		}
	}
}

// S4: a project's build overrides the section's, and a project's provider
// keys override the section's; the provider gets the merged keys and the
// project's stages.
func TestBuildProvider_ProjectOverridesTheSection(t *testing.T) {
	r := &recordingRunner{answer: func(extension.Request) extension.Response {
		data, _ := json.Marshal(PlanData{ImagePrefix: "x:"})
		return extension.Response{Status: extension.StatusOK, Data: data}
	}}
	rel := &releaseconfig.Release{
		Build:     "docker-compose",
		Providers: map[string]map[string]any{"docker-compose": {"file": "docker-compose.yml", "test_service": "tests"}},
		Projects: []releaseconfig.Project{
			{Name: "api", Path: "api", Providers: map[string]map[string]any{"docker-compose": {"file": "api/compose.yml"}},
				Stages: map[string]releaseconfig.Command{"lint": {Run: "x"}, "restore": {Run: "y"}}},
			{Name: "web", Path: "web", Build: "bash"},
		},
	}
	for i := range rel.Projects {
		b, err := NewBuildProvider(testProviders(r), rel, i)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := b.Plan(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.requests) != 2 {
		t.Fatalf("requests = %d", len(r.requests))
	}
	api, web := r.requests[0], r.requests[1]
	if api.Provider != "docker-compose" || api.Slot != "build" || api.Operation != "plan" || api.Config["file"] != "api/compose.yml" || api.Config["test_service"] != "tests" {
		t.Errorf("api request = %+v", api)
	}
	var payload BuildPayload
	if err := api.DecodePayload(&payload); err != nil || payload.Project != "api" || payload.At != "release.projects[0]" || !slices.Equal(payload.Stages, []string{"restore", "lint"}) {
		t.Errorf("api payload = %+v (%v)", payload, err)
	}
	if web.Provider != "bash" || web.Config["shared_host"] != false || !slices.Equal(r.argv[1], []string{"/opt/nerv", "provider", "release-bash", "plan"}) {
		t.Errorf("web request = %+v, argv %q", web, r.argv[1])
	}
}

// A provider's validate failure is the configuration's refusal, with its
// problems.
func TestProviders_ValidateFailureIsAValidationError(t *testing.T) {
	r := &recordingRunner{answer: func(req extension.Request) extension.Response {
		data, _ := json.Marshal(ProblemsData{Problems: []string{"release.projects[0].providers.docker-compose.file: required"}})
		return extension.Response{Status: extension.StatusFailed, Message: "invalid", Data: data}
	}}
	rel := &releaseconfig.Release{Build: "docker-compose", Projects: []releaseconfig.Project{{Name: "api", Path: "."}}}
	b, err := NewBuildProvider(testProviders(r), rel, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := invalidProblems(t, b.Validate(context.Background())); got != "release.projects[0].providers.docker-compose.file: required" {
		t.Errorf("problems = %s", got)
	}
}

func nativeCall(t *testing.T, plugin, op string, config map[string]any, payload any) extension.Response {
	t.Helper()
	req := extension.Request{Contract: extension.ContractVersion, Extension: Extension, Operation: op, Config: config, Payload: payload}
	data, _ := json.Marshal(req)
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}
	if plugin == ConventionalCommits {
		return conventionalCommits(context.Background(), op, req, NativeDeps{})
	}
	return buildProvider(plugin, op, req)
}

func problemsOf(t *testing.T, resp extension.Response) string {
	t.Helper()
	var data ProblemsData
	if err := resp.DecodeData(&data); err != nil || resp.Status != extension.StatusFailed {
		t.Fatalf("response %+v, want failed with problems", resp)
	}
	return strings.Join(data.Problems, "\n")
}

// The conventional-commits engine checks its keys and the projects, each
// problem named by its path in .ai-workflow.yml.
func TestConventionalCommits_ValidateNamesTheKeys(t *testing.T) {
	got := problemsOf(t, nativeCall(t, ConventionalCommits, OpValidate,
		map[string]any{"release_rules": []any{map[string]any{"type": "docs", "release": "huge"}}, "tag_format": "v1"},
		VersionPayload{Projects: []VersionProject{{Name: "a", Path: "a", TagFormat: "x-${version}"}, {Name: "b", Path: "b", TagFormat: "x-${version}"}}}))
	for _, want := range []string{
		`release.providers.conventional-commits.release_rules[0]: release "huge" is not major, minor, patch or none`,
		`release.providers.conventional-commits.tag_format: "v1" must contain ${version} exactly once`,
		`release.projects[1].tag_format: "x-${version}" is also the tag format of project "a"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("problems lack %q:\n%s", want, got)
		}
	}
	if resp := nativeCall(t, ConventionalCommits, OpValidate, map[string]any{"tag_format": "v${version}"}, VersionPayload{}); resp.Failure() {
		t.Errorf("valid config: %+v", resp)
	}
}

// The engine's defaults give each project its tag format and CHANGELOG:
// its own, or the engine's tag_format (prefixed with the project's name in
// a monorepo) and <path>/<changelog.file>.
func TestConventionalCommits_ProjectDefaults(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]any
		projects []VersionProject
		tag      map[string]string
		log      map[string]string
	}{
		{"single project", nil, []VersionProject{{Name: "api", Path: "."}}, map[string]string{"api": "v${version}"}, map[string]string{"api": "CHANGELOG.md"}},
		{"explicit", nil, []VersionProject{{Name: "api", Path: "src/Api", TagFormat: "api@${version}", Changelog: "src/Api/CHANGES.md"}},
			map[string]string{"api": "api@${version}"}, map[string]string{"api": "src/Api/CHANGES.md"}},
		{"monorepo and engine keys", map[string]any{"tag_format": "r${version}", "changelog": map[string]any{"file": "HISTORY.md"}},
			[]VersionProject{{Name: "api", Path: "api"}, {Name: "web", Path: "web", Changelog: `docs\web.md`}},
			map[string]string{"api": "api-r${version}", "web": "web-r${version}"}, map[string]string{"api": "api/HISTORY.md", "web": "docs/web.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, problems := engineConfig(extension.Request{Config: tt.config}, tt.projects)
			if len(problems) > 0 {
				t.Fatal(problems)
			}
			for _, p := range cfg.Projects {
				if p.TagFormat != tt.tag[p.Name] || p.Changelog != tt.log[p.Name] {
					t.Errorf("%s: tag %q changelog %q, want %q %q", p.Name, p.TagFormat, p.Changelog, tt.tag[p.Name], tt.log[p.Name])
				}
			}
		})
	}
}

// docker-compose needs its file, a valid service name, and leaves build,
// test and package to compose; bash takes any stage.
func TestBuildProviders_Validate(t *testing.T) {
	payload := BuildPayload{Project: "api", At: "release.projects[0]", Stages: []string{"lint", "build", "package"}}
	got := problemsOf(t, nativeCall(t, DockerCompose, OpValidate, map[string]any{"test_service": "my tests"}, payload))
	for _, want := range []string{
		"release.projects[0].providers.docker-compose.file: required by the docker-compose build (or release.providers.docker-compose.file for every project)",
		`release.projects[0].providers.docker-compose.test_service: "my tests" is not a compose service name`,
		"release.projects[0].stages.build: docker-compose runs build itself (docker compose build, run and save); remove it, or use build: bash",
		"release.projects[0].stages.package: docker-compose runs package itself",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("problems lack %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "stages.lint") {
		t.Errorf("lint refused:\n%s", got)
	}
	got = problemsOf(t, nativeCall(t, DockerCompose, OpValidate, map[string]any{"file": "../compose.yml"}, BuildPayload{At: "release.projects[0]"}))
	if !strings.Contains(got, `release.projects[0].providers.docker-compose.file: "../compose.yml" escapes the repository root`) {
		t.Errorf("problems:\n%s", got)
	}
	if resp := nativeCall(t, Bash, OpValidate, map[string]any{"shared_host": true}, payload); resp.Failure() {
		t.Errorf("bash validate: %+v", resp)
	}
}

// docker-compose plans compose build and run with the normalised file;
// bash plans no step of its own, and shared_host gives the dotnet stages
// the build-server environment.
func TestBuildProviders_Plan(t *testing.T) {
	var data PlanData
	resp := nativeCall(t, DockerCompose, OpPlan, map[string]any{"file": `deploy\compose.yml`, "test_service": "tests"}, BuildPayload{At: "release.projects[0]"})
	if err := resp.DecodeData(&data); err != nil || resp.Failure() {
		t.Fatalf("plan: %+v %v", resp, err)
	}
	if len(data.Steps) != 2 || !reflect.DeepEqual(data.Steps[0], BuildStep{Stage: "build", Name: "compose:build", Exec: []string{"docker", "compose", "-f", "deploy/compose.yml", "build"}}) ||
		!slices.Equal(data.Steps[1].Exec, []string{"docker", "compose", "-f", "deploy/compose.yml", "run", "--rm", "tests"}) ||
		data.ImagePrefix != "compose:" || data.Paths["file"] != "deploy/compose.yml" {
		t.Errorf("compose plan = %+v", data)
	}
	data = PlanData{}
	resp = nativeCall(t, Bash, OpPlan, map[string]any{"shared_host": true}, BuildPayload{})
	if err := resp.DecodeData(&data); err != nil || len(data.Steps) != 0 || data.ImagePrefix != "docker:" || data.StageEnv["build"]["UseSharedCompilation"] != "false" || data.StageEnv["coverage"] != nil {
		t.Errorf("bash plan = %+v (%v)", data, err)
	}
}
