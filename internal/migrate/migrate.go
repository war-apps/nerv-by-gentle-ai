// Package migrate moves a repository and a user from nerv 4.2.0 to this
// version: the old configuration files (.nerv/nerv.yaml and
// ~/.claude/nerv/nerv.yaml) become .ai-workflow.yml and the user
// config.yml, the release: and pipeline: sections become ship: and
// deploy:, the semantic-release configuration is imported, the in-flight
// openspec changes become ODD feature documents, and the plugins the
// repository used are named for nerv install.
//
// Plan reads and computes every change without writing anything; Apply
// writes them, each file atomically, after a timestamped backup of every
// file it changes or removes. A second Plan after Apply has no change.
package migrate

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/atomicfile"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
)

// MigratedSuffix is appended to an old configuration file once it has been
// moved, so an older nerv no longer reads stale configuration.
const MigratedSuffix = ".migrated"

// BackupSuffix names the user scope's backups, next to each file
// (atomicfile's "<path>.<suffix><timestamp>").
const BackupSuffix = "bak-migrate-"

// Options is one migration.
type Options struct {
	// RepoRoot is the repository's top level; empty migrates no repository.
	RepoRoot string
	// Home is the user's home; with User it locates the user files.
	Home string
	// User migrates the user configuration, and moves the repository's
	// personal keys into it.
	User bool
	// ExtensionsFS holds the provider manifests (task and deploy).
	ExtensionsFS fs.FS
	// OriginURL is the repository's origin URL ("" when it has none).
	OriginURL string
	// Now names the backups and dates a lock's heartbeat.
	Now time.Time
	// Host is this machine's name, compared with a lock's host (""
	// reads os.Hostname).
	Host string
	// Force moves a change whose orchestrator lock is still held.
	Force bool
}

// Kind is what a Change does.
type Kind int

const (
	// Write creates or replaces Path with New.
	Write Kind = iota
	// Rename moves Path to To.
	Rename
	// Remove deletes Path.
	Remove
)

// Change is one file the migration writes, renames or removes.
type Change struct {
	Kind Kind
	// Path is absolute; To is a Rename's destination.
	Path, To string
	// Old is the current content (nil for a new file); New is a Write's.
	Old, New []byte
	// User marks a change of the user scope (backed up next to the file).
	User bool
	// StopAt bounds a Remove's cleanup of the directories it empties: they
	// are removed up to, not including, StopAt (default the repository).
	StopAt string
}

// Plan is everything one migration does.
type Plan struct {
	// RepoRoot is the repository migrated ("" for none).
	RepoRoot string
	Changes  []Change
	// Notes say what was translated; Warnings what has no equivalent or
	// needs the user's hand.
	Notes, Warnings []string
	// Plugins are the extension plugins the old configuration used, in
	// install order.
	Plugins []string
}

// Empty reports whether the plan changes nothing.
func (p *Plan) Empty() bool { return len(p.Changes) == 0 }

// InstallCommand is the nerv install line for the plugins ("" for none).
func (p *Plan) InstallCommand() string {
	if len(p.Plugins) == 0 {
		return ""
	}
	return "nerv install --with " + strings.Join(p.Plugins, ",")
}

func (p *Plan) note(format string, args ...any) {
	p.Notes = append(p.Notes, fmt.Sprintf(format, args...))
}
func (p *Plan) warn(format string, args ...any) {
	p.Warnings = append(p.Warnings, fmt.Sprintf(format, args...))
}

func refuse(format string, args ...any) error {
	return &refusal.Error{Err: fmt.Errorf(format, args...)}
}

// Compute plans the migration of o without writing anything.
func Compute(o Options) (*Plan, error) {
	p := &Plan{RepoRoot: o.RepoRoot}
	if o.RepoRoot == "" && !o.User {
		return nil, refuse("nothing to migrate: give a repository, or --user")
	}
	var user *userState
	if o.User {
		var err error
		if user, err = planUser(p, o); err != nil {
			return nil, err
		}
	}
	if o.RepoRoot != "" {
		if err := planRepo(p, o, user); err != nil {
			return nil, err
		}
	}
	if user != nil {
		user.finish(p)
	}
	return p, nil
}

