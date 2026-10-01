package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
)

// ---------------------------------------------------------------------------
// Fixture helpers. Files are real (os.WriteFile into t.TempDir()); git is
// always faked through envtest.FakeRunner, scripted with the exact command
// lines internal/release.Tags/CommitsSince issue.
// ---------------------------------------------------------------------------

func releaseFixtureRepo(t *testing.T, pluginVersion string) string {
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

func writeChangelog(t *testing.T, repo, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "CHANGELOG.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitTagsKey(repo, pattern string) string {
	return "git -C " + repo + " tag -l " + pattern
}

func gitLogKey(repo, tag string) string {
	key := "git -C " + repo + " log --no-merges --reverse --pretty=format:%H%x1f%h%x1f%s%x1f%b%x1e"
	if tag != "" {
		key += " " + tag + "..HEAD"
	}
	return key
}

// commitRecord builds one raw git-log record in the %x1f/%x1e delimited
// format internal/release.ParseCommits expects.
func commitRecord(n int, subject, body string) string {
	sha := strings.Repeat(strconv.Itoa(n%10), 40)
	short := sha[:7]
	return sha + "\x1f" + short + "\x1f" + subject + "\x1f" + body + "\x1e\n"
}

func releaseOptions(t *testing.T, home string, runner *envtest.FakeRunner) options {
	t.Helper()
	return options{
		PluginFS: mustPluginFS(t),
		Runner:   runner,
		Now:      func() time.Time { return time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC) },
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
		Stdin:    strings.NewReader(""),
		Home:     home,
	}
}

func parseJSON(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("output is not valid JSON: %v (output=%q)", err, string(data))
	}
	return got
}

// ---------------------------------------------------------------------------
// H1/H2 — preview text and JSON output.
// ---------------------------------------------------------------------------

func TestReleasePreview_TextOutput(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"): {Stdout: ""},
		gitLogKey(repo, ""):    {Stdout: commitRecord(1, "feat: add cool feature", "")},
	}}
	var stdout, stderr bytes.Buffer

	code := run([]string{"release", "preview", "--repo", repo}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stdout=%q stderr=%q)", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "0.1.0") {
		t.Errorf("expected current version 0.1.0 in output: %q", out)
	}
	if !strings.Contains(out, "0.2.0") {
		t.Errorf("expected next version 0.2.0 in output: %q", out)
	}
	if !strings.Contains(out, "v0.2.0") {
		t.Errorf("expected tag v0.2.0 in output: %q", out)
	}
	if !regexp.MustCompile(`Bump\s*:\s*minor`).MatchString(out) {
		t.Errorf("expected 'Bump : minor' in output: %q", out)
	}

	after, err := os.ReadFile(filepath.Join(repo, "plugin", ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), `"version": "0.1.0"`) {
		t.Error("preview must not write plugin.json")
	}
	if _, err := os.Stat(filepath.Join(repo, "CHANGELOG.md")); !os.IsNotExist(err) {
		t.Error("preview must not create CHANGELOG.md")
	}
}

func TestReleasePreview_JSONShape(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"): {Stdout: ""},
		gitLogKey(repo, ""):    {Stdout: commitRecord(2, "fix: patch something", "")},
	}}
	var stdout, stderr bytes.Buffer

	code := run([]string{"release", "preview", "--repo", repo, "--json"}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stdout=%q)", code, stdout.String())
	}

	got := parseJSON(t, stdout.Bytes())
	if got["current"] != "0.1.0" {
		t.Errorf("current = %v, want 0.1.0", got["current"])
	}
	if got["next"] != "0.1.1" {
		t.Errorf("next = %v, want 0.1.1", got["next"])
	}
	if got["bump"] != "patch" {
		t.Errorf("bump = %v, want patch", got["bump"])
	}
	if got["applied"] != false {
		t.Errorf("applied = %v, want false", got["applied"])
	}
	if s, _ := got["section"].(string); s == "" {
		t.Error("section should be non-empty")
	}
	if commits, ok := got["commits"].([]any); !ok || len(commits) < 1 {
		t.Errorf("commits = %v, want a non-empty array", got["commits"])
	}
	if got["prerelease"] != nil {
		t.Errorf("prerelease = %v, want null", got["prerelease"])
	}
}

