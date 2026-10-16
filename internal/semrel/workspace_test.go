package semrel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

func TestReleaseURL(t *testing.T) {
	tests := []struct {
		name, public, tag, want string
	}{
		{"https with .git", "https://github.com/acme/widgets.git", "v1.2.0", "https://github.com/acme/widgets/releases/tag/v1.2.0"},
		{"https without .git", "https://github.com/acme/widgets", "api-v1.0.0", "https://github.com/acme/widgets/releases/tag/api-v1.0.0"},
		{"scp-like ssh", "git@github.com:acme/widgets.git", "v2.0.0-alpha.1", "https://github.com/acme/widgets/releases/tag/v2.0.0-alpha.1"},
		{"ssh URL with port", "ssh://github.com:22/acme/widgets.git", "v1.0.0", "https://github.com/acme/widgets/releases/tag/v1.0.0"},
		{"http keeps its scheme and port", "http://git.local:3000/acme/widgets", "v1.0.0", "http://git.local:3000/acme/widgets/releases/tag/v1.0.0"},
		{"tag with a slash", "https://github.com/acme/widgets", "web/v1.0.0", "https://github.com/acme/widgets/releases/tag/web/v1.0.0"},
		{"tag escaped", "https://github.com/acme/widgets", "v1.0.0+build 1", "https://github.com/acme/widgets/releases/tag/v1.0.0+build%201"},
		{"no origin", "", "v1.0.0", ""},
		{"no tag", "https://github.com/acme/widgets", "", ""},
		{"no repository path", "https://github.com/", "v1.0.0", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReleaseURL(tt.public, tt.tag); got != tt.want {
				t.Errorf("ReleaseURL(%q, %q) = %q, want %q", tt.public, tt.tag, got, tt.want)
			}
		})
	}
}

