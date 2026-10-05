// Package atomicfile writes a file's new content without ever leaving a
// reader to observe a partial write: an existing file at the destination
// is first copied to a timestamped backup, the new content is written to a
// temp file in the same directory and renamed over the destination, and an
// optional verification pass can reject a bad write before it lands (or,
// once it has landed, trigger a restore from the backup). It is the shared
// tail of "nerv configure"'s nerv.yaml writes and Claude Code's
// settings.json writes.
package atomicfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Options configures one Save call.
type Options struct {
	// Now names the backup file: "<path>.<BackupSuffix><Now formatted
	// 20060102-150405>".
	Now time.Time
	// BackupSuffix distinguishes one caller's backups from another's
	// (e.g. "bak-configure-", "bak-nerv-").
	BackupSuffix string
	// Verify, when non-nil, is called once with the new content right
	// after it is written to the temp file (before the rename) and once
	// more by re-reading the destination right after the rename. Either
	// call returning an error fails the Save; when a backup was taken,
	// that backup is restored over the destination before Save returns.
	// A nil Verify skips both checks and never restores a failed write.
	Verify func([]byte) error
}

// Save backs up an existing file at path (or creates its parent directory
// when path does not exist yet), then writes data to path atomically: a
// temp file in the same directory, followed by a rename, so a reader never
// observes a partially written file. backupPath is the backup file's path,
// or "" when no backup was taken (path did not exist) or, absent
// verification, when the write failed. Save does not compare data against
// any previous content -- callers decide whether a write is needed at all.
func Save(path string, data []byte, opts Options) (backupPath string, err error) {
	existed := false
	if _, statErr := os.Stat(path); statErr == nil {
		existed = true
		backupPath = fmt.Sprintf("%s.%s%s", path, opts.BackupSuffix, opts.Now.Format("20060102-150405"))
		if cpErr := copyFile(path, backupPath); cpErr != nil {
			return "", cpErr
		}
	} else if errors.Is(statErr, fs.ErrNotExist) {
		if dir := filepath.Dir(path); dir != "" {
			if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
				return "", mkErr
			}
		}
	} else {
		return "", statErr
	}

	if writeErr := writeAtomic(path, data, opts.Verify, 0); writeErr != nil {
		if opts.Verify == nil {
			return "", writeErr
		}
		if existed {
			_ = copyFile(backupPath, path)
		}
		return backupPath, writeErr
	}

	return backupPath, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// writeAtomic writes data to a temp file next to path and renames it over
// path. A non-zero mode is applied to the temp file before the rename; zero
// keeps os.CreateTemp's 0600. When verify is non-nil, it is run against data
// before the rename and against a fresh read of path after the rename.
func writeAtomic(path string, data []byte, verify func([]byte) error, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	tmp, err := os.CreateTemp(dir, ".nerv-atomicfile-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	var chmodErr error
	if mode != 0 {
		chmodErr = tmp.Chmod(mode)
	}
	_, writeErr := tmp.Write(data)
	if chmodErr != nil {
		writeErr = chmodErr
	}
	closeErr := tmp.Close()
	if writeErr != nil {
		os.Remove(tmpPath)
		return writeErr
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return closeErr
	}

	if verify != nil {
		tmpBytes, err := os.ReadFile(tmpPath)
		if err != nil {
			os.Remove(tmpPath)
			return err
		}
		if err := verify(tmpBytes); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("temp file failed to verify: %w", err)
		}
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if verify != nil {
		final, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := verify(final); err != nil {
			return fmt.Errorf("file failed to verify after swap: %w", err)
		}
	}
	return nil
}

// Write replaces the file at path with data atomically (a temp file in the
// same directory, then a rename), without taking a backup. An existing file
// keeps its permission bits; a new file gets perm. When path is a symlink
// to an existing file, the link's target is the file written and the link
// stays. On any failure the destination is left as it was and no temp file
// remains.
func Write(path string, data []byte, perm fs.FileMode) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	mode := perm
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return writeAtomic(path, data, nil, mode)
}
