package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/semrel"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipplan"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

const localHead = "1111111111111111111111111111111111111111"

// shipMonorepoYAML: core and web publish and have every lifecycle hook (web
// only fail); cli only builds; web depends on core.
const shipMonorepoYAML = `release:
  projects:
    - name: core
      path: core
      stages:
        build: { run: "echo build core" }
        publish: { run: "echo publish core" }
      hooks:
        prepare: [{ run: "echo prepare core" }]
        publish: [{ run: "echo publish-hook core" }]
        success: [{ run: "echo success core" }]
        fail: [{ run: "echo fail core" }]
    - name: cli
      path: cli
      stages:
        build: { run: "echo build cli" }
    - name: web
      path: web
      depends_on:
        - project: core
          bump: minor
      stages:
        build: { run: "echo build web" }
        publish: { run: "echo publish web" }
      hooks:
        fail: [{ run: "echo fail web" }]
`

const shipDotnetYAML = `release:
  projects:
    - name: api
      path: .
      presets: [dotnet]
      dotnet:
        project_file: src/Api/Api.csproj
`

const apiCsproj = "<Project Sdk=\"Microsoft.NET.Sdk\">\n  <PropertyGroup>\n    <Version>1.0.0</Version>\n    <PackageReleaseNotes>old notes</PackageReleaseNotes>\n  </PropertyGroup>\n</Project>\n"

// fakeWorkspace is the release clone: a real directory holding the
// configuration, scripted answers for origin and semantic-release.
type fakeWorkspace struct {
	f          *shipFixture
	dir, extra string
	head       string
	public     string
	results    []semrel.ProjectResult
	pushErr    error
	release    func(ctx context.Context, req semrel.ReleaseRequest) (semrel.ProjectResult, error)
	tags       map[string]bool
	requests   []semrel.ReleaseRequest
	computed   []string
	computeErr error
	// originNow is origin's branch head at the end of the run ("" = head);
	// originGone: the branch was deleted on origin; originErr: it cannot be
	// read. originCtxErrs are the context errors BranchOnOrigin saw.
	originNow     string
	originGone    bool
	originErr     error
	originCtxErrs []error
	closed        bool
}

func (w *fakeWorkspace) Dir() string          { return w.dir }
func (w *fakeWorkspace) ArtifactsDir() string { return filepath.Join(w.extra, "artifacts") }
func (w *fakeWorkspace) NotesDir() string     { return filepath.Join(w.extra, "notes") }
func (w *fakeWorkspace) OriginHead() string   { return w.head }
func (w *fakeWorkspace) PublicURL() string    { return w.public }
func (w *fakeWorkspace) Close() error         { w.closed = true; w.f.log("close"); return nil }

func (w *fakeWorkspace) Compute(_ context.Context, project string) ([]semrel.ProjectResult, error) {
	w.computed = append(w.computed, project)
	w.f.log("compute")
	return w.results, w.computeErr
}

func (w *fakeWorkspace) CheckPush(context.Context) error {
	w.f.log("check-push")
	return w.pushErr
}

func (w *fakeWorkspace) Release(ctx context.Context, req semrel.ReleaseRequest) (semrel.ProjectResult, error) {
	w.requests = append(w.requests, req)
	w.f.log("release " + req.Expected.Project)
	if w.release != nil {
		return w.release(ctx, req)
	}
	r := req.Expected
	r.NewReleaseGitHead = "release-commit-" + r.Project
	w.tags[r.NewReleaseGitTag] = true
	return r, nil
}