func TestPushAddress(t *testing.T) {
	tests := []struct {
		name, raw, resolved, want string
		wantErr                   bool
	}{
		{"public https", "https://github.com/acme/w.git", "https://github.com/acme/w.git", "https://github.com/acme/w.git", false},
		{"credentials dropped", "https://bot:s3cr3t@github.com/acme/w.git", "https://bot:s3cr3t@github.com/acme/w.git", "https://github.com/acme/w.git", false},
		{"scp-like", "git@github.com:acme/w.git", "git@github.com:acme/w.git", "git@github.com:acme/w.git", false},
		{"local path", "../origin.git", "/repos/origin.git", "/repos/origin.git", false},
		{"file URL", "file:///repos/origin.git", "file:///repos/origin.git", "file:///repos/origin.git", false},
		{"unsupported scheme", "ftp://example.com/w.git", "ftp://example.com/w.git", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pushAddress(tt.raw, tt.resolved)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("pushAddress(%q) = %q, %v; want %q (error %v)", tt.raw, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

// workspaceRunner fakes a repository whose origin is originURL: the clone
// creates its directory (so a release: section can be written into it), and
// origin's branch head is originHead ("" = not on origin).
func workspaceRunner(t *testing.T, root, originURL, originHead string, node func(req runnerRequest) (Result, error)) *fakeRunner {
	t.Helper()
	inner := fakeRepo(t, root, node)
	base := inner.handle
	inner.handle = func(c Command) (Result, error) {
		switch {
		case isGit(c, "remote", "get-url", "origin"):
			if originURL == "" {
				return Result{Stderr: []byte("error: No such remote 'origin'")}, errors.New("exit status 2")
			}
			return Result{Stdout: []byte(originURL + "\n")}, nil
		case isGit(c, "remote") && c.Args[len(c.Args)-1] == "remote":
			if originURL == "" {
				return Result{Stdout: []byte("upstream\n")}, nil
			}
			return Result{Stdout: []byte("upstream\norigin\n")}, nil
		case isGit(c, "clone"):
			if err := os.MkdirAll(filepath.Join(c.Args[len(c.Args)-1], ".git"), 0o755); err != nil {
				return Result{}, err
			}
			return Result{}, nil
		case isGit(c, "rev-parse", "--verify", "--quiet") && strings.HasPrefix(c.Args[len(c.Args)-1], "refs/remotes/origin/"):
			if originHead == "" {
				return Result{}, errors.New("exit status 1")
			}
			return Result{Stdout: []byte(originHead + "\n")}, nil
		}
		return base(c)
	}
	return inner
}

func openWorkspace(t *testing.T, f *fakeRunner, root string) *Workspace {
	t.Helper()
	ws, err := (&Engine{Runner: f}).NewWorkspace(context.Background(), WorkspaceOptions{RepoRoot: root, Branch: "main", Env: []string{"PATH=/bin"}, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	t.Cleanup(func() { ws.Close() })
	return ws
}

func TestNewWorkspace_ClonesReadsOriginAndChecksOutOriginsBranch(t *testing.T) {
	root := t.TempDir()
	head := strings.Repeat("a", 40)
	f := workspaceRunner(t, root, "https://bot:s3cr3t@github.com/acme/widgets.git", head, nil)
	ws := openWorkspace(t, f, root)

	if filepath.Base(ws.Dir()) != filepath.Base(root) {
		t.Errorf("clone directory %s: want it named after the repository %s", ws.Dir(), filepath.Base(root))
	}
	if ws.OriginHead() != head || ws.PublicURL() != "https://github.com/acme/widgets.git" {
		t.Errorf("origin head %q, public %q", ws.OriginHead(), ws.PublicURL())
	}
	for _, d := range []string{ws.ArtifactsDir(), ws.NotesDir()} {
		if rel, err := filepath.Rel(ws.Dir(), d); err == nil && filepath.IsLocal(rel) {
			t.Errorf("%s is inside the clone", d)
		}
	}

	clones := f.find(func(c Command) bool { return isGit(c, "clone") })
	if len(clones) != 1 {
		t.Fatalf("clones: %v", clones)
	}
	for _, flag := range []string{"--shared", "--no-checkout", "--no-tags"} {
		if !slices.Contains(clones[0].Args, flag) {
			t.Errorf("clone %v: want %s", clones[0].Args, flag)
		}
	}
	if !slices.Contains(clones[0].Args, root) {
		t.Errorf("clone %v: want a clone of the user's repository", clones[0].Args)
	}
	seturl := f.find(func(c Command) bool { return isGit(c, "remote", "set-url", "origin") })
	if len(seturl) != 1 || seturl[0].Args[len(seturl[0].Args)-1] != "https://github.com/acme/widgets.git" {
		t.Errorf("origin URL of the clone: %v", seturl)
	}
	fetch := f.find(func(c Command) bool { return isGit(c, "fetch") })
	if len(fetch) != 1 {
		t.Fatalf("fetches: %v", fetch)
	}
	for _, want := range []string{"--prune", "origin", "+refs/heads/*:refs/remotes/origin/*", "+refs/tags/*:refs/tags/*", "+refs/notes/*:refs/notes/*"} {
		if !slices.Contains(fetch[0].Args, want) {
			t.Errorf("fetch %v: want %s", fetch[0].Args, want)
		}
	}
	if v, _ := lookupEnv(fetch[0].Env, "GIT_TERMINAL_PROMPT"); v != "0" {
		t.Errorf("fetch may prompt: %v", fetch[0].Env)
	}
	cfg := filepath.Join(ws.Dir(), ".git", "config")
	data, err := os.ReadFile(cfg)
	if err != nil || !strings.Contains(string(data), `[url "https://bot:s3cr3t@github.com/acme/widgets.git"]`) ||
		!strings.Contains(string(data), `insteadOf = "https://github.com/acme/widgets.git"`) {
		t.Errorf("clone config lacks the origin rewrite: %q %v", data, err)
	}
	if info, err := os.Stat(cfg); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Errorf("clone config mode: %v %v", info, err)
	}
	checkout := f.find(func(c Command) bool { return isGit(c, "checkout") })
	if len(checkout) != 1 || !slices.Contains(checkout[0].Args, "-B") || !slices.Contains(checkout[0].Args, "refs/remotes/origin/main") {
		t.Errorf("checkout: %v", checkout)
	}
	noSecretAnywhere(t, f, "s3cr3t")
}

// noSecretAnywhere fails when secret is on the command line or in the
// environment of any command the runner was given.
func noSecretAnywhere(t *testing.T, f *fakeRunner, secret string) {
	t.Helper()
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c.Args, " "), secret) {
			t.Errorf("secret on a command line: %s %v", c.Name, c.Args)
		}
		for _, kv := range c.Env {
			if strings.Contains(kv, secret) {
				t.Errorf("secret in the environment of %s %v: %s", c.Name, c.Args, kv)
			}
		}
	}
}

func TestNewWorkspace_OriginBranchMissingLeavesNoCheckout(t *testing.T) {
	root := t.TempDir()
	f := workspaceRunner(t, root, "git@github.com:acme/widgets.git", "", nil)
	ws := openWorkspace(t, f, root)
	if ws.OriginHead() != "" {
		t.Errorf("origin head %q, want none", ws.OriginHead())
	}
	if c := f.find(func(c Command) bool { return isGit(c, "checkout") }); len(c) != 0 {
		t.Errorf("checked out %v", c)
	}
	if data, _ := os.ReadFile(filepath.Join(ws.Dir(), ".git", "config")); strings.Contains(string(data), "insteadOf") {
		t.Errorf("no rewrite is needed when the push address is origin's own URL: %q", data)
	}
}

func TestNewWorkspace_NoOriginIsARefusal(t *testing.T) {
	root := t.TempDir()
	f := workspaceRunner(t, root, "", "", nil)
	_, err := (&Engine{Runner: f}).NewWorkspace(context.Background(), WorkspaceOptions{RepoRoot: root, Branch: "main", Env: []string{}})
	if !errors.Is(err, ErrNoOrigin) {
		t.Fatalf("err = %v, want ErrNoOrigin", err)
	}
	if c := f.find(func(c Command) bool { return isGit(c, "clone") }); len(c) != 0 {
		t.Errorf("cloned without an origin: %v", c)
	}
}

func TestNewWorkspace_RemoteListingFailureIsAnEnvironmentError(t *testing.T) {
	root := t.TempDir()
	f := workspaceRunner(t, root, "https://github.com/acme/w.git", "", nil)
	base := f.handle
	f.handle = func(c Command) (Result, error) {
		if isGit(c, "remote") {
			return Result{}, context.Canceled
		}
		return base(c)
	}
	_, err := (&Engine{Runner: f}).NewWorkspace(context.Background(), WorkspaceOptions{RepoRoot: root, Branch: "main", Env: []string{}})
	if err == nil || errors.Is(err, ErrNoOrigin) || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the git failure, not ErrNoOrigin", err)
	}
}

func TestNewWorkspace_FetchFailureRemovesTheCloneAndHidesCredentials(t *testing.T) {
	root := t.TempDir()
	f := workspaceRunner(t, root, "https://bot:s3cr3t@github.com/acme/widgets.git", "", nil)
	base := f.handle
	var cloneDir string
	f.handle = func(c Command) (Result, error) {
		if isGit(c, "clone") {
			cloneDir = c.Args[len(c.Args)-1]
		}
		if isGit(c, "fetch") {
			return Result{Stderr: []byte("fatal: unable to access 'https://bot:s3cr3t@github.com/acme/widgets.git/'")}, errors.New("exit status 128")
		}
		return base(c)
	}
	_, err := (&Engine{Runner: f}).NewWorkspace(context.Background(), WorkspaceOptions{RepoRoot: root, Branch: "main", Env: []string{}})
	if !errors.Is(err, ErrOriginFetch) {
		t.Fatalf("err = %v, want ErrOriginFetch", err)
	}
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("credentials in the error: %v", err)
	}
	if cloneDir == "" || dirExists(filepath.Dir(cloneDir)) {
		t.Errorf("release clone %q left behind", cloneDir)
	}
}

