package main

import (
	"os"

	"golang.org/x/term"
)

// stdinIsTerminal reports whether os.Stdin is attached to a real
// interactive terminal, via golang.org/x/term's real termios/console
// check. This replaced the previous std-library-only
// os.ModeCharDevice heuristic: on Windows git-bash, redirecting stdin
// from /dev/null (or NUL) still reports as a character device, so that
// heuristic misread a non-interactive invocation
// (`nerv configure --home <tmp> < /dev/null`, back when --home was still
// a public flag — see options.Home's doc comment for why it no longer
// is) as a terminal — the wizard
// then ran for real, treating every EOF as "keep every default and
// answer Y to every yes/no" and executing `claude plugin
// uninstall/install` plus `npx skills add -g` on the developer machine
// (2026-09-29 incident).
//
// Wired into options.IsTerminal by defaultOptions; every other caller
// goes through that field, so tests inject a fixed outcome instead of
// depending on the real process's actual stdin.
func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
