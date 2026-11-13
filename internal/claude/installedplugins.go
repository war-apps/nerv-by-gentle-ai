package claude

import (
	"github.com/war-apps/nerv-kit-by-gentle-ai/claudeplugin"
)

// PluginInstallInfo is one plugin's recorded install path and version, as
// read from installed_plugins.json. It is claudeplugin.PluginInstallInfo,
// re-exported so every nerv package keeps importing this package instead
// of the kit directly.
type PluginInstallInfo = claudeplugin.PluginInstallInfo

// InstalledPlugins reads path (typically
// "<home>/.claude/plugins/installed_plugins.json") and returns the first
// recorded entry for "nerv@nerv". A missing file reports found=false with
// no error; malformed JSON is an error. The embedded plugin has no git
// commit, so cache verification compares this recorded Version instead of
// a gitCommitSha.
func InstalledPlugins(path string) (PluginInstallInfo, bool, error) {
	return InstalledPlugin(path, "nerv@nerv")
}

// InstalledPlugin is InstalledPlugins for any plugin id, such as an
// extension's "task@nerv".
func InstalledPlugin(path, id string) (PluginInstallInfo, bool, error) {
	return claudeplugin.InstalledPlugin(path, id)
}

// CacheAgentsDir is where Claude Code caches nerv@nerv's agent
// frontmatter for a given plugin version:
// "<home>/.claude/plugins/cache/nerv/nerv/<version>/agents".
func CacheAgentsDir(home, pluginVersion string) string {
	return claudeplugin.CacheAgentsDir(home, "nerv", "nerv", pluginVersion)
}
