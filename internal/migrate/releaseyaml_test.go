package migrate_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
)

// releaseYAMLWorld is a home and a repository used only to test the S2
// step: moving release: and deploy: (and, for the user scope,
// defaults.release:/defaults.deploy:) into nerv-release's own
// release.yaml files. Every path is under t.TempDir(): nothing reads the
// real AppData or XDG_CONFIG_HOME.
func releaseYAMLWorld(t *testing.T) world {
	t.Helper()
	return world{t: t, root: filepath.Join(t.TempDir(), "repo"), home: t.TempDir()}
}

func (w world) releaseYAMLHome() string { return filepath.Join(w.home, "release.yaml") }
func (w world) releaseYAMLRepo() string {
	return filepath.Join(w.root, ".ai-workflow", "release.yaml")
}

func (w world) releaseYAMLOptions(user bool) migrate.Options {
	o := migrate.Options{RepoRoot: w.root, Home: w.home, User: user, Now: now, ReleaseYAMLRepo: w.releaseYAMLRepo()}
	if user {
		o.ReleaseYAMLHome = w.releaseYAMLHome()
	}
	return o
}

// The five personal ssh-compose keys move from the user config to the
// global release.yaml at the same deploy.providers.ssh-compose.<key>
// path, and defaults.release/defaults.deploy move under the same names
// (prefixes kept, unlike task.yaml). A run-time deploy key of the user
// config is first moved under defaults.deploy (the existing rule), so it
// ends up in release.yaml's defaults.deploy. Comments and keys of the user
// config outside the moved sections are kept; the emptied deploy: and
// defaults: are removed; a backup is made; a second run changes nothing.
func TestUserReleaseYAML_PersonalKeysAndDefaultsMoveWithPrefixes(t *testing.T) {
	w := releaseYAMLWorld(t)
	user := "# my settings\n" +
		"models:\n" +
		"  misato: { model: fable, effort: high } # keep me\n" +
		"deploy:\n" +
		"  providers:\n" +
		"    ssh-compose:\n" +
		"      infisical_domain: https://infisical.acme\n" +
		"      ssh_auth: key\n" +
		"      ssh_key_file: ~/.ssh/id_deploy\n" +
		"      ssh_known_hosts: ~/.ssh/known_hosts\n" +
		"      ssh_connect_timeout: 10s\n" +
		"      ssh_port: 2222\n" +
		"defaults:\n" +
		"  release:\n" +
		"    projects:\n" +
		"      - name: api\n" +
		"        path: api\n" +
		"        build: bash\n" +
		"        submodules: true\n"
	w.write(w.userPaths().UserConfig, user)

	o := w.releaseYAMLOptions(true)
	o.RepoRoot = ""
	p, backups := w.migrate(o)

	got := yamlMap(t, w.read(o.ReleaseYAMLHome))
	want := yamlMap(t, `
deploy:
  providers:
    ssh-compose:
      infisical_domain: https://infisical.acme
      ssh_auth: key
      ssh_key_file: ~/.ssh/id_deploy
      ssh_known_hosts: ~/.ssh/known_hosts
      ssh_connect_timeout: 10s
defaults:
  release:
    projects:
      - {name: api, path: api, build: bash, submodules: true}
  deploy:
    providers:
      ssh-compose:
        ssh_port: 2222
`)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("release.yaml =\n%s\nwant the map %v", w.read(o.ReleaseYAMLHome), want)
	}
	defaults := got["defaults"].(map[string]any)
	port := defaults["deploy"].(map[string]any)["providers"].(map[string]any)["ssh-compose"].(map[string]any)["ssh_port"]
	if v, ok := port.(int); !ok || v != 2222 {
		t.Errorf("ssh_port = %#v (%T), want int 2222", port, port)
	}
	project := defaults["release"].(map[string]any)["projects"].([]any)[0].(map[string]any)
	if v, ok := project["submodules"].(bool); !ok || !v {
		t.Errorf("submodules = %#v (%T), want bool true", project["submodules"], project["submodules"])
	}

	remaining := w.read(w.userPaths().UserConfig)
	if remaining != "# my settings\nmodels:\n  misato: { model: fable, effort: high } # keep me\n" {
		t.Errorf("user config =\n%q\nwant only the comment and models: kept", remaining)
	}
	if len(backups) == 0 {
		t.Error("no backup of the user config")
	}
	if !anyContains(p.Notes, "deploy.providers.ssh-compose.ssh_auth") || !anyContains(p.Notes, o.ReleaseYAMLHome) {
		t.Errorf("no note about the moved keys in %q", p.Notes)
	}
	if again := w.plan(o); !again.Empty() {
		t.Errorf("a second run has changes: %s", migrate.Render(again))
	}
}

