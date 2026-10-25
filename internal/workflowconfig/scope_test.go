package workflowconfig_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
)

func personal(t *testing.T) workflowconfig.Personal {
	t.Helper()
	p, err := workflowconfig.LoadPersonal(nerv.ExtensionsFS())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScalar_ExtensionSectionsAreReadFromTheProjectOnly(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	write(t, paths.Resolve(home).UserConfig, "tasks:\n  provider: teamwork\n  timer_store: /tmp/t.json\n  providers:\n    teamwork:\n      assignee_id: 7\n      project_id: 1\nrelease:\n  version: conventional-commits\ndeploy:\n  host: github\n  providers:\n    ssh-compose:\n      ssh_auth: key\ngit:\n  base_branch: develop\n")
	write(t, paths.ProjectConfig(repo), "enabled: true\n")

	cfg, err := workflowconfig.Load(home, repo)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Personal = personal(t)
	for _, key := range []string{"tasks.provider", "tasks.providers.teamwork.project_id", "release.version", "deploy.host"} {
		if v, found := cfg.Scalar(key); found {
			t.Errorf("%s = %q from the user file; want unset (project only)", key, v)
		}
	}
	for key, want := range map[string]string{
		"tasks.timer_store":                     "/tmp/t.json",
		"tasks.providers.teamwork.assignee_id":  "7",
		"deploy.providers.ssh-compose.ssh_auth": "key",
		"git.base_branch":                       "develop",
	} {
		if v, _ := cfg.Scalar(key); v != want {
			t.Errorf("%s = %q, want %q (a personal or core key still read from the user file)", key, v, want)
		}
	}
}

// A repository's own provider declares its user-scope keys like a shipped
// one.
func TestLoadPersonal_IncludesProjectProviders(t *testing.T) {
	repo := t.TempDir()
	write(t, filepath.Join(repo, ".ai-workflow", "providers", "task-tracker", "provider.yaml"), `name: tracker
extension: task
slot: provider
contract_version: 1
runtime: agent
task_ref_prefix: TR
mcp:
  server: tracker
  tools: [tracker_get]
config:
  - key: token_file
    scope: user
    type: string
    prompt: Your token file
  - key: board
    scope: repo
    type: string
    prompt: The board
operations:
  getTask: not_implemented
  assign: not_implemented
  moveStage: not_implemented
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
`)
	tree, err := extension.WithProject(nerv.ExtensionsFS(), repo)
	if err != nil {
		t.Fatal(err)
	}
	p, err := workflowconfig.LoadPersonal(tree)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{
		"tasks.providers.tracker.token_file":    true,
		"tasks.providers.tracker.board":         false,
		"tasks.providers.teamwork.assignee_id":  true,
		"deploy.providers.ssh-compose.ssh_port": false,
		"deploy.providers.ssh-compose.ssh_auth": true,
	} {
		if got := p.Has(path); got != want {
			t.Errorf("Has(%s) = %v, want %v", path, got, want)
		}
	}
}

func TestScalar_TheProjectSetsTheExtensionSections(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	write(t, paths.Resolve(home).UserConfig, "tasks:\n  provider: other\n")
	write(t, paths.ProjectConfig(repo), "tasks:\n  provider: teamwork\n")
	cfg, err := workflowconfig.Load(home, repo)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := cfg.Scalar("tasks.provider"); v != "teamwork" {
		t.Errorf("tasks.provider = %q, want teamwork", v)
	}
}

func TestScalar_DefaultsNeverApplyAtRunTime(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	write(t, paths.Resolve(home).UserConfig, "defaults:\n  tasks:\n    provider: teamwork\n  release:\n    version: conventional-commits\n")
	write(t, paths.ProjectConfig(repo), "enabled: true\n")
	cfg, err := workflowconfig.Load(home, repo)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Personal = personal(t)
	for _, key := range []string{"tasks.provider", "release.version"} {
		if v, found := cfg.Scalar(key); found {
			t.Errorf("%s = %q; a default must never apply at run time", key, v)
		}
	}
}

func TestRunTimeUserKeys_ListsWhatNoLongerApplies(t *testing.T) {
	doc := config.Parse("tasks:\n  provider: teamwork\n  stale_after_hours: 8\n  events:\n    run.closed: off\n  providers:\n    teamwork:\n      assignee_id: 7\n      tasklist_id: 3\n      stages: { inDev: DEV }\nrelease:\n  build: bash\ndeploy:\n  providers:\n    ssh-compose:\n      ssh_auth: key\n      ssh_port: 2222\ndefaults:\n  tasks:\n    provider: x\nmodels:\n  aoba: { model: sonnet }\n")
	got, err := workflowconfig.RunTimeUserKeys(doc, personal(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tasks.provider", "tasks.events", "tasks.providers.teamwork.tasklist_id", "release.build", "deploy.providers.ssh-compose.ssh_port"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RunTimeUserKeys = %v, want %v", got, want)
	}
}

func TestRunTimeUserKeys_NoneForPersonalKeysOnly(t *testing.T) {
	doc := config.Parse("tasks:\n  timer_store: x\n  providers:\n    teamwork:\n      assignee_id: 7\ngit:\n  base_branch: main\n")
	got, err := workflowconfig.RunTimeUserKeys(doc, personal(t))
	if err != nil || len(got) != 0 {
		t.Errorf("RunTimeUserKeys = %v, %v; want none", got, err)
	}
}

func TestSeed_IsTheDefaultsSectionWithoutPersonalKeys(t *testing.T) {
	user := config.Parse("defaults:\n  tasks:\n    provider: teamwork\n    events:\n      run.closed: off\n    providers:\n      teamwork:\n        assignee_id: 7\n        project_id: 1\n  release:\n    build: bash\n")
	block, ok := workflowconfig.Seed(user, "tasks", personal(t))
	if !ok {
		t.Fatal("no tasks seed")
	}
	want := "tasks:\n  provider: teamwork\n  events:\n    run.closed: off\n  providers:\n    teamwork:\n      project_id: 1"
	if strings.TrimRight(block, "\n") != want {
		t.Errorf("seed =\n%s\nwant\n%s", block, want)
	}
	if _, ok := workflowconfig.Seed(user, "deploy", personal(t)); ok {
		t.Error("a deploy seed with no defaults.deploy")
	}
	if _, ok := workflowconfig.Seed(config.Parse("tasks:\n  provider: x\n"), "tasks", personal(t)); ok {
		t.Error("a seed from a run-time tasks: section; only defaults.tasks seeds")
	}
}
