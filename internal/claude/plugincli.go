package claude

import (
	"github.com/war-apps/nerv-kit-by-gentle-ai/claudeplugin"
)

// PluginCLI drives Claude Code's "claude plugin" subcommand — the
// marketplace add/remove plus the uninstall/install pair that refreshes the
// plugin cache — through an injected runner.Runner. It is
// claudeplugin.PluginCLI, re-exported so every nerv package keeps importing
// this package instead of the kit directly.
type PluginCLI = claudeplugin.PluginCLI
