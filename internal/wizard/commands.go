package wizard

import (
	"fmt"
	"io"
	"io/fs"

	"github.com/war-apps/nerv-gentle-ai/internal/configure"
)

// runCommandsSection is Section 4: an offer to copy the embedded Teamwork
// /task:* procedures into paths.CommandsDir, through configure.InstallCommands
// — the same write path "nerv configure --install-commands" uses. Mirrors
// configure.ps1's "-- Section 4: Slash commands --" (~2042-2075).
func runCommandsSection(deps Deps, paths configure.Paths, s *session, out io.Writer) (bool, error) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "--- Slash commands ---")

	if _, err := fs.ReadDir(deps.FS, configure.ProceduresDir); err != nil {
		fmt.Fprintf(out, "Teamwork procedures not found at %s; skipping.\n", configure.ProceduresDir)
		return false, nil
	}

	if !s.yesNo(fmt.Sprintf("Install the Teamwork procedures as /task:* commands in %s?", paths.CommandsDir), false) {
		return false, nil
	}

	result, err := configure.InstallCommands(deps, paths)
	if err != nil {
		return false, err
	}

	fmt.Fprintf(out, "Copied: %d file(s).\n", len(result.Written))
	for _, w := range result.Warnings {
		fmt.Fprintln(out, w)
	}
	return result.Changed, nil
}
