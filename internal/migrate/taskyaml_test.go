package migrate_test

import (
	"path/filepath"
	"reflect"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
)

// taskYAMLWorld is a home (and, with repo=true, a repository at its root)
// used only to test the S12 step: moving tasks: (and, for the user scope,
// defaults.tasks:) into nerv-task's own task.yaml files.
func taskYAMLWorld(t *testing.T) world {
	t.Helper()
	return world{t: t, root: filepath.Join(t.TempDir(), "repo"), home: t.TempDir()}
}

func (w world) taskYAMLHome() string { return filepath.Join(w.home, "task.yaml") }
func (w world) taskYAMLRepo() string { return filepath.Join(w.root, ".ai-workflow", "task.yaml") }

func (w world) taskYAMLOptions(user bool) migrate.Options {
	o := migrate.Options{RepoRoot: w.root, Home: w.home, User: user, ExtensionsFS: nerv.ExtensionsFS(), Now: now, TaskYAMLRepo: w.taskYAMLRepo()}
	if user {
		o.TaskYAMLHome = w.taskYAMLHome()
	}
	return o
}

// The user's real config shape (S12, the required scenario): timer_store,
// the teamwork provider's int scope:user keys, and its nested maps
// (known_projects: a map of maps, stages: a map) all move intact to the
// global task.yaml, without the "tasks." prefix; config.yaml's tasks:
// section, now empty, is removed. A second run moves nothing.
func TestUserTaskYAML_RealConfigShapeMigratesIntact(t *testing.T) {
	w := taskYAMLWorld(t)
	user := "tasks:\n" +
		"  timer_store: ~/.claude/work/timers.json\n" +
		"  providers:\n" +
		"    teamwork:\n" +
		"      assignee_id: 7\n" +
		"      default_project_id: 100\n" +
		"      default_tasklist_id: 200\n" +
		"      known_projects:\n" +
		"        proj1:\n" +
		"          name: Project One\n" +
		"          note: first\n" +
		"        proj2:\n" +
		"          name: Project Two\n" +
		"          note: second\n" +
		"      stages:\n" +
		"        inDev: DESARROLLO\n" +
		"        testing: TESTING\n" +
		"        implemented: IMPLEMENTA\n" +
		"        blocked: BLOQUEA\n" +
		"        canceled: CANCEL\n" +
		"        pending: PENDIENTE\n" +
		"        analysis: ANALISIS\n"
	w.write(w.userPaths().UserConfig, user)

	o := w.taskYAMLOptions(true)
	p, _ := w.migrate(o)

	taskYAML := yamlMap(t, w.read(o.TaskYAMLHome))
	want := yamlMap(t, `
timer_store: ~/.claude/work/timers.json
providers:
  teamwork:
    assignee_id: 7
    default_project_id: 100
    default_tasklist_id: 200
    known_projects:
      proj1: {name: Project One, note: first}
      proj2: {name: Project Two, note: second}
    stages: {inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS}
`)
	if !reflect.DeepEqual(taskYAML, want) {
		t.Errorf("task.yaml =\n%s\nwant the map %v", w.read(o.TaskYAMLHome), want)
	}
	// Types survive: ints stay ints, not strings.
	providers := taskYAML["providers"].(map[string]any)
	teamwork := providers["teamwork"].(map[string]any)
	if v, ok := teamwork["assignee_id"].(int); !ok || v != 7 {
		t.Errorf("assignee_id = %#v (%T), want int 7", teamwork["assignee_id"], teamwork["assignee_id"])
	}

	if got := w.read(w.userPaths().UserConfig); yamlMap(t, got) != nil {
		m := yamlMap(t, got)
		if _, has := m["tasks"]; has {
			t.Errorf("the user config still has tasks: %s", got)
		}
	}
	if !anyContains(p.Notes, "user config: tasks.timer_store") {
		t.Errorf("no note about the moved keys in %q", p.Notes)
	}

	if again := w.plan(o); !again.Empty() {
		t.Errorf("a second run has changes: %s", migrate.Render(again))
	}
}

// S12: defaults.tasks (the seed a new repository's tasks: copies from)
// moves under the global task.yaml's defaults: section, keys without the
// "tasks." prefix.
func TestUserTaskYAML_DefaultsTasksMovesUnderDefaults(t *testing.T) {
	w := taskYAMLWorld(t)
	w.write(w.userPaths().UserConfig, "defaults:\n  tasks:\n    provider: teamwork\n    providers:\n      teamwork:\n        project_id: 1\n")

	o := w.taskYAMLOptions(true)
	w.migrate(o)

	got := yamlMap(t, w.read(o.TaskYAMLHome))
	want := yamlMap(t, "defaults:\n  provider: teamwork\n  providers:\n    teamwork:\n      project_id: 1\n")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("task.yaml =\n%s\nwant %v", w.read(o.TaskYAMLHome), want)
	}
	if m := yamlMap(t, w.read(w.userPaths().UserConfig)); m != nil {
		if d, has := m["defaults"].(map[string]any); has {
			if _, has := d["tasks"]; has {
				t.Error("defaults.tasks is still in the user config")
			}
		}
	}
}

