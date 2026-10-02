package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
)

// PluginInstallInfo is one plugin's recorded install path and version, as
// read from installed_plugins.json.
type PluginInstallInfo struct {
	InstallPath string
	Version     string
}

// InstalledPlugins reads path (typically
// "<home>/.claude/plugins/installed_plugins.json") and returns the first
// recorded entry for "nerv@nerv". A missing file reports found=false with
// no error; malformed JSON is an error. The embedded plugin has no git
// commit, so cache verification compares this recorded Version instead of
// a gitCommitSha.
func InstalledPlugins(path string) (PluginInstallInfo, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return PluginInstallInfo{}, false, nil
		}
		return PluginInstallInfo{}, false, err
	}

	var doc struct {
		Plugins map[string][]struct {
			InstallPath string `json:"installPath"`
			Version     string `json:"version"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return PluginInstallInfo{}, false, fmt.Errorf("parse %s: %w", path, err)
	}

	entries := doc.Plugins["nerv@nerv"]
	if len(entries) == 0 {
		return PluginInstallInfo{}, false, nil
	}
	return PluginInstallInfo{InstallPath: entries[0].InstallPath, Version: entries[0].Version}, true, nil
}

// CacheAgentsDir is where Claude Code caches nerv@nerv's agent
// frontmatter for a given plugin version:
// "<home>/.claude/plugins/cache/nerv/nerv/<version>/agents".
func CacheAgentsDir(home, pluginVersion string) string {
	return paths.Paths{Home: home}.CacheAgentsDir(pluginVersion)
}