func TestWorkspace_CloseRemovesEverything(t *testing.T) {
	root := t.TempDir()
	ws := openWorkspace(t, workspaceRunner(t, root, "https://github.com/acme/w.git", "abc", nil), root)
	top := filepath.Dir(ws.Dir())
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	if dirExists(top) {
		t.Errorf("%s still exists", top)
	}
}

func TestWorkspace_ComputeReadsTheCloneOffline(t *testing.T) {
	root := t.TempDir()
	f := workspaceRunner(t, root, "https://github.com/acme/w.git", "abc", func(req runnerRequest) (Result, error) {
		if req.Mode == "plan" {
			return okPlan()
		}
		return respond(map[string]any{"ok": true, "released": true, "nextRelease": map[string]any{"version": "1.1.0", "gitTag": "v1.1.0"}})
	})
	ws := openWorkspace(t, f, root)
	base := f.handle
	f.handle = func(c Command) (Result, error) {
		if isGit(c, "rev-parse", "--show-toplevel") {
			return Result{Stdout: []byte(c.Args[1] + "\n")}, nil
		}
		return base(c)
	}
	before := len(f.find(func(c Command) bool { return isGit(c, "fetch") }))
	results, err := ws.Compute(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].NewReleaseVersion != "1.1.0" || results[0].Project != filepath.Base(root) {
		t.Errorf("results %+v", results)
	}
	if after := len(f.find(func(c Command) bool { return isGit(c, "fetch") })); after != before {
		t.Errorf("Compute read origin again (%d fetches, want %d): the clone already holds origin's refs", after, before)
	}
	for _, c := range f.find(func(c Command) bool { return isGit(c, "clone", "--quiet", "--mirror") }) {
		if !slices.Contains(c.Args, ws.Dir()) {
			t.Errorf("Compute copied %v, want the release clone", c.Args)
		}
	}
}

