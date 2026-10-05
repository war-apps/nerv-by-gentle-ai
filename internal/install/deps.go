// Package install implements the "nerv install", "nerv uninstall", and
// "nerv apply-models" use cases: materializing the embedded plugin,
// registering it in Claude Code's settings.json, refreshing the plugin
// cache, applying model/effort assignments to the cached agents, ensuring
// the Engram "nerv" knowledge base, and installing skills. Registration
// targets a materialized <home>/.nerv/marketplace directory (the plugin
// is embedded in the binary, not a git checkout), and cache verification
// compares the embedded plugin.json version rather than a git commit SHA.
package install

import (
	"io"
	"io/fs"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
)

// Deps bundles every external effect Install/Uninstall/ApplyModels need,
// so tests inject fakes and a real home directory is never touched
// outside this package's own tests (the 2026-09-28 installer incident
// rule, see internal/env's doc comment).
type Deps struct {
	// Home is the resolved home directory; every path this package reads
	// or writes (settings.json, the marketplace, the plugin cache, the
	// user config) is derived from it unless SettingsPath overrides the
	// settings.json location specifically.
	Home string
	// SettingsPath overrides the resolved Claude Code settings.json
	// path. Empty means "<Home>/.claude/settings.json".
	SettingsPath string
	// FS is the embedded plugin tree, typically nerv.PluginFS().
	FS fs.FS
	// Runner launches "gentle-ai", "claude plugin ...", "engram ...",
	// and "npx ..." (skills installs).
	Runner env.Runner
	// Now supplies the clock used to name settings.json's backup file.
	Now func() time.Time
	// LookPath resolves a tool's presence on PATH (engram detection).
	LookPath func(name string) (string, error)
	// ConfirmSync, when set, is asked before the skills step runs the
	// gentle-ai sync (which also rewrites gentle-ai's managed files). Nil
	// means no sync: non-interactive callers leave it unset. Injected so this
	// package never reads os.Stdin.
	ConfirmSync func(skills []string) bool
	// Stdout receives this package's step-by-step progress log.
	Stdout io.Writer
}

// Options are Install's command-line switches.
type Options struct {
	// RequireGentleAI turns a missing or non-4.x gentle-ai into a
	// RefusalError instead of a warning.
	RequireGentleAI bool
	// NoSkills skips the skills-install step entirely.
	NoSkills bool
}

// RefusalError marks a rejected install/uninstall/apply-models request —
// a required-but-missing gentle-ai, or a plugin cache that did not
// refresh to the expected version — that should exit 1, as opposed to an
// unexpected environment failure (I/O, a malformed embedded plugin) that
// should exit 2. Mirrors configure.RefusalError's role for "nerv
// configure"; both are aliases for the one shared refusal type (see
// internal/refusal).
type RefusalError = refusal.Error
