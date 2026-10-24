// Package nerv embeds the Nerv by Gentle-AI plugin tree, and the optional
// extension plugins shipped in the same marketplace, so they can be
// materialized on disk by the nerv CLI without a network round trip.
package nerv

import (
	"embed"
	"io/fs"
)

// pluginFS embeds the whole plugin tree, including dot-files such as
// plugin/.claude-plugin/plugin.json, hence the "all:" prefix.
//
//go:embed all:plugin
var pluginFS embed.FS

// extensionsFS embeds the extension plugins, one folder per plugin under
// plugins/ (the task plugin and its providers).
//
//go:embed all:plugins
var extensionsFS embed.FS

// PluginFS returns the embedded plugin tree rooted at "plugin", so callers
// see paths like ".claude-plugin/plugin.json" instead of
// "plugin/.claude-plugin/plugin.json".
func PluginFS() fs.FS {
	sub, err := fs.Sub(pluginFS, "plugin")
	if err != nil {
		// fs.Sub only fails for a malformed root name; "plugin" is a
		// constant, so this is unreachable in practice.
		panic(err)
	}
	return sub
}

// ExtensionsFS returns the embedded extension plugins rooted at "plugins",
// so callers see one folder per plugin, such as "task/.claude-plugin/plugin.json".
func ExtensionsFS() fs.FS {
	sub, err := fs.Sub(extensionsFS, "plugins")
	if err != nil {
		// Unreachable for the constant root, as in PluginFS.
		panic(err)
	}
	return sub
}