func (w *fakeWorkspace) TagOnOrigin(_ context.Context, tag string) (bool, error) {
	w.f.log("tag? " + tag)
	return w.tags[tag], nil
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

// shipFixture is a fake repository for nerv ship: git answers come from a
// scripted runner, the release configuration from real files, the release
// clone and the steps are recorded fakes sharing one event log.
type shipFixture struct {
	top      string
	runner   *envtest.FakeRunner
	ws       *fakeWorkspace
	opened   []semrel.WorkspaceOptions
	events   []string
	vars     []shiprun.Vars
	failStep map[string]error
	tty      bool
	stdin    string
	env      map[string]string
	tokens   int
	cancel   context.CancelFunc
	signal   os.Signal
	// onFirstStep runs before the first step is recorded.
	onFirstStep func()
}

func (f *shipFixture) log(event string) { f.events = append(f.events, event) }

func newShipFixture(t *testing.T, nervYAML string) *shipFixture {
	t.Helper()
	top := filepath.Join(t.TempDir(), "repo")
	clone := filepath.Join(t.TempDir(), "repo")
	for _, dir := range []string{top, clone} {
		if err := os.MkdirAll(filepath.Join(dir, ".nerv"), 0o755); err != nil {
			t.Fatal(err)
		}
		if nervYAML != "" {
			if err := os.WriteFile(filepath.Join(dir, ".nerv", "nerv.yaml"), []byte(nervYAML), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	f := &shipFixture{
		top: top,
		runner: &envtest.FakeRunner{Responses: map[string]envtest.Response{
			"git -C . rev-parse --show-toplevel":                            {Stdout: top + "\n"},
			"git -C " + top + " symbolic-ref --short HEAD":                  {Stdout: "main\n"},
			"git -C " + top + " rev-parse --verify --quiet refs/heads/main": {Stdout: localHead + "\n"},
		}},
		env: map[string]string{},
	}
	f.ws = &fakeWorkspace{f: f, dir: clone, extra: t.TempDir(), head: localHead, public: "https://github.com/acme/widgets.git", tags: map[string]bool{}}
	for _, d := range []string{f.ws.ArtifactsDir(), f.ws.NotesDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// stepRecorder is the fake process step runner.
type stepRecorder struct{ f *shipFixture }

func (r stepRecorder) Run(_ context.Context, s shiprun.Step, v shiprun.Vars) error {
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
	return f.failStep[event]
}

func (f *shipFixture) options(t *testing.T) options {
	opts := testOptions(t.TempDir())
	opts.Runner = f.runner
	opts.IsTerminal = func() bool { return f.tty }
	opts.Stdin = strings.NewReader(f.stdin)
	opts.Getenv = func(k string) string { return f.env[k] }
	opts.OpenShipWorkspace = func(_ context.Context, o semrel.WorkspaceOptions) (shipWorkspace, error) {
		f.opened = append(f.opened, o)
		f.log("open")
		return f.ws, nil
	}
	opts.ShipSteps = func(root string, _ io.Writer) shipplan.StepRunner {
		if root != f.ws.dir {
			t.Errorf("steps run in %s, want the release clone %s", root, f.ws.dir)
		}
		return stepRecorder{f}
	}
	opts.GitHubToken = func(context.Context) string { f.tokens++; return "gh-tok" }
	opts.SignalContext = func(parent context.Context) (context.Context, context.CancelFunc, func() os.Signal) {
		ctx, cancel := context.WithCancel(parent)
		f.cancel = cancel
		return ctx, cancel, func() os.Signal { return f.signal }
	}
	return opts
}

func (f *shipFixture) run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(append([]string{"ship"}, args...), &out, &errOut, f.options(t))
	return code, out.String(), errOut.String()
}

func shipResult(project, version, tag string) semrel.ProjectResult {
	return semrel.ProjectResult{
		Project: project, Branch: "main",
		NewReleasePublished: true, NewReleaseVersion: version, NewReleaseGitTag: tag,
		NewReleaseNotes:    "## " + version + "\n\n* " + project + " change\n",
		NewReleaseGitHead:  localHead,
		LastReleaseVersion: "1.0.0", LastReleaseGitTag: project + "-v1.0.0",
	}
}

func (f *shipFixture) monorepoResults() {
	f.ws.results = []semrel.ProjectResult{
		shipResult("core", "1.1.0", "core-v1.1.0"),
		{Project: "cli", Branch: "main", LastReleaseVersion: "0.3.0", LastReleaseGitTag: "cli-v0.3.0"},
		shipResult("web", "2.1.0", "web-v2.1.0"),
	}
}

func (f *shipFixture) released() []string {
	var out []string
	for _, e := range f.events {
		if p, ok := strings.CutPrefix(e, "release "); ok {
			out = append(out, p)
		}
	}
	return out
}

func indexOf(events []string, e string) int { return slices.Index(events, e) }

// ---------------------------------------------------------------------------

func TestShip_UsageErrors(t *testing.T) {
	for _, args := range [][]string{{"extra"}, {"--fetch-timeout", "0s"}, {"--nope"}} {
		f := newShipFixture(t, shipMonorepoYAML)
		if code, _, stderr := f.run(t, args...); code != 2 || !strings.Contains(stderr, "Usage: nerv ship") {
			t.Errorf("%v: exit %d, stderr %q", args, code, stderr)
		}
	}
	f := newShipFixture(t, "")
	if code, stdout, _ := f.run(t, "--help"); code != 0 || !strings.Contains(stdout, "--dry-run") {
		t.Errorf("--help: exit %d %q", code, stdout)
	}
}

func TestShip_RefusesWhenTheLocalBranchIsNotOrigins(t *testing.T) {
	tests := []struct {
		name, head, want string
	}{
		{"ahead or behind", strings.Repeat("2", 40), "push or pull"},
		{"not on origin", "", "not on origin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newShipFixture(t, shipMonorepoYAML)
			f.monorepoResults()
			f.ws.head = tt.head
			code, _, stderr := f.run(t, "--yes")
			if code != 1 || !strings.Contains(stderr, tt.want) {
				t.Errorf("exit %d, stderr %q; want 1 and %q", code, stderr, tt.want)
			}
			if slices.Contains(f.events, "compute") || len(f.released()) != 0 {
				t.Errorf("events %v: nothing may run", f.events)
			}
			if !f.ws.closed {
				t.Error("release clone not removed")
			}
		})
	}
}

func TestShip_ReadsOriginOnTheRequestedBranch(t *testing.T) {
	f := newShipFixture(t, shipMonorepoYAML)
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

func TestShip_UnknownProjectAndNoOrigin(t *testing.T) {
	f := newShipFixture(t, shipMonorepoYAML)
	if code, _, stderr := f.run(t, "--yes", "--project", "mobile"); code != 1 || !strings.Contains(stderr, "core, cli, web") {
		t.Errorf("unknown project: exit %d %q", code, stderr)
	}
	f = newShipFixture(t, shipMonorepoYAML)
	opts := f.options(t)
	opts.OpenShipWorkspace = func(context.Context, semrel.WorkspaceOptions) (shipWorkspace, error) {
		return nil, semrel.ErrNoOrigin
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"ship", "--yes"}, &out, &errOut, opts); code != 1 || !strings.Contains(errOut.String(), "origin") {
		t.Errorf("no origin: exit %d %q", code, errOut.String())
	}
}

func TestShip_Confirmation(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		tty        bool
		stdin      string
		wantCode   int
		wantPrompt bool
		wantRel    bool
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
			f := newShipFixture(t, shipMonorepoYAML)
			f.monorepoResults()
			f.tty, f.stdin = tt.tty, tt.stdin
			code, _, stderr := f.run(t, tt.args...)
			if code != tt.wantCode {
				t.Errorf("exit %d, want %d; stderr:\n%s", code, tt.wantCode, stderr)
			}
			if got := strings.Contains(stderr, "Release 2 project(s)? [y/N]"); got != tt.wantPrompt {
				t.Errorf("prompted %v, want %v", got, tt.wantPrompt)
			}
			if got := len(f.released()) > 0; got != tt.wantRel {
				t.Errorf("released %v, want %v", f.released(), tt.wantRel)
			}
			if !tt.wantRel && slices.Contains(f.events, "check-push") {
				t.Error("push check run without a release")
			}
			if tt.name == "not a terminal" && !strings.Contains(stderr, "--yes") {
				t.Errorf("refusal does not name --yes: %s", stderr)
			}
		})
	}
}

func TestShip_ReleasesInDependencyOrderAndSkipsUnreleased(t *testing.T) {
	f := newShipFixture(t, shipMonorepoYAML)
	f.monorepoResults()
	code, stdout, stderr := f.run(t, "--yes")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	want := []string{
		"open", "compute", "check-push",
		"core: echo build core", "core: echo prepare core", "release core",
		"core: echo publish core", "core: echo publish-hook core", "core: echo success core",
		"web: echo build web", "release web", "web: echo publish web",
		"close",
	}
	if !slices.Equal(f.events, want) {
		t.Errorf("events\n %v\nwant\n %v", f.events, want)
	}
	if len(f.ws.requests) != 2 || len(f.ws.requests[1].Computed) != 3 || f.ws.requests[1].Expected.NewReleaseVersion != "2.1.0" {
		t.Errorf("release requests %+v", f.ws.requests)
	}
	for _, s := range []string{"core 1.1.0", "web 2.1.0", "cli: no new release"} {
		if !strings.Contains(stdout, s) {
			t.Errorf("summary lacks %q:\n%s", s, stdout)
		}
	}
	for _, s := range []string{"core 1.1.0 (default channel, tag core-v1.1.0)", "last release 1.0.0", "build", "echo publish core"} {
		if !strings.Contains(stderr, s) {
			t.Errorf("plan listing lacks %q:\n%s", s, stderr)
		}
	}
}

func TestShip_StepsGetTheReleaseVariables(t *testing.T) {
	f := newShipFixture(t, shipMonorepoYAML)
	f.monorepoResults()
	f.ws.results[0].NewReleaseChannel = "alpha"
	if code, _, stderr := f.run(t, "--yes"); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	build, publish := f.vars[0], f.vars[2]
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
	if publish.GitHead != "release-commit-core" {
		t.Errorf("publish runs on %q, want the released commit", publish.GitHead)
	}
}

func TestShip_NothingToRelease(t *testing.T) {
	f := newShipFixture(t, shipMonorepoYAML)
	f.ws.results = []semrel.ProjectResult{
		{Project: "core", Branch: "main"}, {Project: "cli", Branch: "main"}, {Project: "web", Branch: "main"},
	}
	code, stdout, _ := f.run(t)
	if code != 0 || !strings.Contains(stdout, "nothing to release") || len(f.released()) != 0 {
		t.Errorf("exit %d, stdout %q, released %v", code, stdout, f.released())
	}
}

func TestShip_DryRunNeverReleasesOrPublishes(t *testing.T) {
	f := newShipFixture(t, shipMonorepoYAML)
	f.monorepoResults()
	code, stdout, stderr := f.run(t, "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	want := []string{"open", "compute", "core: echo build core", "core: echo prepare core", "web: echo build web", "close"}
	if !slices.Equal(f.events, want) {
		t.Errorf("events %v, want %v", f.events, want)
	}
	if f.tokens != 0 {
		t.Error("a GitHub token was read for a dry run")
	}
	for _, v := range f.vars {
		if !v.DryRun {
			t.Errorf("vars %+v: want DryRun", v)
		}
	}
	if !strings.Contains(stdout, "dry run") || !strings.Contains(stderr, "publish (not run: dry run)") {
		t.Errorf("stdout %q\nstderr %q", stdout, stderr)
	}
}

func TestShip_DotnetProjectFileGetsTheVersionAndReleaseLinkBeforeThePipeline(t *testing.T) {
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
			f := newShipFixture(t, shipDotnetYAML)
			f.ws.public = tt.public
			f.ws.results = []semrel.ProjectResult{shipResult("api", "1.1.0", "v1.1.0")}
			path := filepath.Join(f.ws.dir, "src", "Api", "Api.csproj")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(apiCsproj), 0o644); err != nil {
				t.Fatal(err)
			}
			var atFirstStep string
			f.onFirstStep = func() { data, _ := os.ReadFile(path); atFirstStep = string(data) }
			code, _, stderr := f.run(t, "--dry-run")
			if code != 0 {
				t.Fatalf("exit %d: %s", code, stderr)
			}
			if !strings.Contains(atFirstStep, "<Version>1.1.0</Version>") || !strings.Contains(atFirstStep, "<PackageReleaseNotes>"+tt.wantNotes+"</PackageReleaseNotes>") {
				t.Errorf("csproj at the first step:\n%s", atFirstStep)
			}
			if got := strings.Contains(stderr, "warning"); got != tt.warn {
				t.Errorf("warning %v, want %v:\n%s", got, tt.warn, stderr)
			}
		})
	}
}

