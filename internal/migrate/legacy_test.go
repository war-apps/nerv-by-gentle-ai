package migrate_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
)

// refused checks that o is refused naming every want, and that nothing
// was planned to be written.
func refused(t *testing.T, o migrate.Options, wants ...string) {
	t.Helper()
	p, err := migrate.Compute(o)
	if err == nil {
		t.Fatalf("Compute planned %d change(s), want a refusal", len(p.Changes))
	}
	if !refusal.Is(err) {
		t.Errorf("Compute: %v is not a refusal", err)
	}
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Compute: %q does not name %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "nerv 6.6") {
		t.Errorf("Compute: %q still points to nerv 6.6", err)
	}
}

// legacyCases are the release and deploy sections of an older nerv, each
// in the file it is read from (rel), with section the top-level key
// nerv-release migrate converts ("" for a project provider folder).
var legacyCases = []struct {
	name, rel, content, section string
	wants                       []string
}{
	{"4.2.0 pipeline", ".nerv/nerv.yaml", "enabled: true\npipeline:\n  images:\n    - name: ghcr.io/acme/app\n", "pipeline",
		[]string{".nerv/nerv.yaml: pipeline:", "nerv 4.2.0", "by hand"}},
	{"4.2.0 release", ".nerv/nerv.yaml", "enabled: true\nrelease:\n  semantic_release:\n    version: 24.2.3\n", "release",
		[]string{".nerv/nerv.yaml: release:", "nerv 4.2.0", "by hand"}},
	{"4.2.0 release in the folder", paths.ProjectConfigFile, "enabled: true\nrelease:\n  semantic_release: true\n  projects:\n    - { name: app, path: . }\n", "release",
		[]string{paths.ProjectConfigFile + ": release:", "nerv 4.2.0", "release.semantic_release", "by hand"}},
	{"4.2.0 release presets in the previous file", paths.PreviousProjectConfigFile, "enabled: true\nrelease:\n  projects:\n    - { name: app, path: ., presets: [node] }\n", "release",
		[]string{paths.PreviousProjectConfigFile + ": release:", "nerv 4.2.0", "release.projects[0].presets"}},
	{"5.0.0-alpha.1 ship in the folder", paths.ProjectConfigFile, "enabled: true\nship:\n  projects: []\n", "ship",
		[]string{paths.ProjectConfigFile + ": ship:", "nerv 5.0.0-alpha.1", "by hand"}},
	{"5.0.0-alpha.1 ship in the previous file", paths.PreviousProjectConfigFile, "enabled: true\nship:\n  projects: []\n", "ship",
		[]string{paths.PreviousProjectConfigFile + ": ship:", "nerv 5.0.0-alpha.1"}},
	{"5.x bash deploy target", paths.ProjectConfigFile, "enabled: true\ndeploy:\n  targets:\n    - { name: staging, provider: bash }\n", "deploy",
		[]string{paths.ProjectConfigFile + ": deploy:", "provider: bash", "provider: script by hand"}},
	{"5.x project deploy provider", ".ai-workflow/providers/deploy-k8s/provider.yaml", "name: k8s\nextension: deploy\n", "",
		[]string{".ai-workflow/providers/deploy-k8s", "release-k8s", "extension: release", "by hand"}},
}

// legacyWorld is a repository with only the file rel (and the 4.2.0
// fixture's .nerv/nerv.yaml for a project provider folder).
func legacyWorld(t *testing.T, rel, content string) world {
	t.Helper()
	w := newWorld(t, comprobantes)
	if rel != ".nerv/nerv.yaml" && !strings.HasPrefix(rel, ".ai-workflow/providers/") {
		if err := os.Remove(filepath.Join(w.root, ".nerv", "nerv.yaml")); err != nil {
			t.Fatal(err)
		}
	}
	w.write(rel, content)
	return w
}

// installNervRelease is the command a refusal names without nerv-release.
const installNervRelease = "nerv install --tools nerv-release"

// S4: without nerv-release installed, the release and deploy sections of
// an older nerv are refused, naming the file, the section, and the
// command that installs nerv-release (with which nerv migrate converts
// them).
func TestLegacy_SectionsAreRefusedNamingWhatToMove(t *testing.T) {
	for _, c := range legacyCases {
		t.Run(c.name, func(t *testing.T) {
			w := legacyWorld(t, c.rel, c.content)
			refused(t, w.options(false), append(slices.Clone(c.wants), installNervRelease, "run nerv migrate again")...)
		})
	}
}