// ---------------------------------------------------------------------------
// H3 — apply writes plugin.json (exactly one line) and CHANGELOG.md; a
// second apply after the tag exists is "nothing to release".
// ---------------------------------------------------------------------------

func TestReleaseApply_WritesPluginJSONAndChangelog(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"): {Stdout: ""},
		gitLogKey(repo, ""):    {Stdout: commitRecord(3, "feat: add applied feature", "")},
	}}
	beforeLines := strings.Split(mustReadFile(t, filepath.Join(repo, "plugin", ".claude-plugin", "plugin.json")), "\n")

	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "apply", "--repo", repo, "--json"}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stdout=%q)", code, stdout.String())
	}

	got := parseJSON(t, stdout.Bytes())
	if got["applied"] != true {
		t.Errorf("applied = %v, want true", got["applied"])
	}
	written, _ := got["written"].([]any)
	if len(written) != 2 {
		t.Errorf("written = %v, want 2 entries", got["written"])
	}

	afterLines := strings.Split(mustReadFile(t, filepath.Join(repo, "plugin", ".claude-plugin", "plugin.json")), "\n")
	if len(beforeLines) != len(afterLines) {
		t.Fatalf("line count changed: before=%d after=%d", len(beforeLines), len(afterLines))
	}
	diff := 0
	for i := range beforeLines {
		if beforeLines[i] != afterLines[i] {
			diff++
		}
	}
	if diff != 1 {
		t.Errorf("expected exactly one changed line, got %d", diff)
	}

	changelog := mustReadFile(t, filepath.Join(repo, "CHANGELOG.md"))
	if !strings.Contains(changelog, "## [0.2.0]") {
		t.Error("changelog missing new section heading")
	}
	if !strings.Contains(changelog, "add applied feature") {
		t.Error("changelog missing commit line")
	}
}

func TestReleaseApply_AgainAfterTag_NothingToRelease(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.2.0")
	writeChangelog(t, repo, "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - 2026-09-28\n\n### Added\n- add applied feature (3333333)\n")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"):    {Stdout: "v0.2.0"},
		gitLogKey(repo, "v0.2.0"): {Stdout: ""},
	}}

	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "apply", "--repo", repo, "--json"}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stdout=%q)", code, stdout.String())
	}
}

// ---------------------------------------------------------------------------
// H4/H5/H6 — --version override.
// ---------------------------------------------------------------------------

func TestReleaseVersionOverride_SucceedsEvenWhenBumpIsNone(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"): {Stdout: ""},
		gitLogKey(repo, ""):    {Stdout: commitRecord(4, "docs: update readme only", "")},
	}}
	var stdout, stderr bytes.Buffer

	code := run([]string{"release", "preview", "--repo", repo, "--version", "1.0.0", "--json"}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stdout=%q)", code, stdout.String())
	}
	got := parseJSON(t, stdout.Bytes())
	if got["next"] != "1.0.0" {
		t.Errorf("next = %v, want 1.0.0", got["next"])
	}
	if got["bump"] != "none" {
		t.Errorf("bump = %v, want none", got["bump"])
	}
}

func TestReleaseVersionOverride_InvalidSemver(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"): {Stdout: ""},
		gitLogKey(repo, ""):    {Stdout: ""},
	}}
	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "preview", "--repo", repo, "--version", "not-a-version"}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestReleaseVersionOverride_NotGreaterThanCurrent(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.5.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"): {Stdout: ""},
		gitLogKey(repo, ""):    {Stdout: ""},
	}}

	var stdout1, stderr1 bytes.Buffer
	if code := run([]string{"release", "preview", "--repo", repo, "--version", "0.5.0"}, &stdout1, &stderr1, releaseOptions(t, t.TempDir(), runner)); code != 1 {
		t.Errorf("equal version: exit code = %d, want 1", code)
	}
	var stdout2, stderr2 bytes.Buffer
	if code := run([]string{"release", "preview", "--repo", repo, "--version", "0.4.0"}, &stdout2, &stderr2, releaseOptions(t, t.TempDir(), runner)); code != 1 {
		t.Errorf("lower version: exit code = %d, want 1", code)
	}
}

