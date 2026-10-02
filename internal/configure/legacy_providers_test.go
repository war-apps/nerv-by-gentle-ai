package configure_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
)

// legacyProviderLines are the stub provider sub-blocks that releases before
// the removal of the github-projects and jira providers wrote into
// tasks.providers. Users upgrading still carry them in their nerv.yaml, and
// every write path must strip them (with the comment above them) and nothing
// else.
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

const (
	removedGithub   = "tasks.providers.github-projects"
	removedJira     = "tasks.providers.jira"
	removedProvider = "tasks.provider (jira)"
)

// withoutLegacyBlocks is fixture with the legacy sub-blocks removed.
func withoutLegacyBlocks(t *testing.T, fixture string) string {
	t.Helper()
	return replaceOnce(t, fixture, legacyProviderLines, "")
}

// withoutLegacyProvider is a legacy-jira fixture with the provider line and the
// legacy sub-blocks removed: what any write must leave behind.
func withoutLegacyProvider(t *testing.T, fixture string) string {
	t.Helper()
	lines := strings.SplitAfter(withoutLegacyBlocks(t, fixture), "\n")
	kept := make([]string, 0, len(lines))
	for _, l := range lines {
		if !strings.HasPrefix(l, "  provider: jira") {
			kept = append(kept, l)
		}
	}
	if len(kept) != len(lines)-1 {
		t.Fatalf("want exactly one provider: jira line in the fixture")
	}
	return strings.Join(kept, "")
}

func assertRemoved(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("Removed = %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// --set: the legacy blocks are stripped while the targeted keys, including the
// teamwork keys that sit right next to them, are updated and every other byte
// stays as written.
// ---------------------------------------------------------------------------

func TestSet_LegacyProviderBlocks_AreStripped(t *testing.T) {
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
			assertRemoved(t, result.Removed, removedGithub, removedJira)

			after := readFile(t, configPath)
			for _, gone := range []string{"github-projects", "jira: {", "legacy stubs"} {
				if strings.Contains(after, gone) {
					t.Errorf("%q survived the write:\n%s", gone, after)
				}
			}
			for _, want := range []string{"base_branch: develop2", "blocked: BLOCKED", "default_tasklist_id: 42"} {
				if !strings.Contains(after, want) {
					t.Errorf("output missing %q:\n%s", want, after)
				}
			}
			if got := diffLineCount(withoutLegacyBlocks(t, tc.fixture), after); got != 3 {
				t.Errorf("diff line count against the cleaned fixture = %d, want 3:\n%s", got, after)
			}
		})
	}
}

// A no-op --set on a legacy file writes nothing at all: no rewrite, no backup,
// nothing cleaned.
func TestSet_LegacyProviderBlocks_NoopLeavesFileUntouched(t *testing.T) {
	fixture := legacyRichFixture(t)
	deps, paths, configPath := setupLegacy(t, fixture)

	result, err := configure.Set(deps, paths, []string{"git.base_branch=develop"})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if result.Changed || result.Backup != nil || len(result.Removed) != 0 {
		t.Errorf("Changed = %v, Backup = %v, Removed = %v; want a no-op", result.Changed, result.Backup, result.Removed)
	}
	if got := readFile(t, configPath); got != fixture {
		t.Errorf("file mutated by a no-op --set:\n%s", got)
	}
}

// A file with nothing to strip reports nothing removed.
func TestSet_CleanFile_ReportsNothingRemoved(t *testing.T) {
	deps, paths, _ := setupLegacy(t, richFixtureLF)

	result, err := configure.Set(deps, paths, []string{"git.base_branch=develop2"})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if !result.Changed || len(result.Removed) != 0 {
		t.Errorf("Changed = %v, Removed = %v; want a plain write", result.Changed, result.Removed)
	}
}

// ---------------------------------------------------------------------------
// --set-model: add, then clear, an override; the first write strips the legacy
// blocks, the clear returns the file to the cleaned original bytes.
// ---------------------------------------------------------------------------