func TestShip_VersionMismatchStopsBeforePublishing(t *testing.T) {
	f := newShipFixture(t, shipMonorepoYAML)
	f.monorepoResults()
	f.ws.release = func(_ context.Context, req semrel.ReleaseRequest) (semrel.ProjectResult, error) {
		return semrel.ProjectResult{}, fmt.Errorf("project core: %w: computed 1.1.0, semantic-release released nothing", semrel.ErrVersionMismatch)
	}
	code, stdout, stderr := f.run(t, "--yes")
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if slices.Contains(f.events, "core: echo publish core") || slices.Contains(f.events, "release web") {
		t.Errorf("published or went on after the mismatch: %v", f.events)
	}
	if !slices.Contains(f.events, "core: echo fail core") || !slices.Contains(f.events, "tag? core-v1.1.0") {
		t.Errorf("fail hooks or tag check missing: %v", f.events)
	}
	for _, s := range []string{"nothing was tagged", "not started", "stopped before the tag"} {
		if !strings.Contains(stdout+stderr, s) {
			t.Errorf("report lacks %q:\nstdout %s\nstderr %s", s, stdout, stderr)
		}
	}
}

func TestShip_FailureAfterTheTagRunsFailHooksAndSaysWhatWasPublished(t *testing.T) {
	f := newShipFixture(t, shipMonorepoYAML)
	f.monorepoResults()
	f.failStep = map[string]error{"core: echo publish-hook core": errors.New("exit code 1")}
	code, stdout, stderr := f.run(t, "--yes", "--json")
	if code != 2 {
		t.Errorf("exit %d, want 2; stderr:\n%s", code, stderr)
	}
	failAt := indexOf(f.events, "core: echo publish-hook core")
	if failAt < 0 || indexOf(f.events, "core: echo fail core") < failAt || slices.Contains(f.events, "core: echo success core") || slices.Contains(f.events, "release web") {
		t.Errorf("events %v", f.events)
	}
	var doc shipDocument
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	outcomes := doc.Projects
	if doc.Status != "failed" || doc.Error == "" {
		t.Errorf("document status %q error %q", doc.Status, doc.Error)
	}
	byName := map[string]shipOutcome{}
	for _, o := range outcomes {
		byName[o.Project] = o
	}
	core := byName["core"]
	if core.Status != "failed" || !core.Tagged || core.Tag != "core-v1.1.0" || !slices.Equal(core.Published, []string{"publish"}) || core.Error == "" || !strings.Contains(core.Action, "core-v1.1.0") {
		t.Errorf("core outcome %+v", core)
	}
	if byName["web"].Status != "not-started" || byName["cli"].Status != "no-release" {
		t.Errorf("outcomes %+v", outcomes)
	}
	if !strings.Contains(stderr, "do not run nerv ship again") {
		t.Errorf("stderr lacks the advice:\n%s", stderr)
	}
}

