package nerv

import (
	"strings"
	"testing"
)

// sectionOf returns the text from the line holding exactly heading to the next heading of the same
// or a higher level.
func sectionOf(text, heading string) string {
	i := strings.Index(text, "\n"+heading+"\n")
	if i < 0 {
		return ""
	}
	i++
	level := strings.Count(strings.Fields(heading)[0], "#")
	rest := text[i+len(heading):]
	for _, line := range strings.SplitAfter(rest, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			if n := len(trimmed) - len(strings.TrimLeft(trimmed, "#")); n <= level {
				return text[i : i+len(heading)+strings.Index(rest, line)]
			}
		}
	}
	return text[i:]
}

// S10: Misato's store modes must not override the rule that the ODD
// document is always a file plus its odd/{feature}/tasks mirror.
func TestS10_MisatoStoreModesKeepTheODDDocumentAFile(t *testing.T) {
	text := readPluginText(t, "agents/misato.md")
	for _, h := range []string{"### None mode", "### Engram mode"} {
		if s := sectionOf(text, h); !strings.Contains(s, "odd/tasks/{feature}.md") {
			t.Errorf("misato %q does not exempt the ODD document (always a file plus odd/{feature}/tasks)", h)
		}
	}
}

// S10: the lock is runtime and must be kept out of git when it is
// created, also in a fresh clone that has no .nerv/.gitignore yet.
func TestS10_LockIsExcludedAtCreation(t *testing.T) {
	s := normalizeSpace(sectionOf(readPluginText(t, orchestratorCore), "## Orchestrator lock"))
	if !strings.Contains(s, "creates `.nerv/.gitignore`") {
		t.Errorf("the lock relies on .nerv/ ignoring itself; create .nerv/.gitignore when creating the lock")
	}
}
