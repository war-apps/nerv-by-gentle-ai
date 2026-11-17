package ecosystem

import (
	"os"
	"path/filepath"
	"testing"
)

// writeExecutable creates an empty, executable placeholder file at path,
// creating its parent directories, for Detect's default-install-dir tests.
func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(""), 0o755); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
