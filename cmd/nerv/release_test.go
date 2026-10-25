package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/artifact"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseclone"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseplan"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaserun"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

const localHead = "1111111111111111111111111111111111111111"

// releaseMonorepoYAML: core packages a file and has prepare and fail hooks;
// cli only builds; web depends on core and has a fail hook.
const releaseMonorepoYAML = `release:
  projects:
    - name: core
      path: core
      build: bash
      stages:
        build: { run: "echo build core" }
        package: { run: "echo package core" }
      hooks:
        prepare: [{ run: "echo prepare core" }]
        fail: [{ run: "echo fail core" }]
    - name: cli
      path: cli
      build: bash
      stages:
        build: { run: "echo build cli" }
    - name: web
      path: web
      build: bash
      depends_on:
        - project: core
          bump: minor
      stages:
        build: { run: "echo build web" }
      hooks:
        fail: [{ run: "echo fail web" }]
`

// releaseVersionFilesYAML is one script project at the root with a version
// file.
const releaseVersionFilesYAML = `release:
  projects:
    - name: api
      path: .
      build: bash
      version_files: [src/Api/Api.csproj]
      stages:
        build: { run: "echo build api" }
`

// releaseComposeYAML is one compose project with a test service and an image.
const releaseComposeYAML = `release:
  projects:
    - name: api
      path: api
      build: docker-compose
      providers:
        docker-compose: { file: api/docker-compose.yml, test_service: tests }
      images: ["ghcr.io/acme/api:${version}"]
      hooks:
        fail: [{ run: "echo fail api" }]
`

const apiCsproj = "<Project Sdk=\"Microsoft.NET.Sdk\">\n  <PropertyGroup>\n    <Version>1.0.0</Version>\n    <PackageReleaseNotes>old notes</PackageReleaseNotes>\n  </PropertyGroup>\n</Project>\n"

// fakeWorkspace is the release clone: a real directory holding the
// configuration, scripted answers for origin and the version engine.
type fakeWorkspace struct {
	f          *releaseFixture
	dir, extra string
	head       string
	public     string
	results    []versioning.Result
	computed   []string
	computeErr error
	// branch is how the release: section's branches treat the branch;
	// branchErr, when set, is the engine's answer instead.
	branch    versioning.ReleaseBranch
	branchErr error
	// originNow is origin's branch head at the end of the run ("" = head);
	// originGone: the branch was deleted on origin; originErr: it cannot be
	// read. originCtxErrs are the context errors BranchOnOrigin saw.
	originNow     string
	originGone    bool
	originErr     error
	originCtxErrs []error
	closed        bool
	// removeOnClose removes the artifacts and notes directories on Close,
	// as releaseclone.Workspace.Close does.
	removeOnClose bool
}

func (w *fakeWorkspace) Dir() string { return w.dir }

// Config is the user's local release: section, read from the fixture's
// repository (never the clone), as releaseclone.Workspace.Config.
func (w *fakeWorkspace) Config() *releaseconfig.Release {
	rel, err := releaseconfig.Load(w.f.top)
	if err != nil {
		return nil
	}
	return rel
}

func (w *fakeWorkspace) ArtifactsDir() string { return filepath.Join(w.extra, "artifacts") }
func (w *fakeWorkspace) NotesDir() string     { return filepath.Join(w.extra, "notes") }
func (w *fakeWorkspace) OriginHead() string   { return w.head }
func (w *fakeWorkspace) PublicURL() string    { return w.public }

func (w *fakeWorkspace) Close() error {
	w.closed = true
	w.f.log("close")
	if w.removeOnClose {
		return os.RemoveAll(w.extra)
	}
	return nil
}

func (w *fakeWorkspace) Compute(_ context.Context, projects []string) ([]versioning.Result, error) {
	w.computed = append(w.computed, strings.Join(projects, ","))
	w.f.log("compute")
	results := slices.Clone(w.results)
	for i := range results {
		if results[i].Changelog == "" {
			results[i].Changelog = w.defaultChangelog(results[i].Project)
		}
	}
	return results, w.computeErr
}

// defaultChangelog and defaultTagFormat are the version engine's defaults
// for the project (the engine's own tests check them).
func (w *fakeWorkspace) defaultChangelog(project string) string {
	p, _ := w.Config().Project(project)
	if p.Changelog != "" {
		return p.Changelog
	}
	return path.Join(p.Path, "CHANGELOG.md")
}

func (w *fakeWorkspace) defaultTagFormat(project string) string {
	rel := w.Config()
	p, _ := rel.Project(project)
	switch {
	case p.TagFormat != "":
		return p.TagFormat
	case len(rel.Projects) == 1:
		return "v${version}"
	}
	return project + "-v${version}"
}

// Explicit stands in for the engine's explicit release: the last release
// is the highest tag of the project's format that the clone's scripted
// "git tag --list" answers (the engine's reachability and channel rules
// are tested in internal/versioning and the integration tests), and the
// notes name the version.
func (w *fakeWorkspace) Explicit(ctx context.Context, project, version string) (versioning.Result, error) {
	w.f.log("explicit")
	format := w.defaultTagFormat(project)
	out, _, _, err := w.f.runner.Run(ctx, "git", "-C", w.dir, "tag", "--list")
	if err != nil {
		return versioning.Result{}, err
	}
	r := versioning.Result{Project: project, NewReleasePublished: true, NewReleaseVersion: version, NewReleaseGitTag: releaseplan.TagFor(format, version),
		Changelog:        w.defaultChangelog(project),
		NewReleaseNotes:  "## " + version + "\n\n* " + project + " change\n",
		ChangelogSection: "## " + version + " (2026-10-24)\n\n* " + project + " change\n"}
	for _, tag := range strings.Fields(out) {
		tv, ok := releaseplan.VersionFromTag(format, tag)
		if !ok {
			continue
		}
		if c, _ := releaseplan.CompareVersions(tv, r.LastReleaseVersion); r.LastReleaseVersion == "" || c > 0 {
			r.LastReleaseVersion, r.LastReleaseGitTag = tv, tag
		}
	}
	return r, nil
}

func (w *fakeWorkspace) Branch(context.Context) (versioning.ReleaseBranch, error) {
	w.f.log("branch")
	return w.branch, w.branchErr
}

func (w *fakeWorkspace) BranchOnOrigin(ctx context.Context) (string, error) {
	w.originCtxErrs = append(w.originCtxErrs, ctx.Err())
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	switch {
	case w.originErr != nil:
		return "", w.originErr
	case w.originGone:
		return "", nil
	case w.originNow != "":
		return w.originNow, nil
	}
	return w.head, nil
}

// releaseFixture is a fake repository for nerv release: git answers come from a
// scripted runner, the release configuration from real files, the release
// clone and the steps are recorded fakes sharing one event log.
type releaseFixture struct {
	top      string
	runner   *envtest.FakeRunner
	ws       *fakeWorkspace
	opened   []releaseclone.Options
	events   []string
	vars     []releaserun.Vars
	steps    []releaserun.Step
	failStep map[string]error
	// outputs are the files a step writes in its $NERV_RELEASE_OUTPUTS_DIR
	// (event -> relative path -> content); a docker save step writes its
	// archive.
	outputs map[string]map[string]string
	tty     bool
	stdin   string
	env     map[string]string
	cancel  context.CancelFunc
	signal  os.Signal
	// onFirstStep runs before the first step is recorded.
	onFirstStep func()
}