// userState is the user configuration being built.
type userState struct {
	// path is the new user config; doc its content after the migration.
	path     string
	doc      *config.Document
	original []byte
	exists   bool
}

// set records a personal key moved into the user configuration.
func (u *userState) finish(p *Plan) {
	if u.doc == nil {
		return
	}
	text := []byte(u.doc.String())
	if u.exists && bytes.Equal(text, u.original) {
		return
	}
	if !u.exists && len(bytes.TrimSpace(text)) == 0 {
		return
	}
	c := Change{Kind: Write, Path: u.path, New: text, User: true}
	if u.exists {
		c.Old = u.original
	}
	// The user file is written before the old one is renamed.
	p.Changes = append([]Change{c}, p.Changes...)
}

// Apply writes the plan as one transaction: each changed or removed file
// is backed up first (the repository's under
// .nerv/backups/migrate-<timestamp>/ with its permission bits, the user's
// next to the file), then writes, renames and removes run in that order,
// each write atomic. When any step fails, every step already done is
// undone in reverse order, the backups and the runtime .gitignore this run
// created are removed, and the error says the run was rolled back. It
// returns the backups made (none after a rollback).
func Apply(p *Plan, now time.Time) (backups []string, err error) {
	t := &txn{root: p.RepoRoot, stamp: now.Format("20060102-150405")}
	defer func() {
		if err == nil {
			t.cleanup()
			backups = t.backups
			return
		}
		if rerr := t.rollback(); rerr != nil {
			backups = t.backups
			err = fmt.Errorf("%w; the rollback also failed (%v): restore from the backups listed", err, rerr)
			return
		}
		backups = nil
		err = fmt.Errorf("%w (every change was rolled back; nothing was migrated)", err)
	}()
	if p.RepoRoot != "" && needsRuntime(p) {
		t.mkdirs(paths.RuntimeDir(p.RepoRoot))
		ignore, err := paths.EnsureRuntimeIgnored(p.RepoRoot)
		if err != nil {
			return nil, err
		}
		if ignore != "" {
			t.undo = append(t.undo, func() error { return removeIfExists(ignore) })
		}
	}
	for _, kind := range []Kind{Write, Rename, Remove} {
		for _, c := range p.Changes {
			if c.Kind != kind {
				continue
			}
			if c.Kind == Write && c.Old == nil {
				// A new file already written (EnsureRuntimeIgnored's
				// .gitignore) needs neither a backup nor a write.
				if data, err := os.ReadFile(c.Path); err == nil && bytes.Equal(data, c.New) {
					continue
				}
			}
			if err := t.apply(c, now); err != nil {
				return nil, err
			}
		}
	}
	return nil, nil
}

// txn is one Apply: what it backed up and how to undo each step.
type txn struct {
	root, stamp string
	backups     []string
	// undo runs in reverse order on failure.
	undo []func() error
	// created are the directories this run created, deepest last.
	created []string
	// emptied are the source directories to remove, on success, when
	// empty, each with its stop directory.
	emptied [][2]string
}

// mkdirs creates dir and its missing parents, recording the new ones.
func (t *txn) mkdirs(dir string) error {
	var missing []string
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(d); err == nil || filepath.Dir(d) == d {
			break
		}
		missing = append(missing, d)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for i := len(missing) - 1; i >= 0; i-- {
		t.created = append(t.created, missing[i])
	}
	return nil
}

// backup copies a repository file under .nerv/backups/migrate-<stamp>/,
// keeping its permission bits.
func (t *txn) backup(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(t.root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("%s is outside the repository", path)
	}
	dst := filepath.Join(paths.RuntimeDir(t.root), "backups", "migrate-"+t.stamp, rel)
	if err := t.mkdirs(filepath.Dir(dst)); err != nil {
		return err
	}
	if err := writeMode(dst, data, info.Mode().Perm()); err != nil {
		return err
	}
	t.backups = append(t.backups, dst)
	return nil
}

