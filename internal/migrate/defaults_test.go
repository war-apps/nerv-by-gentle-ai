package migrate_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
	"gopkg.in/yaml.v3"
)

// userWorld is a home with a today's user config.yaml and, at root, a
// repository with today's project file.
func userWorld(t *testing.T, user, project string) world {
	t.Helper()
	w := world{t: t, root: filepath.Join(t.TempDir(), "repo"), home: t.TempDir()}
	if user != "" {
		w.write(w.userPaths().UserConfig, user)
	}
	if project != "" {
		w.write(".ai-workflow/config.yaml", project)
	}
	return w
}

func yamlMap(t *testing.T, text string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := yaml.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("not valid YAML: %v\n%s", err, text)
	}
	return m
}

func userOnly(w world) migrate.Options {
	return migrate.Options{Home: w.home, User: true, ExtensionsFS: nerv.ExtensionsFS(), Now: now}
}

// nerv migrate --user moves the user file's run-time task, release and
// deploy keys under defaults:, keeps the personal ones where they are,
// backs the file up, and a second run changes nothing.
func TestUserDefaults_RunTimeKeysMoveUnderDefaults(t *testing.T) {
	w := userWorld(t, "# my config\ngit:\n  worktree: ask\ntasks:\n  provider: teamwork\n  timer_store: ~/t.json\n  events:\n    run.closed: off\n  providers:\n    teamwork:\n      assignee_id: 7\n      project_id: 1\nrelease:\n  build: bash\ndeploy:\n  host: github\n  providers:\n    ssh-compose:\n      ssh_auth: key\n", "")
	userPath := w.userPaths().UserConfig

	p, backups := w.migrate(userOnly(w))
	got := yamlMap(t, w.read(userPath))
	want := yamlMap(t, `
git: {worktree: ask}
tasks:
  timer_store: ~/t.json
  providers: {teamwork: {assignee_id: 7}}
deploy:
  providers: {ssh-compose: {ssh_auth: key}}
defaults:
  tasks:
    provider: teamwork
    events: {run.closed: "off"}
    providers: {teamwork: {project_id: 1}}
  release: {build: bash}
  deploy: {host: github}
`)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("user config =\n%s\nwant the map %v", w.read(userPath), want)
	}
	contains(t, "the user config", w.read(userPath), "# my config\n")
	if len(backups) != 1 || filepath.Dir(backups[0]) != filepath.Dir(userPath) {
		t.Errorf("backups = %v, want one next to %s", backups, userPath)
	} else if data, err := os.ReadFile(backups[0]); err != nil || !anyContains([]string{string(data)}, "  provider: teamwork\n") {
		t.Errorf("the backup does not hold the old file: %v", err)
	}
	for _, note := range []string{
		"user config: tasks.provider, tasks.events, tasks.providers.teamwork.project_id moved under defaults.tasks",
		"user config: release.build moved under defaults.release",
		"user config: deploy.host moved under defaults.deploy",
	} {
		if !anyContains(p.Notes, note) {
			t.Errorf("no note %q in %q", note, p.Notes)
		}
	}
	if again := w.plan(userOnly(w)); !again.Empty() {
		t.Errorf("a second run has changes: %s", migrate.Render(again))
	}
}

// A user file with personal keys only, or seeds already, has nothing to
// move.
func TestUserDefaults_PersonalKeysAndSeedsStay(t *testing.T) {
	w := userWorld(t, "tasks:\n  stale_after_hours: 8\n  providers:\n    teamwork:\n      assignee_id: 7\ndefaults:\n  tasks:\n    provider: teamwork\n", "")
	before := treeHash(t, w.home)
	if p := w.plan(userOnly(w)); !p.Empty() {
		t.Errorf("changes: %s", migrate.Render(p))
	}
	if treeHash(t, w.home) != before {
		t.Error("Compute wrote the home")
	}
}

// A section whose defaults: are already set is left for the user to merge,
// reported, and the file is not changed.
func TestUserDefaults_ExistingDefaultsAreNotOverwritten(t *testing.T) {
	text := "tasks:\n  provider: teamwork\ndefaults:\n  tasks:\n    provider: other\n"
	w := userWorld(t, text, "")
	p, _ := w.migrate(userOnly(w))
	if got := w.read(w.userPaths().UserConfig); got != text {
		t.Errorf("user config changed:\n%s", got)
	}
	if !anyContains(p.Warnings, "user config: tasks.provider: kept, defaults.tasks is already set; merge it into defaults.tasks by hand") {
		t.Errorf("no warning in %q", p.Warnings)
	}
}

// nerv migrate in a repository whose project file has no tasks.provider
// copies the user's: its seed, or the older user tasks.provider, so a
// repository that relied on it keeps its provider. A second run is a no-op.
func TestRepoSeed_TasksProviderFromTheUser(t *testing.T) {
	for name, user := range map[string]string{
		"seed":   "defaults:\n  tasks:\n    provider: teamwork\n",
		"legacy": "tasks:\n  provider: teamwork\n",
	} {
		t.Run(name, func(t *testing.T) {
			w := userWorld(t, user, "enabled: true\n")
			o := w.options(false, "")
			p, _ := w.migrate(o)
			got := yamlMap(t, w.read(".ai-workflow/config.yaml"))
			if want := map[string]any{"enabled": true, "tasks": map[string]any{"provider": "teamwork"}}; !reflect.DeepEqual(got, want) {
				t.Errorf("project = %v, want %v", got, want)
			}
			if !anyContains(p.Notes, "tasks.provider: teamwork copied from the user config") {
				t.Errorf("no note in %q", p.Notes)
			}
			if w.read(w.userPaths().UserConfig) != user {
				t.Error("the user config changed without --user")
			}
			if again := w.plan(o); !again.Empty() {
				t.Errorf("a second run has changes: %s", migrate.Render(again))
			}
		})
	}
}

// The project's own tasks.provider wins; nothing is copied.
func TestRepoSeed_ProjectProviderWins(t *testing.T) {
	w := userWorld(t, "defaults:\n  tasks:\n    provider: other\n", "enabled: true\ntasks:\n  provider: teamwork\n")
	before := treeHash(t, w.root, w.home)
	if p := w.plan(w.options(false, "")); !p.Empty() {
		t.Errorf("changes: %s", migrate.Render(p))
	}
	if treeHash(t, w.root, w.home) != before {
		t.Error("Compute wrote a file")
	}
}
