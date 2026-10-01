package configure

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ProceduresDir is where the embedded plugin tree carries the Teamwork
// /task:* slash-command procedures, relative to deps.FS's root (i.e.
// plugin/skills/nerv-tasks/providers/teamwork/procedures on disk). Exported
// so internal/wizard can check its existence itself before offering to
// install the commands (see this package's InstallCommands, its own write
// path).
const ProceduresDir = "skills/nerv-tasks/providers/teamwork/procedures"

// InstallCommands copies every "*.md" file from the embedded Teamwork
// procedures directory into paths.CommandsDir, never overwriting a file
// that already exists there — that case is reported as a warning, not an
// error. A missing procedures directory (never expected in a real build,
// since it ships inside the embedded plugin tree) is likewise a warning,
// not a failure.
func InstallCommands(deps Deps, paths Paths) (Result, error) {
	entries, err := fs.ReadDir(deps.FS, ProceduresDir)

	warnings := emptyStrings()
	written := emptyStrings()
	changed := false

	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Teamwork procedures not found at %s; skipping.", ProceduresDir))
	} else {
		if err := os.MkdirAll(paths.CommandsDir, 0o755); err != nil {
			return Result{}, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			destFile := filepath.Join(paths.CommandsDir, entry.Name())
			if _, statErr := os.Stat(destFile); statErr == nil {
				warnings = append(warnings, fmt.Sprintf("Skipped (already exists): %s", entry.Name()))
				continue
			}
			data, err := fs.ReadFile(deps.FS, ProceduresDir+"/"+entry.Name())
			if err != nil {
				return Result{}, err
			}
			if err := os.WriteFile(destFile, data, 0o644); err != nil {
				return Result{}, err
			}
			written = append(written, destFile)
			changed = true
		}
	}

	return Result{
		Changed:    changed,
		Changes:    emptyChanges(),
		Written:    written,
		Warnings:   warnings,
		ConfigPath: paths.Config,
		Backup:     nil,
	}, nil
}
