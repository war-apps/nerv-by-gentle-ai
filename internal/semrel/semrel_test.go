package semrel

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// fakeRunner answers commands through handle and records every call.
type fakeRunner struct {
	mu     sync.Mutex
	calls  []Command
	handle func(c Command) (Result, error)
}

func (f *fakeRunner) Run(_ context.Context, c Command) (Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	return f.handle(c)
}

func (f *fakeRunner) find(pred func(Command) bool) []Command {
	var out []Command
	for _, c := range f.calls {
		if pred(c) {
			out = append(out, c)
		}
	}
	return out
}

func isGit(c Command, sub ...string) bool {
	if c.Name != "git" {
		return false
	}
	args := c.Args
	for len(args) >= 2 && args[0] == "-C" {
		args = args[2:]
	}
	return len(args) >= len(sub) && slices.Equal(args[:len(sub)], sub)
}

func isNodeScript(c Command) bool { return c.Name == "node" && len(c.Stdin) > 0 }

func decodeRequest(t *testing.T, c Command) runnerRequest {
	t.Helper()
	var req runnerRequest
	if err := json.Unmarshal(c.Stdin, &req); err != nil {
		t.Fatalf("runner request: %v", err)
	}
	return req
}

func respond(v any) (Result, error) {
	data, _ := json.Marshal(v)
	return Result{Stdout: []byte("some log line\n" + resultMarker + string(data) + "\n")}, nil
}

// fakeRepo answers git, node --version and npm like a healthy machine; node
// runner requests go to run.
func fakeRepo(t *testing.T, root string, run func(req runnerRequest) (Result, error)) *fakeRunner {
	t.Helper()
	return &fakeRunner{handle: func(c Command) (Result, error) {
		switch {
		case isGit(c, "rev-parse", "--show-toplevel"):
			return Result{Stdout: []byte(root + "\n")}, nil
		case isGit(c, "rev-parse", "--verify"):
			return Result{Stdout: []byte("1111111111111111111111111111111111111111\n")}, nil
		case isGit(c, "symbolic-ref", "--short", "HEAD"):
			return Result{Stdout: []byte("main\n")}, nil
		case isGit(c, "remote", "get-url", "origin"):
			return Result{Stdout: []byte("https://github.com/example/repo.git\n")}, nil
		case isGit(c, "--version"):
			return Result{Stdout: []byte("git version 2.43.0\n")}, nil
		case c.Name == "git":
			return Result{}, nil
		case c.Name == "node" && slices.Equal(c.Args, []string{"--version"}):
			return Result{Stdout: []byte("v24.21.0\n")}, nil
		case c.Name == "npm":
			return Result{}, nil
		case isNodeScript(c):
			var req runnerRequest
			if err := json.Unmarshal(c.Stdin, &req); err != nil {
				return Result{}, err
			}
			return run(req)
		}
		return Result{}, errors.New("unexpected command " + c.Name + " " + strings.Join(c.Args, " "))
	}}
}