// S2, S5: with nerv-release installed, each section moves as it was into
// the repository release.yaml (the same YAML once parsed) and leaves the
// project file; the changed files are backed up, a note says nerv-release
// migrate converts it, and the plan hands the repository to nerv-release
// migrate. A project provider folder stays for nerv-release to rename. A
// second run changes nothing.
func TestLegacy_SectionsMoveToReleaseYAMLWithNervRelease(t *testing.T) {
	for _, c := range legacyCases {
		t.Run(c.name, func(t *testing.T) {
			w := legacyWorld(t, c.rel, c.content)
			o := w.options(false)
			o.ReleaseYAMLRepo = filepath.Join(w.root, ".ai-workflow", "release.yaml")
			p, backups := w.migrate(o)
			if !p.LegacyReleaseRepo || p.LegacyReleaseUser {
				t.Errorf("LegacyReleaseRepo, LegacyReleaseUser = %v, %v, want true, false", p.LegacyReleaseRepo, p.LegacyReleaseUser)
			}
			if !anyContains(p.Notes, "nerv-release migrate") {
				t.Errorf("no note names nerv-release migrate: %q", p.Notes)
			}
			if c.section == "" {
				if !w.exists(c.rel) {
					t.Error("the project provider was moved: nerv-release migrate renames it")
				}
				return
			}
			want := yamlMap(t, c.content)[c.section]
			if got := yamlMap(t, w.read(o.ReleaseYAMLRepo))[c.section]; !reflect.DeepEqual(got, want) {
				t.Errorf("release.yaml %s: = %#v, want %#v", c.section, got, want)
			}
			if _, kept := yamlMap(t, w.read(paths.ProjectConfigFile))[c.section]; kept {
				t.Errorf("%s still has %s:\n%s", paths.ProjectConfigFile, c.section, w.read(paths.ProjectConfigFile))
			}
			if len(backups) == 0 {
				t.Error("no backup was made")
			}
			// nerv-release migrate did not run here: a second run writes
			// nothing, and hands release.yaml's sections off again.
			if again := w.plan(o); !again.Empty() || !again.LegacyReleaseRepo {
				t.Errorf("a second run plans %d change(s), hand-off %v, want none and true", len(again.Changes), again.LegacyReleaseRepo)
			}
		})
	}
}

// Today's release: and deploy: move as before, with no hand-off to
// nerv-release migrate.
func TestLegacy_TodaysSectionsAreNotHandedOff(t *testing.T) {
	w := legacyWorld(t, paths.ProjectConfigFile, "enabled: true\nrelease:\n  projects:\n    - { name: api, path: api, build: bash }\ndeploy:\n  host: github\n")
	o := w.options(false)
	o.ReleaseYAMLRepo = filepath.Join(w.root, ".ai-workflow", "release.yaml")
	p, _ := w.migrate(o)
	if p.LegacyReleaseRepo || anyContains(p.Notes, "nerv-release migrate") {
		t.Errorf("today's sections were handed off: %q", p.Notes)
	}
	if got, want := w.read(o.ReleaseYAMLRepo), "release:\n  projects: [{ name: api, path: api, build: bash }]\ndeploy:\n  host: github\n"; got != want {
		t.Errorf("release.yaml =\n%s\nwant\n%s", got, want)
	}
}

// userBashTargets are a user configuration's 5.x bash deploy target, in
// deploy: or in its defaults.deploy seed.
var userBashTargets = []string{
	"deploy:\n  targets:\n    - { name: staging, provider: bash }\n",
	"defaults:\n  deploy:\n    targets:\n      - name: staging\n        provider: \"bash\"\n",
}

// S4: without nerv-release, a user configuration's bash deploy target is
// refused too, naming the command that installs nerv-release.
func TestLegacy_UserBashTargetsAreRefused(t *testing.T) {
	for _, content := range userBashTargets {
		w := newWorld(t, comprobantes)
		w.write(w.userPaths().UserConfig, content)
		refused(t, migrate.Options{Home: w.home, User: true, Now: now}, "user config", "provider: bash", "provider: script by hand", installNervRelease)
	}
}

