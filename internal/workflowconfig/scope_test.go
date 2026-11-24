package workflowconfig_test

import (
	"reflect"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
)

// personal is the frozen personal keys (PersonalTaskKeys and
// PersonalProviderKeys).
func personal(t *testing.T) workflowconfig.Personal {
	t.Helper()
	return workflowconfig.Personal{}
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

// R-S4 security fix: a project file must not be able to override a
// personal (scope: user) key by also setting it — the value must still
// come from the user file only, and a personal key the project sets but
// the user does not must stay unset (never read from the project).
func TestScalar_ProjectCannotOverridePersonalKeys(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	write(t, paths.Resolve(home).UserConfig, "deploy:\n  providers:\n    ssh-compose:\n      ssh_known_hosts: /home/user/.ssh/known_hosts\n")
	write(t, paths.ProjectConfig(repo), "deploy:\n  host: github\n  providers:\n    ssh-compose:\n      ssh_known_hosts: /tmp/attacker-known-hosts\n      infisical_domain: https://attacker.example\n")

	cfg, err := workflowconfig.Load(home, repo)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Personal = personal(t)

	if v, found := cfg.Scalar("deploy.providers.ssh-compose.ssh_known_hosts"); !found || v != "/home/user/.ssh/known_hosts" {
		t.Errorf("ssh_known_hosts = %q, %v; want the user's value, never the project's", v, found)
	}
	if v, found := cfg.Scalar("deploy.providers.ssh-compose.infisical_domain"); found {
		t.Errorf("infisical_domain = %q from the project; want unset: a personal key the user does not set is never read from the project", v)
	}
	// A non-personal key keeps "project wins".
	if v, found := cfg.Scalar("deploy.host"); !found || v != "github" {
		t.Errorf("deploy.host = %q, %v; want github, true (project wins; not personal)", v, found)
	}
}

// A repository's own provider declares its user-scope keys like a shipped
// one.
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

func TestPersonal_ZeroValueHoldsTheFrozenProviderKeys(t *testing.T) {
	doc := config.Parse("tasks:\n  provider: teamwork\n  stale_after_hours: 8\n  providers:\n    teamwork:\n      assignee_id: 7\n      tasklist_id: 3\n      stages: { inDev: DEV }\ndeploy:\n  providers:\n    ssh-compose:\n      ssh_auth: key\n      ssh_port: 2222\n")
	got, err := workflowconfig.RunTimeUserKeys(doc, workflowconfig.Personal{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tasks.provider", "tasks.providers.teamwork.tasklist_id", "deploy.providers.ssh-compose.ssh_port"}
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
