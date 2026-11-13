// Package plugin materializes the embedded plugin tree onto disk and writes
// the directory-marketplace manifest that registers it in Claude Code.
package plugin

import (
	"io/fs"

	"github.com/war-apps/nerv-kit-by-gentle-ai/claudeplugin"
)

// pluginManifestPath is every plugin's own manifest, relative to its root;
// Extensions (extensions.go) reads it to discover and describe each
// extension and provider folder.
const pluginManifestPath = ".claude-plugin/plugin.json"

// marketplace identifies nerv's own directory marketplace: the core
// plugin's name and this repository's maintainer, exactly as nerv always
// registered it before Materialize moved to nerv-kit.
var marketplace = claudeplugin.Marketplace{Name: "nerv", OwnerName: "Walter Rodriguez"}

// Result reports what Materialize did, as paths relative to dest. It is
// claudeplugin.Result, re-exported so callers keep importing this package
// instead of the kit directly.
type Result = claudeplugin.Result

// Materialize writes the whole tree from src under dest/plugin, byte-exact,
// and writes dest/.claude-plugin/marketplace.json registering it as a
// directory marketplace named "nerv". It is MaterializeBundle with no
// extensions and providers.
func Materialize(src fs.FS, dest string) (Result, error) {
	return claudeplugin.Materialize(src, dest, marketplace)
}

// MaterializeBundle writes the core plugin tree from core under
// dest/plugin and every extension and provider folder of extensions (one
// folder per plugin, see Extensions) under dest/plugins/<name>, byte-exact,
// then writes dest/.claude-plugin/marketplace.json listing the core plugin
// first and the extensions in name order. It is idempotent: a file whose
// bytes already match the source is left untouched, and files under
// dest/plugin or dest/plugins that are no longer present in a source are
// removed. It never writes or removes anything outside dest/plugin,
// dest/plugins and dest/.claude-plugin. A nil extensions writes none.
func MaterializeBundle(core, extensions fs.FS, dest string) (Result, error) {
	return claudeplugin.MaterializeBundle(core, extensions, dest, marketplace)
}
