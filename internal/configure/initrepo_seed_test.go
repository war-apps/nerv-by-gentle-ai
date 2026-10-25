package configure_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
)

// The user's defaults.tasks seeds the tasks: section of a repository being
// initialized: copied once, without its personal keys, with the
// --repo-provider-config entries winning, and listed among the changes.
func TestInitRepo_SeedsTasksFromTheUserDefaults(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	userPath := paths.Resolve(home).UserConfig
	userText := "tasks:\n  provider: ignored\ndefaults:\n  tasks:\n    provider: teamwork\n    providers:\n      teamwork:\n        assignee_id: 7\n        project_id: 1\n"
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPath, []byte(userText), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := configure.Deps{Home: home, Runner: gitToplevelRunner(repo), Now: time.Now, ExtensionsFS: nerv.ExtensionsFS()}

	result, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: repo, ProviderConfig: []string{"teamwork.project_id=5", "teamwork.tasklist_id=222"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(paths.ProjectConfig(repo))
	if err != nil {
		t.Fatal(err)
	}
	doc := config.Parse(string(data))
	for key, want := range map[string]string{
		"tasks.provider":                       "teamwork",
		"tasks.providers.teamwork.project_id":  "5",
		"tasks.providers.teamwork.tasklist_id": "222",
	} {
		if got, _ := doc.Scalar(key); got != want {
			t.Errorf("%s = %q, want %q:\n%s", key, got, want, data)
		}
	}
	if v, found := doc.Scalar("tasks.providers.teamwork.assignee_id"); found {
		t.Errorf("the personal assignee_id %q was copied into the project:\n%s", v, data)
	}
	want := []configure.Change{
		{Key: "tasks.provider", From: "", To: "teamwork (from defaults.tasks)"},
		{Key: "tasks.providers.teamwork.project_id", From: "", To: "1 (from defaults.tasks)"},
		{Key: "tasks.providers.teamwork.project_id", From: "1", To: "5"},
		{Key: "tasks.providers.teamwork.tasklist_id", From: "", To: "222"},
	}
	if len(result.Changes) != len(want) {
		t.Fatalf("Changes = %+v, want %+v", result.Changes, want)
	}
	for i := range want {
		if result.Changes[i] != want[i] {
			t.Errorf("Changes[%d] = %+v, want %+v", i, result.Changes[i], want[i])
		}
	}
	if got, _ := os.ReadFile(userPath); string(got) != userText {
		t.Errorf("the user config changed:\n%s", got)
	}
}

// Without defaults.tasks, InitRepo writes what it wrote before: no tasks:
// section unless --repo-provider-config asks for one.
func TestInitRepo_NoSeedWithoutDefaults(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	userPath := paths.Resolve(home).UserConfig
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPath, []byte("tasks:\n  provider: teamwork\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := configure.Deps{Home: home, Runner: gitToplevelRunner(repo), Now: time.Now, ExtensionsFS: nerv.ExtensionsFS()}
	result, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: repo, Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(paths.ProjectConfig(repo))
	if want := config.FormatProjectFile(config.ProjectValues{BaseBranch: "main"}); string(data) != want {
		t.Errorf("project file =\n%s\nwant\n%s", data, want)
	}
	if len(result.Changes) != 0 {
		t.Errorf("Changes = %+v, want none", result.Changes)
	}
}