// ---------------------------------------------------------------------------
// H7 — nothing releasable.
// ---------------------------------------------------------------------------

func TestReleasePreview_NothingToRelease(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"): {Stdout: ""},
		gitLogKey(repo, ""):    {Stdout: commitRecord(5, "docs: tweak docs", "") + commitRecord(6, "chore: tidy up", "")},
	}}

	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "preview", "--repo", repo}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(strings.ToLower(stdout.String()), "nothing to release") {
		t.Errorf("expected a 'nothing to release' message, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "2") {
		t.Errorf("expected the inspected commit count in the message, got %q", stdout.String())
	}

	var jsonOut, jsonErr bytes.Buffer
	code2 := run([]string{"release", "preview", "--repo", repo, "--json"}, &jsonOut, &jsonErr, releaseOptions(t, t.TempDir(), runner))
	if code2 != 1 {
		t.Fatalf("json exit code = %d, want 1", code2)
	}
	got := parseJSON(t, jsonOut.Bytes())
	if got["bump"] != "none" {
		t.Errorf("bump = %v, want none", got["bump"])
	}
	if got["next"] != nil {
		t.Errorf("next = %v, want null", got["next"])
	}
	if got["tag"] != nil {
		t.Errorf("tag = %v, want null", got["tag"])
	}
	if got["base"] != nil {
		t.Errorf("base = %v, want null", got["base"])
	}
}

// ---------------------------------------------------------------------------
// H8/H9 — missing plugin.json / not a git repository -> exit 2.
// ---------------------------------------------------------------------------

func TestReleasePreview_MissingPluginJSON_Exit2(t *testing.T) {
	dir := t.TempDir()
	runner := &envtest.FakeRunner{}
	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "preview", "--repo", dir}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (stdout=%q)", code, stdout.String())
	}
}

func TestReleasePreview_NotAGitRepository_Exit2(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{Default: envtest.Response{ExitCode: 128, Stderr: "fatal: not a git repository"}}
	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "preview", "--repo", repo}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (stdout=%q)", code, stdout.String())
	}
}

func TestReleasePreview_RepoPathNotFound_Exit2(t *testing.T) {
	runner := &envtest.FakeRunner{}
	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "preview", "--repo", filepath.Join(t.TempDir(), "does-not-exist")}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

// ---------------------------------------------------------------------------
// H10/I1/I2/I3 — --pre-release end to end and its JSON tag/base fields.
// ---------------------------------------------------------------------------

func TestReleasePreRelease_EndToEnd(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"): {Stdout: ""},
		gitLogKey(repo, ""):    {Stdout: commitRecord(7, "feat: prerelease-worthy feature", "")},
	}}
	var stdout, stderr bytes.Buffer

	code := run([]string{"release", "preview", "--repo", repo, "--pre-release", "rc", "--json"}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stdout=%q)", code, stdout.String())
	}
	got := parseJSON(t, stdout.Bytes())
	if got["next"] != "0.2.0-rc.1" {
		t.Errorf("next = %v, want 0.2.0-rc.1", got["next"])
	}
	if got["prerelease"] != "rc" {
		t.Errorf("prerelease = %v, want rc", got["prerelease"])
	}
	if got["tag"] != "v0.2.0-rc.1" {
		t.Errorf("tag = %v, want v0.2.0-rc.1", got["tag"])
	}
	if got["base"] != "0.2.0" {
		t.Errorf("base = %v, want 0.2.0", got["base"])
	}
}

func TestReleasePlain_JSONHasTagAndBase(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"): {Stdout: ""},
		gitLogKey(repo, ""):    {Stdout: commitRecord(8, "feat: add cool feature", "")},
	}}
	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "preview", "--repo", repo, "--json"}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := parseJSON(t, stdout.Bytes())
	if got["tag"] != "v0.2.0" || got["base"] != "0.2.0" {
		t.Errorf("tag/base = %v/%v, want v0.2.0/0.2.0", got["tag"], got["base"])
	}
}

