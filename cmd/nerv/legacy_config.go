package main

import (
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
)

// withLegacyHint appends to a "not configured" message, on its own line
// indented under it, the "nerv migrate" notice when the repository at top
// has only the old .nerv/nerv.yaml, which nerv never reads as
// configuration.
func withLegacyHint(msg, top string) string {
	if hint := workflowconfig.LegacyHint(paths.LegacyProjectConfig(top), paths.ProjectConfig(top)); hint != "" {
		return msg + "\n  " + hint
	}
	return msg
}
