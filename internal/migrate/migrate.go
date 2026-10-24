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
	// Now names the backups.
	Now time.Time
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

// Apply writes the plan: each changed or removed file is backed up first
// (the repository's under .nerv/backups/migrate-<timestamp>/, the user's
// next to the file), then writes, renames and removes run in that order,
// each write atomic. It returns the backups made.
func Apply(p *Plan, now time.Time) ([]string, error) {
	stamp := now.Format("20060102-150405")
	var backups []string
	if p.RepoRoot != "" && needsRuntime(p) {
		if _, err := paths.EnsureRuntimeIgnored(p.RepoRoot); err != nil {
			return nil, err
		}
	}
	backup := func(c Change, path string) error {
		if c.User {
			return nil // atomicfile.Save backs the user files up itself.
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(p.RepoRoot, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("%s is outside the repository", path)
		}
		dst := filepath.Join(paths.RuntimeDir(p.RepoRoot), "backups", "migrate-"+stamp, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
		backups = append(backups, dst)
		return nil
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
			if err := backup(c, c.Path); err != nil {
				return backups, err
			}
			switch c.Kind {
			case Write:
				if c.User {
					b, err := atomicfile.Save(c.Path, c.New, atomicfile.Options{Now: now, BackupSuffix: BackupSuffix})
					if err != nil {
						return backups, err
					}
					if b != "" {
						backups = append(backups, b)
					}
					continue
				}
				if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
					return backups, err
				}
				if err := atomicfile.Write(c.Path, c.New, 0o644); err != nil {
					return backups, err
				}
			case Rename:
				if c.User {
					b := fmt.Sprintf("%s.%s%s", c.Path, BackupSuffix, stamp)
					if err := copyFile(c.Path, b); err != nil {
						return backups, err
					}
					backups = append(backups, b)
				}
				if err := os.MkdirAll(filepath.Dir(c.To), 0o755); err != nil {
					return backups, err
				}
				if err := os.Rename(c.Path, c.To); err != nil {
					return backups, err
				}
				if c.StopAt != "" {
					removeEmptyParents(filepath.Dir(c.Path), c.StopAt)
				}
			case Remove:
				if err := os.Remove(c.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return backups, err
				}
				removeEmptyParents(filepath.Dir(c.Path), orDefault(c.StopAt, p.RepoRoot))
			}
		}
	}
	return backups, nil
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
