package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/war-apps/nerv-by-gentle-ai/internal/skills"
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

// syncConfirmer returns the confirmation hook for the gentle-ai sync: nil
// unless stdin is a terminal (a non-interactive run never syncs), otherwise a
// [y/N] prompt on stdout answered from opts.Stdin. The default is No on a
// blank line or EOF. The answer is read one byte at a time so nothing past
// the line is consumed: the wizard reads the same stdin right after.
func syncConfirmer(opts options, stdout io.Writer) func([]string) bool {
	if opts.IsTerminal == nil || !opts.IsTerminal() || opts.Stdin == nil {
		return nil
	}
	return func(names []string) bool {
		fmt.Fprintf(stdout, "%s [y/N]: ", skills.SyncPrompt(names))
		var line []byte
		sawNewline := false
		buf := make([]byte, 1)
		for {
			n, err := opts.Stdin.Read(buf)
			if n == 1 {
				if buf[0] == '\n' {
					sawNewline = true
					break
				}
				line = append(line, buf[0])
			}
			if err != nil {
				break
			}
		}
		if !sawNewline {
			fmt.Fprintln(stdout)
		}
		answer := strings.ToLower(strings.TrimSpace(string(line)))
		return answer == "y" || answer == "yes"
	}
}
