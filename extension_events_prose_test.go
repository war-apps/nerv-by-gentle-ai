package nerv

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
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

// Every shipped extension that subscribes to core events is reachable by
// the orchestrator: an agent extension, subscribing only to core events.
func TestEmbeddedExtensions_SubscribersAreAgentExtensions(t *testing.T) {
	exts, err := plugin.Extensions(ExtensionsFS())
	if err != nil {
		t.Fatal(err)
	}
	subscribers := 0
	for _, e := range exts {
		if e.Provider {
			continue
		}
		m, err := extension.LoadManifest(ExtensionsFS(), e.Name)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.Events) == 0 {
			continue
		}
		subscribers++
		if m.Runtime != extension.RuntimeAgent {
			t.Errorf("%s subscribes to %v with runtime %s: the orchestrator reaches extensions through their agent", e.Name, m.Events, m.Runtime)
		}
	}
	if subscribers == 0 {
		t.Error("no shipped extension subscribes to the pipeline events: task must")
	}
}
