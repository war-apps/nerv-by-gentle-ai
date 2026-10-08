package models_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/models"
)

// ---------------------------------------------------------------------------
// PluginDefaults — mirrors Get-NervPluginDefaults.
// ---------------------------------------------------------------------------

func TestPluginDefaults_RealEmbeddedFS_16RolesAobaSonnetLow(t *testing.T) {
	defaults, err := models.PluginDefaults(nerv.PluginFS())
	if err != nil {
		t.Fatalf("PluginDefaults() error = %v", err)
	}
	if len(defaults) != 16 {
		t.Fatalf("len(defaults) = %d, want 16", len(defaults))
	}
	aoba, ok := defaults["aoba"]
	if !ok || aoba.Model != "sonnet" || aoba.Effort != "low" {
		t.Fatalf("defaults[aoba] = %+v, ok=%v, want {Model:sonnet Effort:low}", aoba, ok)
	}
	audit, ok := defaults["kaji-audit"]
	if !ok || audit.Model != "sonnet" || audit.Effort != "medium" {
		t.Fatalf("defaults[kaji-audit] = %+v, ok=%v, want {Model:sonnet Effort:medium}", audit, ok)
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