func TestWorkspace_ReleaseIsARealRunGuardedByTheComputedVersion(t *testing.T) {
	root := t.TempDir()
	var got []runnerRequest
	var nodeEnvs [][]string
	f := workspaceRunner(t, root, "https://bot:s3cr3t@github.com/acme/w.git", "abc", func(req runnerRequest) (Result, error) {
		got = append(got, req)
		if req.Mode == "plan" {
			return okPlan()
		}
		return respond(map[string]any{"ok": true, "released": true, "nextRelease": map[string]any{"version": "1.1.0", "gitTag": "web-v1.1.0", "gitHead": "def"}})
	})
	writeConfig(t, root, `release:
  projects:
    - name: core
      path: core
    - name: web
      path: web
      tag_format: "web-v${version}"
      scopes: [ui]
      depends_on:
        - project: core
          bump: minor
`)
	ws := openWorkspace(t, f, root)
	base := f.handle
	f.handle = func(c Command) (Result, error) {
		if isNodeScript(c) {
			nodeEnvs = append(nodeEnvs, c.Env)
		}
		return base(c)
	}
	var out strings.Builder
	r, err := ws.Release(context.Background(), ReleaseRequest{
		Expected: ProjectResult{Project: "web", NewReleasePublished: true, NewReleaseVersion: "1.1.0"},
		Computed: []ProjectResult{{Project: "core", NewReleasePublished: true, NewReleaseVersion: "1.0.1"}},
		Env:      []string{"GH_TOKEN=tok-123"},
		Output:   &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Project != "web" || r.NewReleaseGitTag != "web-v1.1.0" || r.NewReleaseGitHead != "def" || r.Branch != "main" {
		t.Errorf("result %+v", r)
	}
	var rel *runnerRequest
	for i := range got {
		if got[i].Mode == "release" {
			rel = &got[i]
		}
	}
	if rel == nil {
		t.Fatalf("no release request among %+v", got)
	}
	if rel.ExpectedVersion != "1.1.0" || rel.RepositoryURL != "https://github.com/acme/w.git" || rel.TagFormat != "web-v${version}" || rel.MinReleaseType != "minor" || rel.Cwd != ws.Dir() {
		t.Errorf("release request %+v", rel)
	}
	if rel.Changelog != "web/CHANGELOG.md" {
		t.Errorf("changelog %q, want web/CHANGELOG.md (a monorepo project's own)", rel.Changelog)
	}
	if rel.Filter == nil || !slices.Equal(rel.Filter.Paths, []string{"web"}) || !slices.Equal(rel.Filter.Scopes, []string{"ui"}) {
		t.Errorf("filter %+v", rel.Filter)
	}
	last := nodeEnvs[len(nodeEnvs)-1]
	if v, _ := lookupEnv(last, "GH_TOKEN"); v != "tok-123" {
		t.Errorf("GH_TOKEN not passed to the runner")
	}
	noSecretAnywhere(t, f, "s3cr3t")
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c.Args, " "), "tok-123") {
			t.Errorf("token on a command line: %s %v", c.Name, c.Args)
		}
	}
}

