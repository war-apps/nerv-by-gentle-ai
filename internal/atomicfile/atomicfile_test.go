package atomicfile_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-gentle-ai/internal/atomicfile"
)

func TestSave_NoBackupWhenFileDidNotExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "file.txt")
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)

	backup, err := atomicfile.Save(path, []byte("hello\n"), atomicfile.Options{Now: now, BackupSuffix: "bak-test-"})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if backup != "" {
		t.Errorf("backup = %q, want empty (no prior file)", backup)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(content) != "hello\n" {
		t.Errorf("content = %q, want %q", content, "hello\n")
	}
}

func TestSave_BacksUpExistingFileWithTimestampedSuffix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 29, 12, 34, 56, 0, time.UTC)

	backup, err := atomicfile.Save(path, []byte("new\n"), atomicfile.Options{Now: now, BackupSuffix: "bak-test-"})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	wantBackup := path + ".bak-test-20260929-123456"
	if backup != wantBackup {
		t.Errorf("backup = %q, want %q", backup, wantBackup)
	}
	backupContent, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("backup file not created: %v", err)
	}
	if string(backupContent) != "old\n" {
		t.Errorf("backup content = %q, want %q", backupContent, "old\n")
	}
	newContent, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(newContent) != "new\n" {
		t.Errorf("content = %q, want %q", newContent, "new\n")
	}
}

func TestSave_NoLeftoverTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")

	if _, err := atomicfile.Save(path, []byte("x"), atomicfile.Options{Now: time.Now()}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestSave_VerifyFailure_RestoresBackupAndReturnsIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	wantErr := errors.New("bad content")

	backup, err := atomicfile.Save(path, []byte("new\n"), atomicfile.Options{
		Now:          now,
		BackupSuffix: "bak-test-",
		Verify:       func([]byte) error { return wantErr },
	})
	if err == nil {
		t.Fatal("expected a verification error")
	}
	if backup == "" {
		t.Error("backup path should be reported when a backup was taken and verification failed")
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != "old\n" {
		t.Errorf("path not restored from backup after verify failure: %q", after)
	}
}

func TestSave_VerifyFailure_NoBackupTaken_NoRestoreNoBackupPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")

	backup, err := atomicfile.Save(path, []byte("new\n"), atomicfile.Options{
		Now:    time.Now(),
		Verify: func([]byte) error { return errors.New("bad content") },
	})
	if err == nil {
		t.Fatal("expected a verification error")
	}
	if backup != "" {
		t.Errorf("backup = %q, want empty (no prior file to restore)", backup)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("path must not exist after a failed verified write with nothing to restore")
	}
}
