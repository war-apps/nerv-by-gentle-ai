package configure

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/war-apps/nerv-gentle-ai/internal/config"
)

// Store reads and writes the user-scope nerv.yaml on the real filesystem.
// It has no state of its own (a zero-value Store{} is ready to use) — like
// internal/models and internal/skills, it talks to the real disk directly
// through the os package rather than through an injected seam, since it is
// itself part of the seam configure's other functions are tested against.
type Store struct{}

// Load reads path and wraps its content as a *config.Document. A missing
// file is reported as exists=false with an empty Document, not an error —
// callers seed the working document from missingUserConfigHeader in that
// case, mirroring configure.ps1's own
// $niExistingUserYaml resolution (Test-Path ... else empty string).
func (Store) Load(path string) (*config.Document, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return config.Parse(""), false, nil
		}
		return nil, false, err
	}
	return config.Parse(string(data)), true, nil
}

// Save persists doc to path when its text differs from previous (the bytes
// Load returned earlier in the same run): a no-op — no write, no backup —
// when they are identical. Otherwise, when a file already exists at path,
// it is copied to a timestamped "<path>.bak-configure-<yyyyMMdd-HHmmss>"
// backup first (the suffix and format configure.ps1's non-interactive body
// uses for every -Set/-SetModel write, regardless of which one touched the
// file); when it does not exist yet, Save creates path's parent directory
// instead. The new content is then written atomically (temp file + rename).
// Mirrors the shared tail of configure.ps1's -Set/-SetModel handling
// (~1329-1345).
func (Store) Save(path string, doc *config.Document, previous []byte, now time.Time) (written bool, backup string, err error) {
	newText := []byte(doc.String())
	if bytes.Equal(newText, previous) {
		return false, "", nil
	}

	if _, statErr := os.Stat(path); statErr == nil {
		backup = fmt.Sprintf("%s.bak-configure-%s", path, now.Format("20060102-150405"))
		if cpErr := copyFile(path, backup); cpErr != nil {
			return false, "", cpErr
		}
	} else if errors.Is(statErr, fs.ErrNotExist) {
		if dir := filepath.Dir(path); dir != "" {
			if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
				return false, "", mkErr
			}
		}
	} else {
		return false, "", statErr
	}

	if err := atomicWriteFile(path, newText); err != nil {
		return false, "", err
	}
	return true, backup, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// atomicWriteFile writes data to path via a temp file in the same directory
// followed by a rename, so a reader never observes a partially written
// file.
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	tmp, err := os.CreateTemp(dir, ".nerv-configure-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil {
		os.Remove(tmpPath)
		return writeErr
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return closeErr
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
