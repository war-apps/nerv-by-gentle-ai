package configure_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
)

// ---------------------------------------------------------------------------
// installcommands: copies the Teamwork procedures once, skips existing
// files on a second run.
// ---------------------------------------------------------------------------

func TestInstallCommands_CopiesFiles(t *testing.T) {
	home := t.TempDir()
	deps := configure.Deps{FS: nerv.PluginFS()}
	paths := configure.ResolvePaths(home, "")

	result, err := configure.InstallCommands(deps, paths)
	if err != nil {
		t.Fatalf("InstallCommands() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want true")
	}
	entries, err := os.ReadDir(paths.CommandsDir)
	if err != nil {
		t.Fatalf("commands dir not created: %v", err)
	}
	if len(entries) == 0 {
		t.Error("no files copied")
	}
	if len(result.Written) != len(entries) {
		t.Errorf("len(Written) = %d, want %d", len(result.Written), len(entries))
	}
}

func TestInstallCommands_SkipsExistingFilesOnSecondRun(t *testing.T) {
	home := t.TempDir()
	deps := configure.Deps{FS: nerv.PluginFS()}
	paths := configure.ResolvePaths(home, "")

	if _, err := configure.InstallCommands(deps, paths); err != nil {
		t.Fatalf("InstallCommands() (first) error = %v", err)
	}

	result, err := configure.InstallCommands(deps, paths)
	if err != nil {
		t.Fatalf("InstallCommands() (second) error = %v", err)
	}
	if result.Changed {
		t.Error("Changed = true, want false (every file already exists)")
	}
	if len(result.Warnings) == 0 {
		t.Error("Warnings is empty, want a skip warning per already-existing file")
	}
	if len(result.Written) != 0 {
		t.Errorf("Written = %v, want empty", result.Written)
	}
}

// ---------------------------------------------------------------------------
// installcommands against a plugin tree with no Teamwork procedures
// directory: warns, does not fail.
// ---------------------------------------------------------------------------

func TestInstallCommands_MissingProceduresDir_WarnsNoError(t *testing.T) {
	home := t.TempDir()
	deps := configure.Deps{FS: fstest.MapFS{
		"agents/misato.md": &fstest.MapFile{Data: []byte("---\nmodel: fable\n---\n")},
	}}
	paths := configure.ResolvePaths(home, "")

	result, err := configure.InstallCommands(deps, paths)
	if err != nil {
		t.Fatalf("InstallCommands() error = %v", err)
	}
	if result.Changed {
		t.Error("Changed = true, want false")
	}
	if len(result.Warnings) == 0 {
		t.Error("Warnings is empty, want a warning about the missing directory")
	}
	if _, statErr := os.Stat(filepath.Join(paths.CommandsDir)); statErr == nil {
		t.Error("commands dir should not have been created")
	}
}