func TestShip_ReleaseFailureReportsWhetherTheTagExists(t *testing.T) {
	for _, tagged := range []bool{false, true} {
		t.Run(fmt.Sprint("tagged ", tagged), func(t *testing.T) {
			f := newShipFixture(t, shipMonorepoYAML)
			f.monorepoResults()
			f.ws.release = func(context.Context, semrel.ReleaseRequest) (semrel.ProjectResult, error) {
				f.ws.tags["core-v1.1.0"] = tagged
				return semrel.ProjectResult{}, errors.New("semantic-release: EGITHUB boom")
			}
			code, stdout, _ := f.run(t, "--yes", "--json")
			var doc shipDocument
			if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
				t.Fatal(err)
			}
			core := doc.Projects[0]
			if code != 2 || core.Tagged != tagged || core.Status != "failed" || slices.Contains(f.events, "core: echo publish core") {
				t.Errorf("exit %d, core %+v, events %v", code, core, f.events)
			}
			if !slices.Contains(f.events, "core: echo fail core") {
				t.Errorf("fail hooks not run: %v", f.events)
			}
		})
	}
}

func TestShip_PipelineFailureStopsBeforeTheRelease(t *testing.T) {
	f := newShipFixture(t, shipMonorepoYAML)
	f.monorepoResults()
	f.failStep = map[string]error{"core: echo build core": errors.New("exit code 1")}
	code, stdout, _ := f.run(t, "--yes")
	if code != 2 || len(f.released()) != 0 || slices.Contains(f.events, "core: echo fail core") {
		t.Errorf("exit %d, events %v", code, f.events)
	}
	if !strings.Contains(strings.ToLower(stdout), "nothing was tagged") {
		t.Errorf("summary:\n%s", stdout)
	}
}