// apply runs one change and records its undo.
func (t *txn) apply(c Change, now time.Time) error {
	if !c.User {
		if err := t.backup(c.Path); err != nil {
			return err
		}
	}
	switch c.Kind {
	case Write:
		prev, mode, existed, err := snapshot(c.Path)
		if err != nil {
			return err
		}
		if err := t.mkdirs(filepath.Dir(c.Path)); err != nil {
			return err
		}
		if c.User {
			b, err := atomicfile.Save(c.Path, c.New, atomicfile.Options{Now: now, BackupSuffix: BackupSuffix})
			if b != "" {
				t.backups = append(t.backups, b)
			}
			if err != nil {
				return err
			}
		} else if err := atomicfile.Write(c.Path, c.New, 0o644); err != nil {
			return err
		}
		t.undo = append(t.undo, func() error {
			if !existed {
				return removeIfExists(c.Path)
			}
			return atomicfile.Write(c.Path, prev, mode)
		})
	case Rename:
		if c.User {
			b := fmt.Sprintf("%s.%s%s", c.Path, BackupSuffix, t.stamp)
			if err := copyFile(c.Path, b); err != nil {
				return err
			}
			t.backups = append(t.backups, b)
		}
		if _, err := os.Lstat(c.To); err == nil {
			return fmt.Errorf("%s cannot move: %s appeared while nerv migrate ran", c.Path, c.To)
		}
		if err := t.mkdirs(filepath.Dir(c.To)); err != nil {
			return err
		}
		if err := os.Rename(c.Path, c.To); err != nil {
			return err
		}
		t.undo = append(t.undo, func() error { return os.Rename(c.To, c.Path) })
		if c.StopAt != "" {
			t.emptied = append(t.emptied, [2]string{filepath.Dir(c.Path), c.StopAt})
		}
	case Remove:
		prev, mode, existed, err := snapshot(c.Path)
		if err != nil {
			return err
		}
		if err := os.Remove(c.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if existed {
			t.undo = append(t.undo, func() error { return writeMode(c.Path, prev, mode) })
		}
		t.emptied = append(t.emptied, [2]string{filepath.Dir(c.Path), orDefault(c.StopAt, t.root)})
	}
	return nil
}

// rollback undoes every step in reverse order, then removes the backups
// and the directories this run created. It keeps the backups when a step
// cannot be undone.
func (t *txn) rollback() error {
	var errs []error
	for i := len(t.undo) - 1; i >= 0; i-- {
		if err := t.undo[i](); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	for _, b := range t.backups {
		_ = removeIfExists(b)
	}
	for i := len(t.created) - 1; i >= 0; i-- {
		_ = os.Remove(t.created[i]) // only when empty
	}
	return nil
}

// cleanup removes the source directories a successful run emptied.
func (t *txn) cleanup() {
	for _, e := range t.emptied {
		removeEmptyParents(e[0], e[1])
	}
}

// snapshot reads a file's content and mode (existed false when absent).
func snapshot(path string) ([]byte, fs.FileMode, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, false, err
	}
	return data, info.Mode().Perm(), true, nil
}

// writeMode writes data to path with exactly mode (umask aside).
func writeMode(path string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// needsRuntime reports whether the plan writes under the repository.
func needsRuntime(p *Plan) bool {
	for _, c := range p.Changes {
		if !c.User {
			return true
		}
	}
	return false
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}

// removeEmptyParents removes dir and its parents while they are empty,
// up to (not including) root.
func removeEmptyParents(dir, root string) {
	for root != "" && dir != root && strings.HasPrefix(dir, root) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// Render writes the plan as a unified diff per written file and one line
// per rename and removal, paths relative to the repository (or absolute
// for the user files), sorted by path within each kind.
func Render(p *Plan) string {
	var b strings.Builder
	rel := func(path string) string {
		if p.RepoRoot != "" {
			if r, err := filepath.Rel(p.RepoRoot, path); err == nil && !strings.HasPrefix(r, "..") {
				return filepath.ToSlash(r)
			}
		}
		return path
	}
	changes := append([]Change(nil), p.Changes...)
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].Kind != changes[j].Kind {
			return changes[i].Kind < changes[j].Kind
		}
		return changes[i].Path < changes[j].Path
	})
	for _, c := range changes {
		switch c.Kind {
		case Write:
			b.WriteString(UnifiedDiff(rel(c.Path), c.Old, c.New))
		case Rename:
			fmt.Fprintf(&b, "move %s -> %s\n", rel(c.Path), rel(c.To))
		case Remove:
			fmt.Fprintf(&b, "remove %s\n", rel(c.Path))
		}
	}
	return b.String()
}
