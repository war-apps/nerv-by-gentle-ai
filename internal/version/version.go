// Package version exposes the binary and plugin version information printed
// by "nerv version" and asserted by the release guard.
package version

import (
	"encoding/json"
	"fmt"
	"io/fs"
)

// Binary is the nerv binary version. It defaults to "dev" for local builds
// and is overridden at release time with:
//
//	-ldflags "-X github.com/war-apps/nerv-gentle-ai/internal/version.Binary=<tag>"
var Binary = "dev"

// Info bundles the binary and plugin versions together for callers that
// need both, such as "nerv version --json".
type Info struct {
	Binary string `json:"binary"`
	Plugin string `json:"plugin"`
}

// PluginVersion reads the "version" field from .claude-plugin/plugin.json in
// the given filesystem.
func PluginVersion(fsys fs.FS) (string, error) {
	data, err := fs.ReadFile(fsys, ".claude-plugin/plugin.json")
	if err != nil {
		return "", fmt.Errorf("reading .claude-plugin/plugin.json: %w", err)
	}

	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("parsing .claude-plugin/plugin.json: %w", err)
	}
	if manifest.Version == "" {
		return "", fmt.Errorf(".claude-plugin/plugin.json: version is empty")
	}
	return manifest.Version, nil
}
