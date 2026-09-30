// Package paths is the single catalogue of every filesystem location nerv
// resolves from a user's home directory: the user-scope nerv.yaml,
// gentle-ai's state.json, the Claude Code skills/commands directories,
// settings.json, the materialized marketplace, installed_plugins.json, and
// the per-version plugin cache agents directory. Every other package
// resolves these paths from Resolve instead of rebuilding the same joins
// by hand.
package paths

import "path/filepath"

// Paths is every filesystem location nerv resolves from a home directory.
type Paths struct {
	// Home is the resolved home directory every other field is derived
	// from.
	Home string
	// UserConfig is the user-scope nerv.yaml path.
	UserConfig string
	// State is gentle-ai's state.json path.
	State string
	// SkillsDir is the Claude Code user-scope skills directory.
	SkillsDir string
	// CommandsDir is where the Teamwork /task:* procedures are installed.
	CommandsDir string
	// Settings is Claude Code's settings.json path.
	Settings string
	// Marketplace is the materialized <home>/.nerv/marketplace directory.
	Marketplace string
	// InstalledPlugins is Claude Code's installed_plugins.json path.
	InstalledPlugins string
}

// Resolve builds Paths from a resolved home directory.
func Resolve(home string) Paths {
	return Paths{
		Home:             home,
		UserConfig:       filepath.Join(home, ".claude", "nerv", "nerv.yaml"),
		State:            filepath.Join(home, ".gentle-ai", "state.json"),
		SkillsDir:        filepath.Join(home, ".claude", "skills"),
		CommandsDir:      filepath.Join(home, ".claude", "commands", "task"),
		Settings:         filepath.Join(home, ".claude", "settings.json"),
		Marketplace:      filepath.Join(home, ".nerv", "marketplace"),
		InstalledPlugins: filepath.Join(home, ".claude", "plugins", "installed_plugins.json"),
	}
}

// CacheAgentsDir is where Claude Code caches nerv@nerv's agent frontmatter
// for a given plugin version:
// "<home>/.claude/plugins/cache/nerv/nerv/<version>/agents".
func (p Paths) CacheAgentsDir(pluginVersion string) string {
	return filepath.Join(p.Home, ".claude", "plugins", "cache", "nerv", "nerv", pluginVersion, "agents")
}
