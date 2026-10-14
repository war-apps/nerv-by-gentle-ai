package semrel

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureCache_SweepsStaleInstalls(t *testing.T) {
	base := t.TempDir()
	stale := filepath.Join(base, ".install-111")
	fresh := filepath.Join(base, ".install-222")
	other := filepath.Join(base, "0123456789abcdef")
	for _, d := range []string{stale, fresh, other} {
		if err := os.MkdirAll(filepath.Join(d, "node_modules"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	for _, d := range []string{stale, other} {
		if err := os.Chtimes(d, old, old); err != nil {
			t.Fatal(err)
		}
	}
	e := &Engine{Runner: &fakeRunner{handle: func(c Command) (Result, error) { return Result{}, nil }}}
	if _, err := e.ensureCache(context.Background(), base, []string{"semantic-release@24.2.3"}); err != nil {
		t.Fatal(err)
	}
	if dirExists(stale) {
		t.Errorf("stale install %s left behind", stale)
	}
	if !dirExists(fresh) {
		t.Errorf("recent install %s removed (it may belong to a running install)", fresh)
	}
	if !dirExists(other) {
		t.Errorf("a cache entry %s was removed", other)
	}
}
