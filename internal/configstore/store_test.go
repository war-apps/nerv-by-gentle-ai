package configstore_test

import (
	"os"
	"path/filepath"
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

	written, backup, err := (configstore.Store{}).Save(path, doc, original, now)
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

	written, backup, err := (configstore.Store{}).Save(path, doc, original, now)
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

	written, backup, err := (configstore.Store{}).Save(path, doc, nil, now)
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

func TestStore_RoundTrip_LegacyProviderBlocksSurvive(t *testing.T) {
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
	if _, _, err := store.Save(path, doc, []byte(original), time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(original, "base_branch: develop              #", "base_branch: develop2              #", 1)
	if string(after) != want {
		t.Errorf("round trip changed more than base_branch:\ngot:\n%s\nwant:\n%s", after, want)
	}
}
