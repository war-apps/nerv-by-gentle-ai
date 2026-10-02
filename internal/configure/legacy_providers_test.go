package configure_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
)

// legacyProviderLines are the stub provider sub-blocks that releases before
// the removal of the github-projects and jira providers wrote into
// tasks.providers. Users upgrading still carry them in their nerv.yaml, and
// every write path must leave them (and the comment above them) alone.
const legacyProviderLines = "" +
	"    # legacy stubs, kept by hand\n" +
	"    github-projects: { task_ref_prefix: gh, owner: \"\", project_number: 0 }    # later\n" +
	"    jira: { task_ref_prefix: jira, site: \"\", project_key: \"\" }               # later\n"

// legacyRichFixture is richFixtureLF with the legacy blocks between the end of
// providers.teamwork (known_projects) and the sibling tasks.sources key.
func legacyRichFixture(t *testing.T) string {
	t.Helper()
	return withLegacyBlocks(t, richFixtureLF, "  sources:                          # extra work sources")
}

// legacyInlineFixture is inlineFixtureLF with the legacy blocks appended right
// after the teamwork stages line, which is the last line of the file.
func legacyInlineFixture() string {
	return inlineFixtureLF + legacyProviderLines
}

func withLegacyBlocks(t *testing.T, fixture, anchor string) string {
	t.Helper()
	if strings.Count(fixture, anchor) != 1 {
		t.Fatalf("anchor %q must appear exactly once in the fixture", anchor)
	}
	return strings.Replace(fixture, anchor, legacyProviderLines+anchor, 1)
}

// asLegacyJira rewrites tasks.provider to jira, the value PR #50 removed from
// the allowed set, keeping the line's comment.
func asLegacyJira(t *testing.T, fixture string) string {
	t.Helper()
	const from = "  provider: teamwork                # teamwork | none"
	if strings.Count(fixture, from) != 1 {
		t.Fatalf("provider line must appear exactly once in the fixture")
	}
	return strings.Replace(fixture, from, "  provider: jira                    # teamwork | none", 1)
}

func replaceOnce(t *testing.T, s, old, new string) string {
	t.Helper()
	if strings.Count(s, old) != 1 {
		t.Fatalf("%q must appear exactly once", old)
	}
	return strings.Replace(s, old, new, 1)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func setupLegacy(t *testing.T, content string) (configure.Deps, configure.Paths, string) {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, content)
	return newTestDeps(dir, time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)), configure.Paths{Config: configPath}, configPath
}

// ---------------------------------------------------------------------------
// --set: the legacy blocks survive byte for byte while the targeted keys,
// including the teamwork keys that sit right next to them, are updated.
// ---------------------------------------------------------------------------

func TestSet_LegacyProviderBlocks_SurviveBytesIdentical(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
	}{
		{"after-known-projects", legacyRichFixture(t)},
		{"after-teamwork-stages-at-eof", legacyInlineFixture()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps, paths, configPath := setupLegacy(t, tc.fixture)

			result, err := configure.Set(deps, paths, []string{
				"git.base_branch=develop2",
				"tasks.providers.teamwork.stages.blocked=BLOCKED",
				"tasks.providers.teamwork.default_tasklist_id=42",
			})
			if err != nil {
				t.Fatalf("Set() error = %v", err)
			}
			if !result.Changed || len(result.Changes) != 3 {
				t.Fatalf("Changed = %v, Changes = %+v; want 3 changes", result.Changed, result.Changes)
			}

			after := readFile(t, configPath)
			if !strings.Contains(after, legacyProviderLines) {
				t.Errorf("legacy blocks not intact:\n%s", after)
			}
			for _, want := range []string{"base_branch: develop2", "blocked: BLOCKED", "default_tasklist_id: 42"} {
				if !strings.Contains(after, want) {
					t.Errorf("output missing %q:\n%s", want, after)
				}
			}
			if got := diffLineCount(tc.fixture, after); got != 3 {
				t.Errorf("diff line count = %d, want 3:\n%s", got, after)
			}
		})
	}
}

