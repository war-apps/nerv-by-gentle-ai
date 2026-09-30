package models_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	nerv "github.com/war-apps/nerv-gentle-ai"
	"github.com/war-apps/nerv-gentle-ai/internal/config"
	"github.com/war-apps/nerv-gentle-ai/internal/models"
)

// ---------------------------------------------------------------------------
// PluginDefaults — mirrors Get-NervPluginDefaults.
// ---------------------------------------------------------------------------

func TestPluginDefaults_RealEmbeddedFS_18RolesAobaSonnetLow(t *testing.T) {
	defaults, err := models.PluginDefaults(nerv.PluginFS())
	if err != nil {
		t.Fatalf("PluginDefaults() error = %v", err)
	}
	if len(defaults) != 18 {
		t.Fatalf("len(defaults) = %d, want 18", len(defaults))
	}
	aoba, ok := defaults["aoba"]
	if !ok || aoba.Model != "sonnet" || aoba.Effort != "low" {
		t.Fatalf("defaults[aoba] = %+v, ok=%v, want {Model:sonnet Effort:low}", aoba, ok)
	}
}

func TestPluginDefaults_MapFS_ParsesModelAndEffort(t *testing.T) {
	fsys := fstest.MapFS{
		"agents/shinji.md": &fstest.MapFile{Data: []byte("---\nname: shinji\nmodel: sonnet\neffort: medium\n---\nbody\n")},
		"agents/misato.md": &fstest.MapFile{Data: []byte("---\nname: misato\nmodel: fable # a trailing comment\neffort: high\n---\n")},
	}

	defaults, err := models.PluginDefaults(fsys)
	if err != nil {
		t.Fatalf("PluginDefaults() error = %v", err)
	}
	if got := defaults["shinji"]; got.Model != "sonnet" || got.Effort != "medium" {
		t.Fatalf("defaults[shinji] = %+v, want {sonnet medium}", got)
	}
	if got := defaults["misato"]; got.Model != "fable" || got.Effort != "high" {
		t.Fatalf("defaults[misato] = %+v, want {fable high}", got)
	}
}

func TestPluginDefaults_MapFS_SkipsMalformedFrontmatter(t *testing.T) {
	fsys := fstest.MapFS{
		"agents/broken.md": &fstest.MapFile{Data: []byte("no frontmatter here\n")},
	}

	defaults, err := models.PluginDefaults(fsys)
	if err != nil {
		t.Fatalf("PluginDefaults() error = %v", err)
	}
	if _, ok := defaults["broken"]; ok {
		t.Fatalf("defaults[broken] present, want absent for malformed frontmatter")
	}
}

// ---------------------------------------------------------------------------
// Resolve — thin delegation to config.ModelTable.
// ---------------------------------------------------------------------------

func TestResolve_DelegatesToConfigModelTable(t *testing.T) {
	defaults := map[string]config.ModelOverride{"aoba": {Model: "sonnet", Effort: "low"}}
	overrides := map[string]config.ModelOverride{"aoba": {From: "jd-judge-b"}}
	phases := map[string]config.PhaseAssignment{"jd-judge-b": {Model: "opus", Effort: "xhigh"}}

	got := models.Resolve(defaults, overrides, phases)
	want := config.ModelTable(defaults, overrides, phases)

	if len(got) != len(want) || len(got) != 1 {
		t.Fatalf("Resolve() = %+v, want %+v", got, want)
	}
	if got[0] != want[0] {
		t.Fatalf("Resolve()[0] = %+v, want %+v", got[0], want[0])
	}
	if got[0].Source != "gentle-ai:jd-judge-b" || got[0].Model != "opus" || got[0].Effort != "xhigh" {
		t.Fatalf("Resolve()[0] = %+v, unexpected", got[0])
	}
}

// ---------------------------------------------------------------------------
// ApplyToAgentFile — ports tests/install-apply-models.test.ps1's
// Set-NervAgentFrontmatter cases onto a single file's content.
// ---------------------------------------------------------------------------

const shinjiFixture = "---\nname: shinji\nmodel: sonnet\neffort: medium\ntools: Read\n---\n\nbody unchanged\n"

func TestApplyToAgentFile_UpdatesModelAndEffort(t *testing.T) {
	out, changed := models.ApplyToAgentFile([]byte(shinjiFixture), "haiku", "low")
	if !changed {
		t.Fatal("changed = false, want true")
	}
	text := string(out)
	if !strings.Contains(text, "model: haiku\n") {
		t.Fatalf("output missing updated model line: %q", text)
	}
	if !strings.Contains(text, "effort: low\n") {
		t.Fatalf("output missing updated effort line: %q", text)
	}
	if !strings.Contains(text, "body unchanged") {
		t.Fatalf("output body altered: %q", text)
	}
}

func TestApplyToAgentFile_SameValuesReportUnchanged(t *testing.T) {
	out, changed := models.ApplyToAgentFile([]byte(shinjiFixture), "sonnet", "medium")
	if changed {
		t.Fatal("changed = true, want false when values already match")
	}
	if string(out) != shinjiFixture {
		t.Fatalf("output = %q, want the untouched fixture", out)
	}
}

func TestApplyToAgentFile_EmptyModelLeavesModelUntouched(t *testing.T) {
	// Mirrors Case C: an assignment carrying only Model leaves an existing
	// effort: line alone.
	out, changed := models.ApplyToAgentFile([]byte(shinjiFixture), "opus", "")
	if !changed {
		t.Fatal("changed = false, want true")
	}
	text := string(out)
	if !strings.Contains(text, "model: opus\n") {
		t.Fatalf("output missing updated model line: %q", text)
	}
	if !strings.Contains(text, "effort: medium\n") {
		t.Fatalf("output effort line changed, want it untouched: %q", text)
	}
}