func TestSetModel_LegacyProviderBlocks_StrippedOnAddAndClear(t *testing.T) {
	fixture := legacyRichFixture(t)
	cleaned := withoutLegacyBlocks(t, fixture)
	deps, paths, configPath := setupLegacy(t, fixture)

	added, err := configure.SetModel(deps, paths, []string{"hyuga=opus/xhigh"})
	if err != nil {
		t.Fatalf("SetModel(add) error = %v", err)
	}
	assertRemoved(t, added.Removed, removedGithub, removedJira)
	got := readFile(t, configPath)
	if !strings.Contains(got, "hyuga: { model: opus, effort: xhigh }") {
		t.Fatalf("override not written:\n%s", got)
	}
	if strings.Contains(got, "github-projects") || strings.Contains(got, "jira: {") {
		t.Errorf("legacy blocks survived set-model add:\n%s", got)
	}

	cleared, err := configure.SetModel(deps, paths, []string{"hyuga=default"})
	if err != nil {
		t.Fatalf("SetModel(clear) error = %v", err)
	}
	if len(cleared.Removed) != 0 {
		t.Errorf("second write Removed = %v, want none", cleared.Removed)
	}
	// The models: block is regenerated by --set-model (sorted, new header
	// comment), so only everything after it must match the cleaned original.
	const tail = "critical_paths:"
	got = readFile(t, configPath)
	wantAt, gotAt := strings.Index(cleaned, tail), strings.Index(got, tail)
	if wantAt < 0 {
		t.Fatalf("fixture has no %q to compare from", tail)
	}
	if gotAt < 0 || got[gotAt:] != cleaned[wantAt:] {
		t.Errorf("content after models: changed by set-model:\ngot:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// A legacy tasks.provider: jira. Reading it (--print) stays read-only and does
// not fail; the next write of any key drops the line (and the blocks), so the
// default applies like in a new config; setting jira again is refused.
// ---------------------------------------------------------------------------

func TestPrint_LegacyJiraProvider_ReadsAsWrittenAndStaysReadOnly(t *testing.T) {
	fixture := asLegacyJira(t, legacyRichFixture(t))
	_, paths, configPath := setupLegacy(t, fixture)

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
	if got := readFile(t, configPath); got != fixture {
		t.Errorf("--print rewrote the file:\n%s", got)
	}
}

func TestSet_LegacyJiraProvider_AnyWriteDropsProviderAndBlocks(t *testing.T) {
	fixture := asLegacyJira(t, legacyRichFixture(t))
	deps, paths, configPath := setupLegacy(t, fixture)

	result, err := configure.Set(deps, paths, []string{"git.worktree=always"})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	assertRemoved(t, result.Removed, removedProvider, removedGithub, removedJira)
	after := readFile(t, configPath)
	if strings.Contains(after, "provider: jira") || strings.Contains(after, "github-projects") || strings.Contains(after, "jira: {") {
		t.Errorf("legacy settings survived:\n%s", after)
	}
	if got := diffLineCount(withoutLegacyProvider(t, fixture), after); got != 1 {
		t.Errorf("diff line count against the cleaned fixture = %d, want 1:\n%s", got, after)
	}

	// With the line gone, --print resolves tasks.provider like a new config.
	printed, err := configure.Print(printDeps(t.TempDir()), paths)
	if err != nil {
		t.Fatalf("Print() error = %v", err)
	}
	for _, entry := range printed.Values {
		if entry.Key == "tasks.provider" && entry.Value != "teamwork" {
			t.Errorf("tasks.provider after cleanup = %q, want the default teamwork", entry.Value)
		}
	}
}

func TestSetModel_LegacyJiraProvider_DropsProviderAndBlocks(t *testing.T) {
	fixture := asLegacyJira(t, legacyRichFixture(t))
	deps, paths, configPath := setupLegacy(t, fixture)

	result, err := configure.SetModel(deps, paths, []string{"hyuga=opus/xhigh"})
	if err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	assertRemoved(t, result.Removed, removedProvider, removedGithub, removedJira)
	after := readFile(t, configPath)
	if strings.Contains(after, "provider: jira") || strings.Contains(after, "github-projects") {
		t.Errorf("legacy settings survived set-model:\n%s", after)
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

	result, err := configure.Set(deps, paths, []string{"tasks.provider=teamwork"})
	if err != nil {
		t.Fatalf("Set(provider=teamwork) error = %v", err)
	}
	// The provider line now holds a supported value, so only the blocks go.
	assertRemoved(t, result.Removed, removedGithub, removedJira)
	after := readFile(t, configPath)
	if !strings.Contains(after, "  provider: teamwork") || strings.Contains(after, "provider: jira") {
		t.Errorf("provider not moved to teamwork:\n%s", after)
	}
	if strings.Contains(after, "github-projects") {
		t.Errorf("legacy blocks survived:\n%s", after)
	}
	if got := diffLineCount(withoutLegacyBlocks(t, fixture), after); got != 1 {
		t.Errorf("diff line count against the cleaned fixture = %d, want 1:\n%s", got, after)
	}
}

// ---------------------------------------------------------------------------
// --init-repo on an existing project config only strips the legacy provider
// settings; --repo-provider never rewrites what is already there.
// ---------------------------------------------------------------------------

func TestInitRepo_ExistingLegacyJiraProjectConfig_OnlyCleaned(t *testing.T) {
	repoDir := t.TempDir()
	projectPath := filepath.Join(repoDir, ".nerv", "nerv.yaml")
	legacy := "enabled: true\ntasks:\n  provider: jira   # legacy\n  providers:\n" + legacyProviderLines
	writeFixture(t, projectPath, legacy)
	deps := configure.Deps{Runner: gitToplevelRunner(repoDir), Now: func() time.Time { return time.Now() }}

	result, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: repoDir, Provider: "teamwork"})
	if err != nil {
		t.Fatalf("InitRepo() error = %v", err)
	}
	if !result.Changed || len(result.Warnings) != 0 || len(result.Removed) == 0 {
		t.Errorf("Changed = %v, Warnings = %v, Removed = %v; want cleaned, no warning", result.Changed, result.Warnings, result.Removed)
	}
	const want = "enabled: true\ntasks:\n  providers:\n"
	if got := readFile(t, projectPath); got != want {
		t.Errorf("project config = %q, want %q", got, want)
	}
}
