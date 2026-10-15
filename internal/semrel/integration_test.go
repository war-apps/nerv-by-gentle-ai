//go:build semrel_integration

// Integration tests: real git, node, npm and semantic-release. Run with
//
//	go test -tags semrel_integration -run Integration -v ./internal/semrel/
//
// They skip when node or npm is missing or the npm registry is unreachable.
// The first run installs semantic-release into a temporary cache (about 300
// packages).

package semrel

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

var integrationCache string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "nerv-semrel-cache-")
	if err != nil {
		panic(err)
	}
	integrationCache = dir
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func requireToolchain(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test")
	}
	for _, tool := range []string{"git", "node", "npm"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "npm", "view", "semantic-release@"+shipconfig.DefaultSemanticReleaseVersion, "version").Run(); err != nil {
		t.Skipf("npm registry unreachable: %v", err)
	}
}

// testEnv is the environment without release credentials or CI markers:
// computing a version must need neither.
func testEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "GH_TOKEN", "GITHUB_TOKEN", "NPM_TOKEN", "GIT_CREDENTIALS":
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
}

type repo struct {
	t   *testing.T
	dir string
}

// newRepo creates a repository whose origin is a local bare repository
// (origin's tags and semantic-release notes are read from it), configured as
// a relative path, which must resolve against the repository and not against
// NERV's temporary mirror.
func newRepo(t *testing.T) *repo {
	t.Helper()
	origin := &repo{t: t, dir: t.TempDir()}
	origin.git("init", "-q", "--bare")
	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	rel, err := filepath.Rel(r.dir, origin.dir)
	if err != nil {
		t.Fatal(err)
	}
	r.git("remote", "add", "origin", rel)
	return r
}

