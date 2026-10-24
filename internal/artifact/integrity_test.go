package artifact

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The manifest records the sha256 of notes.md and CHANGELOG.section.md, and
// Verify checks them: deploy publishes both, so a tampered one is refused.
func TestVerify_ChecksTheNotesAndTheChangelogSection(t *testing.T) {
	for _, name := range []string{NotesFile, ChangelogFile} {
		t.Run(name, func(t *testing.T) {
			in := sample(t)
			dir, err := Write(t.TempDir(), in, false)
			if err != nil {
				t.Fatal(err)
			}
			m, err := Verify(dir)
			if err != nil {
				t.Fatal(err)
			}
			if m.NotesSHA256 != sum(in.Notes) || m.ChangelogSHA256 != sum(in.ChangelogSection) {
				t.Errorf("manifest digests notes %q changelog %q", m.NotesSHA256, m.ChangelogSHA256)
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte("tampered"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(dir); err == nil || !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), "sha256") {
				t.Errorf("verify: %v, want a sha256 mismatch naming %s", err, name)
			}
		})
	}
}

// A file the manifest does not list, anywhere in the artifact, is refused:
// deploy publishes what the manifest lists and nothing else.
func TestVerify_RefusesUnexpectedFiles(t *testing.T) {
	for _, rel := range []string{"outputs/extra.tar", "outputs/sub/extra.bin", "release-files/extra.txt", "extra.md"} {
		t.Run(rel, func(t *testing.T) {
			dir, err := Write(t.TempDir(), sample(t), false)
			if err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(dir); err == nil || !strings.Contains(err.Error(), rel) || !strings.Contains(err.Error(), "not listed") {
				t.Errorf("verify: %v, want %s refused as not listed in the manifest", err, rel)
			}
		})
	}
}

// --force renames the new artifact in before removing the old one, so a
// failed swap leaves the previous artifact intact.
func TestWrite_ForceKeepsTheOldArtifactWhenTheSwapFails(t *testing.T) {
	repo := t.TempDir()
	in := sample(t)
	dir, err := Write(repo, in, false)
	if err != nil {
		t.Fatal(err)
	}
	orig := rename
	t.Cleanup(func() { rename = orig })
	rename = func(from, to string) error {
		if to == dir && strings.Contains(filepath.Base(from), ".tmp-") {
			return errors.New("injected rename failure")
		}
		return orig(from, to)
	}
	in.Notes = "new notes\n"
	if _, err := Write(repo, in, true); err == nil {
		t.Fatal("forced write with a failing swap succeeded")
	}
	m, err := Verify(dir)
	if err != nil {
		t.Fatalf("the old artifact is gone or broken after a failed swap: %v", err)
	}
	if m.NotesSHA256 != sum(sample(t).Notes) {
		t.Errorf("the old artifact's notes changed")
	}
	entries, _ := os.ReadDir(filepath.Dir(dir))
	if len(entries) != 1 {
		t.Errorf("project directory holds %d entries after a failed swap, want only the version", len(entries))
	}
}