// The global release.yaml is written before the user config loses the
// moved sections: Render lists the steps in Apply's order.
func TestUserReleaseYAML_ReleaseYAMLIsWrittenBeforeTheUserConfig(t *testing.T) {
	w := releaseYAMLWorld(t)
	w.write(w.userPaths().UserConfig, "deploy:\n  providers:\n    ssh-compose:\n      ssh_auth: key\n")
	o := w.releaseYAMLOptions(true)
	o.RepoRoot = ""

	rendered := migrate.Render(w.plan(o))
	release := strings.Index(rendered, o.ReleaseYAMLHome)
	user := strings.Index(rendered, w.userPaths().UserConfig)
	if release < 0 || user < 0 || release > user {
		t.Errorf("release.yaml is not written before the user config:\n%s", rendered)
	}
}

// A key already set in the global release.yaml is never overwritten: it
// stays in the user config and is reported; the other keys move.
func TestUserReleaseYAML_ConflictIsReportedAndKept(t *testing.T) {
	w := releaseYAMLWorld(t)
	w.write(w.userPaths().UserConfig, "deploy:\n  providers:\n    ssh-compose:\n      ssh_auth: key\n      ssh_key_file: ~/.ssh/id_deploy\n")
	o := w.releaseYAMLOptions(true)
	o.RepoRoot = ""
	w.write(o.ReleaseYAMLHome, "deploy:\n  providers:\n    ssh-compose:\n      ssh_auth: password\n")

	p, _ := w.migrate(o)

	got := yamlMap(t, w.read(o.ReleaseYAMLHome))
	want := yamlMap(t, "deploy:\n  providers:\n    ssh-compose:\n      ssh_auth: password\n      ssh_key_file: ~/.ssh/id_deploy\n")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("release.yaml =\n%s\nwant %v", w.read(o.ReleaseYAMLHome), want)
	}
	userDoc := yamlMap(t, w.read(w.userPaths().UserConfig))
	wantUser := yamlMap(t, "deploy:\n  providers:\n    ssh-compose:\n      ssh_auth: key\n")
	if !reflect.DeepEqual(userDoc, wantUser) {
		t.Errorf("user config = %v, want %v", userDoc, wantUser)
	}
	if !anyContains(p.Warnings, "deploy.providers.ssh-compose.ssh_auth: kept") {
		t.Errorf("no conflict warning in %q", p.Warnings)
	}
}

// The repository's release: and deploy: move to .ai-workflow/release.yaml
// with their prefixes; the project file keeps its other keys and
// comments, and is backed up; a second run moves nothing.
func TestRepoReleaseYAML_SectionsMoveToRepoReleaseYAML(t *testing.T) {
	w := releaseYAMLWorld(t)
	w.write(".ai-workflow/config.yaml", "# project\nenabled: true # on\n"+
		"release:\n  projects:\n    - name: api\n      path: api\n      build: bash\n"+
		"deploy:\n  host: github\n  targets:\n    - name: staging\n      provider: ssh-compose\n      config:\n        ssh_port: 2222\n"+
		"odd:\n  tdd: strict\n")

	o := w.releaseYAMLOptions(false)
	p, backups := w.migrate(o)

	got := yamlMap(t, w.read(o.ReleaseYAMLRepo))
	want := yamlMap(t, `
release:
  projects:
    - {name: api, path: api, build: bash}
deploy:
  host: github
  targets:
    - {name: staging, provider: ssh-compose, config: {ssh_port: 2222}}
`)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("repo release.yaml =\n%s\nwant %v", w.read(o.ReleaseYAMLRepo), want)
	}
	if project := w.read(".ai-workflow/config.yaml"); project != "# project\nenabled: true # on\nodd:\n  tdd: strict\n" {
		t.Errorf("project file =\n%q", project)
	}
	if !anyContains(backups, "config.yaml") {
		t.Errorf("no backup of the project file in %q", backups)
	}
	if !anyContains(p.Notes, "release.projects") || !anyContains(p.Notes, "deploy.host") {
		t.Errorf("no note about the moved keys in %q", p.Notes)
	}
	if again := w.plan(o); !again.Empty() {
		t.Errorf("a second run has changes: %s", migrate.Render(again))
	}
}

