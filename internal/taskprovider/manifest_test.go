package taskprovider_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/taskprovider"
)

// valid is the smallest manifest the contract accepts: every operation the
// subscribed events need is implemented, the rest declared not_implemented.
const valid = `name: acme
extension: task
slot: provider
contract_version: 1
runtime: agent
task_ref_prefix: AC
mcp:
  server: acme
  tools: [acme_get_task, acme_move_task]
config:
  - key: assignee_id
    scope: user
    type: string
    prompt: Acme user id assigned on start
  - key: project_id
    scope: repo
    type: int
    default: "0"
    prompt: Acme project id
stages:
  inDev: DEV
  testing: TEST
operations:
  getTask: procedures/get-task.md
  assign: procedures/assign.md
  moveStage: procedures/move-stage.md
  logTime: not_implemented
  updateTimelog: not_implemented
  deleteTimelog: not_implemented
  listTimelogs: not_implemented
  createTask: not_implemented
  createSubtask: not_implemented
  comment: not_implemented
  complete: not_implemented
  setPriority: not_implemented
  listTasks: not_implemented
events: [run.started, verification.started]
`

// taskExtension is the task extension's manifest nerv ships.
func taskExtension(t *testing.T) *extension.Manifest {
	t.Helper()
	m, err := extension.LoadManifest(nerv.ExtensionsFS(), taskprovider.Extension)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func mustParse(t *testing.T, text string) *taskprovider.Manifest {
	t.Helper()
	m, err := taskprovider.Parse(taskExtension(t), []byte(text))
	if err != nil {
		t.Fatalf("Parse: unexpected error: %v", err)
	}
	return m
}

// wantProblem asserts Parse rejects text with an error naming want.
func wantProblem(t *testing.T, text, want string) {
	t.Helper()
	_, err := taskprovider.Parse(taskExtension(t), []byte(text))
	if err == nil {
		t.Fatalf("Parse: expected an error containing %q, got none", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Parse: error %q does not contain %q", err, want)
	}
}

func TestParse_Valid(t *testing.T) {
	m := mustParse(t, valid)
	if m.Name != "acme" || m.TaskRefPrefix != "AC" || m.MCP.Server != "acme" {
		t.Fatalf("unexpected identity: %+v", m)
	}
	if got := len(m.Config); got != 2 || m.Config[1].Default != "0" || m.Config[1].Scope != "repo" {
		t.Fatalf("unexpected config: %+v", m.Config)
	}
	if m.Stages["inDev"] != "DEV" {
		t.Fatalf("unexpected stages: %+v", m.Stages)
	}
	if !m.Implements("moveStage") || m.Implements("comment") {
		t.Fatalf("Implements: moveStage=%v comment=%v", m.Implements("moveStage"), m.Implements("comment"))
	}
	if !m.Subscribes("run.started") || m.Subscribes("run.closed") {
		t.Fatalf("Subscribes: wrong answer for %v", m.Events)
	}
}

func TestParse_UnknownField(t *testing.T) {
	wantProblem(t, valid+"extra: true\n", "extra")
}

func TestParse_Identity(t *testing.T) {
	wantProblem(t, strings.Replace(valid, "name: acme", "name: Acme Corp", 1), "name")
	wantProblem(t, strings.Replace(valid, "contract_version: 1", "contract_version: 2", 1), "contract_version 2 is not supported")
	wantProblem(t, strings.Replace(valid, "task_ref_prefix: AC", "task_ref_prefix: ac", 1), "task_ref_prefix")
}

func TestParse_MCPToolsRequired(t *testing.T) {
	// S8: the provider names the exact tools it needs, so the dispatcher
	// can check them before running anything.
	wantProblem(t, strings.Replace(valid, "tools: [acme_get_task, acme_move_task]", "tools: []", 1), "mcp.tools")
	wantProblem(t, strings.Replace(valid, "server: acme", "server: \"\"", 1), "mcp.server")
	wantProblem(t, strings.Replace(valid, "acme_move_task]", "acme_get_task]", 1), "acme_get_task: duplicated")
}

func TestParse_Config(t *testing.T) {
	wantProblem(t, strings.Replace(valid, "scope: user", "scope: global", 1), "scope")
	wantProblem(t, strings.Replace(valid, "type: string", "type: float", 1), "type")
	wantProblem(t, strings.Replace(valid, "key: project_id", "key: assignee_id", 1), "assignee_id: duplicated")
	wantProblem(t, strings.Replace(valid, "    prompt: Acme project id\n", "", 1), "prompt")
	wantProblem(t, strings.Replace(valid, `default: "0"`, `default: "zero"`, 1), "default")
	// A map key is edited in the configuration files, so it needs no prompt.
	mapKey := strings.Replace(valid, "config:\n", "config:\n  - key: comment_labels\n    scope: user\n    type: map\n", 1)
	if m := mustParse(t, mapKey); m.Config[0].Scalar() {
		t.Errorf("a map key reported as scalar: %+v", m.Config[0])
	}
}

func TestParse_Stages(t *testing.T) {
	// S14: events speak canonical stage keys; anything else is a typo.
	wantProblem(t, strings.Replace(valid, "inDev: DEV", "doing: DEV", 1), "stages.doing")
}

func TestParse_Operations(t *testing.T) {
	wantProblem(t, strings.Replace(valid, "  listTasks: not_implemented\n", "", 1), "operations.listTasks: missing")
	wantProblem(t, strings.Replace(valid, "listTasks: not_implemented", "listTasks: not_implemented\n  frobnicate: x.md", 1), "operations.frobnicate")
	wantProblem(t, strings.Replace(valid, "procedures/assign.md", "../assign.md", 1), "operations.assign")
	wantProblem(t, strings.Replace(valid, "procedures/assign.md", "/abs/assign.md", 1), "operations.assign")
	wantProblem(t, strings.Replace(valid, "procedures/assign.md", "procedures/assign.txt", 1), "operations.assign")
}

// S6: a task provider is an agent provider of the task extension's
// provider slot, carrying the task fields that extension declares.
func TestParse_CommonContract(t *testing.T) {
	wantProblem(t, strings.Replace(valid, "slot: provider", "slot: tracker", 1), `slot "tracker" is not a slot of extension task`)
	wantProblem(t, strings.Replace(valid, "extension: task", "extension: deploy", 1), `extension "deploy" is not task`)
	wantProblem(t, strings.Replace(valid, "task_ref_prefix: AC\n", "", 1), "task_ref_prefix is required by extension task")
	wantProblem(t, strings.Replace(valid, "stages:\n  inDev: DEV\n  testing: TEST\n", "stages: [DEV]\n", 1), "stages must be a map")
}

// The provider slot of plugins/task/extension.yaml lists exactly the
// operations this package knows, all optional.
func TestTaskExtension_SlotMatchesTheOperations(t *testing.T) {
	slot, ok := taskExtension(t).Slot(taskprovider.Slot)
	if !ok {
		t.Fatal("no provider slot")
	}
	got := slot.OperationNames()
	want := slices.Sorted(slices.Values(taskprovider.Operations))
	if !slices.Equal(got, want) {
		t.Errorf("slot operations %v, want %v", got, want)
	}
	for op, level := range slot.Operations {
		if level != extension.Optional {
			t.Errorf("%s is %s: every task operation is optional, events decide what a provider needs", op, level)
		}
	}
	for event := range taskprovider.Events {
		if !taskExtension(t).Subscribes(event) {
			t.Errorf("the task extension does not subscribe to %s", event)
		}
	}
}

func TestParse_EventsNeedTheirOperations(t *testing.T) {
	// run.halted needs comment and moveStage; comment is not_implemented.
	wantProblem(t, strings.Replace(valid, "verification.started]", "run.halted]", 1), "run.halted needs comment")
	wantProblem(t, strings.Replace(valid, "verification.started]", "run.exploded]", 1), "events: run.exploded")
}

func TestCheckFiles(t *testing.T) {
	m := mustParse(t, valid)
	fsys := fstest.MapFS{
		"procedures/get-task.md":   {Data: []byte("x")},
		"procedures/assign.md":     {Data: []byte("x")},
		"procedures/move-stage.md": {Data: []byte("x")},
	}
	if err := m.CheckFiles(fsys); err != nil {
		t.Fatalf("CheckFiles: unexpected error: %v", err)
	}
	delete(fsys, "procedures/assign.md")
	err := m.CheckFiles(fsys)
	if err == nil || !strings.Contains(err.Error(), "procedures/assign.md") {
		t.Fatalf("CheckFiles: expected the missing file named, got %v", err)
	}
}

func TestMissingTools(t *testing.T) {
	m := mustParse(t, valid)
	got := m.MissingTools([]string{"mcp__acme__acme_get_task"})
	if len(got) != 1 || got[0] != "acme_move_task" {
		t.Fatalf("MissingTools: got %v", got)
	}
	if got := m.MissingTools([]string{"acme_get_task", "mcp__acme__acme_move_task"}); len(got) != 0 {
		t.Fatalf("MissingTools: got %v, want none", got)
	}
	// A same-named tool from another server does not count.
	if got := m.MissingTools([]string{"mcp__other__acme_get_task", "acme_move_task"}); len(got) != 1 {
		t.Fatalf("MissingTools: another server's tool was accepted: %v", got)
	}
	// The server the provider's own plugin ships (.mcp.json) counts, with
	// the plugin name as Claude Code spells it; another plugin's does not.
	for _, prefix := range []string{"mcp__plugin_task-acme_acme__", "mcp__plugin_task_acme_acme__"} {
		if got := m.MissingTools([]string{prefix + "acme_get_task", prefix + "acme_move_task"}); len(got) != 0 {
			t.Errorf("MissingTools with %s: got %v, want none", prefix, got)
		}
	}
	if got := m.MissingTools([]string{"mcp__plugin_task-other_acme__acme_get_task", "acme_move_task"}); len(got) != 1 {
		t.Errorf("MissingTools: another plugin's server was accepted: %v", got)
	}
}
