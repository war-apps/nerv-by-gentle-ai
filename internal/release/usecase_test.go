package release_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-gentle-ai/internal/release"
)

// ---------------------------------------------------------------------------
// Fixture helpers, mirroring cmd/nerv/release_test.go's: real files under
// t.TempDir(), git always faked through envtest.FakeRunner.
// ---------------------------------------------------------------------------

func fixtureRepo(t *testing.T, pluginVersion string) string {
	t.Helper()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugin", ".claude-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("{\n  \"name\": \"nerv-release-fixture\",\n  \"version\": \"%s\",\n  \"description\": \"x\",\n  \"author\": { \"name\": \"Test\" }\n}\n", pluginVersion)
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func tagsKey(repo, pattern string) string {
	return "git -C " + repo + " tag -l " + pattern
}

func logKey(repo, tag string) string {
	key := "git -C " + repo + " log --no-merges --reverse --pretty=format:%H%x1f%h%x1f%s%x1f%b%x1e"
	if tag != "" {
		key += " " + tag + "..HEAD"
	}
	return key
}

func oneCommit(n int, subject string) string {
	sha := strings.Repeat(strconv.Itoa(n%10), 40)
	return sha + "\x1f" + sha[:7] + "\x1f" + subject + "\x1f" + "\x1e\n"
}

func testDeps(repo string, runner *envtest.FakeRunner) release.Deps {
	return release.Deps{
		Repo:   repo,
		Runner: runner,
		Now:    func() time.Time { return time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC) },
	}
}

// ---------------------------------------------------------------------------
// Preview — version/bump computation.
// ---------------------------------------------------------------------------

func TestPreview_ComputesMinorBumpAndTag(t *testing.T) {
	repo := fixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		tagsKey(repo, "v*"): {Stdout: ""},
		logKey(repo, ""):    {Stdout: oneCommit(1, "feat: add cool feature")},
	}}

	result, err := release.Preview(testDeps(repo, runner), release.PreviewOptions{})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if result.Bump != "minor" {
		t.Errorf("Bump = %q, want minor", result.Bump)
	}
	if result.Next == nil || *result.Next != "0.2.0" {
		t.Errorf("Next = %v, want 0.2.0", result.Next)
	}
	if result.Tag == nil || *result.Tag != "v0.2.0" {
		t.Errorf("Tag = %v, want v0.2.0", result.Tag)
	}
	if result.Base == nil || *result.Base != "0.2.0" {
		t.Errorf("Base = %v, want 0.2.0", result.Base)
	}
}

func TestPreview_NothingReleasable_ReportsInspectedCount(t *testing.T) {
	repo := fixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		tagsKey(repo, "v*"): {Stdout: ""},
		logKey(repo, ""):    {Stdout: oneCommit(1, "docs: tweak docs") + oneCommit(2, "chore: tidy up")},
	}}

	result, err := release.Preview(testDeps(repo, runner), release.PreviewOptions{})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if result.Next != nil {
		t.Errorf("Next = %v, want nil", result.Next)
	}
	if result.InspectedCommits != 2 {
		t.Errorf("InspectedCommits = %d, want 2", result.InspectedCommits)
	}
	if result.Since != "the repository root" {
		t.Errorf("Since = %q, want %q", result.Since, "the repository root")
	}
}

func TestPreview_VersionOverride_MustBeGreater(t *testing.T) {
	repo := fixtureRepo(t, "0.5.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		tagsKey(repo, "v*"): {Stdout: ""},
		logKey(repo, ""):    {Stdout: ""},
	}}

	_, err := release.Preview(testDeps(repo, runner), release.PreviewOptions{Version: "0.4.0"})
	if err == nil {
		t.Fatal("expected an error for a --version not greater than current")
	}
	if !refusal.Is(err) {
		t.Errorf("error = %v (%T), want a *refusal.Error", err, err)
	}
}

func TestPreview_PreReleaseLabel_ComputesRcOne(t *testing.T) {
	repo := fixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		tagsKey(repo, "v*"): {Stdout: ""},
		logKey(repo, ""):    {Stdout: oneCommit(1, "feat: prerelease-worthy feature")},
	}}

	result, err := release.Preview(testDeps(repo, runner), release.PreviewOptions{PreRelease: "rc"})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if result.Next == nil || *result.Next != "0.2.0-rc.1" {
		t.Errorf("Next = %v, want 0.2.0-rc.1", result.Next)
	}
	if result.PreRelease == nil || *result.PreRelease != "rc" {
		t.Errorf("PreRelease = %v, want rc", result.PreRelease)
	}
}