// A personal ssh-compose key in the project file never goes into the
// repository's release.yaml (nor, with --user, into the global one): it
// stays in the project file and is reported, and the rest still moves.
func TestRepoReleaseYAML_PersonalKeysAreReportedNotMoved(t *testing.T) {
	for _, user := range []bool{false, true} {
		t.Run(map[bool]string{false: "repository only", true: "with --user"}[user], func(t *testing.T) {
			w := releaseYAMLWorld(t)
			w.write(".ai-workflow/config.yaml", "deploy:\n  host: github\n  providers:\n    ssh-compose:\n      ssh_auth: key\n")

			o := w.releaseYAMLOptions(user)
			p, _ := w.migrate(o)

			got := yamlMap(t, w.read(o.ReleaseYAMLRepo))
			if want := yamlMap(t, "deploy:\n  host: github\n"); !reflect.DeepEqual(got, want) {
				t.Errorf("repo release.yaml = %v, want %v", got, want)
			}
			project := yamlMap(t, w.read(".ai-workflow/config.yaml"))
			if want := yamlMap(t, "deploy:\n  providers:\n    ssh-compose:\n      ssh_auth: key\n"); !reflect.DeepEqual(project, want) {
				t.Errorf("project file = %v, want %v", project, want)
			}
			if !anyContains(p.Warnings, "deploy.providers.ssh-compose.ssh_auth: a personal setting") {
				t.Errorf("no personal-key warning in %q", p.Warnings)
			}
			if user && w.exists(o.ReleaseYAMLHome) {
				t.Errorf("the global release.yaml was written from the repository:\n%s", w.read(o.ReleaseYAMLHome))
			}
		})
	}
}

// A key already set in the repository's release.yaml is never
// overwritten: it stays in the project file and is reported.
func TestRepoReleaseYAML_ConflictIsReportedAndKept(t *testing.T) {
	w := releaseYAMLWorld(t)
	w.write(".ai-workflow/config.yaml", "deploy:\n  host: github\n  git:\n    remote: origin\n")
	o := w.releaseYAMLOptions(false)
	w.write(o.ReleaseYAMLRepo, "deploy:\n  host: gitlab\n")

	p, _ := w.migrate(o)

	if got, want := yamlMap(t, w.read(o.ReleaseYAMLRepo)), yamlMap(t, "deploy:\n  host: gitlab\n  git: {remote: origin}\n"); !reflect.DeepEqual(got, want) {
		t.Errorf("repo release.yaml = %v, want %v", got, want)
	}
	if got, want := yamlMap(t, w.read(".ai-workflow/config.yaml")), yamlMap(t, "deploy:\n  host: github\n"); !reflect.DeepEqual(got, want) {
		t.Errorf("project file = %v, want %v", got, want)
	}
	if !anyContains(p.Warnings, "deploy.host: kept") {
		t.Errorf("no conflict warning in %q", p.Warnings)
	}
}

// --dry-run (Compute without Apply) writes nothing.
func TestReleaseYAML_DryRunWritesNothing(t *testing.T) {
	w := releaseYAMLWorld(t)
	w.write(w.userPaths().UserConfig, "deploy:\n  providers:\n    ssh-compose:\n      ssh_auth: key\n")
	w.write(".ai-workflow/config.yaml", "deploy:\n  host: github\n")
	o := w.releaseYAMLOptions(true)
	before := treeHash(t, w.home, w.root)

	if p := w.plan(o); p.Empty() {
		t.Fatal("expected changes to plan")
	}
	if treeHash(t, w.home, w.root) != before {
		t.Error("Compute wrote a file")
	}
}

// Without nerv-release installed (ReleaseYAMLHome/ReleaseYAMLRepo left
// ""), nothing about release:/deploy: changes.
// Without nerv-release (no release.yaml path) a personal release key of
// the project file is still reported, and stays where it is.
func TestRepoReleaseYAML_PersonalKeysAreReportedWithoutNervRelease(t *testing.T) {
	w := releaseYAMLWorld(t)
	text := "deploy:\n  host: github\n  providers:\n    ssh-compose:\n      ssh_auth: key\n"
	w.write(".ai-workflow/config.yaml", text)
	p, _ := w.migrate(migrate.Options{RepoRoot: w.root, Home: w.home, Now: now})
	if got := w.read(".ai-workflow/config.yaml"); got != text {
		t.Errorf("project file changed:\n%s", got)
	}
	if !anyContains(p.Warnings, "deploy.providers.ssh-compose.ssh_auth: a personal setting") {
		t.Errorf("no personal-key warning in %q", p.Warnings)
	}
}