func writeConfig(t *testing.T, root, yaml string) {
	t.Helper()
	dir := filepath.Join(root, ".nerv")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nerv.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func okPlan() (Result, error) {
	return respond(map[string]any{"ok": true, "packages": []string{"@semantic-release/commit-analyzer", "@semantic-release/release-notes-generator"}})
}

func TestCacheKey(t *testing.T) {
	a := cacheKey([]string{"semantic-release@24.2.3", "@semantic-release/git@10.0.1"})
	b := cacheKey([]string{"@semantic-release/git@10.0.1", "semantic-release@24.2.3"})
	c := cacheKey([]string{"semantic-release@24.2.4", "@semantic-release/git@10.0.1"})
	if a != b {
		t.Errorf("key depends on spec order: %s != %s", a, b)
	}
	if a == c {
		t.Errorf("key ignores the semantic-release version")
	}
	if len(a) != 16 {
		t.Errorf("key %q: want 16 hex characters", a)
	}
}

func TestPinnedSpecs(t *testing.T) {
	got := pinnedSpecs("24.2.3", []string{"@semantic-release/exec@6.0.3", "semantic-release-slack-bot@4.0.2"})
	want := []string{
		"@semantic-release/changelog@" + ChangelogVersion,
		"@semantic-release/exec@6.0.3",
		"@semantic-release/git@" + GitVersion,
		"conventional-changelog-conventionalcommits@" + ConventionalCommitsVersion,
		"semantic-release-slack-bot@4.0.2",
		"semantic-release@24.2.3",
	}
	if !slices.Equal(got, want) {
		t.Errorf("pinnedSpecs:\n got %v\nwant %v", got, want)
	}
}

func TestEnsureCache_InstallsOnceAndReuses(t *testing.T) {
	base := t.TempDir()
	specs := pinnedSpecs("24.2.3", nil)
	f := &fakeRunner{handle: func(c Command) (Result, error) { return Result{}, nil }}
	e := &Engine{Runner: f}

	dir, err := e.ensureCache(context.Background(), base, specs)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(base, cacheKey(specs)); dir != want {
		t.Errorf("cache dir %s, want %s", dir, want)
	}
	installs := f.find(func(c Command) bool { return c.Name == "npm" })
	if len(installs) != 1 {
		t.Fatalf("npm calls %d, want 1", len(installs))
	}
	args := installs[0].Args
	if args[0] != "install" || !slices.Contains(args, "--no-audit") || !slices.Contains(args, "--no-fund") || !slices.Contains(args, "--prefix") {
		t.Errorf("npm args %v", args)
	}
	for _, s := range specs {
		if !slices.Contains(args, s) {
			t.Errorf("npm args miss %s", s)
		}
	}
	prefix := args[slices.Index(args, "--prefix")+1]
	if prefix == dir {
		t.Errorf("npm installed straight into the final dir; want a temporary dir renamed into place")
	}
	if _, err := os.Stat(filepath.Join(dir, readyMarker)); err != nil {
		t.Errorf("ready marker: %v", err)
	}

	again, err := e.ensureCache(context.Background(), base, specs)
	if err != nil || again != dir {
		t.Fatalf("second ensureCache = %s, %v", again, err)
	}
	if n := len(f.find(func(c Command) bool { return c.Name == "npm" })); n != 1 {
		t.Errorf("npm ran %d times, want once", n)
	}
}

func TestEnsureCache_FailedInstallLeavesNothing(t *testing.T) {
	base := t.TempDir()
	f := &fakeRunner{handle: func(c Command) (Result, error) {
		return Result{Stderr: []byte("npm ERR! 404")}, errors.New("exit status 1")
	}}
	e := &Engine{Runner: f}
	if _, err := e.ensureCache(context.Background(), base, []string{"semantic-release@24.2.3"}); err == nil {
		t.Fatal("want an error")
	}
	entries, _ := os.ReadDir(base)
	if len(entries) != 0 {
		t.Errorf("cache base not empty after a failed install: %v", entries)
	}
}

func TestCheckPins(t *testing.T) {
	specs := pinnedSpecs("24.2.3", []string{"semantic-release-slack-bot@4.0.2"})
	ok := []string{
		"@semantic-release/commit-analyzer", "@semantic-release/release-notes-generator",
		"@semantic-release/npm", "@semantic-release/github", "conventional-changelog-angular",
		"@semantic-release/changelog", "@semantic-release/git", "@semantic-release/exec",
		"conventional-changelog-conventionalcommits", "semantic-release-slack-bot",
	}
	if err := checkPins(ok, nil, specs); err != nil {
		t.Errorf("all pinned: %v", err)
	}
	err := checkPins(append(ok, "@acme/release-config", "conventional-changelog-eslint"), nil, specs)
	if err == nil {
		t.Fatal("unpinned packages accepted")
	}
	for _, want := range []string{"@acme/release-config", "conventional-changelog-eslint", "release.semantic_release.extra_plugins"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
	if err := checkPins(ok, []string{"./local-config.js"}, specs); err == nil || !strings.Contains(err.Error(), "./local-config.js") {
		t.Errorf("unresolved shareable config: %v", err)
	}
}

func TestPlanProjects(t *testing.T) {
	single := &shipconfig.Release{Projects: []shipconfig.Project{{Name: "api", Path: ".", Scopes: []string{"api"}, TagFormat: shipconfig.DefaultTagFormat}}}
	singleTagged := &shipconfig.Release{Projects: []shipconfig.Project{{Name: "api", Path: "src", Scopes: []string{"api"}, TagFormat: "api@${version}"}}}
	multi := &shipconfig.Release{Projects: []shipconfig.Project{
		{Name: "web", Path: "web", Scopes: []string{"web"}, TagFormat: "web-v${version}", DependsOn: []shipconfig.Dependency{{Project: "api", Bump: "patch"}}},
		{Name: "api", Path: "api", SharedPaths: []string{"libs/**"}, Scopes: []string{"api", "core"}, TagFormat: "api-v${version}"},
		{Name: "docs", Path: "docs", Scopes: []string{"docs"}, TagFormat: "docs-v${version}"},
	}}

	tests := []struct {
		name    string
		rel     *shipconfig.Release
		only    string
		want    []project
		wantErr string
	}{
		{name: "no release section", rel: nil, want: []project{{Name: "repo"}}},
		{name: "single project keeps the repo's tagFormat and sees every commit", rel: single, want: []project{{Name: "api"}}},
		{name: "single project with its own tag and path", rel: singleTagged, want: []project{{Name: "api", TagFormat: "api@${version}", Filter: &commitFilter{Paths: []string{"src"}, Scopes: []string{"api"}}}}},
		{name: "monorepo in dependency order", rel: multi, want: []project{
			{Name: "api", TagFormat: "api-v${version}", Filter: &commitFilter{Paths: []string{"api", "libs/**"}, Scopes: []string{"api", "core"}}},
			{Name: "web", TagFormat: "web-v${version}", Filter: &commitFilter{Paths: []string{"web"}, Scopes: []string{"web"}}, DependsOn: []shipconfig.Dependency{{Project: "api", Bump: "patch"}}},
			{Name: "docs", TagFormat: "docs-v${version}", Filter: &commitFilter{Paths: []string{"docs"}, Scopes: []string{"docs"}}},
		}},
		{name: "one project brings its dependencies", rel: multi, only: "web", want: []project{
			{Name: "api", TagFormat: "api-v${version}", Filter: &commitFilter{Paths: []string{"api", "libs/**"}, Scopes: []string{"api", "core"}}},
			{Name: "web", TagFormat: "web-v${version}", Filter: &commitFilter{Paths: []string{"web"}, Scopes: []string{"web"}}, DependsOn: []shipconfig.Dependency{{Project: "api", Bump: "patch"}}},
		}},
		{name: "unknown project", rel: multi, only: "mobile", wantErr: `unknown project "mobile"`},
		{name: "project without release section", rel: nil, only: "api", wantErr: "no release: section"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := planProjects(tt.rel, "repo", tt.only)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("planProjects:\n got %s\nwant %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestMinReleaseType(t *testing.T) {
	released := map[string]ProjectResult{
		"api":  {Project: "api", NewReleasePublished: true},
		"lib":  {Project: "lib", NewReleasePublished: true},
		"idle": {Project: "idle"},
	}
	tests := []struct {
		name string
		deps []shipconfig.Dependency
		want string
	}{
		{"no dependencies", nil, ""},
		{"released dependency patch", []shipconfig.Dependency{{Project: "api", Bump: "patch"}}, "patch"},
		{"highest bump wins", []shipconfig.Dependency{{Project: "api", Bump: "patch"}, {Project: "lib", Bump: "minor"}}, "minor"},
		{"none ignores the release", []shipconfig.Dependency{{Project: "api", Bump: "none"}}, ""},
		{"unreleased dependency", []shipconfig.Dependency{{Project: "idle", Bump: "minor"}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := minReleaseType(tt.deps, released); got != tt.want {
				t.Errorf("minReleaseType = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	last := &release{Version: "2.8.1", GitHead: "aaa", GitTag: "v2.8.1"}
	tests := []struct {
		name string
		resp runnerResponse
		want ProjectResult
	}{
		{
			name: "prerelease on a channel",
			resp: runnerResponse{OK: true, Released: true, LastRelease: last, NextRelease: &release{Type: "minor", Version: "2.9.0-alpha.10", Channel: "alpha", GitHead: "bbb", GitTag: "v2.9.0-alpha.10", Notes: "## notes"}},
			want: ProjectResult{
				Project: "api", Branch: "develop", NewReleasePublished: true,
				NewReleaseVersion: "2.9.0-alpha.10", NewReleaseMajorVersion: "2", NewReleaseMinorVersion: "9", NewReleasePatchVersion: "0",
				NewReleaseChannel: "alpha", NewReleaseNotes: "## notes", NewReleaseGitHead: "bbb", NewReleaseGitTag: "v2.9.0-alpha.10",
				LastReleaseVersion: "2.8.1", LastReleaseGitHead: "aaa", LastReleaseGitTag: "v2.8.1",
			},
		},
		{
			name: "default channel",
			resp: runnerResponse{OK: true, Released: true, NextRelease: &release{Version: "1.0.0", GitHead: "ccc", GitTag: "v1.0.0"}},
			want: ProjectResult{
				Project: "api", Branch: "develop", NewReleasePublished: true,
				NewReleaseVersion: "1.0.0", NewReleaseMajorVersion: "1", NewReleaseMinorVersion: "0", NewReleasePatchVersion: "0",
				NewReleaseGitHead: "ccc", NewReleaseGitTag: "v1.0.0",
			},
		},
		{
			name: "nothing to release keeps the last release",
			resp: runnerResponse{OK: true, LastRelease: last},
			want: ProjectResult{Project: "api", Branch: "develop", LastReleaseVersion: "2.8.1", LastReleaseGitHead: "aaa", LastReleaseGitTag: "v2.8.1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalize("api", "develop", tt.resp); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("normalize:\n got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestOutputs_AreTheTwelveActionOutputs(t *testing.T) {
	r := ProjectResult{NewReleasePublished: true, NewReleaseVersion: "1.2.3"}
	outs := r.Outputs()
	names := make([]string, len(outs))
	for i, o := range outs {
		names[i] = o.Name
	}
	want := []string{
		"new_release_published", "new_release_version", "new_release_major_version", "new_release_minor_version",
		"new_release_patch_version", "new_release_channel", "new_release_notes", "new_release_git_head",
		"new_release_git_tag", "last_release_version", "last_release_git_head", "last_release_git_tag",
	}
	if !slices.Equal(names, want) {
		t.Errorf("outputs %v", names)
	}
	if outs[0].Value != "true" || outs[1].Value != "1.2.3" {
		t.Errorf("values %v", outs[:2])
	}
	data, _ := json.Marshal(r)
	for _, n := range want {
		if !strings.Contains(string(data), `"`+n+`"`) {
			t.Errorf("json lacks %s: %s", n, data)
		}
	}
}

func TestNodeEnv_StripsCIDetection(t *testing.T) {
	got := nodeEnv([]string{"PATH=/bin", "CI=true", "GITHUB_ACTIONS=true", "GITLAB_CI=1", "GH_TOKEN=x", "HOME=/h"})
	want := []string{"PATH=/bin", "GH_TOKEN=x", "HOME=/h"}
	if !slices.Equal(got, want) {
		t.Errorf("nodeEnv = %v, want %v", got, want)
	}
}

func TestParseNodeVersion(t *testing.T) {
	tests := []struct {
		in   string
		ok   bool
		want bool
	}{
		{"v24.21.0\n", true, true},
		{"v20.8.1", true, true},
		{"v20.8.0", true, false},
		{"v18.19.0", true, false},
		{"garbage", false, false},
	}
	for _, tt := range tests {
		ok, err := nodeSupported(tt.in)
		if (err == nil) != tt.ok || ok != tt.want {
			t.Errorf("nodeSupported(%q) = %v, %v", tt.in, ok, err)
		}
	}
}

func TestCompute_DisposableCheckoutRemovedOnError(t *testing.T) {
	root := t.TempDir()
	f := fakeRepo(t, root, func(req runnerRequest) (Result, error) {
		return respond(map[string]any{"ok": false, "error": "EINVALIDBRANCH boom"})
	})
	e := &Engine{Runner: f}
	_, err := e.Compute(context.Background(), Options{RepoRoot: root, Branch: "develop", CacheDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "EINVALIDBRANCH boom") {
		t.Fatalf("err = %v", err)
	}
	clones := f.find(func(c Command) bool { return isGit(c, "clone") })
	if len(clones) != 1 {
		t.Fatalf("clone calls %d", len(clones))
	}
	args := clones[0].Args
	mirror := args[len(args)-1]
	if !slices.Contains(args, "--mirror") || !slices.Contains(args, "--shared") || args[len(args)-2] != root {
		t.Errorf("clone args %v: want a shared mirror of %s", args, root)
	}
	fetches := f.find(func(c Command) bool { return isGit(c, "fetch") && slices.Contains(c.Args, mirror) })
	if len(fetches) != 1 || !slices.Contains(fetches[0].Args, "+refs/notes/*:refs/notes/*") || !slices.Contains(fetches[0].Args, "+refs/tags/*:refs/tags/*") ||
		fetchURL(fetches[0]) != "https://github.com/example/repo.git" || !slices.Contains(fetches[0].Env, "GIT_TERMINAL_PROMPT=0") {
		t.Errorf("origin tags and notes not fetched into the mirror: %v", fetches)
	}
	adds := f.find(func(c Command) bool { return isGit(c, "worktree", "add") })
	if len(adds) != 1 || adds[0].Args[1] != mirror || adds[0].Args[len(adds[0].Args)-1] != "develop" {
		t.Fatalf("worktree add %v: want one, from the mirror, on develop", adds)
	}
	for _, c := range f.calls {
		if c.Name == "git" && c.Args[1] == root && !isGit(c, "rev-parse") && !isGit(c, "symbolic-ref") && !isGit(c, "remote", "get-url") && !isGit(c, "config", "--get") && !isGit(c, "clone") {
			t.Errorf("command writes to the user's repository: %v", c)
		}
	}
	if _, err := os.Stat(filepath.Dir(mirror)); !os.IsNotExist(err) {
		t.Errorf("temporary checkout still exists: %v", err)
	}
}

func TestCompute_OfflineOrNoOriginSkipsTheFetch(t *testing.T) {
	for _, tt := range []struct {
		name     string
		offline  bool
		noOrigin bool
	}{{"offline", true, false}, {"no origin", false, true}} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			f := fakeRepo(t, root, func(req runnerRequest) (Result, error) {
				if req.Mode == "plan" {
					return okPlan()
				}
				return respond(map[string]any{"ok": true})
			})
			inner := f.handle
			f.handle = func(c Command) (Result, error) {
				if tt.noOrigin && isGit(c, "remote", "get-url", "origin") {
					return Result{Stderr: []byte("error: No such remote 'origin'")}, errors.New("exit status 2")
				}
				return inner(c)
			}
			_, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", Offline: tt.offline, CacheDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			if n := len(f.find(func(c Command) bool { return isGit(c, "fetch") })); n != 0 {
				t.Errorf("fetches %d, want none", n)
			}
		})
	}
}

func TestCompute_OriginFetchFailureIsAnError(t *testing.T) {
	root := t.TempDir()
	f := fakeRepo(t, root, func(req runnerRequest) (Result, error) { return okPlan() })
	inner := f.handle
	f.handle = func(c Command) (Result, error) {
		if isGit(c, "fetch") {
			return Result{Stderr: []byte("fatal: could not read Username")}, errors.New("exit status 128")
		}
		return inner(c)
	}
	_, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "offline") || !strings.Contains(err.Error(), "could not read Username") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompute_DefaultBranchIsCurrent(t *testing.T) {
	root := t.TempDir()
	f := fakeRepo(t, root, func(req runnerRequest) (Result, error) {
		if req.Mode == "plan" {
			return okPlan()
		}
		return respond(map[string]any{"ok": true, "released": false})
	})
	results, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Branch != "main" || results[0].Project != filepath.Base(root) {
		t.Errorf("results %+v", results)
	}
}

func TestCompute_MonorepoOrderFiltersAndDependsOn(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `release:
  projects:
    - name: web
      path: web
      depends_on: [{ project: api }]
    - name: api
      path: api
      scopes: [api, core]
`)
	var runs []runnerRequest
	f := fakeRepo(t, root, func(req runnerRequest) (Result, error) {
		if req.Mode == "plan" {
			return okPlan()
		}
		runs = append(runs, req)
		if req.TagFormat == "api-v${version}" {
			return respond(map[string]any{"ok": true, "released": true, "nextRelease": map[string]any{"version": "1.1.0", "gitTag": "api-v1.1.0"}})
		}
		return respond(map[string]any{"ok": true, "released": true, "nextRelease": map[string]any{"version": "0.3.1", "gitTag": "web-v0.3.1"}})
	})
	results, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs %d", len(runs))
	}
	if runs[0].TagFormat != "api-v${version}" || runs[1].TagFormat != "web-v${version}" {
		t.Errorf("order: %s then %s", runs[0].TagFormat, runs[1].TagFormat)
	}
	if runs[0].Changelog != "api/CHANGELOG.md" || runs[1].Changelog != "web/CHANGELOG.md" {
		t.Errorf("changelogs %q, %q: want each project's own", runs[0].Changelog, runs[1].Changelog)
	}
	if runs[0].MinReleaseType != "" || runs[1].MinReleaseType != "patch" {
		t.Errorf("min release types %q, %q", runs[0].MinReleaseType, runs[1].MinReleaseType)
	}
	if runs[0].Filter == nil || !slices.Equal(runs[0].Filter.Scopes, []string{"api", "core"}) || !slices.Equal(runs[0].Filter.Paths, []string{"api"}) {
		t.Errorf("api filter %+v", runs[0].Filter)
	}
	if filepath.Base(runs[0].RepositoryURL) != "repo.git" || filepath.Dir(runs[0].RepositoryURL) != filepath.Dir(runs[0].Cwd) {
		t.Errorf("repositoryUrl %q: want the disposable mirror next to the worktree %q", runs[0].RepositoryURL, runs[0].Cwd)
	}
	if runs[0].Cwd == root || runs[0].Cwd == "" {
		t.Errorf("run cwd %q: want the temporary worktree", runs[0].Cwd)
	}
	if len(results) != 2 || results[0].Project != "api" || results[1].Project != "web" || results[1].NewReleaseVersion != "0.3.1" {
		t.Errorf("results %+v", results)
	}
}

func TestCompute_OnlyRequestedProjectReturned(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `release:
  projects:
    - { name: api, path: api }
    - { name: web, path: web, depends_on: [{ project: api, bump: minor }] }
`)
	var mins []string
	f := fakeRepo(t, root, func(req runnerRequest) (Result, error) {
		if req.Mode == "plan" {
			return okPlan()
		}
		mins = append(mins, req.MinReleaseType)
		return respond(map[string]any{"ok": true, "released": true, "nextRelease": map[string]any{"version": "1.0.0"}})
	})
	results, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", Project: "web", CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Project != "web" {
		t.Errorf("results %+v", results)
	}
	if !slices.Equal(mins, []string{"", "minor"}) {
		t.Errorf("min release types %v", mins)
	}
}

func TestCompute_UnpinnedPluginStopsBeforeRunning(t *testing.T) {
	root := t.TempDir()
	runs := 0
	f := fakeRepo(t, root, func(req runnerRequest) (Result, error) {
		if req.Mode == "plan" {
			return respond(map[string]any{"ok": true, "packages": []string{"semantic-release-slack-bot"}})
		}
		runs++
		return respond(map[string]any{"ok": true})
	})
	_, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "semantic-release-slack-bot") {
		t.Fatalf("err = %v", err)
	}
	if runs != 0 {
		t.Errorf("semantic-release ran %d times despite an unpinned plugin", runs)
	}
	clones := f.find(func(c Command) bool { return isGit(c, "clone") })
	if len(clones) != 1 {
		t.Fatalf("clone calls %d", len(clones))
	}
	if tmp := filepath.Dir(clones[0].Args[len(clones[0].Args)-1]); dirExists(tmp) {
		t.Errorf("temporary checkout %s left behind", tmp)
	}
}

func TestCompute_NodeMissing(t *testing.T) {
	root := t.TempDir()
	f := &fakeRunner{handle: func(c Command) (Result, error) {
		if c.Name == "node" {
			return Result{}, exec.ErrNotFound
		}
		if isGit(c, "rev-parse", "--show-toplevel") {
			return Result{Stdout: []byte(root)}, nil
		}
		return Result{}, nil
	}}
	_, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "Node.js") {
		t.Fatalf("err = %v", err)
	}
}

// A rev-parse that succeeds but prints no commit is still "not a local
// branch", reported without a fake wrapped cause.
func TestCompute_EmptyBranchCommitIsNotALocalBranch(t *testing.T) {
	root := t.TempDir()
	f := fakeRepo(t, root, func(runnerRequest) (Result, error) {
		t.Fatal("the runner must not start")
		return Result{}, nil
	})
	healthy := f.handle
	f.handle = func(c Command) (Result, error) {
		if isGit(c, "rev-parse", "--verify") {
			return Result{}, nil
		}
		return healthy(c)
	}
	_, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), `branch "main": not a local branch`) || strings.Contains(err.Error(), "%!w") {
		t.Fatalf("err = %v", err)
	}
}

func dirExists(dir string) bool {
	st, err := os.Stat(dir)
	return err == nil && st.IsDir()
}

func TestParseResponse(t *testing.T) {
	if _, err := parseResponse([]byte("no marker here"), nil); err == nil {
		t.Error("missing marker accepted")
	}
	resp, err := parseResponse([]byte(resultMarker+`{"ok":false}`+"\nnoise\n"+resultMarker+`{"ok":true,"released":true}`+"\n"), nil)
	if err != nil || !resp.Released {
		t.Errorf("last marker line: %+v, %v", resp, err)
	}
}
