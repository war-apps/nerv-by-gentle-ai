// Package configstore reads and writes the user-scope nerv.yaml on the
// real filesystem. It is the disk-access seam beneath "nerv configure":
// configure is a use case (validation, diffing, orchestration), depending
// downward on configstore for the actual Load/Save, exactly like install
// does — so install never needs to depend upward on configure just to
// read the same file.
package configstore

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/war-apps/nerv-gentle-ai/internal/atomicfile"
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
// callers seed the working document from their own missing-file header in
// that case.
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
// backup first — the same suffix and format for every "nerv configure
// --set"/"--set-model" write, regardless of which one touched the file;
// when it does not exist yet, Save creates path's parent directory
// instead. The new content is then written atomically (temp file +
// rename).
func (Store) Save(path string, doc *config.Document, previous []byte, now time.Time) (written bool, backup string, err error) {
	newText := []byte(doc.String())
	if bytes.Equal(newText, previous) {
		return false, "", nil
	}

	backup, err = atomicfile.Save(path, newText, atomicfile.Options{
		Now:          now,
		BackupSuffix: "bak-configure-",
	})
	if err != nil {
		return false, "", err
	}
	return true, backup, nil
}
