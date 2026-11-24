package nerv

import (
	"io/fs"
	"strings"
	"testing"
)

// S6: the orchestrator hands each pipeline event to the nerv-task tool,
// whose nerv-events.yaml declares the events it takes and its agent
// (task:dispatcher); it names no extension of its own.
func TestOrchestrator_EventsGoToTheNervTaskTool(t *testing.T) {
	data, err := fs.ReadFile(PluginFS(), "skills/nerv-orchestrator/references/task-events.md")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(string(data)), " ")
	for _, want := range []string{"`nerv-events.yaml`", "`events`", "`agent`", "(`task:dispatcher`)"} {
		if !strings.Contains(text, want) {
			t.Errorf("task-events.md never says how events reach the nerv-task tool (%s)", want)
		}
	}
	for _, gone := range []string{"extension.yaml", "docs/extensions.md", "task extension"} {
		if strings.Contains(text, gone) {
			t.Errorf("task-events.md still names the removed extension contract (%s)", gone)
		}
	}
}