func TestShip_GitHubTokenOnlyWhenTheEnvironmentHasNone(t *testing.T) {
	f := newShipFixture(t, shipMonorepoYAML)
	f.monorepoResults()
	if code, _, stderr := f.run(t, "--yes"); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if f.tokens != 1 || !slices.Contains(f.ws.requests[0].Env, "GH_TOKEN=gh-tok") {
		t.Errorf("tokens %d, env %v", f.tokens, f.ws.requests[0].Env)
	}
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		f = newShipFixture(t, shipMonorepoYAML)
		f.monorepoResults()
		f.env[name] = "from-env"
		if code, _, _ := f.run(t, "--yes"); code != 0 || f.tokens != 0 || len(f.ws.requests[0].Env) != 0 {
			t.Errorf("%s set: tokens %d, env %v", name, f.tokens, f.ws.requests[0].Env)
		}
	}
}

func TestShip_PushDeniedStopsBeforeThePipeline(t *testing.T) {
	f := newShipFixture(t, shipMonorepoYAML)
	f.monorepoResults()
	f.ws.pushErr = fmt.Errorf("%w (https://github.com/acme/widgets.git) with your credentials", semrel.ErrPushDenied)
	code, _, stderr := f.run(t, "--yes")
	if code != 2 || slices.Contains(f.events, "core: echo build core") || !strings.Contains(stderr, "credentials") {
		t.Errorf("exit %d, events %v, stderr %s", code, f.events, stderr)
	}
}