// S2, S5: with nerv-release, the user configuration's bash deploy target
// moves as it was to the global release.yaml's defaults.deploy (a deploy:
// block goes under defaults.deploy first, as every user deploy: key), and
// the plan hands the user scope to nerv-release migrate.
func TestLegacy_UserBashTargetsMoveToTheGlobalReleaseYAML(t *testing.T) {
	want := []any{map[string]any{"name": "staging", "provider": "bash"}}
	for _, content := range userBashTargets {
		w := newWorld(t, comprobantes)
		w.write(w.userPaths().UserConfig, content)
		o := migrate.Options{Home: w.home, User: true, Now: now, ReleaseYAMLHome: filepath.Join(w.home, "release.yaml")}
		p, backups := w.migrate(o)
		if !p.LegacyReleaseUser || p.LegacyReleaseRepo {
			t.Errorf("LegacyReleaseUser, LegacyReleaseRepo = %v, %v, want true, false", p.LegacyReleaseUser, p.LegacyReleaseRepo)
		}
		if !anyContains(p.Notes, "nerv-release migrate") {
			t.Errorf("no note names nerv-release migrate: %q", p.Notes)
		}
		defaults, _ := yamlMap(t, w.read(o.ReleaseYAMLHome))["defaults"].(map[string]any)
		deploy, _ := defaults["deploy"].(map[string]any)
		if !reflect.DeepEqual(deploy["targets"], want) {
			t.Errorf("global release.yaml =\n%s", w.read(o.ReleaseYAMLHome))
		}
		if strings.Contains(w.read(w.userPaths().UserConfig), "bash") {
			t.Errorf("the user config kept the target:\n%s", w.read(w.userPaths().UserConfig))
		}
		if len(backups) == 0 {
			t.Error("no backup was made")
		}
	}
}

// A project provider folder named deploy-<name> that is not a 5.x deploy
// provider (no extension: deploy) is left alone.
func TestLegacy_OtherProjectProvidersAreLeftAlone(t *testing.T) {
	w := newWorld(t, comprobantes)
	w.write(".ai-workflow/providers/deploy-k8s/provider.yaml", "name: deploy-k8s\nextension: task\n")
	w.migrate(w.options(false))
	if !w.exists(".ai-workflow/providers/deploy-k8s/provider.yaml") {
		t.Error("a task provider named deploy-k8s was moved")
	}
}

// An older nerv's section moves to release.yaml as its own text, comments
// and layout kept, for nerv-release migrate to convert as nerv 6.6 did in
// place (it keeps the comments of the stages it keeps).
func TestLegacy_SectionsMoveAsTheirOwnText(t *testing.T) {
	block := "release:\n  semantic_release:\n    version: 24.2.3\n  projects:\n    - name: lib\n      path: lib\n      presets: [dotnet]\n      stages:\n        # our own lint\n        lint:\n          run: dotnet format\n"
	pipeline := "pipeline:\n  images:\n    - name: ghcr.io/acme/app   # the app image\n"
	w := legacyWorld(t, ".nerv/nerv.yaml", "enabled: true\n"+block+pipeline)
	o := w.options(false)
	o.ReleaseYAMLRepo = filepath.Join(w.root, ".ai-workflow", "release.yaml")
	w.migrate(o)
	got := w.read(o.ReleaseYAMLRepo)
	for _, want := range []string{block, pipeline} {
		if !strings.Contains(got, want) {
			t.Errorf("release.yaml lacks the section as written:\n%s\nwant it to contain:\n%s", got, want)
		}
	}
}

// legacyReleaseYAMLWorld is a repository already in today's shape (its
// config.yaml migrated, nothing older left in it) whose nerv-release
// files are about to be written by the test, with both release.yaml
// targets set: a nerv migrate that moved an older nerv's sections to
// release.yaml before nerv-release migrate failed or refused.
func legacyReleaseYAMLWorld(t *testing.T) (world, migrate.Options) {
	t.Helper()
	w := legacyWorld(t, paths.ProjectConfigFile, "enabled: true\n")
	o := w.options(true)
	o.ReleaseYAMLRepo = filepath.Join(w.root, ".ai-workflow", "release.yaml")
	o.ReleaseYAMLHome = filepath.Join(w.home, "release.yaml")
	w.migrate(o)
	return w, o
}

