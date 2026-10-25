package deployrun

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
)

// A release file is never written through a symlink the repository holds,
// and a component the check cannot inspect fails closed.
func TestCheckReleasePath_RefusesSymlinksAndUnreadableComponents(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "docs")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "locked", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := workspace{dir: dir}

	if err := w.checkReleasePath("docs/CHANGELOG.md"); err == nil || !refusal.Is(err) {
		t.Errorf("a release file under a repository symlink: %v, want a refusal", err)
	}
	if err := w.checkReleasePath("src/CHANGELOG.md"); err != nil {
		t.Errorf("a release file in a new folder: %v, want nil", err)
	}

	if err := os.Chmod(filepath.Join(dir, "locked"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "locked"), 0o755) })
	if _, err := os.Lstat(filepath.Join(dir, "locked", "sub")); err == nil {
		t.Skip("running with permissions that ignore the mode")
	}
	if err := w.checkReleasePath("locked/sub/CHANGELOG.md"); err == nil || !refusal.Is(err) {
		t.Errorf("an unreadable component: %v, want a refusal (fail closed)", err)
	}
}
