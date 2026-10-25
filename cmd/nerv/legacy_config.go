package main

import (
	"fmt"
	"io"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
)

// withLegacyHint appends to a "not configured" message, on its own line
// indented under it, the "nerv migrate" notice when the repository at top
// has only the 4.2.0 .nerv/nerv.yaml, which nerv never reads as
// configuration.
func withLegacyHint(msg, top string) string {
	if hint := workflowconfig.LegacyHint(paths.LegacyProjectConfig(top), paths.ProjectConfig(top)); hint != "" {
		return msg + "\n  " + hint
	}
	return msg
}

// refusedPreviousUser reports, on stderr as "nerv <command>: ...", a
// previous user config.yml that home still has, which nerv never reads:
// the command then exits 1 instead of silently ignoring its settings.
func refusedPreviousUser(stderr io.Writer, command, home string) bool {
	if err := paths.Resolve(home).RefusePreviousUser(); err != nil {
		fmt.Fprintf(stderr, "nerv %s: %v\n", command, err)
		return true
	}
	return false
}
