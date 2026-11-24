package nerv

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// taskEvents are the pipeline events the orchestrator emits to
// task:dispatcher, in name order, frozen as the task extension's contract
// last declared them (nerv-task owns the contract now).
var taskEvents = []string{"audit.ranked", "issue.deferred", "run.closed", "run.halted", "run.started", "verification.started", "wave.started"}

// teamworkPattern matches any mention of the Teamwork tracker or its MCP
// tools, in any case.
var teamworkPattern = regexp.MustCompile(`(?i)teamwork|mcp__teamwork`)

// Core nerv carries no tracker logic: no file in the plugin tree mentions
// Teamwork or its MCP tools. Trackers live in the task plugin's providers.
func TestCorePlugin_NeverMentionsTeamwork(t *testing.T) {
	err := fs.WalkDir(PluginFS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(PluginFS(), path)
		if err != nil {
			return err
		}
		if loc := teamworkPattern.FindIndex(data); loc != nil {
			t.Errorf("plugin/%s mentions %q; trackers belong to the task plugin's providers", path, data[loc[0]:loc[1]])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The core tasks skill is gone: the task plugin replaces it.
func TestCorePlugin_HasNoTasksSkill(t *testing.T) {
	if _, err := fs.Stat(PluginFS(), "skills/nerv-tasks"); err == nil {
		t.Error("plugin/skills/nerv-tasks still exists; the task plugin replaces it")
	}
}

// The orchestrator talks to the task plugin only through task:dispatcher and
// names every event of the contract.
func TestOrchestrator_EmitsEveryTaskEvent(t *testing.T) {
	var text strings.Builder
	err := fs.WalkDir(PluginFS(), "skills/nerv-orchestrator", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		data, err := fs.ReadFile(PluginFS(), path)
		text.Write(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	const path = "skills/nerv-orchestrator/**"
	all := text.String()
	assertContainsAll(t, path, all, "task:dispatcher", "task-events.yaml", "/task:replay")
	for _, event := range taskEvents {
		if !strings.Contains(all, "`"+event+"`") {
			t.Errorf("%s never names the event `%s`", path, event)
		}
	}
	if strings.Contains(all, "tracker_event") {
		t.Errorf("%s still logs tracker_event; the entry is task_event", path)
	}
}
