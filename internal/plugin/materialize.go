// Package plugin materializes the embedded plugin tree onto disk and writes
// the directory-marketplace manifest that registers it in Claude Code.
package plugin

import (
	"io/fs"

	"github.com/war-apps/nerv-kit-by-gentle-ai/claudeplugin"
)

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
// directory marketplace named "nerv". It is idempotent: a file whose
// bytes already match the source is left untouched, and files under
// dest/plugin or dest/plugins (where an earlier nerv materialized its
// extension plugins) that are no longer present in src are removed. It
// never writes or removes anything outside dest/plugin, dest/plugins and
// dest/.claude-plugin.
func Materialize(src fs.FS, dest string) (Result, error) {
	return claudeplugin.Materialize(src, dest, marketplace)
}