func (r *repo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.dir}, args...)...)
	cmd.Env = testEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *repo) write(rel, content string) {
	r.t.Helper()
	p := filepath.Join(r.dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// commit writes the files and commits them with message.
func (r *repo) commit(message string, files ...string) {
	r.t.Helper()
	for _, f := range files {
		r.write(f, message+"\n")
	}
	r.git("add", "-A")
	r.git("commit", "-q", "--allow-empty", "-m", message)
}

func compute(t *testing.T, opts Options) map[string]ProjectResult {
	t.Helper()
	opts.Env, opts.CacheDir = testEnv(), integrationCache
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	results, err := Compute(ctx, opts)
	if err != nil {
		// Never print opts.Env: it holds the caller's credentials.
		t.Fatalf("Compute(branch %q, project %q, offline %v): %v", opts.Branch, opts.Project, opts.Offline, err)
	}
	out := map[string]ProjectResult{}
	for _, r := range results {
		out[r.Project] = r
	}
	return out
}

// sole returns the only result, which must belong to project name.
func sole(t *testing.T, results map[string]ProjectResult, name string) ProjectResult {
	t.Helper()
	r, ok := results[name]
	if len(results) != 1 || !ok {
		t.Fatalf("want one result for %q, got %+v", name, results)
	}
	return r
}

// dimRelease is dim.comprobantes.api's package.json "release" configuration
// (branches main / release/rc / develop / hotfix/*, conventionalcommits
// preset, changelog, npm without publish, exec with a verifyReleaseCmd
// script, git, github): computing a version must not need GH_TOKEN, the exec
// scripts or any write.
const dimRelease = `{
  "name": "widgets", "version": "0.0.0", "private": true,
  "repository": "https://github.com/example/widgets.git",
  "release": {
    "branches": [
      "main",
      { "name": "release/rc", "channel": "beta", "prerelease": "rc" },
      { "name": "develop", "channel": "alpha", "prerelease": "alpha" },
      { "name": "hotfix/*", "channel": "beta", "prerelease": true }
    ],
    "plugins": [
      ["@semantic-release/commit-analyzer", { "preset": "conventionalcommits", "parserOpts": { "noteKeywords": ["BREAKING CHANGE", "BREAKING CHANGES", "BREAKING"] } }],
      ["@semantic-release/release-notes-generator", { "preset": "conventionalcommits", "parserOpts": { "noteKeywords": ["BREAKING CHANGE", "BREAKING CHANGES", "BREAKING"] } }],
      ["@semantic-release/changelog", { "changelogFile": "src/Api/CHANGELOG.md" }],
      ["@semantic-release/npm", { "npmPublish": false, "pkgRoot": "." }],
      ["@semantic-release/exec", {
        "verifyReleaseCmd": "bash scripts/set-release-outputs.sh \"${nextRelease.version}\" \"${nextRelease.channel || ''}\" \"${nextRelease.gitTag}\"",
        "prepareCmd": "bash scripts/bump-csproj.sh \"${nextRelease.version}\""
      }],
      ["@semantic-release/git", { "assets": ["src/Api/CHANGELOG.md", "package.json", "src/Api/Dim.Comprobantes.Api.csproj"] }],
      "@semantic-release/github"
    ]
  }
}
`

func TestIntegration_SingleProjectPrerelease(t *testing.T) {
	requireToolchain(t)
	r := newRepo(t)
	r.write("package.json", dimRelease)
	r.commit("chore: init", "src/Api/a.txt")
	r.git("tag", "v1.0.0")
	r.commit("feat: add widgets", "src/Api/b.txt")
	r.git("tag", "v1.1.0")
	r.git("checkout", "-q", "-b", "develop")
	r.commit("feat(api): add gadgets", "src/Api/c.txt")
	// What CI's semantic-release did for the first alpha: a tag plus a git
	// note recording its channel, both pushed to origin.
	r.git("tag", "v1.2.0-alpha.1")
	r.git("notes", "--ref", "semantic-release-v1.2.0-alpha.1", "add", "-m", `{"channels":["alpha"]}`, "v1.2.0-alpha.1")
	r.git("push", "-q", "origin", "main", "develop", "--tags", "refs/notes/*:refs/notes/*")
	// A developer clone has the tags but never the notes (git fetch does not
	// bring them).
	r.git("update-ref", "-d", "refs/notes/semantic-release-v1.2.0-alpha.1")
	r.commit("fix: repair gadgets", "src/Api/d.txt")
	r.git("checkout", "-q", "main")
	before := r.git("for-each-ref", "--format=%(refname) %(objectname)")

	name := filepath.Base(r.dir)
	dev := sole(t, compute(t, Options{RepoRoot: r.dir, Branch: "develop"}), name)
	if !dev.NewReleasePublished || dev.NewReleaseVersion != "1.2.0-alpha.2" || dev.NewReleaseChannel != "alpha" || dev.NewReleaseGitTag != "v1.2.0-alpha.2" {
		t.Errorf("develop: %+v", dev)
	}
	if dev.SemanticReleaseConfig != ConfigRepository || dev.SemanticReleaseConfigFile != "package.json" {
		t.Errorf("config source %q %q, want the repository's package.json", dev.SemanticReleaseConfig, dev.SemanticReleaseConfigFile)
	}
	if dev.NewReleaseMajorVersion != "1" || dev.NewReleaseMinorVersion != "2" || dev.NewReleasePatchVersion != "0" {
		t.Errorf("develop version parts: %+v", dev)
	}
	if dev.LastReleaseVersion != "1.2.0-alpha.1" || dev.LastReleaseGitTag != "v1.2.0-alpha.1" || dev.NewReleaseGitHead != r.git("rev-parse", "develop") {
		t.Errorf("develop last release / head: %+v", dev)
	}
	if !strings.Contains(dev.NewReleaseNotes, "repair gadgets") || strings.Contains(dev.NewReleaseNotes, "add gadgets") ||
		!strings.Contains(dev.NewReleaseNotes, "https://github.com/example/widgets/commit/") {
		t.Errorf("develop notes:\n%s", dev.NewReleaseNotes)
	}
	t.Logf("develop -> %s (channel %q), last %s", dev.NewReleaseVersion, dev.NewReleaseChannel, dev.LastReleaseGitTag)

	// Offline, only local refs: the alpha tag has no channel note, so
	// semantic-release does not see it as the last alpha.
	off := sole(t, compute(t, Options{RepoRoot: r.dir, Branch: "develop", Offline: true}), name)
	if off.LastReleaseVersion != "1.1.0" {
		t.Errorf("offline develop: %+v", off)
	}
	t.Logf("offline develop -> %s, last %s", off.NewReleaseVersion, off.LastReleaseGitTag)

	main := sole(t, compute(t, Options{RepoRoot: r.dir}), name)
	if main.Branch != "main" || main.NewReleasePublished || main.LastReleaseVersion != "1.1.0" {
		t.Errorf("main (current branch): %+v", main)
	}

	if after := r.git("for-each-ref", "--format=%(refname) %(objectname)"); after != before {
		t.Errorf("refs of the user's repository changed:\nbefore %s\nafter  %s", before, after)
	}
	if wts := r.git("worktree", "list"); strings.Count(wts, "\n") != 0 {
		t.Errorf("worktree left behind:\n%s", wts)
	}
	if status := r.git("status", "--porcelain"); status != "" || r.git("branch", "--show-current") != "main" {
		t.Errorf("checkout touched: %q on %s", status, r.git("branch", "--show-current"))
	}
}

const angularMain = `{ "branches": ["main"], "plugins": ["@semantic-release/commit-analyzer", "@semantic-release/release-notes-generator"] }`

func TestIntegration_MonorepoPathOrScope(t *testing.T) {
	requireToolchain(t)
	r := newRepo(t)
	r.write(".releaserc.json", angularMain)
	r.write(".nerv/nerv.yaml", `release:
  projects:
    - { name: api, path: api }
    - { name: web, path: web }
    - { name: cli, path: cli }
`)
	r.commit("chore: init", "api/a.txt", "web/a.txt", "cli/a.txt")
	for _, tag := range []string{"api-v1.0.0", "web-v1.0.0", "cli-v1.0.0"} {
		r.git("tag", tag)
	}
	r.commit("feat(api): share the client with web", "web/client.txt")
	r.commit("fix(docs): typo", "docs/readme.txt")

	got := compute(t, Options{RepoRoot: r.dir, Branch: "main"})
	if api := got["api"]; !api.NewReleasePublished || api.NewReleaseVersion != "1.1.0" || api.NewReleaseGitTag != "api-v1.1.0" || api.LastReleaseGitTag != "api-v1.0.0" {
		t.Errorf("api (by scope): %+v", api)
	}
	if web := got["web"]; !web.NewReleasePublished || web.NewReleaseVersion != "1.1.0" || web.NewReleaseGitTag != "web-v1.1.0" {
		t.Errorf("web (by path): %+v", web)
	}
	if strings.Contains(got["api"].NewReleaseNotes, "typo") {
		t.Errorf("api notes include another project's commit:\n%s", got["api"].NewReleaseNotes)
	}
	// The origin is a local repository: there is no page to link to, so the
	// notes have no links (never https:/<local path>/...).
	if notes := got["api"].NewReleaseNotes; !strings.Contains(notes, "share the client with web") || strings.Contains(notes, "](") || strings.Contains(notes, "https:/") {
		t.Errorf("api notes with a local origin:\n%s", notes)
	}
	if cli := got["cli"]; cli.NewReleasePublished || cli.LastReleaseVersion != "1.0.0" {
		t.Errorf("cli (untouched): %+v", cli)
	}
	t.Logf("api -> %s, web -> %s, cli released %v", got["api"].NewReleaseVersion, got["web"].NewReleaseVersion, got["cli"].NewReleasePublished)
}

func TestIntegration_DependsOnBump(t *testing.T) {
	requireToolchain(t)
	r := newRepo(t)
	r.write(".releaserc.json", angularMain)
	r.write(".nerv/nerv.yaml", `release:
  projects:
    - name: app
      path: app
      depends_on: [{ project: core }]
    - name: tool
      path: tool
      depends_on: [{ project: core, bump: none }]
    - name: sdk
      path: sdk
      depends_on: [{ project: core, bump: minor }]
    - name: core
      path: core
`)
	r.commit("chore: init", "app/a.txt", "tool/a.txt", "sdk/a.txt", "core/a.txt")
	for _, tag := range []string{"app-v1.0.0", "tool-v1.0.0", "sdk-v1.0.0", "core-v1.0.0"} {
		r.git("tag", tag)
	}
	r.commit("fix: core bug", "core/b.txt")

	results, err := Compute(context.Background(), Options{RepoRoot: r.dir, Branch: "main", Env: testEnv(), CacheDir: integrationCache})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	got := map[string]ProjectResult{}
	for _, res := range results {
		order = append(order, res.Project)
		got[res.Project] = res
	}
	if strings.Join(order, ",") != "core,app,tool,sdk" {
		t.Errorf("order %v", order)
	}
	if v := got["core"].NewReleaseVersion; v != "1.0.1" {
		t.Errorf("core %q", v)
	}
	if v := got["app"].NewReleaseVersion; v != "1.0.1" {
		t.Errorf("app (patch from core) %q", v)
	}
	if got["tool"].NewReleasePublished {
		t.Errorf("tool (bump none) released %s", got["tool"].NewReleaseVersion)
	}
	if v := got["sdk"].NewReleaseVersion; v != "1.1.0" {
		t.Errorf("sdk (minor from core) %q", v)
	}
	t.Logf("core -> %s, app -> %s, sdk -> %s, tool released %v", got["core"].NewReleaseVersion, got["app"].NewReleaseVersion, got["sdk"].NewReleaseVersion, got["tool"].NewReleasePublished)
}

func TestIntegration_UnpinnedPlugin(t *testing.T) {
	requireToolchain(t)
	r := newRepo(t)
	r.write(".releaserc.json", `{ "branches": ["main"], "plugins": ["@semantic-release/commit-analyzer", "semantic-release-slack-bot"] }`)
	r.commit("feat: x", "a.txt")
	_, err := Compute(context.Background(), Options{RepoRoot: r.dir, Branch: "main", Env: testEnv(), CacheDir: integrationCache})
	if err == nil || !strings.Contains(err.Error(), "semantic-release-slack-bot") || !strings.Contains(err.Error(), "extra_plugins") {
		t.Fatalf("err = %v", err)
	}
}

// TestIntegration_NotesLinkThePublicOrigin: with no repository in the
// configuration or package.json, the notes link to the user's origin (as in
// CI), without its credentials, offline too; with no origin they have no
// links.
func TestIntegration_NotesLinkThePublicOrigin(t *testing.T) {
	requireToolchain(t)
	r := newRepo(t)
	r.write(".releaserc.json", angularMain)
	r.commit("chore: init", "a.txt")
	r.git("tag", "v1.0.0")
	r.commit("fix: repair widgets", "a.txt")

	r.git("remote", "set-url", "origin", "https://nerv:s3cr3t@github.com/example/widgets.git")
	name := filepath.Base(r.dir)
	linked := sole(t, compute(t, Options{RepoRoot: r.dir, Branch: "main", Offline: true}), name).NewReleaseNotes
	if !strings.Contains(linked, "https://github.com/example/widgets/compare/v1.0.0...v1.0.1") ||
		!strings.Contains(linked, "https://github.com/example/widgets/commit/") || strings.Contains(linked, "s3cr3t") {
		t.Errorf("notes with a GitHub origin:\n%s", linked)
	}

	r.git("remote", "remove", "origin")
	plain := sole(t, compute(t, Options{RepoRoot: r.dir, Branch: "main"}), name).NewReleaseNotes
	if !strings.Contains(plain, "## 1.0.1 (") || !strings.Contains(plain, "repair widgets") || strings.Contains(plain, "](") || strings.Contains(plain, "http") {
		t.Errorf("notes without an origin:\n%s", plain)
	}
	t.Logf("no-origin notes:\n%s", plain)
}

// hotfixRelease has dim.comprobantes.api's branch shape: a hotfix/* wildcard
// whose prerelease id is the branch name, which is invalid for any name with
// a slash (EPRERELEASEBRANCH) once such a branch exists.
const hotfixRelease = `{
  "branches": [
    "main",
    { "name": "develop", "channel": "alpha", "prerelease": "alpha" },
    { "name": "hotfix/*", "channel": "beta", "prerelease": true }
  ],
  "plugins": ["@semantic-release/commit-analyzer", "@semantic-release/release-notes-generator"]
}`

// TestIntegration_BranchesAreOrigins: semantic-release sees origin's
// branches, as in CI, not the developer's local ones; the requested branch is
// the local one (its unpushed commits count).
func TestIntegration_BranchesAreOrigins(t *testing.T) {
	requireToolchain(t)
	r := newRepo(t)
	r.write(".releaserc.json", hotfixRelease)
	r.commit("chore: init", "a.txt")
	r.git("tag", "v1.0.0")
	r.git("checkout", "-q", "-b", "develop")
	r.commit("fix: pushed fix", "b.txt")
	r.git("push", "-q", "origin", "main", "develop", "--tags")
	// Unpushed work: a feature on develop and a local-only hotfix branch
	// (which CI never sees).
	r.commit("feat: local feature", "c.txt")
	r.git("branch", "hotfix/tw-1-error-config")
	name := filepath.Base(r.dir)

	for _, offline := range []bool{false, true} {
		dev := sole(t, compute(t, Options{RepoRoot: r.dir, Branch: "develop", Offline: offline}), name)
		// feat from the local, unpushed commit: a minor alpha.
		if dev.NewReleaseVersion != "1.1.0-alpha.1" || dev.NewReleaseChannel != "alpha" || dev.NewReleaseGitHead != r.git("rev-parse", "develop") {
			t.Errorf("develop (offline %v): %+v", offline, dev)
		}
		main := sole(t, compute(t, Options{RepoRoot: r.dir, Branch: "main", Offline: offline}), name)
		if main.NewReleasePublished || main.LastReleaseVersion != "1.0.0" {
			t.Errorf("main (offline %v): %+v", offline, main)
		}
	}

	// A branch only on origin is visible: without a local main there is
	// still a release branch for develop to follow.
	r.git("branch", "-D", "main")
	for _, offline := range []bool{false, true} {
		dev := sole(t, compute(t, Options{RepoRoot: r.dir, Branch: "develop", Offline: offline}), name)
		if dev.NewReleaseVersion != "1.1.0-alpha.1" || dev.LastReleaseVersion != "1.0.0" {
			t.Errorf("develop without a local main (offline %v): %+v", offline, dev)
		}
	}
}

// releaseConfig releases main with a CHANGELOG committed by the git plugin;
// no plugin needs a token.
const releaseConfig = `{
  "name": "widgets", "version": "0.0.0", "private": true,
  "release": {
    "branches": ["main"],
    "plugins": [
      "@semantic-release/commit-analyzer",
      "@semantic-release/release-notes-generator",
      "@semantic-release/changelog",
      ["@semantic-release/git", { "assets": ["CHANGELOG.md"] }]
    ]
  }
}
`

func TestIntegration_WorkspaceReleasesToOrigin(t *testing.T) {
	requireToolchain(t)
	r := newRepo(t)
	origin := &repo{t: t, dir: filepath.Join(r.dir, r.git("remote", "get-url", "origin"))}
	// semantic-release fetches origin's HEAD (git fetch --tags <url>), which
	// must name an existing branch, as on any hosted origin.
	origin.git("symbolic-ref", "HEAD", "refs/heads/main")
	r.write("package.json", releaseConfig)
	r.commit("chore: init", "a.txt")
	r.git("tag", "v1.0.0")
	r.commit("feat: add gadgets", "b.txt")
	r.git("push", "-q", "origin", "main", "--tags")
	before := r.git("for-each-ref", "--format=%(refname) %(objectname)")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	ws, err := NewWorkspace(ctx, WorkspaceOptions{RepoRoot: r.dir, Branch: "main", Env: testEnv(), CacheDir: integrationCache})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if ws.OriginHead() != r.git("rev-parse", "main") {
		t.Fatalf("origin head %s", ws.OriginHead())
	}
	results, err := ws.Compute(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].NewReleaseVersion != "1.1.0" {
		t.Fatalf("computed %+v", results)
	}
	if err := ws.CheckPush(ctx); err != nil {
		t.Fatal(err)
	}

	// A version other than the computed one stops before the tag.
	wrong := results[0]
	wrong.NewReleaseVersion = "9.9.9"
	if _, err := ws.Release(ctx, ReleaseRequest{Expected: wrong, Computed: results}); !errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("mismatch: err = %v", err)
	}
	if tags := origin.git("tag", "--list"); tags != "v1.0.0" {
		t.Fatalf("origin tags after the mismatch: %q", tags)
	}

	var log strings.Builder
	got, err := ws.Release(ctx, ReleaseRequest{Expected: results[0], Computed: results, Output: &log})
	if err != nil {
		t.Fatalf("Release: %v\n%s", err, log.String())
	}
	if got.NewReleaseVersion != "1.1.0" || got.NewReleaseGitTag != "v1.1.0" {
		t.Errorf("released %+v", got)
	}
	if !strings.Contains(log.String(), "Published release 1.1.0") {
		t.Errorf("semantic-release log not streamed:\n%s", log.String())
	}
	if origin.git("rev-parse", "v1.1.0^{commit}") != origin.git("rev-parse", "main") {
		t.Errorf("tag v1.1.0 is not on origin's main")
	}
	if msg := origin.git("log", "-1", "--format=%s", "main"); !strings.HasPrefix(msg, "chore(release): 1.1.0") {
		t.Errorf("origin's main head: %q", msg)
	}
	if changelog := origin.git("show", "main:CHANGELOG.md"); !strings.Contains(changelog, "add gadgets") {
		t.Errorf("CHANGELOG on origin:\n%s", changelog)
	}
	if ok, err := ws.TagOnOrigin(ctx, "v1.1.0"); !ok || err != nil {
		t.Errorf("TagOnOrigin = %v, %v", ok, err)
	}
	if after := r.git("for-each-ref", "--format=%(refname) %(objectname)"); after != before {
		t.Errorf("the user's refs changed:\nbefore %s\nafter  %s", before, after)
	}
	if status := r.git("status", "--porcelain"); status != "" {
		t.Errorf("the user's checkout changed: %q", status)
	}
}

// releaseAll computes in a fresh release clone of r and releases every
// project with a new release, in order.
func releaseAll(t *testing.T, r *repo) []ProjectResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	ws, err := NewWorkspace(ctx, WorkspaceOptions{RepoRoot: r.dir, Branch: "main", Env: testEnv(), CacheDir: integrationCache})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	results, err := ws.Compute(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var out []ProjectResult
	for _, res := range results {
		if !res.NewReleasePublished {
			continue
		}
		var log strings.Builder
		got, err := ws.Release(ctx, ReleaseRequest{Expected: res, Computed: results, Output: &log})
		if err != nil {
			t.Fatalf("release %s: %v\n%s", res.Project, err, log.String())
		}
		out = append(out, got)
	}
	return out
}

// newOriginRepo is newRepo with origin's HEAD on main, ready for real
// releases.
func newOriginRepo(t *testing.T) (*repo, *repo) {
	r := newRepo(t)
	origin := &repo{t: t, dir: filepath.Join(r.dir, r.git("remote", "get-url", "origin"))}
	origin.git("symbolic-ref", "HEAD", "refs/heads/main")
	return r, origin
}

func TestIntegration_MonorepoProjectsWriteTheirOwnChangelog(t *testing.T) {
	requireToolchain(t)
	r, origin := newOriginRepo(t)
	r.write("package.json", releaseConfig)
	writeConfig(t, r.dir, "release:\n  projects:\n    - name: api\n      path: api\n      tag_format: \"api-v${version}\"\n    - name: web\n      path: web\n      tag_format: \"web-v${version}\"\n")
	r.commit("chore: init", "api/a.txt", "web/a.txt")
	r.git("tag", "api-v1.0.0")
	r.git("tag", "web-v1.0.0")
	r.commit("feat: api endpoint", "api/b.txt")
	r.commit("fix: web layout", "web/b.txt")
	r.git("push", "-q", "origin", "main", "--tags")

	got := releaseAll(t, r)
	if len(got) != 2 || got[0].NewReleaseGitTag != "api-v1.1.0" || got[1].NewReleaseGitTag != "web-v1.0.1" {
		t.Fatalf("released %+v", got)
	}
	if cl := origin.git("show", "main:api/CHANGELOG.md"); !strings.Contains(cl, "api endpoint") || strings.Contains(cl, "web layout") {
		t.Errorf("api/CHANGELOG.md:\n%s", cl)
	}
	if cl := origin.git("show", "main:web/CHANGELOG.md"); !strings.Contains(cl, "web layout") || strings.Contains(cl, "api endpoint") {
		t.Errorf("web/CHANGELOG.md:\n%s", cl)
	}
	if out, err := exec.Command("git", "-C", origin.dir, "cat-file", "-e", "main:CHANGELOG.md").CombinedOutput(); err == nil {
		t.Errorf("a repository-level CHANGELOG.md was written: %s", out)
	}
	for tag, want := range map[string]string{"api-v1.1.0": "api/CHANGELOG.md", "web-v1.0.1": "web/CHANGELOG.md"} {
		if files := origin.git("show", "--name-only", "--format=", tag); files != want {
			t.Errorf("release commit %s holds %q, want only %s", tag, files, want)
		}
	}
}

func TestIntegration_SingleProjectKeepsTheConfiguredChangelog(t *testing.T) {
	requireToolchain(t)
	r, origin := newOriginRepo(t)
	r.write("package.json", strings.NewReplacer(
		`"@semantic-release/changelog",`, `["@semantic-release/changelog", { "changelogFile": "docs/CHANGES.md" }],`,
		`{ "assets": ["CHANGELOG.md"] }`, `{ "assets": ["docs/CHANGES.md"] }`).Replace(releaseConfig))
	writeConfig(t, r.dir, "release:\n  projects:\n    - name: widgets\n      path: .\n")
	r.commit("chore: init", "a.txt")
	r.git("tag", "v1.0.0")
	r.commit("feat: add gadgets", "b.txt")
	r.git("push", "-q", "origin", "main", "--tags")

	got := releaseAll(t, r)
	if len(got) != 1 || got[0].NewReleaseGitTag != "v1.1.0" {
		t.Fatalf("released %+v", got)
	}
	if files := origin.git("show", "--name-only", "--format=", "v1.1.0"); files != "docs/CHANGES.md" {
		t.Errorf("release commit holds %q, want the configured docs/CHANGES.md only", files)
	}
}

func TestIntegration_BuiltInDefaultWhenTheRepositoryHasNone(t *testing.T) {
	requireToolchain(t)
	r, _ := newOriginRepo(t)
	r.commit("chore: init", "a.txt")
	r.git("tag", "v1.0.0")
	r.git("checkout", "-q", "-b", "develop")
	r.commit("feat: add gadgets", "b.txt")
	r.git("checkout", "-q", "-b", "release/1.0", "main")
	r.commit("fix: repair widgets", "c.txt")
	r.git("checkout", "-q", "main")
	r.git("push", "-q", "origin", "main", "develop", "release/1.0", "--tags")

	name := filepath.Base(r.dir)
	dev := sole(t, compute(t, Options{RepoRoot: r.dir, Branch: "develop"}), name)
	if dev.NewReleaseVersion != "1.1.0-alpha.1" || dev.NewReleaseChannel != "alpha" || dev.SemanticReleaseConfig != ConfigBuiltIn {
		t.Errorf("develop: %+v", dev)
	}
	rc := sole(t, compute(t, Options{RepoRoot: r.dir, Branch: "release/1.0"}), name)
	if rc.NewReleaseVersion != "1.0.1-rc.1" || rc.NewReleaseChannel != "beta" || rc.NewReleaseGitTag != "v1.0.1-rc.1" {
		t.Errorf("release/1.0: %+v", rc)
	}
	t.Logf("default: develop -> %s (%s), release/1.0 -> %s (%s)", dev.NewReleaseVersion, dev.NewReleaseChannel, rc.NewReleaseVersion, rc.NewReleaseChannel)

	// A second release branch on origin: the fixed id rc cannot serve both.
	r.git("checkout", "-q", "-b", "release/1.1", "main")
	r.commit("fix: another", "d.txt")
	r.git("push", "-q", "origin", "release/1.1")
	_, err := Compute(context.Background(), Options{RepoRoot: r.dir, Branch: "release/1.0", Env: testEnv(), CacheDir: integrationCache})
	if !errors.Is(err, ErrFixedPrereleaseGlob) || !strings.Contains(err.Error(), "release/1.0, release/1.1") {
		t.Errorf("two release branches: err = %v", err)
	}
}

// TestIntegration_ConfigDetectionForms: semantic-release's own search
// (through the runner's plan mode) finds every configuration form, and
// reports none for a package.json without "release".
func TestIntegration_ConfigDetectionForms(t *testing.T) {
	requireToolchain(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	e := &Engine{}
	env := testEnv()
	cacheDir, specs, err := e.prepareNode(ctx, env, nil, []project{{Name: "x"}}, integrationCache)
	if err != nil {
		t.Fatal(err)
	}
	scriptDir := t.TempDir()
	if err := writeRunner(scriptDir); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(scriptDir, runnerMain)
	jsonCfg := `{ "branches": ["main"] }`
	tests := []struct{ file, content, want string }{
		{"package.json", `{ "name": "x", "release": { "branches": ["main"] } }`, "package.json"},
		{".releaserc", jsonCfg, ".releaserc"},
		{".releaserc.json", jsonCfg, ".releaserc.json"},
		{".releaserc.yaml", "branches: [main]\n", ".releaserc.yaml"},
		{".releaserc.yml", "branches: [main]\n", ".releaserc.yml"},
		{".releaserc.js", `module.exports = { branches: ["main"] };`, ".releaserc.js"},
		{".releaserc.cjs", `module.exports = { branches: ["main"] };`, ".releaserc.cjs"},
		{".releaserc.mjs", `export default { branches: ["main"] };`, ".releaserc.mjs"},
		{".config/releaserc.json", jsonCfg, ".config/releaserc.json"},
		{"release.config.js", `module.exports = { branches: ["main"] };`, "release.config.js"},
		{"release.config.cjs", `module.exports = { branches: ["main"] };`, "release.config.cjs"},
		{"release.config.mjs", `export default { branches: ["main"] };`, "release.config.mjs"},
		{"package.json", `{ "name": "x" }`, ""},
		{"README.md", "nothing\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.file+" "+tt.want, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, filepath.FromSlash(tt.file))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := e.checkPlan(ctx, script, dir, env, cacheDir, specs)
			if err != nil {
				t.Fatal(err)
			}
			if filepath.ToSlash(got) != tt.want {
				t.Errorf("config file %q, want %q", got, tt.want)
			}
		})
	}
}