// B1 (S5): an older nerv's sections nerv migrate moved to release.yaml
// on an earlier run, left unconverted because nerv-release migrate
// failed or refused, are handed to nerv-release migrate again: the
// repository's and the global release.yaml are inspected, with nothing
// for nerv itself to write.
func TestLegacy_UnconvertedReleaseYAMLIsHandedOffAgain(t *testing.T) {
	w, o := legacyReleaseYAMLWorld(t)
	w.write(o.ReleaseYAMLRepo, "pipeline:\n  images:\n    - name: ghcr.io/acme/app\n"+
		"ship:\n  projects: []\n"+
		"release:\n  semantic_release:\n    version: 24.2.3\n"+
		"deploy:\n  targets:\n    - { name: staging, provider: bash }\n")
	w.write(o.ReleaseYAMLHome, "defaults:\n  deploy:\n    targets:\n      - name: staging\n        provider: bash\n")
	p := w.plan(o)
	if !p.Empty() {
		t.Errorf("Compute planned %d change(s), want none: %+v", len(p.Changes), p.Changes)
	}
	if !p.LegacyReleaseRepo || !p.LegacyReleaseUser {
		t.Errorf("LegacyReleaseRepo, LegacyReleaseUser = %v, %v, want true, true", p.LegacyReleaseRepo, p.LegacyReleaseUser)
	}
	for _, want := range []string{o.ReleaseYAMLRepo, o.ReleaseYAMLHome} {
		if !slices.ContainsFunc(p.Notes, func(n string) bool {
			return strings.Contains(n, want+" still holds") && strings.Contains(n, "nerv-release migrate converts")
		}) {
			t.Errorf("no note says %s still holds them for nerv-release migrate: %q", want, p.Notes)
		}
	}
	for _, want := range []string{"pipeline:", "ship:", "release: (4.2.0)", "deploy: (provider: bash)", "defaults: (provider: bash)"} {
		if !anyContains(p.Notes, want) {
			t.Errorf("no note names %s: %q", want, p.Notes)
		}
	}
}

// A release.yaml in today's shape is not handed off again.
func TestLegacy_TodaysReleaseYAMLIsNotHandedOff(t *testing.T) {
	w, o := legacyReleaseYAMLWorld(t)
	w.write(o.ReleaseYAMLRepo, "release:\n  projects:\n    - { name: api, path: api, build: bash }\ndeploy:\n  host: github\n  targets:\n    - { name: staging, provider: script }\n")
	w.write(o.ReleaseYAMLHome, "defaults:\n  deploy:\n    targets:\n      - { name: staging, provider: script }\n")
	p := w.plan(o)
	if !p.Empty() || p.LegacyReleaseRepo || p.LegacyReleaseUser || anyContains(p.Notes, "nerv-release migrate") {
		t.Errorf("today's release.yaml was handed off: %d change(s), %v, %v, %q", len(p.Changes), p.LegacyReleaseRepo, p.LegacyReleaseUser, p.Notes)
	}
}

// A 4.2.0 release: is never merged into a release: the repository's
// release.yaml already has: nerv migrate refuses, naming both, and writes
// nothing (nerv 6.6 did not merge two release: sections either).
func TestLegacy_A420ReleaseIsNotMergedIntoAnExistingOne(t *testing.T) {
	w := legacyWorld(t, ".nerv/nerv.yaml", "enabled: true\nrelease:\n  semantic_release:\n    version: 24.2.3\n  projects:\n    - { name: lib, path: lib, presets: [dotnet] }\n")
	o := w.options(false)
	o.ReleaseYAMLRepo = filepath.Join(w.root, ".ai-workflow", "release.yaml")
	existing := "release:\n  projects:\n    - { name: existing, path: . }\n"
	w.write(".ai-workflow/release.yaml", existing)
	refused(t, o, "release:", "4.2.0", o.ReleaseYAMLRepo, "does not merge")
	if got := w.read(".ai-workflow/release.yaml"); got != existing {
		t.Errorf("release.yaml changed:\n%s", got)
	}
}