// S12: a key already set in task.yaml is never overwritten; it is
// reported and kept in the user config.
func TestUserTaskYAML_ConflictIsReportedAndKept(t *testing.T) {
	w := taskYAMLWorld(t)
	w.write(w.userPaths().UserConfig, "tasks:\n  timer_store: ~/mine.json\n")
	o := w.taskYAMLOptions(true)
	w.write(o.TaskYAMLHome, "timer_store: ~/already-there.json\n")

	p, _ := w.migrate(o)

	if got := w.read(o.TaskYAMLHome); got != "timer_store: ~/already-there.json\n" {
		t.Errorf("task.yaml changed:\n%s", got)
	}
	if got := w.read(w.userPaths().UserConfig); yamlMap(t, got)["tasks"] == nil {
		t.Errorf("the conflicting key was removed from the user config:\n%s", got)
	}
	if !anyContains(p.Warnings, "tasks.timer_store") {
		t.Errorf("no conflict warning in %q", p.Warnings)
	}
}

// S12 (repository): .ai-workflow/config.yaml's tasks: section moves to the
// repository's own .ai-workflow/task.yaml, keys without the "tasks."
// prefix; a second run moves nothing.
func TestRepoTaskYAML_RepoKeysMoveToRepoTaskYAML(t *testing.T) {
	w := taskYAMLWorld(t)
	w.write(".ai-workflow/config.yaml", "enabled: true\ntasks:\n  provider: teamwork\n  providers:\n    teamwork:\n      project_id: 1\n      tasklist_id: 2\n")

	o := w.taskYAMLOptions(false)
	w.migrate(o)

	got := yamlMap(t, w.read(o.TaskYAMLRepo))
	want := yamlMap(t, "provider: teamwork\nproviders:\n  teamwork: {project_id: 1, tasklist_id: 2}\n")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("repo task.yaml =\n%s\nwant %v", w.read(o.TaskYAMLRepo), want)
	}
	project := yamlMap(t, w.read(".ai-workflow/config.yaml"))
	if _, has := project["tasks"]; has {
		t.Errorf("tasks: is still in the project file: %v", project)
	}
	if again := w.plan(o); !again.Empty() {
		t.Errorf("a second run has changes: %s", migrate.Render(again))
	}
}

// S12 never moves a repository-only value into the global file: without
// --user a personal provider key left in the project file (already
// warned about by the existing personal-keys check) is not swept into the
// repository's task.yaml either.
func TestRepoTaskYAML_NeverMovesPersonalKeysWithoutUser(t *testing.T) {
	w := taskYAMLWorld(t)
	w.write(".ai-workflow/config.yaml", "tasks:\n  provider: teamwork\n  providers:\n    teamwork:\n      project_id: 1\n      assignee_id: 42\n")

	o := w.taskYAMLOptions(false)
	w.migrate(o)

	got := yamlMap(t, w.read(o.TaskYAMLRepo))
	if _, has := got["assignee_id"]; has {
		t.Errorf("a personal key leaked into the repository task.yaml: %v", got)
	}
	project := yamlMap(t, w.read(".ai-workflow/config.yaml"))
	tasks := project["tasks"].(map[string]any)
	if _, has := tasks["providers"].(map[string]any)["teamwork"].(map[string]any)["assignee_id"]; !has {
		t.Error("the personal key was removed from the project file without --user")
	}
}

// --dry-run (Compute without Apply) never writes anything.
func TestTaskYAML_DryRunWritesNothing(t *testing.T) {
	w := taskYAMLWorld(t)
	w.write(w.userPaths().UserConfig, "tasks:\n  timer_store: ~/t.json\n")
	o := w.taskYAMLOptions(true)
	before := treeHash(t, w.home)

	p := w.plan(o)
	if p.Empty() {
		t.Fatal("expected changes to plan")
	}
	if treeHash(t, w.home) != before {
		t.Error("Compute wrote the home")
	}
	if w.exists(o.TaskYAMLHome) {
		t.Error("Compute created task.yaml")
	}
}

// Without nerv-task installed (TaskYAMLHome/TaskYAMLRepo left ""), nothing
// about tasks: changes: the step is entirely opt-in per caller.
func TestTaskYAML_SkippedWhenPathsAreEmpty(t *testing.T) {
	w := taskYAMLWorld(t)
	w.write(w.userPaths().UserConfig, "tasks:\n  timer_store: ~/t.json\n")
	o := migrate.Options{Home: w.home, User: true, ExtensionsFS: nerv.ExtensionsFS(), Now: now}
	if p := w.plan(o); !p.Empty() {
		t.Errorf("changes without TaskYAMLHome: %s", migrate.Render(p))
	}
}
