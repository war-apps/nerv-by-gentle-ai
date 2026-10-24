package artifact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// L12: writing an artifact writes the runtime folder's own .gitignore, so
// .nerv/ is never committed, without touching the repository's .gitignore.
func TestWrite_IgnoresTheRuntimeFolder(t *testing.T) {
	repo := t.TempDir()
	if _, err := Write(repo, sample(t), false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repo, ".nerv", ".gitignore"))
	if err != nil || string(data) != "*\n" {
		t.Fatalf(".nerv/.gitignore = %q, %v; want \"*\\n\"", data, err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".gitignore")); !os.IsNotExist(err) {
		t.Errorf("the repository's .gitignore was written: %v", err)
	}
}

// An existing runtime .gitignore is kept as it is.
func TestWrite_KeepsTheRuntimeGitignore(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".nerv"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".nerv", ".gitignore"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(repo, sample(t), false); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(repo, ".nerv", ".gitignore")); string(data) != "mine\n" {
		t.Errorf(".nerv/.gitignore = %q, want it kept", data)
	}
}

// L10: CheckRoot refuses a .nerv/artifacts (or .nerv) symlink before
// anything runs, and writes nothing.
func TestCheckRoot_RefusesASymlinkedArtifactsFolder(t *testing.T) {
	for _, link := range []string{".nerv", filepath.Join(".nerv", "artifacts")} {
		t.Run(link, func(t *testing.T) {
			repo := t.TempDir()
			if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, link)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(t.TempDir(), filepath.Join(repo, link)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			err := CheckRoot(repo)
			if err == nil || !strings.Contains(err.Error(), "outside the repository") {
				t.Fatalf("CheckRoot = %v, want a refusal", err)
			}
		})
	}
	if err := CheckRoot(t.TempDir()); err != nil {
		t.Errorf("CheckRoot of a repository without .nerv = %v", err)
	}
	repo := t.TempDir()
	if err := CheckRoot(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".nerv")); !os.IsNotExist(err) {
		t.Errorf("CheckRoot wrote .nerv: %v", err)
	}
}
