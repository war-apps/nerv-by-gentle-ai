package main

import "github.com/war-apps/nerv-gentle-ai/internal/refusal"

// exitCodeFor maps a use-case error to the process exit code every nerv
// subcommand uses: 0 for nil, 1 for a *refusal.Error (an unknown key, an
// invalid value, a missing tag section, ...), 2 for anything else (an
// unexpected environment failure).
func exitCodeFor(err error) int {
	if err == nil {
		return 0
	}
	if refusal.Is(err) {
		return 1
	}
	return 2
}