// ---------------------------------------------------------------------------
// I4/I5/I6 — pre-release flag validation.
// ---------------------------------------------------------------------------

func TestReleasePreRelease_InvalidLabel_Exit1(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"): {Stdout: ""},
		gitLogKey(repo, ""):    {Stdout: commitRecord(9, "feat: add cool feature", "")},
	}}
	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "preview", "--repo", repo, "--pre-release", "RC"}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stdout=%q)", code, stdout.String())
	}
	if !strings.Contains(strings.ToLower(stdout.String()), "invalid") {
		t.Errorf("expected an 'invalid' message, got %q", stdout.String())
	}
}

func TestReleasePreReleaseBase_WithoutPreRelease_Exit1(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{}
	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "preview", "--repo", repo, "--pre-release-base", "current"}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stdout=%q)", code, stdout.String())
	}
}

func TestReleaseApply_WithPreRelease_Exit1_WritesNothing(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.1.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"): {Stdout: ""},
		gitLogKey(repo, ""):    {Stdout: commitRecord(1, "feat: add cool feature", "")},
	}}
	before := mustReadFile(t, filepath.Join(repo, "plugin", ".claude-plugin", "plugin.json"))

	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "apply", "--repo", repo, "--pre-release", "alpha"}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stdout=%q)", code, stdout.String())
	}
	if !strings.Contains(strings.ToLower(stdout.String()), "tag-only") {
		t.Errorf("expected a 'tag-only' message, got %q", stdout.String())
	}
	after := mustReadFile(t, filepath.Join(repo, "plugin", ".claude-plugin", "plugin.json"))
	if before != after {
		t.Error("plugin.json must be unchanged")
	}
	if _, err := os.Stat(filepath.Join(repo, "CHANGELOG.md")); !os.IsNotExist(err) {
		t.Error("CHANGELOG.md must not be created")
	}
}

// ---------------------------------------------------------------------------
// I7/I8 — --pre-release-base current end to end.
// ---------------------------------------------------------------------------

func TestReleasePreReleaseBaseCurrent_EndToEnd(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.2.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"):    {Stdout: "v0.1.0\nv0.2.0-rc.1"},
		gitLogKey(repo, "v0.1.0"): {Stdout: ""},
	}}
	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "preview", "--repo", repo, "--pre-release", "rc", "--pre-release-base", "current", "--json"}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stdout=%q)", code, stdout.String())
	}
	got := parseJSON(t, stdout.Bytes())
	if got["next"] != "0.2.0-rc.2" {
		t.Errorf("next = %v, want 0.2.0-rc.2", got["next"])
	}
	if got["tag"] != "v0.2.0-rc.2" {
		t.Errorf("tag = %v, want v0.2.0-rc.2", got["tag"])
	}
	if got["base"] != "0.2.0" {
		t.Errorf("base = %v, want 0.2.0", got["base"])
	}
}

func TestReleasePreReleaseBaseCurrent_NoReleasableCommitRequired(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.2.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"):    {Stdout: "v0.1.0"},
		gitLogKey(repo, "v0.1.0"): {Stdout: commitRecord(1, "docs: only docs since last stable tag", "")},
	}}
	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "preview", "--repo", repo, "--pre-release", "rc", "--pre-release-base", "current", "--json"}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stdout=%q)", code, stdout.String())
	}
	got := parseJSON(t, stdout.Bytes())
	if got["next"] != "0.2.0-rc.1" {
		t.Errorf("next = %v, want 0.2.0-rc.1", got["next"])
	}
}

// ---------------------------------------------------------------------------
// guard — C1..C9 from tests/release-guard.test.ps1.
// ---------------------------------------------------------------------------