func TestReleaseYAML_SkippedWhenPathsAreEmpty(t *testing.T) {
	w := releaseYAMLWorld(t)
	w.write(w.userPaths().UserConfig, "deploy:\n  providers:\n    ssh-compose:\n      ssh_auth: key\ndefaults:\n  deploy:\n    host: github\n")
	w.write(".ai-workflow/config.yaml", "deploy:\n  host: github\n")
	o := migrate.Options{RepoRoot: w.root, Home: w.home, User: true, Now: now}
	p := w.plan(o)
	for _, c := range p.Changes {
		if c.Path != filepath.Join(w.root, ".nerv", ".gitignore") {
			t.Errorf("a change without the release.yaml paths: %s", migrate.Render(p))
		}
	}
}

// The task and release moves run together in one migration, each to its
// own file, and the user config ends up with neither section.
func TestReleaseYAML_TaskAndReleaseMoveTogether(t *testing.T) {
	w := releaseYAMLWorld(t)
	w.write(w.userPaths().UserConfig, "tasks:\n  timer_store: ~/t.json\ndeploy:\n  providers:\n    ssh-compose:\n      ssh_auth: key\n")
	w.write(".ai-workflow/config.yaml", "tasks:\n  provider: teamwork\ndeploy:\n  host: github\n")
	o := w.releaseYAMLOptions(true)
	o.TaskYAMLHome = w.taskYAMLHome()
	o.TaskYAMLRepo = w.taskYAMLRepo()

	w.migrate(o)

	checks := []struct{ path, want string }{
		{o.TaskYAMLHome, "timer_store: ~/t.json\n"},
		{o.TaskYAMLRepo, "provider: teamwork\n"},
		{o.ReleaseYAMLHome, "deploy:\n  providers:\n    ssh-compose:\n      ssh_auth: key\n"},
		{o.ReleaseYAMLRepo, "deploy:\n  host: github\n"},
	}
	for _, c := range checks {
		if got, want := yamlMap(t, w.read(c.path)), yamlMap(t, c.want); !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %v, want %v", c.path, got, want)
		}
	}
	if got := strings.TrimSpace(w.read(w.userPaths().UserConfig)); got != "" {
		t.Errorf("user config still has settings:\n%s", got)
	}
	if got := strings.TrimSpace(w.read(".ai-workflow/config.yaml")); got != "" {
		t.Errorf("project file still has settings:\n%s", got)
	}
}

// An anchored value and its alias (which nerv's release: loader accepts)
// move resolved, never as the alias's name.
func TestRepoReleaseYAML_AliasesMoveResolved(t *testing.T) {
	w := releaseYAMLWorld(t)
	w.write(".ai-workflow/config.yaml", "release:\n  projects:\n    - &base\n      name: api\n      path: api\n    - <<: *base\n      name: worker\n      path: worker\n")
	o := w.releaseYAMLOptions(false)
	w.migrate(o)

	got := yamlMap(t, w.read(o.ReleaseYAMLRepo))
	want := yamlMap(t, "release:\n  projects:\n    - {name: api, path: api}\n    - {name: worker, path: worker}\n")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("repo release.yaml =\n%s\nwant %v", w.read(o.ReleaseYAMLRepo), want)
	}
}

// quotedLookalikes are strings that read as another type when unquoted.
var quotedLookalikes = []string{"010", "1.0", "true", "null", "~", "8"}

// assertStringsKept checks that section k and every item of section l are
// still the strings v, not the number, bool or null they look like.
func assertStringsKept(t *testing.T, section map[string]any, v string) {
	t.Helper()
	if got, ok := section["k"].(string); !ok || got != v {
		t.Errorf("k = %#v, want the string %q", section["k"], v)
	}
	list, _ := section["l"].([]any)
	if len(list) != 1 {
		t.Fatalf("l = %#v, want one item", section["l"])
	}
	if got, ok := list[0].(string); !ok || got != v {
		t.Errorf("l[0] = %#v, want the string %q", list[0], v)
	}
}

// A quoted string that looks like a number, bool or null stays a string
// in release.yaml, as a value and inside a list.
func TestUserReleaseYAML_QuotedLookalikesStayStrings(t *testing.T) {
	for _, v := range quotedLookalikes {
		t.Run(v, func(t *testing.T) {
			w := releaseYAMLWorld(t)
			w.write(w.userPaths().UserConfig, "defaults:\n  release:\n    k: \""+v+"\"\n    l: [\""+v+"\"]\n")
			o := w.releaseYAMLOptions(true)
			o.RepoRoot = ""
			w.migrate(o)
			got := yamlMap(t, w.read(o.ReleaseYAMLHome))
			defaults, _ := got["defaults"].(map[string]any)
			release, _ := defaults["release"].(map[string]any)
			assertStringsKept(t, release, v)
		})
	}
}
