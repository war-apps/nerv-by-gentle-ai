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