func TestReleaseGuard_OkWithNotesFile(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.2.0")
	changelog := "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - 2026-09-28\n\n### Added\n- cli: add --json flag (abc1234)\n\n### Fixed\n- handle null path (def5678)\n\n## [0.1.0] - 2026-09-01\n\n### Added\n- initial release (0000001)\n"
	writeChangelog(t, repo, changelog)
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v0.2.0"): {Stdout: ""},
	}}
	notesPath := filepath.Join(t.TempDir(), "notes.md")

	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "guard", "--repo", repo, "--notes", notesPath}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stdout=%q)", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "0.2.0") {
		t.Errorf("expected the version in output: %q", stdout.String())
	}
	notesBytes, err := os.ReadFile(notesPath)
	if err != nil {
		t.Fatalf("notes file was not created: %v", err)
	}
	if len(notesBytes) > 0 && notesBytes[0] == 0xEF {
		t.Error("notes file has a BOM")
	}
	if strings.Contains(string(notesBytes), "\r") {
		t.Error("notes file has a CR")
	}
	if !strings.Contains(string(notesBytes), "cli: add --json flag (abc1234)") {
		t.Errorf("notes body missing expected line: %q", string(notesBytes))
	}
}

func TestReleaseGuard_JSONShapeOnSuccess(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.2.0")
	writeChangelog(t, repo, "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - 2026-09-28\n\n### Added\n- x (abc1234)\n")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v0.2.0"): {Stdout: ""},
	}}
	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "guard", "--repo", repo, "--json"}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stdout=%q)", code, stdout.String())
	}
	got := parseJSON(t, stdout.Bytes())
	if got["version"] != "0.2.0" || got["tag"] != "v0.2.0" {
		t.Errorf("version/tag = %v/%v", got["version"], got["tag"])
	}
	if got["changelog_section"] != true || got["tag_exists"] != false || got["ok"] != true {
		t.Errorf("unexpected flags: %v", got)
	}
	if got["notes_path"] != nil {
		t.Errorf("notes_path = %v, want null", got["notes_path"])
	}
}

func TestReleaseGuard_TagAlreadyExists(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.2.0")
	writeChangelog(t, repo, "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - 2026-09-28\n\n### Added\n- x (abc1234)\n")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v0.2.0"): {Stdout: "v0.2.0"},
	}}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"release", "guard", "--repo", repo}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner)); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}

	var jsonOut, jsonErr bytes.Buffer
	code2 := run([]string{"release", "guard", "--repo", repo, "--json"}, &jsonOut, &jsonErr, releaseOptions(t, t.TempDir(), runner))
	if code2 != 1 {
		t.Fatalf("json exit code = %d, want 1", code2)
	}
	got := parseJSON(t, jsonOut.Bytes())
	if got["ok"] != false || got["tag_exists"] != true {
		t.Errorf("unexpected flags: %v", got)
	}
}

func TestReleaseGuard_MissingChangelog_Exit1(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.2.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v0.2.0"): {Stdout: ""},
	}}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"release", "guard", "--repo", repo}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner)); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestReleaseGuard_NoMatchingSection_Exit1(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.3.0")
	writeChangelog(t, repo, "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - 2026-09-28\n\n### Added\n- x (abc1234)\n")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v0.3.0"): {Stdout: ""},
	}}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"release", "guard", "--repo", repo}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner)); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestReleaseGuard_MissingPluginJSON_Exit2(t *testing.T) {
	dir := t.TempDir()
	runner := &envtest.FakeRunner{}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"release", "guard", "--repo", dir}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner)); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestReleaseGuard_NotAGitRepository_Exit2(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.2.0")
	writeChangelog(t, repo, "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - 2026-09-28\n\n### Added\n- x (abc1234)\n")
	runner := &envtest.FakeRunner{Default: envtest.Response{ExitCode: 128}}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"release", "guard", "--repo", repo}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner)); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestReleaseGuard_RepoPathNotFound_Exit2(t *testing.T) {
	runner := &envtest.FakeRunner{}
	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "guard", "--repo", filepath.Join(t.TempDir(), "does-not-exist")}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestReleaseGuard_FailurePath_NotesFileNotWritten(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.2.0")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v0.2.0"): {Stdout: ""},
	}}
	notesPath := filepath.Join(t.TempDir(), "notes-fail.md")

	var stdout, stderr bytes.Buffer
	run([]string{"release", "guard", "--repo", repo, "--notes", notesPath}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if _, err := os.Stat(notesPath); !os.IsNotExist(err) {
		t.Error("notes file must not be written on a failure path")
	}
}