func TestWorkspace_ReleaseVersionMismatch(t *testing.T) {
	tests := []struct {
		name string
		resp map[string]any
	}{
		{"another version", map[string]any{"ok": true, "released": true, "nextRelease": map[string]any{"version": "1.2.0"}}},
		{"no release", map[string]any{"ok": true, "released": false}},
		{"runner guard", map[string]any{"ok": false, "error": "semantic-release would release 1.2.0", "versionMismatch": map[string]any{"expected": "1.1.0", "actual": "1.2.0"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			f := workspaceRunner(t, root, "https://github.com/acme/w.git", "abc", func(req runnerRequest) (Result, error) {
				if req.Mode == "plan" {
					return okPlan()
				}
				res, err := respond(tt.resp)
				if tt.resp["ok"] == false {
					err = errors.New("exit status 1")
				}
				return res, err
			})
			ws := openWorkspace(t, f, root)
			_, err := ws.Release(context.Background(), ReleaseRequest{Expected: ProjectResult{Project: filepath.Base(root), NewReleasePublished: true, NewReleaseVersion: "1.1.0"}})
			if !errors.Is(err, ErrVersionMismatch) {
				t.Fatalf("err = %v, want ErrVersionMismatch", err)
			}
		})
	}
}

func TestWorkspace_ReleaseErrorTextIsNotAMismatchSignal(t *testing.T) {
	root := t.TempDir()
	f := workspaceRunner(t, root, "https://github.com/acme/w.git", "abc", func(req runnerRequest) (Result, error) {
		if req.Mode == "plan" {
			return okPlan()
		}
		res, _ := respond(map[string]any{"ok": false, "error": "EGITHUB a commit message mentioning ENERVVERSIONMISMATCH"})
		return res, errors.New("exit status 1")
	})
	ws := openWorkspace(t, f, root)
	_, err := ws.Release(context.Background(), ReleaseRequest{Expected: ProjectResult{Project: filepath.Base(root), NewReleasePublished: true, NewReleaseVersion: "1.1.0"}})
	if err == nil || errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("err = %v, want a plain failure", err)
	}
}

