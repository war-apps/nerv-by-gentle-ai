// Package configure implements the "nerv configure" use case: reading and
// editing the user-scope nerv.yaml, per-role model overrides, project-scope
// repo initialization, and installing the Teamwork /task:* slash commands.
// It is the non-interactive half of "nerv configure"; the interactive
// setup wizard lives in internal/wizard.
//
// This package owns file I/O (Store) and process launches (through
// env.Runner) for the configure use case; cmd/nerv only wires flags into
// it. It never reads HOME/USERPROFILE directly — callers resolve Home
// through ResolveHome and pass it in via Deps, the same seam rule as every
// other internal package (see internal/env's doc comment).
package configure

import (
	"io/fs"
	"time"

	"github.com/war-apps/nerv-gentle-ai/internal/env"
	"github.com/war-apps/nerv-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-gentle-ai/internal/refusal"
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
// otherwise env.HomeDir().
func ResolveHome(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return env.HomeDir()
}

// ResolvePaths builds Paths from a resolved home directory and an optional
// --config override (empty means the default `<home>/.claude/nerv/nerv.yaml`),
// plus the skills/commands directories --print and --install-commands use.
// The joins themselves live in the shared internal/paths catalogue.
func ResolvePaths(home, configOverride string) Paths {
	resolved := paths.Resolve(home)
	config := configOverride
	if config == "" {
		config = resolved.UserConfig
	}
	return Paths{
		Config:      config,
		State:       resolved.State,
		SkillsDir:   resolved.SkillsDir,
		CommandsDir: resolved.CommandsDir,
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
// backup }`.
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
// permissions, a missing home directory) that should exit 2. It is an
// alias for the one shared refusal type every nerv use case constructs
// (see internal/refusal).
type RefusalError = refusal.Error

// missingUserConfigHeader seeds the in-memory working document when the
// user-scope nerv.yaml does not exist yet, so Set/SetModel can auto-vivify
// the managed blocks the same way against either a real or a bootstrapped
// file. It is never written to disk by itself — only once a real managed
// value has actually changed and Store.Save has something new to persist.
const missingUserConfigHeader = "# ~/.claude/nerv/nerv.yaml -- NERV user-scope configuration\n# Generated/updated by nerv configure\n"

func emptyChanges() []Change {
	return []Change{}
}

func emptyStrings() []string {
	return []string{}
}