// A no-op --set on a legacy file writes nothing at all: no rewrite, no backup.
func TestSet_LegacyProviderBlocks_NoopLeavesFileUntouched(t *testing.T) {
	fixture := legacyRichFixture(t)
	deps, paths, configPath := setupLegacy(t, fixture)

	result, err := configure.Set(deps, paths, []string{"git.base_branch=develop"})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if result.Changed || result.Backup != nil {
		t.Errorf("Changed = %v, Backup = %v; want a no-op", result.Changed, result.Backup)
	}
	if got := readFile(t, configPath); got != fixture {
		t.Errorf("file mutated by a no-op --set:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// --set-model: add, then clear, an override; the legacy blocks survive and the
// clear returns the file to the exact original bytes.
// ---------------------------------------------------------------------------

func TestSetModel_LegacyProviderBlocks_SurviveAddAndClear(t *testing.T) {
	fixture := legacyRichFixture(t)
	deps, paths, configPath := setupLegacy(t, fixture)

	if _, err := configure.SetModel(deps, paths, []string{"hyuga=opus/xhigh"}); err != nil {
		t.Fatalf("SetModel(add) error = %v", err)
	}
	added := readFile(t, configPath)
	if !strings.Contains(added, "hyuga: { model: opus, effort: xhigh }") {
		t.Fatalf("override not written:\n%s", added)
	}
	if !strings.Contains(added, legacyProviderLines) {
		t.Errorf("legacy blocks lost after set-model add:\n%s", added)
	}

	if _, err := configure.SetModel(deps, paths, []string{"hyuga=default"}); err != nil {
		t.Fatalf("SetModel(clear) error = %v", err)
	}
	// The models: block is regenerated by --set-model (sorted, new header
	// comment), so only everything after it must match the original bytes.
	const tail = "critical_paths:"
	got := readFile(t, configPath)
	wantAt, gotAt := strings.Index(fixture, tail), strings.Index(got, tail)
	if wantAt < 0 {
		t.Fatalf("fixture has no %q to compare from", tail)
	}
	if gotAt < 0 || got[gotAt:] != fixture[wantAt:] {
		t.Errorf("content after models: changed by set-model:\ngot:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// A legacy tasks.provider: jira. Reading it must not fail, every rewrite of
// another key keeps the line exactly as written, setting jira again is
// refused, and moving to a supported provider changes only that line.
// ---------------------------------------------------------------------------

func TestPrint_LegacyJiraProvider_ReadsAsWritten(t *testing.T) {
	_, paths, _ := setupLegacy(t, asLegacyJira(t, legacyRichFixture(t)))

	result, err := configure.Print(printDeps(t.TempDir()), paths)
	if err != nil {
		t.Fatalf("Print() error = %v", err)
	}
	found := false
	for _, entry := range result.Values {
		if entry.Key == "tasks.provider" {
			found = true
			if entry.Value != "jira" {
				t.Errorf("tasks.provider = %q, want the legacy value jira", entry.Value)
			}
		}
	}
	if !found {
		t.Error("tasks.provider missing from the printed values")
	}
}

func TestSet_LegacyJiraProvider_OtherKeysKeepProviderAndBlocks(t *testing.T) {
	fixture := asLegacyJira(t, legacyRichFixture(t))
	deps, paths, configPath := setupLegacy(t, fixture)

	if _, err := configure.Set(deps, paths, []string{"git.worktree=always"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	after := readFile(t, configPath)
	if !strings.Contains(after, "  provider: jira                    # teamwork | none") {
		t.Errorf("legacy provider line changed:\n%s", after)
	}
	if !strings.Contains(after, legacyProviderLines) {
		t.Errorf("legacy blocks lost:\n%s", after)
	}
	if got := diffLineCount(fixture, after); got != 1 {
		t.Errorf("diff line count = %d, want 1:\n%s", got, after)
	}

	if _, err := configure.SetModel(deps, paths, []string{"hyuga=opus/xhigh"}); err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	after = readFile(t, configPath)
	if !strings.Contains(after, "  provider: jira ") || !strings.Contains(after, legacyProviderLines) {
		t.Errorf("set-model altered the legacy provider or blocks:\n%s", after)
	}
}

func TestSet_LegacyJiraProvider_RefusesJiraAndAllowsMovingAway(t *testing.T) {
	fixture := asLegacyJira(t, legacyRichFixture(t))
	deps, paths, configPath := setupLegacy(t, fixture)

	_, err := configure.Set(deps, paths, []string{"tasks.provider=jira"})
	var refusal *configure.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("Set(provider=jira) error = %v, want *RefusalError", err)
	}
	if got := readFile(t, configPath); got != fixture {
		t.Errorf("a refused --set touched the file:\n%s", got)
	}

	if _, err := configure.Set(deps, paths, []string{"tasks.provider=teamwork"}); err != nil {
		t.Fatalf("Set(provider=teamwork) error = %v", err)
	}
	after := readFile(t, configPath)
	if !strings.Contains(after, "  provider: teamwork") || strings.Contains(after, "provider: jira") {
		t.Errorf("provider not moved to teamwork:\n%s", after)
	}
	if !strings.Contains(after, legacyProviderLines) {
		t.Errorf("legacy blocks lost:\n%s", after)
	}
	if got := diffLineCount(fixture, after); got != 1 {
		t.Errorf("diff line count = %d, want 1:\n%s", got, after)
	}
}

// ---------------------------------------------------------------------------
// --init-repo never rewrites an existing project config: one that still
// carries the legacy provider is left byte for byte, with a warning.
// ---------------------------------------------------------------------------

func TestInitRepo_ExistingLegacyJiraProjectConfig_LeftUntouched(t *testing.T) {
	repoDir := t.TempDir()
	projectPath := filepath.Join(repoDir, ".nerv", "nerv.yaml")
	legacy := "enabled: true\ntasks:\n  provider: jira   # legacy\n  providers:\n" + legacyProviderLines
	writeFixture(t, projectPath, legacy)
	deps := configure.Deps{Runner: gitToplevelRunner(repoDir), Now: func() time.Time { return time.Now() }}

	result, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: repoDir, Provider: "teamwork"})
	if err != nil {
		t.Fatalf("InitRepo() error = %v", err)
	}
	if result.Changed || len(result.Warnings) != 1 {
		t.Errorf("Changed = %v, Warnings = %v; want untouched with one warning", result.Changed, result.Warnings)
	}
	if got := readFile(t, projectPath); got != legacy {
		t.Errorf("existing project config rewritten:\n%s", got)
	}
}