func TestShip_InterruptedExitsWith128PlusTheSignal(t *testing.T) {
	f := newShipFixture(t, shipMonorepoYAML)
	f.monorepoResults()
	f.signal = syscall.SIGINT
	f.ws.release = func(ctx context.Context, req semrel.ReleaseRequest) (semrel.ProjectResult, error) {
		f.cancel()
		return semrel.ProjectResult{}, ctx.Err()
	}
	code, _, stderr := f.run(t, "--yes")
	if code != 130 || !strings.Contains(stderr, "interrupted") {
		t.Errorf("exit %d, stderr %s", code, stderr)
	}
	if slices.Contains(f.events, "core: echo fail core") {
		t.Errorf("fail hooks run after an interrupt: %v", f.events)
	}
	if !f.ws.closed {
		t.Error("release clone not removed")
	}
}

func TestShip_ProjectKeepsDependencyResultsForDependsOn(t *testing.T) {
	f := newShipFixture(t, shipMonorepoYAML)
	f.monorepoResults()
	f.ws.release = func(_ context.Context, req semrel.ReleaseRequest) (semrel.ProjectResult, error) {
		hasCore := slices.ContainsFunc(req.Computed, func(r semrel.ProjectResult) bool {
			return r.Project == "core" && r.NewReleasePublished
		})
		if !hasCore {
			return semrel.ProjectResult{}, fmt.Errorf("%w: computed results lack core: %+v", semrel.ErrVersionMismatch, req.Computed)
		}
		return req.Expected, nil
	}
	code, _, stderr := f.run(t, "--yes", "--project", "web")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if got := f.released(); !slices.Equal(got, []string{"web"}) {
		t.Errorf("released %v, want only web", got)
	}
}

