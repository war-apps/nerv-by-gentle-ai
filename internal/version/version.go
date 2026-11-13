// Package version exposes the binary and plugin version information printed
// by "nerv version" and asserted by the release guard.
package version

import (
	"io/fs"

	"github.com/war-apps/nerv-kit-by-gentle-ai/claudeplugin"
)

// Binary is the nerv binary version. It defaults to "dev" for local builds
// and is overridden at release time with:
//
//	-ldflags "-X github.com/war-apps/nerv-by-gentle-ai/internal/version.Binary=<tag>"
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
	return claudeplugin.PluginVersion(fsys)
}
