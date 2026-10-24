package install

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// legacyCommandNames are the /task:* procedures earlier releases copied
// into <home>/.claude/commands/task. Such a copy shadows the command the
// task@nerv plugin serves under the same name.
var legacyCommandNames = []string{
	"all", "analysis", "blocked", "canceled", "clear", "close", "done", "me",
	"pending", "priority", "start", "stop", "stopAll", "take", "testing", "timers",
}

// legacyCommandsDir is where earlier releases copied the /task:* procedures.
func legacyCommandsDir(home string) string {
	return filepath.Join(home, ".claude", "commands", "task")
}

// findLegacyCommands lists, in name order, the old copies in dir: the *.md
// files named after one of legacyCommandNames. A missing dir has none.
func findLegacyCommands(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		name, isMarkdown := strings.CutSuffix(e.Name(), ".md")
		if e.IsDir() || !isMarkdown || !slices.Contains(legacyCommandNames, name) {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	return files
}

// reportLegacyCommands prints the old /task:* copies Install finds and, when
// deps.ConfirmRemoveLegacyCommands accepts, deletes exactly those files and
// then the directory when nothing else is left in it. Otherwise it prints
// the remedy. A failed removal is a warning, never an install failure.
func reportLegacyCommands(deps Deps) {
	dir := legacyCommandsDir(deps.Home)
	files := findLegacyCommands(dir)
	if len(files) == 0 {
		return
	}

	fmt.Fprintf(deps.Stdout, "\nOld /task:* command copies found in %s:\n", dir)
	for _, f := range files {
		fmt.Fprintf(deps.Stdout, "  %s\n", f)
	}

	if deps.ConfirmRemoveLegacyCommands == nil || !deps.ConfirmRemoveLegacyCommands(files) {
		fmt.Fprintln(deps.Stdout, "These copies shadow the task@nerv plugin's /task:* commands and can be removed.")
		return
	}

	removed := 0
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			fmt.Fprintf(deps.Stdout, "Warning: could not remove %s: %v\n", f, err)
			continue
		}
		removed++
	}
	fmt.Fprintf(deps.Stdout, "Removed %d old /task:* command copies.\n", removed)

	if left, err := os.ReadDir(dir); err == nil && len(left) == 0 {
		if err := os.Remove(dir); err != nil {
			fmt.Fprintf(deps.Stdout, "Warning: could not remove %s: %v\n", dir, err)
		}
	}
}
