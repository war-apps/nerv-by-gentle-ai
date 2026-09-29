package main

import "os"

// stdinIsTerminal reports whether os.Stdin is attached to a real terminal
// (character device), the same std-library-only heuristic used everywhere
// else in this codebase's seams: no test should depend on the real
// process's actual stdin, so this is a package-level function variable
// tests can override.
var stdinIsTerminal = func() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