func TestPreview_RepositoryPathNotFound(t *testing.T) {
	runner := &envtest.FakeRunner{}
	_, err := release.Preview(testDeps(filepath.Join(t.TempDir(), "missing"), runner), release.PreviewOptions{})
	if err == nil {
		t.Fatal("expected an error for a missing repository path")
	}
	if refusal.Is(err) {
		t.Error("repository-not-found should not be a refusal (it maps to exit 2, an environment error)")
	}
}

// ---------------------------------------------------------------------------
// Apply — writes plugin.json and CHANGELOG.md.
// ---------------------------------------------------------------------------

func TestApply_WritesPluginJSONAndChangelog(t *testing.T) {
	repo := fixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		tagsKey(repo, "v*"): {Stdout: ""},
		logKey(repo, ""):    {Stdout: oneCommit(1, "feat: add applied feature")},
	}}

	result, err := release.Apply(testDeps(repo, runner), release.PreviewOptions{})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.Applied {
		t.Fatal("Applied = false, want true")
	}
	if len(result.Written) != 2 {
		t.Fatalf("Written = %v, want 2 entries", result.Written)
	}

	pluginJSON, err := os.ReadFile(result.Written[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pluginJSON), `"version": "0.2.0"`) {
		t.Errorf("plugin.json not updated: %q", pluginJSON)
	}

	changelog, err := os.ReadFile(result.Written[1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(changelog), "## [0.2.0]") {
		t.Error("changelog missing new section heading")
	}
	if !strings.Contains(string(changelog), "add applied feature") {
		t.Error("changelog missing commit line")
	}
}

func TestApply_NothingReleasable_WritesNothing(t *testing.T) {
	repo := fixtureRepo(t, "0.2.0")
	writeChangelogFixture(t, repo, "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - 2026-09-28\n\n### Added\n- x (0000000)\n")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		tagsKey(repo, "v*"):    {Stdout: "v0.2.0"},
		logKey(repo, "v0.2.0"): {Stdout: ""},
	}}

	result, err := release.Apply(testDeps(repo, runner), release.PreviewOptions{})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Applied {
		t.Error("Applied = true, want false (nothing releasable)")
	}
}

func writeChangelogFixture(t *testing.T, repo, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "CHANGELOG.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Guard — readiness plus the notes file.
// ---------------------------------------------------------------------------

func TestGuard_OkWritesNotesFile(t *testing.T) {
	repo := fixtureRepo(t, "0.2.0")
	writeChangelogFixture(t, repo, "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - 2026-09-28\n\n### Added\n- cli: add --json flag (abc1234)\n")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		tagsKey(repo, "v0.2.0"): {Stdout: ""},
	}}
	notesPath := filepath.Join(t.TempDir(), "notes.md")

	result, err := release.Guard(testDeps(repo, runner), release.GuardOptions{NotesPath: notesPath})
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}
	if !result.Ok {
		t.Fatalf("Ok = false, want true (reason=%q)", result.Reason)
	}
	notes, err := os.ReadFile(notesPath)
	if err != nil {
		t.Fatalf("notes file not written: %v", err)
	}
	if !strings.Contains(string(notes), "cli: add --json flag (abc1234)") {
		t.Errorf("notes body missing expected line: %q", notes)
	}
}

func TestGuard_TagAlreadyExists_NotOkNoNotesFile(t *testing.T) {
	repo := fixtureRepo(t, "0.2.0")
	writeChangelogFixture(t, repo, "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - 2026-09-28\n\n### Added\n- x (abc1234)\n")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		tagsKey(repo, "v0.2.0"): {Stdout: "v0.2.0"},
	}}
	notesPath := filepath.Join(t.TempDir(), "notes.md")

	result, err := release.Guard(testDeps(repo, runner), release.GuardOptions{NotesPath: notesPath})
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}
	if result.Ok {
		t.Error("Ok = true, want false (tag already exists)")
	}
	if !result.TagExists {
		t.Error("TagExists = false, want true")
	}
	if _, statErr := os.Stat(notesPath); !os.IsNotExist(statErr) {
		t.Error("notes file must not be written on a failure path")
	}
}

func TestGuard_MissingPluginJSON_ReturnsEnvironmentError(t *testing.T) {
	dir := t.TempDir()
	runner := &envtest.FakeRunner{}
	_, err := release.Guard(testDeps(dir, runner), release.GuardOptions{})
	if err == nil {
		t.Fatal("expected an error for a missing plugin.json")
	}
	if refusal.Is(err) {
		t.Error("missing plugin.json should not be a refusal (it maps to exit 2)")
	}
}
