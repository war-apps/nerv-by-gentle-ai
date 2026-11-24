package nerv

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// taskResults are the outcomes the task dispatcher reports per provider,
// in contract order, frozen as the task extension's contract last
// declared them (nerv-task owns the contract now).
var taskResults = []string{"ok", "skipped", "blocked", "failed", "needs_input"}

// Every place that lists the dispatcher's result values names exactly
// taskResults.
func TestTaskEnvelope_ResultListsMatchContract(t *testing.T) {
	want := strings.Join(taskResults, " | ")
	listRe := regexp.MustCompile(`ok \| skipped \| blocked \| failed[^\n]*`)
	for _, src := range []struct {
		fsys fs.FS
		path string
	}{
		{PluginFS(), "skills/_shared/nerv-artifacts.md"},
		{ExtensionsFS(), "task/skills/task/SKILL.md"},
	} {
		data, err := fs.ReadFile(src.fsys, src.path)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range listRe.FindAllString(string(data), -1) {
			if !strings.Contains(m, want) {
				t.Errorf("%s lists results %q, want %q", src.path, m, want)
			}
		}
	}
}

// S18: with the task plugin installed but switched off (tasks.provider:
// none) or with no enabled provider, the dispatcher answers a skipped
// envelope; the orchestrator must then record nothing: no task-events.yaml
// entry and no task_event log line.
func TestOrchestrator_SilentWhenNoProviderHandlesTheEvent(t *testing.T) {
	data, err := fs.ReadFile(PluginFS(), "skills/nerv-orchestrator/references/task-events.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"`provider: none`", "switched off", "`skipped`"} {
		if !strings.Contains(text, want) {
			t.Errorf("task-events.md never says how a %s envelope is handled", want)
		}
	}
	if !regexp.MustCompile(`(?s)provider: none.{0,300}(not recorded|records nothing|no log entry)`).MatchString(text) {
		t.Error("task-events.md does not say a `provider: none` skipped envelope is not recorded (S18)")
	}
}
