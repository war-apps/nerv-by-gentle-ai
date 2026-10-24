// Package paths is the single catalogue of every filesystem location nerv
// resolves from a user's home directory or a repository root: the
// user-scope config.yml and the project-scope .ai-workflow.yml (plus the
// legacy nerv.yaml locations nerv no longer reads, kept only to point at
// "nerv migrate"), the .nerv/ runtime folder, gentle-ai's state.json, the Claude Code skills directory,
// settings.json, the materialized marketplace, installed_plugins.json, and
// the per-version plugin cache agents directory. Every other package
// resolves these paths from Resolve instead of rebuilding the same joins
// by hand.
package paths

import (
	"errors"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// ProjectConfigFile is the project-scope configuration file, at the
// repository root. It is meant to be committed.
const ProjectConfigFile = ".ai-workflow.yml"

// RuntimeDirName is the repository's runtime-only folder (artifacts, the
// orchestrator lock, logs). It is git-ignored and never holds configuration.
const RuntimeDirName = ".nerv"

// EnsureRuntimeIgnored writes <repoRoot>/.nerv/.gitignore of "*" when it
// is missing, so the runtime folder is never committed without touching the
// repository's own .gitignore. Every writer under .nerv/ calls it. It
// returns the path written, or "" when the file already existed.
func EnsureRuntimeIgnored(repoRoot string) (string, error) {
	ignore := filepath.Join(RuntimeDir(repoRoot), ".gitignore")
	if _, err := os.Lstat(ignore); !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(RuntimeDir(repoRoot), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(ignore, []byte("*\n"), 0o644); err != nil {
		return "", err
	}
	return ignore, nil
}

// ProjectConfig is the project-scope configuration path of the repository
// at repoRoot: "<repoRoot>/.ai-workflow.yml".
func ProjectConfig(repoRoot string) string {
	return filepath.Join(repoRoot, ProjectConfigFile)
}

// LegacyProjectConfig is where the project configuration lived before
// .ai-workflow.yml: "<repoRoot>/.nerv/nerv.yaml". nerv never reads it as
// configuration; it only reports that "nerv migrate" moves it.
func LegacyProjectConfig(repoRoot string) string {
	return filepath.Join(repoRoot, RuntimeDirName, "nerv.yaml")
}

// RuntimeDir is the repository's runtime folder: "<repoRoot>/.nerv".
func RuntimeDir(repoRoot string) string {
	return filepath.Join(repoRoot, RuntimeDirName)
}

// DefaultConfigDir is os.UserConfigDir's layout for home on goos, without
// reading the environment: "<home>/AppData/Roaming" on Windows,
// "<home>/Library/Application Support" on macOS, "<home>/lib" on Plan 9
// and "<home>/.config" everywhere else.
func DefaultConfigDir(home, goos string) string {
	switch goos {
	case "windows":
		return filepath.Join(home, "AppData", "Roaming")
	case "darwin", "ios":
		return filepath.Join(home, "Library", "Application Support")
	case "plan9":
		return filepath.Join(home, "lib")
	default:
		return filepath.Join(home, ".config")
	}
}

// userConfigDir is os.UserConfigDir when home is the real user's home, so
// XDG_CONFIG_HOME or %AppData% apply exactly as the platform defines them.
// Any other home (a test's fake one) gets DefaultConfigDir, so the real
// environment never redirects a fake home's configuration to the real
// user's.
func userConfigDir(home string) string {
	if real := realHomeDir(); real != "" && sameDir(real, home) {
		if dir, err := os.UserConfigDir(); err == nil {
			return dir
		}
	}
	return DefaultConfigDir(home, runtime.GOOS)
}

// realHomeDir is the account's home from the user database, not from
// $HOME, so a sandbox that fakes $HOME is never mistaken for the real home
// (os.UserConfigDir would follow its XDG_CONFIG_HOME). It falls back to
// os.UserHomeDir when the user database is unavailable.
func realHomeDir() string {
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		return u.HomeDir
	}
	if dir, err := os.UserHomeDir(); err == nil {
		return dir
	}
	return ""
}

// sameDir compares two directories, case-insensitively on Windows.
func sameDir(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Paths is every filesystem location nerv resolves from a home directory.
type Paths struct {
	// Home is the resolved home directory every other field is derived
	// from.
	Home string
	// UserConfig is the user-scope configuration path:
	// "<os.UserConfigDir()>/ai-workflow/config.yml".
	UserConfig string
	// LegacyUserConfig is where the user configuration lived before
	// config.yml: "<home>/.claude/nerv/nerv.yaml". nerv never reads it as
	// configuration; it only reports that "nerv migrate" moves it.
	LegacyUserConfig string
	// State is gentle-ai's state.json path.
	State string
	// SkillsDir is the Claude Code user-scope skills directory.
	SkillsDir string
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
		UserConfig:       filepath.Join(userConfigDir(home), "ai-workflow", "config.yml"),
		LegacyUserConfig: filepath.Join(home, ".claude", "nerv", "nerv.yaml"),
		State:            filepath.Join(home, ".gentle-ai", "state.json"),
		SkillsDir:        filepath.Join(home, ".claude", "skills"),
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