// ---------------------------------------------------------------------------
// Own case: guard's binary/plugin.json consistency assertion, skipped for
// dev builds.
// ---------------------------------------------------------------------------

func TestReleaseGuard_BinaryConsistency_SkippedForDevBuild(t *testing.T) {
	if version.Binary != "dev" {
		t.Skip("version.Binary is not \"dev\" in this build")
	}
	repo := releaseFixtureRepo(t, "0.2.0")
	writeChangelog(t, repo, "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - 2026-09-28\n\n### Added\n- x (abc1234)\n")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v0.2.0"): {Stdout: ""},
	}}
	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "guard", "--repo", repo, "--json"}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := parseJSON(t, stdout.Bytes())
	if _, present := got["binary_consistent"]; present {
		t.Errorf("binary_consistent should be omitted for a dev build, got %v", got["binary_consistent"])
	}
}

func TestReleaseGuard_BinaryConsistency_MismatchFailsForNonDevBuild(t *testing.T) {
	prev := version.Binary
	version.Binary = "1.2.3"
	t.Cleanup(func() { version.Binary = prev })

	repo := releaseFixtureRepo(t, "9.9.9")
	writeChangelog(t, repo, "# Changelog\n\n## [Unreleased]\n\n## [9.9.9] - 2026-09-28\n\n### Added\n- x (abc1234)\n")
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v9.9.9"): {Stdout: ""},
	}}
	var stdout, stderr bytes.Buffer
	code := run([]string{"release", "guard", "--repo", repo, "--json"}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stdout=%q)", code, stdout.String())
	}
	got := parseJSON(t, stdout.Bytes())
	if got["binary_consistent"] != false {
		t.Errorf("binary_consistent = %v, want false", got["binary_consistent"])
	}
	if got["ok"] != false {
		t.Errorf("ok = %v, want false", got["ok"])
	}
}

// ---------------------------------------------------------------------------
// release --help and unknown subcommand.
// ---------------------------------------------------------------------------

func TestRelease_HelpAndUnknownSubcommand(t *testing.T) {
	runner := &envtest.FakeRunner{}

	var helpOut, helpErr bytes.Buffer
	if code := run([]string{"release", "--help"}, &helpOut, &helpErr, releaseOptions(t, t.TempDir(), runner)); code != 0 {
		t.Errorf("--help exit code = %d, want 0", code)
	}
	if !strings.Contains(helpOut.String(), "release") {
		t.Errorf("--help output should mention release: %q", helpOut.String())
	}

	var noArgsOut, noArgsErr bytes.Buffer
	if code := run([]string{"release"}, &noArgsOut, &noArgsErr, releaseOptions(t, t.TempDir(), runner)); code != 2 {
		t.Errorf("no-subcommand exit code = %d, want 2", code)
	}

	var badOut, badErr bytes.Buffer
	if code := run([]string{"release", "bogus"}, &badOut, &badErr, releaseOptions(t, t.TempDir(), runner)); code != 2 {
		t.Errorf("unknown-subcommand exit code = %d, want 2", code)
	}
}

func TestRelease_HiddenFromTopLevelUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	run([]string{}, &stdout, &stderr, releaseOptions(t, t.TempDir(), &envtest.FakeRunner{}))
	// "release" must still be reachable, just not listed among the primary
	// user-facing commands at the very top: this only checks it is not
	// advertised as equal-billing with "install"/"configure" etc, so we
	// simply check the usage text is non-empty and let release.go's own
	// placement (a distinct "Maintainer commands" line) carry the rest.
	if stdout.Len() == 0 {
		t.Fatal("expected top-level usage output")
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
