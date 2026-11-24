//go:build !windows

package main

import (
	"os"

	"golang.org/x/term"
)

// stdoutColor reports whether stdout is a terminal; Unix terminals show
// ANSI colors as they are.
func stdoutColor() bool { return term.IsTerminal(int(os.Stdout.Fd())) }