func TestWorkspace_CheckPushAndTagOnOrigin(t *testing.T) {
	root := t.TempDir()
	f := workspaceRunner(t, root, "https://github.com/acme/w.git", "abc", nil)
	base := f.handle
	denied := false
	f.handle = func(c Command) (Result, error) {
		switch {
		case isGit(c, "push"):
			if denied {
				return Result{Stderr: []byte("remote: Permission denied")}, errors.New("exit status 128")
			}
			return Result{}, nil
		case isGit(c, "ls-remote"):
			if strings.HasSuffix(c.Args[len(c.Args)-1], "v1.0.0") {
				return Result{Stdout: []byte("abc\trefs/tags/v1.0.0\n")}, nil
			}
			if c.Args[len(c.Args)-1] == "refs/heads/main" {
				return Result{Stdout: []byte("def\trefs/heads/main\n")}, nil
			}
			return Result{}, nil
		}
		return base(c)
	}
	ws := openWorkspace(t, f, root)
	if err := ws.CheckPush(context.Background()); err != nil {
		t.Fatal(err)
	}
	push := f.find(func(c Command) bool { return isGit(c, "push") })
	if len(push) != 1 || !slices.Contains(push[0].Args, "--dry-run") || !slices.Contains(push[0].Args, "HEAD:refs/heads/main") {
		t.Errorf("push check %v", push)
	}
	denied = true
	if err := ws.CheckPush(context.Background()); !errors.Is(err, ErrPushDenied) {
		t.Errorf("err = %v, want ErrPushDenied", err)
	}
	if ok, err := ws.TagOnOrigin(context.Background(), "v1.0.0"); !ok || err != nil {
		t.Errorf("TagOnOrigin(v1.0.0) = %v, %v", ok, err)
	}
	if ok, err := ws.TagOnOrigin(context.Background(), "v9.9.9"); ok || err != nil {
		t.Errorf("TagOnOrigin(v9.9.9) = %v, %v", ok, err)
	}
	if head, err := ws.BranchOnOrigin(context.Background()); head != "def" || err != nil {
		t.Errorf("BranchOnOrigin = %q, %v", head, err)
	}
}

func TestWorkspace_ReleaseWarnsOnceWhenTheChangelogCannotApply(t *testing.T) {
	root := t.TempDir()
	f := workspaceRunner(t, root, "https://github.com/acme/w.git", "abc", func(req runnerRequest) (Result, error) {
		if req.Mode == "plan" {
			return okPlan()
		}
		return respond(map[string]any{"ok": true, "released": true, "changelogIgnored": true,
			"nextRelease": map[string]any{"version": "1.1.0", "gitTag": "x"}})
	})
	writeConfig(t, root, "release:\n  projects:\n    - name: api\n      path: api\n    - name: web\n      path: web\n")
	ws := openWorkspace(t, f, root)
	var out strings.Builder
	for _, p := range []string{"api", "web"} {
		if _, err := ws.Release(context.Background(), ReleaseRequest{Expected: ProjectResult{Project: p, NewReleasePublished: true, NewReleaseVersion: "1.1.0"}, Output: &out}); err != nil {
			t.Fatal(err)
		}
	}
	if n := strings.Count(out.String(), "@semantic-release/changelog"); n != 1 {
		t.Errorf("warning printed %d times, want once:\n%s", n, out.String())
	}
}

