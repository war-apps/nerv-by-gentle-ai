package nerv

import (
	"io/fs"
	"strings"
	"testing"
)

// S6: the orchestrator hands each pipeline event to the installed
// extensions whose extension.yaml subscribes to it, through their agent
// entry; it names no extension of its own.
func TestOrchestrator_EventsGoToTheSubscribedExtensions(t *testing.T) {
	data, err := fs.ReadFile(PluginFS(), "skills/nerv-orchestrator/references/task-events.md")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(string(data)), " ")
	for _, want := range []string{"`extension.yaml`", "`events`", "`runtime: agent`", "`agent`"} {
		if !strings.Contains(text, want) {
			t.Errorf("task-events.md never says how events reach the subscribed extensions (%s)", want)
		}
	}
}
