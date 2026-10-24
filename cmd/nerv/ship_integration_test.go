//go:build semrel_integration

// Integration tests of nerv ship and nerv pipeline release: real git, node,
// npm, semantic-release and bash, against a local bare repository as origin
// (never a real remote).
// Run with
//
//	go test -tags semrel_integration -run Integration -v ./cmd/nerv/
//
// They skip when git, node, npm or bash is missing or the npm registry is
// unreachable.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/artifact"
	"github.com/war-apps/nerv-by-gentle-ai/internal/semrel"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

func requireShipToolchain(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test")
	}
	for _, tool := range []string{"git", "node", "npm", "bash"} {
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

// shipEnv isolates git from the user's configuration and removes every
// release credential: nothing here may reach a real remote or registry.
func shipEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
		"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_NOSYSTEM": "1",
	} {
		t.Setenv(k, v)
	}
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN", "NPM_TOKEN", "GIT_CREDENTIALS", "NUGET_API_KEY"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

type gitDir struct {
	t   *testing.T
	dir string
}

func (g gitDir) git(args ...string) string {
	g.t.Helper()
	out, err := exec.Command("git", append([]string{"-C", g.dir}, args...)...).CombinedOutput()
	if err != nil {
		g.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (g gitDir) write(rel, content string) {
	g.t.Helper()
	p := filepath.Join(g.dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		g.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		g.t.Fatal(err)
	}
}

func (g gitDir) commit(message string, files ...string) {
	g.t.Helper()
	for _, f := range files {
		g.write(f, message+"\n")
	}
	g.git("add", "-A")
	g.git("commit", "-q", "-m", message)
}

// newShipRepos returns a bare origin (HEAD on main, as on a hosted origin)
// and a working clone of it on main.
func newShipRepos(t *testing.T) (origin, work gitDir) {
	t.Helper()
	origin = gitDir{t, filepath.Join(t.TempDir(), "origin.git")}
	if out, err := exec.Command("git", "init", "-q", "--bare", "-b", "main", origin.dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	work = gitDir{t, filepath.Join(t.TempDir(), "widgets")}
	if out, err := exec.Command("git", "init", "-q", "-b", "main", work.dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	work.git("remote", "add", "origin", origin.dir)
	return origin, work
}

const shipReleaseConfig = `{
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

// markerProject is a script-mode release: project whose stages and hooks
// append one line each to marker, with the NERV_RELEASE_* values they saw;
// its package stage writes <name>.txt to the outputs.
func markerProject(name, path, tagFormat, marker string) string {
	line := func(what string) string {
		return `{ run: 'echo "` + what + ` $NERV_RELEASE_PROJECT $NERV_RELEASE_VERSION $NERV_RELEASE_TAG dry=$NERV_RELEASE_DRY_RUN" >> ` + marker + `' }`
	}
	p := "    - name: " + name + "\n      path: " + path + "\n      build: { mode: script }\n"
	if tagFormat != "" {
		p += "      tag_format: \"" + tagFormat + "\"\n"
	}
	return p + `      stages:
        build: ` + line("build") + `
        package: { run: 'echo "$NERV_RELEASE_VERSION" > "$NERV_RELEASE_OUTPUTS_DIR/` + name + `.txt"' }
      hooks:
        prepare:
          - { run: 'test -s "$NERV_RELEASE_NOTES_FILE"' }
          - ` + line("prepare") + `
        fail: [` + line("fail") + `]
`
}

func shipOptions(t *testing.T, cache string) options {
	opts := defaultOptions()
	opts.Stdin = strings.NewReader("")
	opts.IsTerminal = func() bool { return false }
	opts.GitHubToken = func(context.Context) string { return "" }
	opts.OpenShipWorkspace = func(ctx context.Context, o semrel.WorkspaceOptions) (shipWorkspace, error) {
		o.CacheDir = cache
		return semrel.NewWorkspace(ctx, o)
	}
	return opts
}

func runShipCommand(t *testing.T, cache string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(append([]string{"ship"}, args...), &stdout, &stderr, shipOptions(t, cache))
	return code, stdout.String(), stderr.String()
}

func readMarker(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// refs are a repository's refs and their objects.
func refs(g gitDir) string { return g.git("for-each-ref", "--format=%(refname) %(objectname)") }

// readArtifact verifies the artifact of project's version in work.
func readArtifact(t *testing.T, work gitDir, project, version string) (*artifact.Manifest, string) {
	t.Helper()
	dir := artifact.Dir(work.dir, project, version)
	m, err := artifact.Verify(dir)
	if err != nil {
		t.Fatalf("artifact %s/%s: %v", project, version, err)
	}
	return m, dir
}

func TestIntegration_ShipPackagesWhatOriginHasAndNeverWritesToGit(t *testing.T) {
	requireShipToolchain(t)
	shipEnv(t)
	cache := t.TempDir()
	marker := filepath.Join(t.TempDir(), "marker.log")
	origin, work := newShipRepos(t)
	work.write("package.json", shipReleaseConfig)
	work.write(".ai-workflow.yml", "release:\n  projects:\n"+markerProject("widgets", ".", "", marker))
	work.commit("chore: init", "a.txt")
	work.git("tag", "v1.0.0")
	work.commit("feat: add gadgets", "b.txt")
	work.git("push", "-q", "origin", "main", "--tags")
	originBefore, workBefore := refs(origin), refs(work)

	// A dry run builds and runs the prepare hooks in the clone, nothing else.
	code, stdout, stderr := runShipCommand(t, cache, "--cwd", work.dir, "--dry-run")
	if code != 0 {
		t.Fatalf("dry run: exit %d\n%s", code, stderr)
	}
	t.Logf("dry run stderr:\n%s", stderr)
	want := []string{"build widgets 1.1.0 v1.1.0 dry=true", "prepare widgets 1.1.0 v1.1.0 dry=true"}
	if got := readMarker(t, marker); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("dry run marker %q, want %q", got, want)
	}
	if _, err := os.Stat(artifact.Root(work.dir)); !os.IsNotExist(err) || !strings.Contains(stdout, "dry run") {
		t.Errorf("dry run: artifacts %v, summary %s", err, stdout)
	}
	os.Remove(marker)

	// Not a terminal and no --yes: refused before anything runs.
	if code, _, stderr := runShipCommand(t, cache, "--cwd", work.dir); code != 1 || !strings.Contains(stderr, "--yes") {
		t.Errorf("without --yes: exit %d\n%s", code, stderr)
	}
	// An approval of another version is refused before anything runs.
	if code, _, stderr := runShipCommand(t, cache, "--cwd", work.dir, "--yes", "--expect", "widgets=1.0.1"); code != 1 || !strings.Contains(stderr, "expected 1.0.1, computed 1.1.0") {
		t.Errorf("--expect mismatch: exit %d\n%s", code, stderr)
	}
	if readMarker(t, marker) != nil {
		t.Error("steps ran without a confirmation or despite the --expect refusal")
	}

	code, stdout, stderr = runShipCommand(t, cache, "--cwd", work.dir, "--yes", "--expect", "widgets=1.1.0")
	if code != 0 {
		t.Fatalf("ship: exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
	t.Logf("summary: %s", stdout)
	want = []string{"build widgets 1.1.0 v1.1.0 dry=false", "prepare widgets 1.1.0 v1.1.0 dry=false"}
	if got := readMarker(t, marker); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("marker\n %q\nwant\n %q", got, want)
	}
	// No git write anywhere: origin and the user's refs are unchanged, the
	// checkout is clean (.nerv/artifacts ignores itself).
	if got := refs(origin); got != originBefore {
		t.Errorf("origin changed:\nbefore %s\nafter  %s", originBefore, got)
	}
	if got := refs(work); got != workBefore {
		t.Errorf("the user's refs changed:\nbefore %s\nafter  %s", workBefore, got)
	}
	if status := work.git("status", "--porcelain"); status != "" {
		t.Errorf("the user's checkout changed: %q", status)
	}
	m, dir := readArtifact(t, work, "widgets", "1.1.0")
	if m.Tag != "v1.1.0" || m.BaseCommit != origin.git("rev-parse", "main") || m.BuildMode != "script" || len(m.Outputs) != 1 || m.Outputs[0].Path != "outputs/widgets.txt" {
		t.Errorf("manifest %+v", m)
	}
	if len(m.ReleaseFiles) != 1 || m.ReleaseFiles[0].Path != "CHANGELOG.md" {
		t.Fatalf("release files %+v", m.ReleaseFiles)
	}
	if cl, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(m.ReleaseFiles[0].Stored))); !strings.Contains(string(cl), "add gadgets") {
		t.Errorf("stored CHANGELOG:\n%s", cl)
	}
	if !strings.Contains(stdout, "packaged in "+dir) {
		t.Errorf("summary does not print the artifact path: %s", stdout)
	}

	// The same version again: refused unless --force.
	if code, _, stderr := runShipCommand(t, cache, "--cwd", work.dir, "--yes"); code != 1 || !strings.Contains(stderr, "--force") {
		t.Errorf("rerun: exit %d\n%s", code, stderr)
	}
	if code, _, stderr := runShipCommand(t, cache, "--cwd", work.dir, "--yes", "--force"); code != 0 {
		t.Errorf("rerun --force: exit %d\n%s", code, stderr)
	}
	if got := refs(origin); got != originBefore {
		t.Errorf("origin changed by the forced run")
	}
}

func TestIntegration_ShipMonorepoInOrder(t *testing.T) {
	requireShipToolchain(t)
	shipEnv(t)
	cache := t.TempDir()
	marker := filepath.Join(t.TempDir(), "marker.log")
	origin, work := newShipRepos(t)
	work.write("package.json", shipReleaseConfig)
	work.write(".ai-workflow.yml", "release:\n  projects:\n"+
		markerProject("api", "api", "api-v${version}", marker)+
		markerProject("web", "web", "web-v${version}", marker)+
		markerProject("docs", "docs", "docs-v${version}", marker))
	work.commit("chore: init", "api/a.txt", "web/a.txt", "docs/a.txt")
	work.git("tag", "api-v1.0.0")
	work.git("tag", "web-v1.0.0")
	work.git("tag", "docs-v1.0.0")
	work.commit("feat: api endpoint", "api/b.txt")
	work.commit("fix: web layout", "web/b.txt")
	work.git("push", "-q", "origin", "main", "--tags")
	originBefore := refs(origin)

	code, stdout, stderr := runShipCommand(t, cache, "--cwd", work.dir, "--yes")
	if code != 0 {
		t.Fatalf("ship: exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
	t.Logf("summary: %s", stdout)
	readArtifact(t, work, "api", "1.1.0")
	m, _ := readArtifact(t, work, "web", "1.0.1")
	if len(m.ReleaseFiles) != 1 || m.ReleaseFiles[0].Path != "web/CHANGELOG.md" {
		t.Errorf("web release files %+v", m.ReleaseFiles)
	}
	if _, err := os.Stat(filepath.Join(artifact.Root(work.dir), "docs")); !os.IsNotExist(err) || !strings.Contains(stdout, "docs: no new release") {
		t.Errorf("docs was packaged (%v):\n%s", err, stdout)
	}
	if refs(origin) != originBefore {
		t.Error("origin changed")
	}
	got := readMarker(t, marker)
	if len(got) != 4 || !strings.HasPrefix(got[0], "build api 1.1.0 api-v1.1.0") || !strings.HasPrefix(got[2], "build web 1.0.1 web-v1.0.1") {
		t.Errorf("marker:\n%s", strings.Join(got, "\n"))
	}
}

// A dependent whose only change is a dependency's release is raised by
// depends_on when shipped alone; the dependency is computed, not packaged.
func TestIntegration_ShipProjectRaisedByDependsOn(t *testing.T) {
	requireShipToolchain(t)
	shipEnv(t)
	cache := t.TempDir()
	marker := filepath.Join(t.TempDir(), "marker.log")
	origin, work := newShipRepos(t)
	work.write("package.json", shipReleaseConfig)
	work.write(".ai-workflow.yml", "release:\n  projects:\n"+
		markerProject("core", "core", "core-v${version}", marker)+
		markerProject("app", "app", "app-v${version}", marker)+
		"      depends_on:\n        - project: core\n          bump: minor\n")
	work.commit("chore: init", "core/a.txt", "app/a.txt")
	work.git("tag", "core-v1.0.0")
	work.git("tag", "app-v1.0.0")
	work.commit("fix: core bug", "core/b.txt")
	work.git("push", "-q", "origin", "main", "--tags")

	code, stdout, stderr := runShipCommand(t, cache, "--cwd", work.dir, "--yes", "--project", "app")
	if code != 0 {
		t.Fatalf("ship --project app: exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
	readArtifact(t, work, "app", "1.1.0")
	if _, err := os.Stat(filepath.Join(artifact.Root(work.dir), "core")); !os.IsNotExist(err) {
		t.Errorf("core was packaged: %v", err)
	}
	if tags := origin.git("tag", "--list"); tags != "app-v1.0.0\ncore-v1.0.0" {
		t.Errorf("origin tags %q changed", tags)
	}
	if got := readMarker(t, marker); len(got) == 0 || !strings.HasPrefix(got[0], "build app 1.1.0 app-v1.1.0") {
		t.Errorf("marker:\n%s", strings.Join(got, "\n"))
	}
}

// The release: section is the user's local one: origin has none, the
// working tree has it uncommitted, and its hooks run.
func TestIntegration_ShipUsesTheLocalUncommittedSection(t *testing.T) {
	requireShipToolchain(t)
	shipEnv(t)
	cache := t.TempDir()
	marker := filepath.Join(t.TempDir(), "marker.log")
	origin, work := newShipRepos(t)
	work.write("package.json", shipReleaseConfig)
	work.commit("chore: init", "a.txt")
	work.git("tag", "v1.0.0")
	work.commit("feat: add gadgets", "b.txt")
	work.git("push", "-q", "origin", "main", "--tags")
	// Not committed: only the working tree has it.
	work.write(".ai-workflow.yml", "release:\n  projects:\n"+markerProject("widgets", ".", "", marker))
	if status := work.git("status", "--porcelain"); !strings.Contains(status, ".ai-workflow.yml") {
		t.Fatalf("the section should be uncommitted: %q", status)
	}

	code, stdout, stderr := runShipCommand(t, cache, "--cwd", work.dir, "--yes", "--expect", "widgets=1.1.0")
	if code != 0 {
		t.Fatalf("ship: exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
	readArtifact(t, work, "widgets", "1.1.0")
	got := readMarker(t, marker)
	if len(got) != 2 || !strings.HasPrefix(got[0], "build widgets 1.1.0 v1.1.0") || !strings.HasPrefix(got[1], "prepare widgets") {
		t.Errorf("the local section's stages and hooks did not run:\n%s", strings.Join(got, "\n"))
	}
	if out, err := exec.Command("git", "-C", origin.dir, "cat-file", "-e", "main:.ai-workflow.yml").CombinedOutput(); err == nil {
		t.Errorf("the local section reached origin: %s", out)
	}
}

// A repository without a semantic-release configuration computes with the
// built-in one (dim's): develop gets X.Y.Z-alpha.1 on the alpha channel.
func TestIntegration_ShipWithTheBuiltInConfiguration(t *testing.T) {
	requireShipToolchain(t)
	shipEnv(t)
	cache := t.TempDir()
	origin, work := newShipRepos(t)
	work.write(".ai-workflow.yml", "release:\n  projects:\n    - name: widgets\n      path: .\n      build: { mode: script }\n      stages:\n        build: { run: 'true' }\n")
	work.commit("chore: init", "a.txt")
	work.git("tag", "v1.0.0")
	work.git("checkout", "-q", "-b", "develop")
	work.commit("feat: add gadgets", "b.txt")
	work.git("push", "-q", "origin", "main", "develop", "--tags")
	originBefore := refs(origin)

	code, stdout, stderr := runShipCommand(t, cache, "--cwd", work.dir, "--yes", "--json")
	if code != 0 {
		t.Fatalf("ship: exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"semantic_release_config":"default"`) || !strings.Contains(stdout, `"status":"packaged"`) || !strings.Contains(stderr, "built-in default (dim)") {
		t.Errorf("source not reported:\nstdout %s", stdout)
	}
	m, _ := readArtifact(t, work, "widgets", "1.1.0-alpha.1")
	if m.Tag != "v1.1.0-alpha.1" || m.Channel != "alpha" || m.Branch != "develop" || m.BaseCommit != origin.git("rev-parse", "develop") {
		t.Errorf("manifest %+v", m)
	}
	if refs(origin) != originBefore {
		t.Error("origin changed")
	}
}

// nerv pipeline release with the built-in semantic-release configuration
// and a version file: dry run, release on develop, then nothing to release.
func TestIntegration_PipelineReleaseWithTheBuiltInConfiguration(t *testing.T) {
	requireShipToolchain(t)
	shipEnv(t)
	cache := t.TempDir()
	origin, work := newShipRepos(t)
	work.write("src/Api/Api.csproj", apiCsproj)
	work.commit("chore: init", "a.txt")
	work.git("tag", "v1.0.0")
	work.git("checkout", "-q", "-b", "develop")
	work.commit("feat: add gadgets", "b.txt")
	work.git("push", "-q", "origin", "main", "develop", "--tags")
	// The pipeline: section is local (untracked).
	work.write(".ai-workflow.yml", "pipeline:\n  dotnet:\n    solution: App.slnx\n  images:\n    - name: ghcr.io/acme/app\n  release:\n    version_files: [src/Api/Api.csproj]\n")
	originBefore := origin.git("for-each-ref", "--format=%(refname) %(objectname)")
	outputs := filepath.Join(work.dir, ".nerv", "artifacts", "pipeline", "release.json")
	readJSON := func() map[string]any {
		t.Helper()
		data, err := os.ReadFile(outputs)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	runRelease := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := run(append([]string{"pipeline", "release", "--cwd", work.dir}, args...), &stdout, &stderr, shipOptions(t, cache))
		return code, stdout.String(), stderr.String()
	}

	code, _, stderr := runRelease("--dry-run")
	if code != 0 {
		t.Fatalf("dry run: exit %d\n%s", code, stderr)
	}
	if got := origin.git("for-each-ref", "--format=%(refname) %(objectname)"); got != originBefore {
		t.Errorf("the dry run changed origin")
	}
	if m := readJSON(); m["dry_run"] != true || m["published"] != true || m["version"] != "1.1.0-alpha.1" || m["semantic_release_config"] != "default" {
		t.Errorf("dry-run release.json %v", m)
	}

	code, stdout, stderr := runRelease("--yes", "--json", "--expect", filepath.Base(work.dir)+"=1.1.0-alpha.1")
	if code != 0 {
		t.Fatalf("release: exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
	head := origin.git("rev-parse", "develop")
	if origin.git("rev-parse", "v1.1.0-alpha.1^{commit}") != head {
		t.Errorf("tag v1.1.0-alpha.1 is not origin's develop")
	}
	if files := origin.git("show", "--name-only", "--format=", "v1.1.0-alpha.1"); files != "CHANGELOG.md\nsrc/Api/Api.csproj" {
		t.Errorf("release commit holds %q, want the CHANGELOG and the version file", files)
	}
	if cs := origin.git("show", "develop:src/Api/Api.csproj"); !strings.Contains(cs, "<Version>1.1.0-alpha.1</Version>") {
		t.Errorf("version file on origin:\n%s", cs)
	}
	if !strings.Contains(stderr, "[pipeline:release] nerv pipeline release: warning: origin has no public address") {
		t.Errorf("no warning for the release link on a local origin:\n%s", stderr)
	}
	m := readJSON()
	if m["published"] != true || m["dry_run"] != false || m["version"] != "1.1.0-alpha.1" || m["channel"] != "alpha" || m["git_tag"] != "v1.1.0-alpha.1" || m["git_head"] != head {
		t.Errorf("release.json %v", m)
	}
	if !strings.Contains(stdout, `"git_tag":"v1.1.0-alpha.1"`) {
		t.Errorf("--json stdout %s", stdout)
	}

	// Pull the release commit; nothing new to release.
	work.git("pull", "-q", "origin", "develop")
	code, _, stderr = runRelease("--yes")
	if m := readJSON(); code != 0 || m["published"] != false || m["last_version"] != "1.1.0-alpha.1" {
		t.Errorf("no release: exit %d, %v\n%s", code, m, stderr)
	}
}

// With the repository's own configuration, version files its
// @semantic-release/git assets do not list are refused before anything is
// released; once listed, they enter the release commit.
func TestIntegration_PipelineReleaseRepositoryConfigAssets(t *testing.T) {
	requireShipToolchain(t)
	shipEnv(t)
	cache := t.TempDir()
	origin, work := newShipRepos(t)
	work.write("package.json", shipReleaseConfig) // git assets: CHANGELOG.md only
	work.write("src/Api/Api.csproj", apiCsproj)
	work.commit("chore: init", "a.txt")
	work.git("tag", "v1.0.0")
	work.commit("feat: add gadgets", "b.txt")
	work.git("push", "-q", "origin", "main", "--tags")
	work.write(".ai-workflow.yml", "pipeline:\n  dotnet:\n    solution: App.slnx\n  images:\n    - name: ghcr.io/acme/app\n  release:\n    version_files: [src/Api/Api.csproj]\n")
	runRelease := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := run(append([]string{"pipeline", "release", "--cwd", work.dir}, args...), &stdout, &stderr, shipOptions(t, cache))
		return code, stdout.String(), stderr.String()
	}

	code, stdout, stderr := runRelease("--yes", "--json")
	if code != 1 || !strings.Contains(stdout, `"status":"refused"`) || !strings.Contains(stdout, `src/Api/Api.csproj`) || !strings.Contains(stdout, "package.json") {
		t.Fatalf("uncovered: exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
	if tags := origin.git("tag", "--list"); tags != "v1.0.0" {
		t.Errorf("tagged despite the refusal: %q", tags)
	}

	work.write("package.json", strings.Replace(shipReleaseConfig, `{ "assets": ["CHANGELOG.md"] }`, `{ "assets": ["CHANGELOG.md", "src/**/*.csproj"] }`, 1))
	work.commit("chore: commit the csproj in releases")
	work.git("push", "-q", "origin", "main")
	if code, stdout, stderr := runRelease("--yes", "--json"); code != 0 {
		t.Fatalf("covered: exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
	if files := origin.git("show", "--name-only", "--format=", "v1.1.0"); files != "CHANGELOG.md\nsrc/Api/Api.csproj" {
		t.Errorf("release commit holds %q", files)
	}
}