func (f *releaseFixture) log(event string) { f.events = append(f.events, event) }

func newReleaseFixture(t *testing.T, nervYAML string) *releaseFixture {
	t.Helper()
	top := filepath.Join(t.TempDir(), "repo")
	clone := filepath.Join(t.TempDir(), "repo")
	for _, dir := range []string{top, clone} {
		if err := os.MkdirAll(filepath.Join(dir, ".nerv"), 0o755); err != nil {
			t.Fatal(err)
		}
		if nervYAML != "" {
			if err := os.MkdirAll(filepath.Join(dir, ".ai-workflow"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".ai-workflow/config.yaml"), []byte(nervYAML), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	f := &releaseFixture{
		top: top,
		runner: &envtest.FakeRunner{Responses: map[string]envtest.Response{
			"git -C . rev-parse --show-toplevel":                            {Stdout: top + "\n"},
			"git -C " + top + " symbolic-ref --short HEAD":                  {Stdout: "main\n"},
			"git -C " + top + " rev-parse --verify --quiet refs/heads/main": {Stdout: localHead + "\n"},
			"git -C " + clone + " tag --list":                               {Stdout: ""},
		}},
		env: map[string]string{},
	}
	f.ws = &fakeWorkspace{f: f, dir: clone, extra: t.TempDir(), head: localHead, public: "https://github.com/acme/widgets.git", branch: versioning.ReleaseBranch{Name: "main"}}
	// Origin's branch (the clone) has every project directory the local
	// configuration names.
	if rel, err := releaseconfig.Load(top); err == nil {
		for _, p := range rel.Projects {
			if err := os.MkdirAll(filepath.Join(clone, filepath.FromSlash(p.Path)), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, d := range []string{f.ws.ArtifactsDir(), f.ws.NotesDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// stepRecorder is the fake process step runner.
type stepRecorder struct {
	f *releaseFixture
	t *testing.T
}

func (r stepRecorder) Run(_ context.Context, s releaserun.Step, v releaserun.Vars) error {
	f := r.f
	if f.onFirstStep != nil {
		f.onFirstStep()
		f.onFirstStep = nil
	}
	event := v.Project + ": " + s.Command.Run
	if len(s.Exec) > 0 {
		event = v.Project + ": " + strings.Join(s.Exec, " ")
	}
	f.log(event)
	f.vars = append(f.vars, v)
	f.steps = append(f.steps, s)
	if err := f.failStep[event]; err != nil {
		return err
	}
	write := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
	for rel, content := range f.outputs[event] {
		write(filepath.Join(s.Command.Env[releaseplan.OutputsEnv], filepath.FromSlash(rel)), content)
	}
	if len(s.Exec) > 3 && s.Exec[0] == "docker" && s.Exec[1] == "save" {
		write(s.Exec[3], "image "+s.Exec[4])
	}
	return nil
}

func (f *releaseFixture) options(t *testing.T) options {
	opts := testOptions(t.TempDir())
	opts.Runner = f.runner
	opts.IsTerminal = func() bool { return f.tty }
	opts.Stdin = strings.NewReader(f.stdin)
	opts.Getenv = func(k string) string { return f.env[k] }
	opts.OpenReleaseWorkspace = func(_ context.Context, o releaseclone.Options) (releaseWorkspace, error) {
		f.opened = append(f.opened, o)
		f.log("open")
		return f.ws, nil
	}
	opts.ReleaseSteps = func(root string, _ io.Writer) releaseplan.StepRunner {
		if root != f.ws.dir {
			t.Errorf("steps run in %s, want the release clone %s", root, f.ws.dir)
		}
		return stepRecorder{f, t}
	}
	opts.SignalContext = func(parent context.Context) (context.Context, context.CancelFunc, func() os.Signal) {
		ctx, cancel := context.WithCancel(parent)
		f.cancel = cancel
		return ctx, cancel, func() os.Signal { return f.signal }
	}
	return opts
}

func (f *releaseFixture) run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(append([]string{"release"}, args...), &out, &errOut, f.options(t))
	return code, out.String(), errOut.String()
}

func (f *releaseFixture) runJSON(t *testing.T, args ...string) (int, releaseDocument, string) {
	t.Helper()
	code, stdout, stderr := f.run(t, append(args, "--json")...)
	var doc releaseDocument
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%q\nstderr %s", err, stdout, stderr)
	}
	return code, doc, stderr
}

func releaseResult(project, version, tag string) versioning.Result {
	return versioning.Result{
		Project: project, Branch: "main",
		NewReleasePublished: true, NewReleaseVersion: version, NewReleaseGitTag: tag,
		NewReleaseNotes:    "## " + version + "\n\n* " + project + " change\n",
		ChangelogSection:   "## " + version + " (2026-10-23)\n\n* " + project + " change\n",
		NewReleaseGitHead:  localHead,
		LastReleaseVersion: "1.0.0", LastReleaseGitTag: project + "-v1.0.0",
	}
}

func (f *releaseFixture) monorepoResults() {
	f.ws.results = []versioning.Result{
		releaseResult("core", "1.1.0", "core-v1.1.0"),
		{Project: "cli", Branch: "main", LastReleaseVersion: "0.3.0", LastReleaseGitTag: "cli-v0.3.0"},
		releaseResult("web", "2.1.0", "web-v2.1.0"),
	}
}

// packaged lists the artifacts in the repository, as <project>/<version>.
func (f *releaseFixture) packaged(t *testing.T) []string {
	t.Helper()
	var out []string
	root := artifact.Root(f.top)
	projects, _ := os.ReadDir(root)
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		versions, _ := os.ReadDir(filepath.Join(root, p.Name()))
		for _, v := range versions {
			out = append(out, p.Name()+"/"+v.Name())
		}
	}
	return out
}

func indexOf(events []string, e string) int { return slices.Index(events, e) }

// noGitWrites fails when the run asked git for anything but reads.
func (f *releaseFixture) noGitWrites(t *testing.T) {
	t.Helper()
	for _, c := range f.runner.Calls {
		args := c.Args
		if len(args) > 2 && args[0] == "-C" {
			args = args[2:]
		}
		if c.Name != "git" || len(args) == 0 {
			continue
		}
		if slices.Contains([]string{"push", "commit", "notes", "update-ref"}, args[0]) || (args[0] == "tag" && !slices.Equal(args, []string{"tag", "--list"})) {
			t.Errorf("git write: git %s", strings.Join(c.Args, " "))
		}
	}
}

// ---------------------------------------------------------------------------

func TestRelease_UsageErrors(t *testing.T) {
	for _, args := range [][]string{{"extra"}, {"--fetch-timeout", "0s"}, {"--nope"}, {"--version", "1.0"}, {"--version", "v1.0.0"}} {
		f := newReleaseFixture(t, releaseMonorepoYAML)
		if code, _, stderr := f.run(t, args...); code != 2 || !strings.Contains(stderr, "Usage: nerv release") || len(f.events) != 0 {
			t.Errorf("%v: exit %d, events %v, stderr %q", args, code, f.events, stderr)
		}
	}
	f := newReleaseFixture(t, "")
	if code, stdout, _ := f.run(t, "--help"); code != 0 || !strings.Contains(stdout, "--dry-run") || !strings.Contains(stdout, "--version <semver>") || !strings.Contains(stdout, "--force") {
		t.Errorf("--help: exit %d %q", code, stdout)
	}
}

func TestRelease_RefusesWhenTheLocalBranchIsNotOrigins(t *testing.T) {
	tests := []struct {
		name, head, want string
	}{
		{"ahead or behind", strings.Repeat("2", 40), "push or pull"},
		{"not on origin", "", "not on origin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReleaseFixture(t, releaseMonorepoYAML)
			f.monorepoResults()
			f.ws.head = tt.head
			code, _, stderr := f.run(t, "--yes")
			if code != 1 || !strings.Contains(stderr, tt.want) {
				t.Errorf("exit %d, stderr %q; want 1 and %q", code, stderr, tt.want)
			}
			if slices.Contains(f.events, "compute") || len(f.packaged(t)) != 0 {
				t.Errorf("events %v: nothing may run", f.events)
			}
			if !f.ws.closed {
				t.Error("release clone not removed")
			}
		})
	}
}

func TestRelease_ReadsOriginOnTheRequestedBranch(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	f.monorepoResults()
	f.runner.Responses["git -C "+f.top+" rev-parse --verify --quiet refs/heads/develop"] = envtest.Response{Stdout: localHead + "\n"}
	if code, _, stderr := f.run(t, "--yes", "--branch", "develop", "--fetch-timeout", "5s", "--project", "web"); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if len(f.opened) != 1 || f.opened[0].RepoRoot != f.top || f.opened[0].Branch != "develop" || f.opened[0].FetchTimeout.String() != "5s" {
		t.Errorf("workspace options %+v", f.opened)
	}
	if !slices.Equal(f.ws.computed, []string{""}) {
		t.Errorf("computed %v", f.ws.computed)
	}
}

func TestRelease_UnknownProjectAndNoOrigin(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	if code, _, stderr := f.run(t, "--yes", "--project", "mobile"); code != 1 || !strings.Contains(stderr, "core, cli, web") {
		t.Errorf("unknown project: exit %d %q", code, stderr)
	}
	f = newReleaseFixture(t, releaseMonorepoYAML)
	opts := f.options(t)
	opts.OpenReleaseWorkspace = func(context.Context, releaseclone.Options) (releaseWorkspace, error) {
		return nil, releaseclone.ErrNoOrigin
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"release", "--yes"}, &out, &errOut, opts); code != 1 || !strings.Contains(errOut.String(), "origin") {
		t.Errorf("no origin: exit %d %q", code, errOut.String())
	}
}

// The build provider is required, with no default; a preset key says what
// to write instead; an unknown build provider or a key it does not declare
// is refused, and so is a provider's own refusal of its keys (its
// validate). Each is refused before origin is read.
func TestRelease_RequiresTheBuildProvider(t *testing.T) {
	tests := []struct {
		name, yaml string
		want       []string
	}{
		{"no release section", "", []string{"no release: section", "build provider (build: docker-compose or bash)"}},
		{"no build provider", "release:\n  projects:\n    - { name: api, path: . }\n", []string{"release.projects[0].build: required by nerv release (the build provider, such as docker-compose or bash; or set release.build for every project): there is no default"}},
		{"unknown build provider", "release:\n  build: maven\n  projects:\n    - { name: api, path: . }\n", []string{`release.build: unknown build provider "maven" (want bash, docker-compose)`}},
		{"unknown provider key", "release:\n  projects:\n    - name: api\n      path: .\n      build: bash\n      providers:\n        bash: { sdk: \"8\" }\n", []string{"release.projects[0].providers.bash.sdk: unknown key (want shared_host)"}},
		{"provider refusal", "release:\n  projects:\n    - name: api\n      path: .\n      build: docker-compose\n      stages:\n        build: { run: make }\n", []string{
			"release.projects[0].providers.docker-compose.file: required by the docker-compose build",
			"release.projects[0].stages.build: docker-compose runs build itself",
		}},
		{"engine refusal", "release:\n  providers:\n    conventional-commits: { release_rules: [{ type: docs, release: huge }] }\n  projects:\n    - { name: api, path: ., build: bash }\n", []string{
			`release.providers.conventional-commits.release_rules[0]: release "huge" is not major, minor, patch or none`,
		}},
		{"old shape", "release:\n  branches: [main]\n  projects:\n    - name: api\n      path: .\n      build: { mode: compose, file: c.yml }\n", []string{
			"release.branches: moved to release.providers.conventional-commits.branches",
			"release.projects[0].build.mode: build is the build provider's name now",
			"release.projects[0].build.file: moved to release.projects[0].providers.docker-compose.file",
		}},
		{"old preset", "release:\n  projects:\n    - name: api\n      path: .\n      presets: [dotnet]\n      dotnet: { project_file: Api.csproj }\n", []string{
			"release.projects[0].presets: the presets were removed; set the build provider instead",
			"release.projects[0].dotnet: the dotnet preset was removed",
		}},
		{"old publish stage", "release:\n  projects:\n    - name: api\n      path: .\n      build: bash\n      stages:\n        publish: { run: x }\n", []string{"publishing moved to nerv deploy"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReleaseFixture(t, tt.yaml)
			code, doc, _ := f.runJSON(t, "--yes")
			if code != 1 || doc.Status != "refused" || len(f.events) != 0 {
				t.Fatalf("exit %d, %+v, events %v", code, doc, f.events)
			}
			for _, w := range tt.want {
				if !strings.Contains(doc.Error, w) {
					t.Errorf("error lacks %q: %s", w, doc.Error)
				}
			}
		})
	}
}

func TestRelease_Confirmation(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		tty        bool
		stdin      string
		wantCode   int
		wantPrompt bool
		wantPack   bool
	}{
		{"terminal yes", nil, true, "y\n", 0, true, true},
		{"terminal YES", nil, true, "YES\n", 0, true, true},
		{"terminal no", nil, true, "n\n", 1, true, false},
		{"terminal blank", nil, true, "\n", 1, true, false},
		{"terminal EOF", nil, true, "", 1, true, false},
		{"not a terminal", nil, false, "y\n", 1, false, false},
		{"--yes", []string{"--yes"}, false, "", 0, false, true},
		{"--dry-run", []string{"--dry-run"}, false, "", 0, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReleaseFixture(t, releaseMonorepoYAML)
			f.monorepoResults()
			f.tty, f.stdin = tt.tty, tt.stdin
			code, _, stderr := f.run(t, tt.args...)
			if code != tt.wantCode {
				t.Errorf("exit %d, want %d; stderr:\n%s", code, tt.wantCode, stderr)
			}
			if got := strings.Contains(stderr, "Package 2 project(s)? [y/N]"); got != tt.wantPrompt {
				t.Errorf("prompted %v, want %v", got, tt.wantPrompt)
			}
			if got := len(f.packaged(t)) > 0; got != tt.wantPack {
				t.Errorf("packaged %v, want %v", f.packaged(t), tt.wantPack)
			}
			if !tt.wantPack && !tt.wantPrompt && tt.name != "--dry-run" && slices.ContainsFunc(f.events, func(e string) bool { return strings.Contains(e, ": echo") }) {
				t.Errorf("steps run without confirmation: %v", f.events)
			}
			if tt.name == "not a terminal" && !strings.Contains(stderr, "--yes to package without asking") {
				t.Errorf("refusal does not say what --yes does: %s", stderr)
			}
		})
	}
}

func TestRelease_PackagesInDependencyOrderAndSkipsUnreleased(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	f.monorepoResults()
	f.ws.removeOnClose = true
	f.outputs = map[string]map[string]string{"core: echo package core": {"core.zip": "zip", "site/index.html": "<html>"}}
	code, stdout, stderr := f.run(t, "--yes")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	want := []string{
		"open", "compute",
		"core: echo build core", "core: echo package core", "core: echo prepare core",
		"web: echo build web",
		"close",
	}
	if !slices.Equal(f.events, want) {
		t.Errorf("events\n %v\nwant\n %v", f.events, want)
	}
	f.noGitWrites(t)
	if got := f.packaged(t); !slices.Equal(got, []string{"core/1.1.0", "web/2.1.0"}) {
		t.Errorf("artifacts %v", got)
	}
	coreDir := artifact.Dir(f.top, "core", "1.1.0")
	for _, s := range []string{"core 1.1.0 (tag core-v1.1.0): packaged in " + coreDir, "web 2.1.0", "cli: no new release"} {
		if !strings.Contains(stdout, s) {
			t.Errorf("summary lacks %q:\n%s", s, stdout)
		}
	}
	for _, s := range []string{"2 project(s) to package", "core 1.1.0 (default channel, tag core-v1.1.0, build bash)", "last release 1.0.0", "echo package core", "artifact " + coreDir, "nerv release: core 1.1.0: artifact " + coreDir} {
		if !strings.Contains(stderr, s) {
			t.Errorf("listing lacks %q:\n%s", s, stderr)
		}
	}
	// The artifact survives the clone's removal and holds what the
	// package stage wrote.
	m, err := artifact.Verify(coreDir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Project != "core" || m.Version != "1.1.0" || m.Tag != "core-v1.1.0" || m.BaseCommit != localHead || m.Branch != "main" || m.BuildProvider != "bash" || m.CreatedAt.IsZero() {
		t.Errorf("manifest %+v", m)
	}
	if len(m.Outputs) != 2 || m.Outputs[0].Path != "outputs/core.zip" || m.Outputs[0].Kind != artifact.KindFile || m.Outputs[1].Kind != artifact.KindDir {
		t.Errorf("outputs %+v", m.Outputs)
	}
	notes, _ := os.ReadFile(filepath.Join(coreDir, artifact.NotesFile))
	section, _ := os.ReadFile(filepath.Join(coreDir, artifact.ChangelogFile))
	// The notes and the CHANGELOG section are the engine's own (the section
	// carries the release date, as @semantic-release/changelog writes it).
	if string(notes) != f.ws.results[0].NewReleaseNotes || string(section) != f.ws.results[0].ChangelogSection {
		t.Errorf("notes %q, section %q", notes, section)
	}
	if _, err := os.Stat(f.ws.extra); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the workspace was not removed: %v", err)
	}
}

func TestRelease_StepsGetTheReleaseVariables(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	f.monorepoResults()
	f.ws.results[0].NewReleaseChannel = "alpha"
	if code, _, stderr := f.run(t, "--yes"); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	build := f.vars[0]
	if build.Project != "core" || build.ProjectPath != "core" || build.Version != "1.1.0" || build.Channel != "alpha" ||
		build.Tag != "core-v1.1.0" || build.GitHead != localHead || build.LastVersion != "1.0.0" || build.Branch != "main" || build.DryRun {
		t.Errorf("build vars %+v", build)
	}
	notes, err := os.ReadFile(build.NotesFile)
	if err != nil || string(notes) != f.ws.results[0].NewReleaseNotes {
		t.Errorf("notes file %s: %q %v", build.NotesFile, notes, err)
	}
	if rel, err := filepath.Rel(f.ws.dir, build.NotesFile); err == nil && filepath.IsLocal(rel) {
		t.Errorf("notes file inside the clone: %s", build.NotesFile)
	}
	env := f.steps[0].Command.Env
	if env[releaseplan.OutputsEnv] != filepath.Join(f.ws.ArtifactsDir(), "core", "outputs") || env[releaseplan.ArtifactsEnv] != filepath.Join(f.ws.ArtifactsDir(), "core") {
		t.Errorf("step env %v", env)
	}
}

func TestRelease_NothingToRelease(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	f.ws.results = []versioning.Result{
		{Project: "core", Branch: "main"}, {Project: "cli", Branch: "main"}, {Project: "web", Branch: "main"},
	}
	code, stdout, _ := f.run(t)
	if code != 0 || !strings.Contains(stdout, "nothing to release") || len(f.packaged(t)) != 0 {
		t.Errorf("exit %d, stdout %q, packaged %v", code, stdout, f.packaged(t))
	}
}

func TestRelease_DryRunWritesNoArtifact(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	f.monorepoResults()
	code, stdout, stderr := f.run(t, "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	want := []string{"open", "compute", "core: echo build core", "core: echo package core", "core: echo prepare core", "web: echo build web", "close"}
	if !slices.Equal(f.events, want) {
		t.Errorf("events %v, want %v", f.events, want)
	}
	f.noGitWrites(t)
	for _, v := range f.vars {
		if !v.DryRun {
			t.Errorf("vars %+v: want DryRun", v)
		}
	}
	if _, err := os.Stat(artifact.Root(f.top)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a dry run created %s: %v", artifact.Root(f.top), err)
	}
	if !strings.Contains(stdout, "dry run: no artifact was written") || !strings.Contains(stderr, "dry run: the pipeline runs, no artifact is written") {
		t.Errorf("stdout %q\nstderr %q", stdout, stderr)
	}
}

// The version files and the CHANGELOG section are written in the clone
// before the pipeline, and their new contents are stored in the artifact;
// the user's repository never gets them.
func TestRelease_ReleaseFilesInTheCloneAndTheArtifact(t *testing.T) {
	tests := []struct {
		name, public, wantNotes string
		warn                    bool
	}{
		{"https origin", "https://github.com/acme/widgets.git", "https://github.com/acme/widgets/releases/tag/v1.1.0", false},
		{"ssh origin", "git@github.com:acme/widgets.git", "https://github.com/acme/widgets/releases/tag/v1.1.0", false},
		{"no public origin", "", "old notes", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReleaseFixture(t, releaseVersionFilesYAML)
			f.ws.public = tt.public
			f.ws.results = []versioning.Result{releaseResult("api", "1.1.0", "v1.1.0")}
			csproj := filepath.Join(f.ws.dir, "src", "Api", "Api.csproj")
			if err := os.MkdirAll(filepath.Dir(csproj), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(csproj, []byte(apiCsproj), 0o644); err != nil {
				t.Fatal(err)
			}
			changelog := filepath.Join(f.ws.dir, "CHANGELOG.md")
			if err := os.WriteFile(changelog, []byte("# Changelog\n\n## 1.0.0\n\n* first\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			var atFirstStep, changelogAtFirstStep string
			f.onFirstStep = func() {
				data, _ := os.ReadFile(csproj)
				atFirstStep = string(data)
				data, _ = os.ReadFile(changelog)
				changelogAtFirstStep = string(data)
			}
			code, _, stderr := f.run(t, "--yes")
			if code != 0 {
				t.Fatalf("exit %d: %s", code, stderr)
			}
			if !strings.Contains(atFirstStep, "<Version>1.1.0</Version>") || !strings.Contains(atFirstStep, "<PackageReleaseNotes>"+tt.wantNotes+"</PackageReleaseNotes>") {
				t.Errorf("csproj at the first step:\n%s", atFirstStep)
			}
			wantChangelog := "# Changelog\n\n## 1.1.0 (2026-10-23)\n\n* api change\n\n## 1.0.0\n\n* first\n"
			if changelogAtFirstStep != wantChangelog {
				t.Errorf("CHANGELOG at the first step:\n%q\nwant\n%q", changelogAtFirstStep, wantChangelog)
			}
			if got := strings.Contains(stderr, "warning"); got != tt.warn {
				t.Errorf("warning %v, want %v:\n%s", got, tt.warn, stderr)
			}
			dir := artifact.Dir(f.top, "api", "1.1.0")
			m, err := artifact.Verify(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(m.ReleaseFiles) != 2 || m.ReleaseFiles[0].Path != "src/Api/Api.csproj" || m.ReleaseFiles[1].Path != "CHANGELOG.md" {
				t.Fatalf("release files %+v", m.ReleaseFiles)
			}
			stored, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(m.ReleaseFiles[1].Stored)))
			if string(stored) != wantChangelog {
				t.Errorf("stored CHANGELOG %q", stored)
			}
			if _, err := os.Stat(filepath.Join(f.top, "CHANGELOG.md")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the user's repository got a CHANGELOG: %v", err)
			}
		})
	}
}

func TestRelease_AnExistingArtifactIsRefusedUnlessForced(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	f.monorepoResults()
	if code, _, stderr := f.run(t, "--yes", "--project", "core"); code != 0 {
		t.Fatalf("first run: exit %d: %s", code, stderr)
	}
	dir := artifact.Dir(f.top, "core", "1.1.0")
	marker := filepath.Join(dir, "stale")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f.events = nil
	code, doc, _ := f.runJSON(t, "--yes", "--project", "core")
	if code != 1 || doc.Status != "refused" || !strings.Contains(doc.Error, dir+" already exists: pass --force") {
		t.Fatalf("second run: exit %d, %+v", code, doc)
	}
	if want := []string{"open", "compute", "close"}; !slices.Equal(f.events, want) {
		t.Errorf("events %v, want %v: nothing may run", f.events, want)
	}
	// A dry run does not care.
	if code, _, stderr := f.run(t, "--dry-run", "--project", "core"); code != 0 {
		t.Errorf("dry run: exit %d: %s", code, stderr)
	}
	if code, doc, stderr := f.runJSON(t, "--yes", "--force", "--project", "core"); code != 0 || doc.Status != "packaged" {
		t.Fatalf("--force: exit %d, %+v\n%s", code, doc, stderr)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("--force kept the old artifact's files: %v", err)
	}
}

func TestRelease_ExplicitVersionSkipsComputing(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	f.runner.Responses["git -C "+f.ws.dir+" tag --list"] = envtest.Response{Stdout: "core-v1.0.0\ncore-v1.1.0-rc.2\nweb-v9.0.0\nv5.0.0\n"}
	code, doc, stderr := f.runJSON(t, "--yes", "--project", "core", "--version", "1.1.0", "--expect", "core=1.1.0")
	if code != 0 || doc.Status != "packaged" || doc.SemanticReleaseConfig != "" {
		t.Fatalf("exit %d, %+v\n%s", code, doc, stderr)
	}
	if slices.Contains(f.events, "compute") {
		t.Errorf("computed with --version: %v", f.events)
	}
	core := doc.Projects[0]
	if len(doc.Projects) != 1 || core.Version != "1.1.0" || core.Tag != "core-v1.1.0" || core.Channel != "" || core.LastVersion != "1.1.0-rc.2" {
		t.Errorf("outcome %+v", core)
	}
	if !strings.Contains(stderr, "version 1.1.0 from --version (not computed)") || !strings.Contains(stderr, "last release 1.1.0-rc.2 (tag core-v1.1.0-rc.2)") {
		t.Errorf("listing:\n%s", stderr)
	}
	m, err := artifact.Read(artifact.Dir(f.top, "core", "1.1.0"))
	if err != nil || m.BaseCommit != localHead || m.Channel != "" {
		t.Fatalf("manifest %+v %v", m, err)
	}
	section, _ := os.ReadFile(filepath.Join(artifact.Dir(f.top, "core", "1.1.0"), artifact.ChangelogFile))
	if !strings.HasPrefix(string(section), "## 1.1.0 (") {
		t.Errorf("CHANGELOG section %q", section)
	}
	f.noGitWrites(t)

	// L7, L15: a prerelease's channel is the branch's (release/* is rc on
	// the beta channel), never its prerelease identifier.
	f = newReleaseFixture(t, releaseMonorepoYAML)
	f.ws.branch = versioning.ReleaseBranch{Name: "release/3.0", Prerelease: "rc", Channel: "beta"}
	if code, doc, _ := f.runJSON(t, "--dry-run", "--project", "web", "--version", "3.0.0-rc.1"); code != 0 || doc.Projects[0].Channel != "beta" || doc.Projects[0].Tag != "web-v3.0.0-rc.1" {
		t.Errorf("prerelease: exit %d, %+v", code, doc)
	}
}

// L15: --version must fit the branch: its prerelease identifier on a
// prerelease branch, a stable version on a release branch, and a release
// branch at all.
func TestRelease_ExplicitVersionMustFitTheBranch(t *testing.T) {
	tests := []struct {
		name    string
		branch  versioning.ReleaseBranch
		err     error
		version string
		want    string
	}{
		{"stable on a prerelease branch", versioning.ReleaseBranch{Name: "develop", Prerelease: "alpha", Channel: "alpha"}, nil, "3.0.0", "develop is a prerelease branch whose prerelease identifier is alpha; pass a version such as 3.0.0-alpha.1"},
		{"another prerelease id", versioning.ReleaseBranch{Name: "release/3.0", Prerelease: "rc", Channel: "beta"}, nil, "3.0.0-beta.1", "prerelease identifier is rc"},
		{"prerelease on a release branch", versioning.ReleaseBranch{Name: "main"}, nil, "3.0.0-rc.1", "main is a release branch, which releases stable versions; pass one without a prerelease, such as 3.0.0"},
		{"not a release branch", versioning.ReleaseBranch{}, errors.New("branch topic is not a release branch (the configured branches are main)"), "3.0.0", "branch topic is not a release branch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReleaseFixture(t, releaseMonorepoYAML)
			f.ws.branch, f.ws.branchErr = tt.branch, tt.err
			code, doc, _ := f.runJSON(t, "--dry-run", "--project", "web", "--version", tt.version)
			if code != 1 || doc.Status != "refused" || !strings.Contains(doc.Error, tt.want) {
				t.Errorf("exit %d, %+v; want a refusal with %q", code, doc, tt.want)
			}
			if slices.ContainsFunc(f.events, func(e string) bool { return strings.Contains(e, ": echo") }) {
				t.Errorf("steps ran: %v", f.events)
			}
		})
	}
}

// L10: with --version, a --expect mismatch says the two flags disagree, not
// that origin moved.
func TestRelease_ExpectWithAnExplicitVersion(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	code, doc, _ := f.runJSON(t, "--yes", "--project", "core", "--version", "1.2.0", "--expect", "core=1.3.0")
	if code != 1 || !strings.Contains(doc.Error, "--version does not match --expect") || strings.Contains(doc.Error, "Origin probably moved") {
		t.Errorf("exit %d, %+v", code, doc)
	}
}

func TestRelease_ExplicitVersionChecks(t *testing.T) {
	tests := []struct {
		name string
		args []string
		tags string
		want string
		open bool
	}{
		{"monorepo needs --project", []string{"--version", "2.0.0"}, "", "declares 3 projects, each with its own version: pass --project too", false},
		{"not newer than the last tag", []string{"--project", "core", "--version", "1.1.0"}, "core-v1.1.0\n", "--version 1.1.0: project core already has 1.1.0 (tag core-v1.1.0); the version must be newer than the last one", true},
		{"older than the last tag", []string{"--project", "core", "--version", "1.0.9"}, "core-v1.0.0\ncore-v1.1.0\n", "already has 1.1.0", true},
		{"not the expected version", []string{"--project", "core", "--version", "1.2.0", "--expect", "core=1.3.0"}, "", "core: expected 1.3.0, computed 1.2.0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReleaseFixture(t, releaseMonorepoYAML)
			f.runner.Responses["git -C "+f.ws.dir+" tag --list"] = envtest.Response{Stdout: tt.tags}
			code, doc, _ := f.runJSON(t, append(tt.args, "--yes")...)
			if code != 1 || doc.Status != "refused" || !strings.Contains(doc.Error, tt.want) {
				t.Fatalf("exit %d, %+v; want a refusal with %q", code, doc, tt.want)
			}
			if got := slices.Contains(f.events, "open"); got != tt.open {
				t.Errorf("events %v: opened %v, want %v", f.events, got, tt.open)
			}
			if slices.ContainsFunc(f.events, func(e string) bool { return strings.Contains(e, ": echo") }) {
				t.Errorf("steps ran: %v", f.events)
			}
		})
	}
}

// newComposeFixture is a fixture for releaseComposeYAML, whose clone has the
// compose file.
func newComposeFixture(t *testing.T) *releaseFixture {
	t.Helper()
	f := newReleaseFixture(t, releaseComposeYAML)
	f.ws.results = []versioning.Result{releaseResult("api", "1.4.0", "v1.4.0")}
	if err := os.WriteFile(filepath.Join(f.ws.dir, "api", "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestRelease_ComposeModeBuildsTestsAndSavesTheImages(t *testing.T) {
	f := newComposeFixture(t)
	code, doc, stderr := f.runJSON(t, "--yes")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	tar := filepath.Join(f.ws.ArtifactsDir(), "api", "outputs", "ghcr.io_acme_api_1.4.0.tar")
	want := []string{
		"open", "compute",
		"api: docker compose -f api/docker-compose.yml build",
		"api: docker compose -f api/docker-compose.yml run --rm tests",
		"api: docker save -o " + tar + " ghcr.io/acme/api:1.4.0",
		"close",
	}
	if !slices.Equal(f.events, want) {
		t.Errorf("events\n %v\nwant\n %v", f.events, want)
	}
	m, err := artifact.Verify(doc.Projects[0].Artifact)
	if err != nil {
		t.Fatal(err)
	}
	if m.BuildProvider != "docker-compose" || !slices.Equal(m.Images, []string{"ghcr.io/acme/api:1.4.0"}) || len(m.Outputs) != 1 ||
		m.Outputs[0] != (artifact.Output{Path: "outputs/ghcr.io_acme_api_1.4.0.tar", Kind: artifact.KindImageTar, SHA256: m.Outputs[0].SHA256}) {
		t.Errorf("manifest %+v", m)
	}

	// The test service's exit code is the test result.
	f = newComposeFixture(t)
	f.failStep = map[string]error{"api: docker compose -f api/docker-compose.yml run --rm tests": errors.New("exit code 3")}
	code, doc, _ = f.runJSON(t, "--yes")
	if code != 2 || doc.Status != "failed" || doc.Projects[0].Status != "failed" || !slices.Contains(f.events, "api: echo fail api") || len(f.packaged(t)) != 0 {
		t.Errorf("exit %d, %+v, events %v", code, doc, f.events)
	}
}

// A build.images mapping gets 4.2.0's tags: release tags the version
// reference with the others, saves them all into one archive and records
// every reference in the manifest, so deploy pushes them all.
func TestRelease_ImageMappingSavesAndRecordsEveryTag(t *testing.T) {
	f := newReleaseFixture(t, strings.Replace(releaseComposeYAML, `images: ["ghcr.io/acme/api:${version}"]`, `images: [{ name: ghcr.io/acme/api }]`, 1))
	f.ws.results = []versioning.Result{releaseResult("api", "1.4.0", "v1.4.0")}
	if err := os.WriteFile(filepath.Join(f.ws.dir, "api", "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, doc, stderr := f.runJSON(t, "--yes")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	tar := filepath.Join(f.ws.ArtifactsDir(), "api", "outputs", "ghcr.io_acme_api_1.4.0.tar")
	for _, want := range []string{
		"api: docker tag ghcr.io/acme/api:1.4.0 ghcr.io/acme/api:latest",
		"api: docker save -o " + tar + " ghcr.io/acme/api:1.4.0 ghcr.io/acme/api:latest",
	} {
		if !slices.Contains(f.events, want) {
			t.Errorf("events %v lack %q", f.events, want)
		}
	}
	m, err := artifact.Verify(doc.Projects[0].Artifact)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.Images, []string{"ghcr.io/acme/api:1.4.0", "ghcr.io/acme/api:latest"}) || len(m.Outputs) != 1 {
		t.Errorf("manifest images %q, outputs %+v", m.Images, m.Outputs)
	}
}

func TestRelease_PipelineFailureRunsFailHooksAndWritesNoArtifact(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	f.monorepoResults()
	f.failStep = map[string]error{"core: echo build core": errors.New("exit code 1")}
	code, stdout, _ := f.run(t, "--yes")
	if code != 2 || len(f.packaged(t)) != 0 || !slices.Contains(f.events, "core: echo fail core") || slices.Contains(f.events, "web: echo build web") {
		t.Errorf("exit %d, events %v", code, f.events)
	}
	if !strings.Contains(stdout, "Nothing was written to your repository or to origin for core") {
		t.Errorf("summary:\n%s", stdout)
	}
	f.noGitWrites(t)
}

func TestRelease_InterruptedExitsWith128PlusTheSignal(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	f.monorepoResults()
	f.signal = syscall.SIGINT
	f.onFirstStep = func() { f.cancel() }
	f.failStep = map[string]error{"core: echo build core": context.Canceled}
	code, _, stderr := f.run(t, "--yes")
	if code != 130 || !strings.Contains(stderr, "interrupted") {
		t.Errorf("exit %d, stderr %s", code, stderr)
	}
	if slices.Contains(f.events, "core: echo fail core") {
		t.Errorf("fail hooks run after an interrupt: %v", f.events)
	}
	if !f.ws.closed || len(f.packaged(t)) != 0 {
		t.Errorf("release clone removed %v, artifacts %v", f.ws.closed, f.packaged(t))
	}
}

func TestRelease_ProjectComputesEveryProjectForDependsOn(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	f.monorepoResults()
	code, doc, stderr := f.runJSON(t, "--yes", "--project", "web")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !slices.Equal(f.ws.computed, []string{""}) || len(doc.Projects) != 1 || doc.Projects[0].Project != "web" {
		t.Errorf("computed %v, outcomes %+v", f.ws.computed, doc.Projects)
	}
	if got := f.packaged(t); !slices.Equal(got, []string{"web/2.1.0"}) {
		t.Errorf("packaged %v, want only web", got)
	}
}

func TestRelease_RerunAdviceSaysPullWhenOriginMoved(t *testing.T) {
	tests := []struct {
		name      string
		originNow string
		wantPull  bool
	}{
		{"origin unchanged", "", false},
		{"origin moved", strings.Repeat("9", 40), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReleaseFixture(t, releaseMonorepoYAML)
			f.monorepoResults()
			f.ws.originNow = tt.originNow
			// core is packaged, then web fails in its pipeline.
			f.failStep = map[string]error{"web: echo build web": errors.New("exit code 1")}
			code, doc, _ := f.runJSON(t, "--yes")
			web := doc.Projects[2]
			if code != 2 || web.Status != "failed" || doc.Projects[0].Status != "packaged" {
				t.Fatalf("exit %d, %+v", code, doc)
			}
			if got := strings.Contains(web.Action, "git pull"); got != tt.wantPull {
				t.Errorf("pull advice %v, want %v: %s", got, tt.wantPull, web.Action)
			}
			if !strings.Contains(web.Action, "run nerv release again") {
				t.Errorf("action %s", web.Action)
			}
		})
	}
}

func TestRelease_JSONDocumentOnEveryPath(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(f *releaseFixture)
		args       []string
		wantCode   int
		wantStatus string
		projects   int
	}{
		{"packaged", func(f *releaseFixture) {}, []string{"--yes"}, 0, "packaged", 3},
		{"dry run", func(f *releaseFixture) {}, []string{"--dry-run"}, 0, "dry-run", 3},
		{"nothing to release", func(f *releaseFixture) {
			f.ws.results = []versioning.Result{{Project: "core"}, {Project: "cli"}, {Project: "web"}}
		}, []string{"--yes"}, 0, "nothing-to-release", 3},
		{"refused: behind origin", func(f *releaseFixture) { f.ws.head = strings.Repeat("2", 40) }, []string{"--yes"}, 1, "refused", 0},
		{"refused: no --yes", func(f *releaseFixture) {}, nil, 1, "refused", 0},
		{"declined", func(f *releaseFixture) { f.tty, f.stdin = true, "n\n" }, nil, 1, "declined", 0},
		{"failed: compute", func(f *releaseFixture) { f.ws.computeErr = errors.New("node missing") }, []string{"--yes"}, 2, "failed", 0},
		{"interrupted", func(f *releaseFixture) {
			f.signal = syscall.SIGINT
			f.onFirstStep = func() { f.cancel() }
			f.failStep = map[string]error{"core: echo build core": context.Canceled}
		}, []string{"--yes"}, 130, "interrupted", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReleaseFixture(t, releaseMonorepoYAML)
			f.monorepoResults()
			tt.setup(f)
			code, doc, _ := f.runJSON(t, tt.args...)
			if code != tt.wantCode || doc.Status != tt.wantStatus || len(doc.Projects) != tt.projects || doc.Projects == nil {
				t.Errorf("exit %d, document %+v; want exit %d, status %s, %d projects", code, doc, tt.wantCode, tt.wantStatus, tt.projects)
			}
			if (code != 0) != (doc.Error != "") {
				t.Errorf("error %q with exit %d", doc.Error, code)
			}
		})
	}
}

func TestRelease_RerunAdviceWhenOriginCannotBeReadOrTheBranchIsGone(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(w *fakeWorkspace)
		want    []string
		notWant []string
	}{
		{"unreadable", func(w *fakeWorkspace) { w.originErr = errors.New("ssh: connect timed out") }, []string{"git fetch", "run nerv release again"}, nil},
		{"deleted", func(w *fakeWorkspace) { w.originGone = true }, []string{"deleted on origin"}, []string{"git pull"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReleaseFixture(t, releaseMonorepoYAML)
			f.monorepoResults()
			tt.setup(f.ws)
			f.failStep = map[string]error{"core: echo build core": errors.New("exit code 1")}
			code, doc, _ := f.runJSON(t, "--yes")
			action := doc.Projects[0].Action
			for _, w := range tt.want {
				if !strings.Contains(action, w) {
					t.Errorf("action lacks %q: %s", w, action)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(action, w) {
					t.Errorf("action has %q: %s", w, action)
				}
			}
			if code != 2 {
				t.Errorf("exit %d", code)
			}
		})
	}
}

func TestRelease_InterruptedRunStillReadsOriginForTheAdvice(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	f.monorepoResults()
	f.signal = syscall.SIGINT
	f.ws.originNow = strings.Repeat("9", 40)
	f.onFirstStep = func() { f.cancel() }
	f.failStep = map[string]error{"core: echo build core": context.Canceled}
	code, doc, _ := f.runJSON(t, "--yes")
	if code != 130 || doc.Status != "interrupted" {
		t.Fatalf("exit %d, %+v", code, doc)
	}
	if len(f.ws.originCtxErrs) == 0 || f.ws.originCtxErrs[0] != nil {
		t.Errorf("origin read with context errors %v, want a live context", f.ws.originCtxErrs)
	}
	if action := doc.Projects[0].Action; !strings.Contains(action, "pull first (git pull)") || strings.Contains(action, "git fetch") {
		t.Errorf("action %s", action)
	}
}

func TestRelease_ExpectBindsTheRunToTheApprovedVersions(t *testing.T) {
	tests := []struct {
		name      string
		expect    []string
		wantCode  int
		wantInErr []string
	}{
		{"match", []string{"core=1.1.0", "web=2.1.0"}, 0, nil},
		{"other version", []string{"core=1.0.9", "web=2.1.0"}, 1, []string{"core: expected 1.0.9, computed 1.1.0"}},
		{"releasing project not listed", []string{"core=1.1.0"}, 1, []string{"web: not expected, computed 2.1.0"}},
		{"expected project has no release", []string{"core=1.1.0", "web=2.1.0", "cli=0.4.0"}, 1, []string{"cli: expected 0.4.0, computed no release"}},
		{"unknown project", []string{"core=1.1.0", "web=2.1.0", "mobile=1.0.0"}, 1, []string{"mobile: expected 1.0.0, computed no release"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReleaseFixture(t, releaseMonorepoYAML)
			f.monorepoResults()
			args := []string{"--yes"}
			for _, e := range tt.expect {
				args = append(args, "--expect", e)
			}
			code, doc, stderr := f.runJSON(t, args...)
			if code != tt.wantCode {
				t.Fatalf("exit %d, want %d; stderr:\n%s", code, tt.wantCode, stderr)
			}
			if tt.wantCode == 0 {
				if doc.Status != "packaged" || len(f.packaged(t)) != 2 {
					t.Errorf("status %s, packaged %v", doc.Status, f.packaged(t))
				}
				return
			}
			if doc.Status != "refused" {
				t.Errorf("status %q, want refused", doc.Status)
			}
			for _, w := range append(tt.wantInErr, "dry run") {
				if !strings.Contains(doc.Error, w) {
					t.Errorf("error lacks %q: %s", w, doc.Error)
				}
			}
			if want := []string{"open", "compute", "close"}; !slices.Equal(f.events, want) {
				t.Errorf("events %v, want %v: nothing may run", f.events, want)
			}
		})
	}
}

func TestRelease_ExpectSyntaxIsAUsageError(t *testing.T) {
	for _, bad := range [][]string{
		{"--expect", "core"},
		{"--expect", "=1.0.0"},
		{"--expect", "core="},
		{"--expect", "core=1.0 .0"},
		{"--expect", "Core Lib=1.0.0"},
		{"--expect", "core=1.0.0", "--expect", "core=1.0.1"},
	} {
		f := newReleaseFixture(t, releaseMonorepoYAML)
		f.monorepoResults()
		code, _, stderr := f.run(t, append([]string{"--yes"}, bad...)...)
		if code != 2 || !strings.Contains(stderr, "--expect") || len(f.events) != 0 {
			t.Errorf("%v: exit %d, events %v, stderr %q", bad, code, f.events, stderr)
		}
	}
}

func TestRelease_UsesTheLocalConfigurationNotOrigins(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	f.monorepoResults()
	// Origin's committed copy (in the clone) has other commands.
	other := strings.ReplaceAll(releaseMonorepoYAML, "echo build core", "echo origin build core")
	if err := os.MkdirAll(filepath.Join(f.ws.dir, ".ai-workflow"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.ws.dir, ".ai-workflow/config.yaml"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := f.run(t, "--yes"); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !slices.Contains(f.events, "core: echo build core") || slices.Contains(f.events, "core: echo origin build core") {
		t.Errorf("events %v: want the local configuration's commands", f.events)
	}
}

// releasePathsYAML names one path per checked field; libs/**/gen is a glob.
const releasePathsYAML = `release:
  projects:
    - name: api
      path: api
      shared_paths: [libs/common, "libs/**/gen"]
      build: docker-compose
      providers:
        docker-compose: { file: api/compose.yml }
      version_files: [api/Api.csproj]
      stages:
        lint: { run: "echo lint", workdir: tools/build }
      hooks:
        prepare: [{ run: "echo prepare", workdir: tools/hooks }]
`

func TestRelease_LocalPathsMissingOnOriginAreRefused(t *testing.T) {
	tests := []struct {
		name   string
		remove []string
		want   []string
	}{
		{"every path on origin (the glob is not checked)", nil, nil},
		{"project path", []string{"api"}, []string{`"api" (project api, path)`}},
		{"shared path", []string{"libs/common"}, []string{`"libs/common" (project api, shared_paths)`}},
		{"version file", []string{"api/Api.csproj"}, []string{`"api/Api.csproj" (project api, version_files)`}},
		{"stage workdir", []string{"tools/build"}, []string{`"tools/build" (project api, stages.lint.workdir)`}},
		{"hook workdir", []string{"tools/hooks"}, []string{`"tools/hooks" (project api, hooks.prepare.workdir)`}},
		{"several, all listed", []string{"api/Api.csproj", "tools/hooks"}, []string{`"api/Api.csproj" (project api, version_files)`, `"tools/hooks" (project api, hooks.prepare.workdir)`}},
		// The build provider's own files are checked when it plans the
		// releasing project, before any stage runs.
		{"compose file", []string{"api/compose.yml"}, []string{`"api/compose.yml" (project api, providers.docker-compose.file)`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReleaseFixture(t, releasePathsYAML)
			f.ws.results = []versioning.Result{releaseResult("api", "1.1.0", "v1.1.0")}
			for _, d := range []string{"libs/common", "tools/build", "tools/hooks"} {
				if err := os.MkdirAll(filepath.Join(f.ws.dir, d), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for _, file := range []string{"api/Api.csproj", "api/compose.yml"} {
				if err := os.WriteFile(filepath.Join(f.ws.dir, file), []byte(apiCsproj), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for _, r := range tt.remove {
				if err := os.RemoveAll(filepath.Join(f.ws.dir, r)); err != nil {
					t.Fatal(err)
				}
			}
			code, doc, stderr := f.runJSON(t, "--dry-run")
			if tt.want == nil {
				if code != 0 || doc.Status != "dry-run" {
					t.Errorf("exit %d, %+v\nstderr %s", code, doc, stderr)
				}
				return
			}
			if code != 1 || doc.Status != "refused" || !strings.Contains(doc.Error, "not on origin's main") {
				t.Errorf("exit %d, %+v\nstderr %s", code, doc, stderr)
			}
			for _, w := range tt.want {
				if !strings.Contains(doc.Error, w) {
					t.Errorf("refusal lacks %s: %s", w, doc.Error)
				}
			}
			if strings.Contains(doc.Error, "libs/**/gen") {
				t.Errorf("the glob shared path was checked: %s", doc.Error)
			}
			// Refused before any version is computed, or, for the build
			// provider's files, before any stage runs.
			want := []string{"open", "close"}
			if tt.name == "compose file" {
				want = []string{"open", "compute", "close"}
			}
			if !slices.Equal(f.events, want) {
				t.Errorf("events %v, want %v", f.events, want)
			}
		})
	}
}

func TestRelease_SaysWhereTheVersionConfigCameFrom(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	f.monorepoResults()
	code, doc, stderr := f.runJSON(t, "--dry-run")
	if code != 0 || doc.SemanticReleaseConfig != "repository" || !strings.Contains(stderr, "version config: release: section (.ai-workflow/config.yaml)") {
		t.Errorf("exit %d, document %+v\nstderr %s", code, doc, stderr)
	}
}

// L10: a .nerv/artifacts that leads outside the repository is refused
// before any stage runs (a dry run writes no artifact, so it is not
// checked).
func TestRelease_RefusesASymlinkedArtifactsFolderBeforeRunning(t *testing.T) {
	f := newReleaseFixture(t, releaseMonorepoYAML)
	f.monorepoResults()
	if err := os.Symlink(t.TempDir(), filepath.Join(f.top, ".nerv", "artifacts")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	code, doc, _ := f.runJSON(t, "--yes")
	if code != 1 || doc.Status != "refused" || !strings.Contains(doc.Error, "outside the repository") {
		t.Fatalf("exit %d, %+v", code, doc)
	}
	if slices.Contains(f.events, "open") {
		t.Errorf("origin was read: %v", f.events)
	}
	if code, _, _ := f.runJSON(t, "--dry-run"); code != 0 {
		t.Errorf("dry run: exit %d", code)
	}
}

func TestPrependChangelog(t *testing.T) {
	tests := []struct{ old, want string }{
		{"", "## 2.0.0\n"},
		{"# Changelog\n\n## 1.0.0\n", "# Changelog\n\n## 2.0.0\n\n## 1.0.0\n"},
		{"## 1.0.0\n", "## 2.0.0\n\n## 1.0.0\n"},
	}
	for _, tt := range tests {
		if got := prependChangelog(tt.old, "## 2.0.0\n\n"); got != tt.want {
			t.Errorf("prependChangelog(%q) = %q, want %q", tt.old, got, tt.want)
		}
	}
}
