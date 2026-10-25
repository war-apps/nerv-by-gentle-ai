package configure_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
)

// projectTaskProvider is a task provider a repository ships in
// .ai-workflow/providers/task-tracker/: it implements nothing yet.
const projectTaskProvider = `name: tracker
extension: task
slot: provider
contract_version: 1
runtime: agent
task_ref_prefix: TR
mcp:
  server: tracker
  tools: [tracker_get]
config:
  - key: board
    scope: repo
    type: string
    prompt: The tracker board of this repository
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
`

func writeProjectTaskProvider(t *testing.T, repo string) {
	t.Helper()
	path := filepath.Join(repo, ".ai-workflow", "providers", "task-tracker", "provider.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(projectTaskProvider), 0o644); err != nil {
		t.Fatal(err)
	}
}

// S9: the repository's own task providers are listed after the shipped
// ones, and --repo-provider-config takes their keys.
func TestRepoProviders_ListsTheProjectTaskProviders(t *testing.T) {
	repo := t.TempDir()
	writeProjectTaskProvider(t, repo)
	providers, err := configure.RepoProviders(nerv.ExtensionsFS(), repo)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range providers {
		names = append(names, p.Plugin)
	}
	if len(names) != 2 || names[0] != "task-teamwork" || names[1] != "task-tracker" {
		t.Errorf("providers = %v, want [task-teamwork task-tracker]", names)
	}
}

func TestInitRepo_ProjectTaskProviderConfig(t *testing.T) {
	repo := t.TempDir()
	writeProjectTaskProvider(t, repo)
	deps := configure.Deps{Runner: gitToplevelRunner(repo), Now: time.Now, ExtensionsFS: nerv.ExtensionsFS()}
	if _, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: repo, Base: "main", ProviderConfig: []string{"tracker.board=ops"}}); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(repo, ".ai-workflow", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := config.Parse(string(data)).Scalar("tasks.providers.tracker.board"); got != "ops" {
		t.Errorf("tasks.providers.tracker.board = %q:\n%s", got, data)
	}
}