func TestWorkspace_UsesTheUserRepositorysLocalConfiguration(t *testing.T) {
	root := t.TempDir()
	// The user's working tree (committed or not) declares api and web.
	writeConfig(t, root, "release:\n  projects:\n    - name: api\n      path: api\n    - name: web\n      path: web\n")
	var runs []runnerRequest
	f := workspaceRunner(t, root, "https://github.com/acme/w.git", "abc", func(req runnerRequest) (Result, error) {
		if req.Mode == "plan" {
			return okPlan()
		}
		runs = append(runs, req)
		return respond(map[string]any{"ok": true, "released": true, "nextRelease": map[string]any{"version": "1.1.0", "gitTag": "t"}})
	})
	base := f.handle
	f.handle = func(c Command) (Result, error) {
		if isGit(c, "rev-parse", "--show-toplevel") && c.Args[1] != root {
			return Result{Stdout: []byte(c.Args[1] + "\n")}, nil
		}
		return base(c)
	}
	ws := openWorkspace(t, f, root)
	// Origin's copy in the clone says something else; it must be ignored.
	writeConfig(t, ws.Dir(), "release:\n  projects:\n    - name: other\n      path: .\n")

	if rel := ws.Config(); rel == nil || len(rel.Projects) != 2 || rel.Projects[1].Name != "web" {
		t.Fatalf("Config() = %+v, want the user's api and web", rel)
	}
	results, err := ws.Compute(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Project != "api" || results[1].Project != "web" {
		t.Errorf("computed %+v", results)
	}
	if _, err := ws.Release(context.Background(), ReleaseRequest{Expected: results[1], Computed: results}); err != nil {
		t.Fatalf("Release(web): %v", err)
	}
	last := runs[len(runs)-1]
	if last.Mode != "release" || last.Filter == nil || !slices.Equal(last.Filter.Paths, []string{"web"}) {
		t.Errorf("release request %+v", last)
	}
}

func TestWorkspace_SectionlessUserRepositoryIgnoresTheClonesSection(t *testing.T) {
	root := t.TempDir()
	f := workspaceRunner(t, root, "https://github.com/acme/w.git", "abc", nil)
	ws := openWorkspace(t, f, root)
	writeConfig(t, ws.Dir(), "release:\n  projects:\n    - name: other\n      path: .\n")
	if ws.Config() != nil {
		t.Errorf("Config() = %+v, want nil: the user's repository has no release: section", ws.Config())
	}
}

func TestNewWorkspace_InvalidLocalConfigurationStopsBeforeCloning(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, "release:\n  projects:\n    - name: api\n")
	f := workspaceRunner(t, root, "https://github.com/acme/w.git", "abc", nil)
	_, err := (&Engine{Runner: f}).NewWorkspace(context.Background(), WorkspaceOptions{RepoRoot: root, Branch: "main", Env: []string{}})
	var invalid *shipconfig.ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want a ValidationError", err)
	}
	if c := f.find(func(c Command) bool { return isGit(c, "clone") }); len(c) != 0 {
		t.Errorf("cloned despite an invalid configuration")
	}
}

func TestWorkspace_CheckoutTag(t *testing.T) {
	root := t.TempDir()
	f := workspaceRunner(t, root, "https://github.com/acme/w.git", "abc", nil)
	base := f.handle
	f.handle = func(c Command) (Result, error) {
		if isGit(c, "rev-parse", "--verify", "--quiet") && strings.HasPrefix(c.Args[len(c.Args)-1], "refs/tags/") {
			switch c.Args[len(c.Args)-1] {
			case "refs/tags/v1.1.0^{commit}":
				return Result{Stdout: []byte("def\n")}, nil
			case "refs/tags/broken^{commit}":
				return Result{Stderr: []byte("fatal: not a git repository")}, exitCodeErr(128)
			case "refs/tags/slow^{commit}":
				return Result{}, context.DeadlineExceeded
			}
			// rev-parse --verify --quiet: exit 1, no output, for a missing ref.
			return Result{}, exitCodeErr(1)
		}
		return base(c)
	}
	ws := openWorkspace(t, f, root)
	commit, err := ws.CheckoutTag(context.Background(), "v1.1.0")
	if err != nil || commit != "def" {
		t.Fatalf("CheckoutTag = %q, %v", commit, err)
	}
	co := f.find(func(c Command) bool { return isGit(c, "checkout", "--quiet", "--detach") })
	if len(co) != 1 || co[0].Args[len(co[0].Args)-1] != "def" {
		t.Errorf("checkout %v", co)
	}
	if _, err := ws.CheckoutTag(context.Background(), "v9.9.9"); !errors.Is(err, ErrTagNotOnOrigin) {
		t.Errorf("missing tag: err = %v", err)
	}
	for tag, cause := range map[string]string{"broken": "not a git repository", "slow": "deadline exceeded"} {
		_, err := ws.CheckoutTag(context.Background(), tag)
		if err == nil || errors.Is(err, ErrTagNotOnOrigin) || !strings.Contains(err.Error(), cause) {
			t.Errorf("%s: err = %v, want the environment failure with its cause", tag, err)
		}
	}
}

// exitCodeErr is a process error with an exit code, as exec.ExitError.
type exitCodeErr int

func (e exitCodeErr) Error() string { return fmt.Sprintf("exit status %d", int(e)) }
func (e exitCodeErr) ExitCode() int { return int(e) }
