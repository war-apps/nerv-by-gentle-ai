// Package configure implements the "nerv configure" use case: reading and
// editing the user-scope nerv.yaml, per-role model overrides, project-scope
// repo initialization, and installing the Teamwork /task:* slash commands.
// It is the non-interactive half of plugin/tools/configure.ps1 and
// plugin/tools/configure-models.ps1 (see tests/configure.test.ps1 groups
// F/G/H/J/K and tests/configure-models.test.ps1 groups D/E, this package's
// executable specification); the interactive wizard is P3.
//
// This package owns file I/O (Store) and process launches (through
// env.Runner) for the configure use case; cmd/nerv only wires flags into
// it. It never reads HOME/USERPROFILE directly — callers resolve Home
// through ResolveHome and pass it in via Deps, the same seam rule as every
// other internal package (see internal/env's doc comment).
package configure

import (
	"io/fs"
	"path/filepath"
	"time"

	"github.com/war-apps/nerv-gentle-ai/internal/env"
)

// Deps bundles configure's external effects so tests can inject fakes: no
// test outside internal/env's own tests should touch a real process, and no
// test in this package should touch the real home directory or the real
// embedded plugin tree unless it explicitly wants that realism.
type Deps struct {
	// Home is the already-resolved home directory (see ResolveHome) —
	// this package never reads $HOME/$USERPROFILE itself.
	Home string
	// FS is the embedded plugin tree, typically nerv.PluginFS(), rooted
	// so "agents/misato.md" and "skills-manifest.json" are
	// top-level entries.
	FS fs.FS
	// Runner launches "git" (InitRepo) and "gentle-ai"/PATH lookups
	// (Print's prerequisites).
	Runner env.Runner
	// Now supplies the clock used to name Store's backup files.
	Now func() time.Time
	// LookPath resolves a tool's presence on PATH (Print's prerequisites).
	LookPath func(name string) (string, error)
}

// Paths is every filesystem location "nerv configure" reads or writes,
// resolved from a home directory and an optional --config override.
type Paths struct {
	// Config is the user-scope nerv.yaml path.
	Config string
	// State is gentle-ai's state.json path, used to resolve from:<phase>
	// overrides for -Print's models table.
	State string
	// SkillsDir is the Claude Code user-scope skills directory.
	SkillsDir string
	// CommandsDir is where InstallCommands copies the Teamwork
	// /task:* procedures.
	CommandsDir string
}

// ResolveHome resolves the home directory to use: override when non-empty,
// otherwise env.HomeDir(). Mirrors configure.ps1's
// `$homeDirResolved = if ($HomeDir) { $HomeDir } else { Get-NervHomeDir }`.
func ResolveHome(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return env.HomeDir()
}

// ResolvePaths builds Paths from a resolved home directory and an optional
// --config override (empty means the default `<home>/.claude/nerv/nerv.yaml`).
// Mirrors configure.ps1's $userConfigPath/$statePathResolved resolution plus
// the skills/commands directories used by -Print and -InstallCommands.
func ResolvePaths(home, configOverride string) Paths {
	config := configOverride
	if config == "" {
		config = filepath.Join(home, ".claude", "nerv", "nerv.yaml")
	}
	return Paths{
		Config:      config,
		State:       filepath.Join(home, ".gentle-ai", "state.json"),
		SkillsDir:   filepath.Join(home, ".claude", "skills"),
		CommandsDir: filepath.Join(home, ".claude", "commands", "task"),
	}
}

// Change is one managed key's (or "models.<role>"'s) before/after value, as
// reported in Result.Changes.
type Change struct {
	Key  string `json:"key"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Result is the JSON summary shape shared by Set, SetModel, InitRepo, and
// InstallCommands: `{ changed, changes, written, warnings, config_path,
// backup }`. Mirrors configure.ps1's $niSummary.
type Result struct {
	Changed    bool     `json:"changed"`
	Changes    []Change `json:"changes"`
	Written    []string `json:"written"`
	Warnings   []string `json:"warnings"`
	ConfigPath string   `json:"config_path"`
	// Backup is nil (JSON null) when no backup was written this run.
	Backup *string `json:"backup"`
}

// RefusalError marks a rejected request — an unknown key, an invalid value,
// a malformed argument, a missing path, or a non-git directory — that
// should exit 1, as opposed to an unexpected environment failure (I/O,
// permissions, a missing home directory) that should exit 2. Mirrors
// configure.ps1's distinction between its many `Write-Host "...";  exit 1`
// rejections and its single outer `catch { ...; exit 2 }`.
type RefusalError struct {
	Err error
}

func (e *RefusalError) Error() string { return e.Err.Error() }
func (e *RefusalError) Unwrap() error { return e.Err }

// missingUserConfigHeader seeds the in-memory working document when the
// user-scope nerv.yaml does not exist yet, exactly as configure.ps1 does
// for its own non-interactive body, so Set/SetModel can auto-vivify the
// managed blocks the same way against either a real or a bootstrapped file.
// It is never written to disk by itself — only once a real managed value
// has actually changed and Store.Save has something new to persist.
const missingUserConfigHeader = "# ~/.claude/nerv/nerv.yaml -- NERV user-scope configuration\n# Generated/updated by nerv configure\n"

func emptyChanges() []Change {
	return []Change{}
}

func emptyStrings() []string {
	return []string{}
}