func TestShip_ReleasedAnotherVersionSaysWhichTagExists(t *testing.T) {
	f := newShipFixture(t, shipMonorepoYAML)
	f.monorepoResults()
	f.ws.originNow = strings.Repeat("9", 40)
	f.ws.release = func(context.Context, semrel.ReleaseRequest) (semrel.ProjectResult, error) {
		f.ws.tags["core-v1.2.0"] = true
		r := shipResult("core", "1.2.0", "core-v1.2.0")
		return r, fmt.Errorf("project core: %w: computed 1.1.0, semantic-release released 1.2.0 (tag core-v1.2.0)", semrel.ErrVersionMismatch)
	}
	code, stdout, _ := f.run(t, "--yes", "--json")
	var doc shipDocument
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatal(err)
	}
	core := doc.Projects[0]
	if code != 2 || !core.Tagged || slices.Contains(f.events, "core: echo publish core") {
		t.Errorf("exit %d, core %+v, events %v", code, core, f.events)
	}
	for _, want := range []string{"released 1.2.0", "core-v1.2.0", "instead of the confirmed 1.1.0", "publish stage did not run", "do not run nerv ship again"} {
		if !strings.Contains(core.Action, want) {
			t.Errorf("action lacks %q: %s", want, core.Action)
		}
	}
}

func TestShip_RerunAdviceSaysPullWhenOriginMoved(t *testing.T) {
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
			f := newShipFixture(t, shipMonorepoYAML)
			f.monorepoResults()
			f.ws.originNow = tt.originNow
			// core is released, then web fails in its pipeline.
			f.failStep = map[string]error{"web: echo build web": errors.New("exit code 1")}
			code, stdout, _ := f.run(t, "--yes", "--json")
			var doc shipDocument
			if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
				t.Fatal(err)
			}
			web := doc.Projects[2]
			if code != 2 || web.Status != "failed" || doc.Projects[0].Status != "released" {
				t.Fatalf("exit %d, %+v", code, doc)
			}
			if got := strings.Contains(web.Action, "git pull"); got != tt.wantPull {
				t.Errorf("pull advice %v, want %v: %s", got, tt.wantPull, web.Action)
			}
			if !strings.Contains(web.Action, "run nerv ship again") {
				t.Errorf("action %s", web.Action)
			}
		})
	}
}

func TestShip_JSONDocumentOnEveryPath(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(f *shipFixture)
		args       []string
		wantCode   int
		wantStatus string
		projects   int
	}{
		{"released", func(f *shipFixture) {}, []string{"--yes"}, 0, "released", 3},
		{"dry run", func(f *shipFixture) {}, []string{"--dry-run"}, 0, "dry-run", 3},
		{"nothing to release", func(f *shipFixture) { f.ws.results = []semrel.ProjectResult{{Project: "core"}} }, []string{"--yes"}, 0, "nothing-to-release", 1},
		{"refused: behind origin", func(f *shipFixture) { f.ws.head = strings.Repeat("2", 40) }, []string{"--yes"}, 1, "refused", 0},
		{"refused: no --yes", func(f *shipFixture) {}, nil, 1, "refused", 0},
		{"declined", func(f *shipFixture) { f.tty, f.stdin = true, "n\n" }, nil, 1, "declined", 0},
		{"failed: compute", func(f *shipFixture) { f.ws.computeErr = errors.New("node missing") }, []string{"--yes"}, 2, "failed", 0},
		{"interrupted", func(f *shipFixture) {
			f.signal = syscall.SIGINT
			f.ws.release = func(ctx context.Context, _ semrel.ReleaseRequest) (semrel.ProjectResult, error) {
				f.cancel()
				return semrel.ProjectResult{}, ctx.Err()
			}
		}, []string{"--yes"}, 130, "interrupted", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newShipFixture(t, shipMonorepoYAML)
			f.monorepoResults()
			tt.setup(f)
			code, stdout, stderr := f.run(t, append(tt.args, "--json")...)
			var doc shipDocument
			if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
				t.Fatalf("stdout is not one JSON document: %v\n%q\nstderr %s", err, stdout, stderr)
			}
			if code != tt.wantCode || doc.Status != tt.wantStatus || len(doc.Projects) != tt.projects || doc.Projects == nil {
				t.Errorf("exit %d, document %+v; want exit %d, status %s, %d projects", code, doc, tt.wantCode, tt.wantStatus, tt.projects)
			}
			if (code != 0) != (doc.Error != "") {
				t.Errorf("error %q with exit %d", doc.Error, code)
			}
		})
	}
}