func TestApplyToAgentFile_InsertsMissingEffortLineAfterModel(t *testing.T) {
	fixture := "---\nname: x\nmodel: sonnet\ntools: Read\n---\nbody\n"
	out, changed := models.ApplyToAgentFile([]byte(fixture), "", "high")
	if !changed {
		t.Fatal("changed = false, want true")
	}
	lines := strings.Split(string(out), "\n")
	modelIdx := indexOf(lines, "model: sonnet")
	if modelIdx < 0 {
		t.Fatalf("model line not found in output: %v", lines)
	}
	if lines[modelIdx+1] != "effort: high" {
		t.Fatalf("line after model = %q, want %q", lines[modelIdx+1], "effort: high")
	}
}

func TestApplyToAgentFile_PreservesTrailingCommentAndCRLF(t *testing.T) {
	fixture := "---\r\nname: misato\r\nmodel: fable # a trailing comment kept verbatim\r\neffort: high\r\n---\r\n"
	out, changed := models.ApplyToAgentFile([]byte(fixture), "opus", "")
	if !changed {
		t.Fatal("changed = false, want true")
	}
	text := string(out)
	if !strings.Contains(text, "model: opus # a trailing comment kept verbatim\r\n") {
		t.Fatalf("output did not preserve the trailing comment: %q", text)
	}
	if strings.ContainsRune(strings.ReplaceAll(text, "\r\n", ""), '\n') {
		t.Fatalf("output introduced a bare LF: %q", text)
	}
}

func TestApplyToAgentFile_NoModelKeyLeavesFileUnchanged(t *testing.T) {
	fixture := "---\nname: x\ntools: Read\n---\nbody\n"
	out, changed := models.ApplyToAgentFile([]byte(fixture), "sonnet", "low")
	if changed {
		t.Fatal("changed = true, want false when there is no model: key")
	}
	if string(out) != fixture {
		t.Fatalf("output = %q, want the untouched fixture", out)
	}
}

func TestApplyToAgentFile_MalformedFrontmatterLeavesFileUnchanged(t *testing.T) {
	fixture := "not frontmatter at all\n"
	out, changed := models.ApplyToAgentFile([]byte(fixture), "sonnet", "low")
	if changed {
		t.Fatal("changed = true, want false for malformed frontmatter")
	}
	if string(out) != fixture {
		t.Fatalf("output = %q, want the untouched fixture", out)
	}
}

func indexOf(lines []string, prefix string) int {
	for i, l := range lines {
		if strings.TrimRight(l, "\r") == prefix {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// ApplyToDir — Case A/B/D of tests/install-apply-models.test.ps1, over a
// real temp directory.
// ---------------------------------------------------------------------------

func writeAgentFixture(t *testing.T, dir, role, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, role+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestApplyToDir_ChangedUpToDateAndSkippedCounts(t *testing.T) {
	dir := t.TempDir()
	writeAgentFixture(t, dir, "aoba", "---\nname: aoba\nmodel: sonnet\neffort: low\n---\n")
	writeAgentFixture(t, dir, "misato", "---\nname: misato\nmodel: fable\neffort: high\n---\n")
	writeAgentFixture(t, dir, "broken", "not frontmatter\n")

	assignments := map[string]config.ModelOverride{
		"aoba":   {Model: "haiku", Effort: "low"},  // changes
		"misato": {Model: "fable", Effort: "high"}, // already up to date
		"broken": {Model: "sonnet", Effort: "low"}, // malformed -> skipped
		"ghost":  {Model: "sonnet", Effort: "low"}, // file absent -> skipped
	}

	summary, err := models.ApplyToDir(dir, assignments)
	if err != nil {
		t.Fatalf("ApplyToDir() error = %v", err)
	}
	if summary.Changed != 1 || summary.UpToDate != 1 || summary.Skipped != 2 {
		t.Fatalf("summary = %+v, want {Changed:1 UpToDate:1 Skipped:2}", summary)
	}

	aobaContent, err := os.ReadFile(filepath.Join(dir, "aoba.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(aobaContent), "model: haiku") {
		t.Fatalf("aoba.md not updated: %q", aobaContent)
	}
}

func TestApplyToDir_RestoresRemovedOverrideToPluginDefault(t *testing.T) {
	dir := t.TempDir()
	// Cache still carries a previous override (haiku/low); merging plugin
	// defaults with an empty override map should restore sonnet/low.
	writeAgentFixture(t, dir, "aoba", "---\nname: aoba\nmodel: haiku\neffort: low\n---\n")

	defaults, err := models.PluginDefaults(nerv.PluginFS())
	if err != nil {
		t.Fatalf("PluginDefaults() error = %v", err)
	}

	summary, err := models.ApplyToDir(dir, defaults)
	if err != nil {
		t.Fatalf("ApplyToDir() error = %v", err)
	}
	if summary.Changed == 0 {
		t.Fatalf("summary = %+v, want at least one changed file (aoba restored to default)", summary)
	}

	aobaContent, err := os.ReadFile(filepath.Join(dir, "aoba.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(aobaContent), "model: sonnet") || !strings.Contains(string(aobaContent), "effort: low") {
		t.Fatalf("aoba.md not restored to plugin default: %q", aobaContent)
	}
}
