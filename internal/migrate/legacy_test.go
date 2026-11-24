package migrate_test

import (
	"os"
	"path/filepath"
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
}

// S7: the release and deploy sections of an older nerv are no longer
// converted: each is refused, naming the file and the section to move by
// hand.
func TestLegacy_SectionsAreRefusedNamingWhatToMove(t *testing.T) {
	cases := []struct {
		name, rel, content string
		wants              []string
	}{
		{"4.2.0 pipeline", ".nerv/nerv.yaml", "enabled: true\npipeline:\n  images:\n    - name: ghcr.io/acme/app\n",
			[]string{".nerv/nerv.yaml: pipeline:", "nerv 4.2.0", "by hand", "nerv 6.6"}},
		{"4.2.0 release", ".nerv/nerv.yaml", "enabled: true\nrelease:\n  semantic_release:\n    version: 24.2.3\n",
			[]string{".nerv/nerv.yaml: release:", "nerv 4.2.0", "by hand"}},
		{"4.2.0 release in the folder", paths.ProjectConfigFile, "enabled: true\nrelease:\n  semantic_release: true\n  projects:\n    - { name: app, path: . }\n",
			[]string{paths.ProjectConfigFile + ": release:", "nerv 4.2.0", "release.semantic_release", "by hand"}},
		{"4.2.0 release presets in the previous file", paths.PreviousProjectConfigFile, "enabled: true\nrelease:\n  projects:\n    - { name: app, path: ., presets: [node] }\n",
			[]string{paths.PreviousProjectConfigFile + ": release:", "nerv 4.2.0", "release.projects[0].presets"}},
		{"5.0.0-alpha.1 ship in the folder", paths.ProjectConfigFile, "enabled: true\nship:\n  projects: []\n",
			[]string{paths.ProjectConfigFile + ": ship:", "nerv 5.0.0-alpha.1", "by hand"}},
		{"5.0.0-alpha.1 ship in the previous file", paths.PreviousProjectConfigFile, "enabled: true\nship:\n  projects: []\n",
			[]string{paths.PreviousProjectConfigFile + ": ship:", "nerv 5.0.0-alpha.1"}},
		{"5.x bash deploy target", paths.ProjectConfigFile, "enabled: true\ndeploy:\n  targets:\n    - { name: staging, provider: bash }\n",
			[]string{paths.ProjectConfigFile + ": deploy:", "provider: bash", "provider: script by hand"}},
		{"5.x project deploy provider", ".ai-workflow/providers/deploy-k8s/provider.yaml", "name: k8s\nextension: deploy\n",
			[]string{".ai-workflow/providers/deploy-k8s", "release-k8s", "extension: release", "by hand"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := newWorld(t, comprobantes)
			if c.rel != ".nerv/nerv.yaml" && !strings.HasPrefix(c.rel, ".ai-workflow/providers/") {
				// Only the file under test.
				if err := os.Remove(filepath.Join(w.root, ".nerv", "nerv.yaml")); err != nil {
					t.Fatal(err)
				}
			}
			w.write(c.rel, c.content)
			refused(t, w.options(false), c.wants...)
		})
	}
}

// S7: a user configuration's 5.x bash deploy target, in deploy: or in its
// defaults.deploy seed, is refused too.
func TestLegacy_UserBashTargetsAreRefused(t *testing.T) {
	for _, content := range []string{
		"deploy:\n  targets:\n    - { name: staging, provider: bash }\n",
		"defaults:\n  deploy:\n    targets:\n      - name: staging\n        provider: \"bash\"\n",
	} {
		w := newWorld(t, comprobantes)
		w.write(w.userPaths().UserConfig, content)
		refused(t, migrate.Options{Home: w.home, User: true, Now: now}, "user config", "provider: bash", "provider: script by hand")
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
