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
// Kept as a package-level function variable, the same seam every other
// external effect in this codebase uses: no test should depend on the
// real process's actual stdin, and the real x/term check itself cannot
// be exercised from a unit test (it depends on a real OS file
// descriptor/console) — only this seam's two outcomes are tested.
var stdinIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
