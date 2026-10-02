package configstore_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configstore"
)

// ---------------------------------------------------------------------------
// Store.Load
// ---------------------------------------------------------------------------

func TestStore_Load_MissingFileReportsNotExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nerv.yaml")

	doc, exists, err := (configstore.Store{}).Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if exists {
		t.Error("exists = true, want false")
	}
	if doc.String() != "" {
		t.Errorf("doc = %q, want empty", doc.String())
	}
}

func TestStore_Load_ExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nerv.yaml")
	if err := os.WriteFile(path, []byte("enabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	doc, exists, err := (configstore.Store{}).Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !exists {
		t.Error("exists = false, want true")
	}
	if doc.String() != "enabled: true\n" {
		t.Errorf("doc = %q, want fixture content", doc.String())
	}
}

// ---------------------------------------------------------------------------
// Store.Save — no-op detection (group F: "empty answers" byte-identical
// with no backup, ported as a direct Save() call carrying identical bytes).
// ---------------------------------------------------------------------------

func TestStore_Save_NoopWhenBytesIdentical(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nerv.yaml")
	original := []byte("enabled: true\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	doc := config.Parse(string(original))
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

	written, backup, _, err := (configstore.Store{}).Save(path, doc, original, now)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if written {
		t.Error("written = true, want false (no-op)")
	}
	if backup != "" {
		t.Errorf("backup = %q, want empty", backup)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("dir has %d entries, want 1 (no backup file created)", len(entries))
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Errorf("file changed on a no-op Save(): %q", after)
	}
}

// ---------------------------------------------------------------------------
// Store.Save — writes a timestamped backup and an atomic new version
// (group G: e2e-backup-created).
// ---------------------------------------------------------------------------

func TestStore_Save_WritesTimestampedBackupAndNewContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nerv.yaml")
	original := []byte("enabled: true\ngit:\n  base_branch: develop\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	doc := config.Parse(string(original))
	doc.SetScalar("git.base_branch", "develop2")
	now := time.Date(2026, 9, 29, 12, 34, 56, 0, time.UTC)

	written, backup, _, err := (configstore.Store{}).Save(path, doc, original, now)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !written {
		t.Fatal("written = false, want true")
	}
	wantBackup := path + ".bak-configure-20260929-123456"
	if backup != wantBackup {
		t.Errorf("backup = %q, want %q", backup, wantBackup)
	}
	if _, err := os.Stat(backup); err != nil {
		t.Errorf("backup file not created: %v", err)
	}
	backupContent, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(backupContent) != string(original) {
		t.Errorf("backup content = %q, want original %q", backupContent, original)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "base_branch: develop2") {
		t.Errorf("file not updated: %q", after)
	}
}

// ---------------------------------------------------------------------------
// Store.Save — creates the parent directory and writes with no backup when
// the file did not exist yet.
// ---------------------------------------------------------------------------

func TestStore_Save_CreatesParentDirWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "nerv.yaml")

	doc := config.Parse("enabled: true\n")
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)

	written, backup, _, err := (configstore.Store{}).Save(path, doc, nil, now)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !written {
		t.Fatal("written = false, want true")
	}
	if backup != "" {
		t.Errorf("backup = %q, want empty (no prior file to back up)", backup)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(content) != "enabled: true\n" {
		t.Errorf("content = %q, want %q", content, "enabled: true\n")
	}
}

// ---------------------------------------------------------------------------
// Store.Load -> edit -> Store.Save keeps the legacy provider sub-blocks
// (github-projects, jira) that releases before their removal left in
// tasks.providers, byte for byte, with the legacy tasks.provider value too.
// ---------------------------------------------------------------------------

func TestStore_Save_StripsRemovedProviderSettings(t *testing.T) {
	const legacyBlocks = "" +
		"    # legacy stubs, kept by hand\n" +
		"    github-projects: { task_ref_prefix: gh, owner: \"\", project_number: 0 }    # later\n" +
		"    jira: { task_ref_prefix: jira, site: \"\", project_key: \"\" }               # later\n"
	original := "" +
		"enabled: true\n" +
		"git:\n" +
		"  base_branch: develop              # default base\n" +
		"tasks:\n" +
		"  provider: jira                    # legacy value\n" +
		"  providers:\n" +
		"    teamwork:\n" +
		"      task_ref_prefix: tw\n" +
		legacyBlocks
	path := filepath.Join(t.TempDir(), "nerv.yaml")
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	store := configstore.Store{}
	doc, _, err := store.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	doc.SetScalar("git.base_branch", "develop2")
	written, backup, removed, err := store.Save(path, doc, []byte(original), time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !written {
		t.Fatal("written = false, want true")
	}
	wantRemoved := []string{"tasks.provider (jira)", "tasks.providers.github-projects", "tasks.providers.jira"}
	if !slices.Equal(removed, wantRemoved) {
		t.Errorf("removed = %v, want %v", removed, wantRemoved)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(original, "base_branch: develop              #", "base_branch: develop2              #", 1)
	want = strings.Replace(want, "  provider: jira                    # legacy value\n", "", 1)
	want = strings.Replace(want, legacyBlocks, "", 1)
	if string(after) != want {
		t.Errorf("Save changed more than base_branch and the removed providers:\ngot:\n%s\nwant:\n%s", after, want)
	}

	// The backup keeps the file as it was, legacy settings included.
	backupContent, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("backup not written: %v", err)
	}
	if string(backupContent) != original {
		t.Errorf("backup content = %q, want the original", backupContent)
	}
}

// A document with nothing to remove saves exactly as before and reports nothing.
func TestStore_Save_CleanDocumentReportsNothingRemoved(t *testing.T) {
	original := "enabled: true\ntasks:\n  provider: teamwork\n"
	path := filepath.Join(t.TempDir(), "nerv.yaml")
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := config.Parse(original)
	doc.SetScalar("tasks.provider", "none")

	written, _, removed, err := (configstore.Store{}).Save(path, doc, []byte(original), time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !written || len(removed) != 0 {
		t.Errorf("written = %v, removed = %v; want a plain write with nothing removed", written, removed)
	}
	after, _ := os.ReadFile(path)
	if string(after) != "enabled: true\ntasks:\n  provider: none\n" {
		t.Errorf("file = %q", after)
	}
}