func TestShip_RerunAdviceWhenOriginCannotBeReadOrTheBranchIsGone(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(w *fakeWorkspace)
		want    []string
		notWant []string
	}{
		{"unreadable", func(w *fakeWorkspace) { w.originErr = errors.New("ssh: connect timed out") }, []string{"git fetch", "run nerv ship again"}, nil},
		{"deleted", func(w *fakeWorkspace) { w.originGone = true }, []string{"deleted on origin"}, []string{"git pull"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newShipFixture(t, shipMonorepoYAML)
			f.monorepoResults()
			tt.setup(f.ws)
			f.failStep = map[string]error{"core: echo build core": errors.New("exit code 1")}
			code, stdout, _ := f.run(t, "--yes", "--json")
			var doc shipDocument
			if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
				t.Fatal(err)
			}
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

func TestShip_InterruptedRunStillReadsOriginForTheAdvice(t *testing.T) {
	f := newShipFixture(t, shipMonorepoYAML)
	f.monorepoResults()
	f.signal = syscall.SIGINT
	f.ws.originNow = strings.Repeat("9", 40)
	f.onFirstStep = func() { f.cancel() }
	f.failStep = map[string]error{"core: echo build core": context.Canceled}
	code, stdout, _ := f.run(t, "--yes", "--json")
	var doc shipDocument
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatal(err)
	}
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

func TestShip_ExpectBindsTheReleaseToTheApprovedVersions(t *testing.T) {
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
			f := newShipFixture(t, shipMonorepoYAML)
			f.monorepoResults()
			args := []string{"--yes", "--json"}
			for _, e := range tt.expect {
				args = append(args, "--expect", e)
			}
			code, stdout, stderr := f.run(t, args...)
			var doc shipDocument
			if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
				t.Fatalf("stdout: %v\n%s", err, stdout)
			}
			if code != tt.wantCode {
				t.Fatalf("exit %d, want %d; stderr:\n%s", code, tt.wantCode, stderr)
			}
			if tt.wantCode == 0 {
				if doc.Status != "released" || len(f.released()) != 2 {
					t.Errorf("status %s, released %v", doc.Status, f.released())
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

func TestShip_ExpectSyntaxIsAUsageError(t *testing.T) {
	for _, bad := range [][]string{
		{"--expect", "core"},
		{"--expect", "=1.0.0"},
		{"--expect", "core="},
		{"--expect", "core=1.0 .0"},
		{"--expect", "Core Lib=1.0.0"},
		{"--expect", "core=1.0.0", "--expect", "core=1.0.1"},
	} {
		f := newShipFixture(t, shipMonorepoYAML)
		f.monorepoResults()
		code, _, stderr := f.run(t, append([]string{"--yes"}, bad...)...)
		if code != 2 || !strings.Contains(stderr, "--expect") || len(f.events) != 0 {
			t.Errorf("%v: exit %d, events %v, stderr %q", bad, code, f.events, stderr)
		}
	}
}
