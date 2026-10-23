package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
)

// Commands treat a repository or home that has only the pre-rename
// configuration file as unconfigured and print one line naming the old
// file and "nerv migrate".

func writeLegacy(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// hintLines returns the lines of out that name old and nerv migrate.
func hintLines(out, old string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, old) && strings.Contains(l, `"nerv migrate"`) {
			lines = append(lines, l)
		}
	}
	return lines
}

func TestLegacyConfig_ConfigureHintsAndReadsTheNewUserFile(t *testing.T) {
	home := t.TempDir()
	old := paths.Resolve(home).LegacyUserConfig
	writeLegacy(t, old, "git:\n  base_branch: legacy\n")
	var stdout, stderr bytes.Buffer

	code := run([]string{"configure", "--set", "git.base_branch=trunk"}, &stdout, &stderr, testOptions(home))
	if code != 0 {
		t.Fatalf("exit %d\nstdout %s\nstderr %s", code, stdout.String(), stderr.String())
	}
	if got := hintLines(stderr.String(), old); len(got) != 1 {
		t.Errorf("stderr %q: want exactly one hint line naming %s", stderr.String(), old)
	}
	data, err := os.ReadFile(paths.Resolve(home).UserConfig)
	if err != nil || !strings.Contains(string(data), "base_branch: trunk") || strings.Contains(string(data), "legacy") {
		t.Errorf("new user file = %q, %v; want a fresh file with only the new value", data, err)
	}
	if data, _ := os.ReadFile(old); string(data) != "git:\n  base_branch: legacy\n" {
		t.Errorf("old file changed: %q", data)
	}
}

func TestLegacyConfig_ConfigureWithExplicitConfigPrintsNoHint(t *testing.T) {
	home := t.TempDir()
	writeLegacy(t, paths.Resolve(home).LegacyUserConfig, "git:\n  base_branch: legacy\n")
	var stdout, stderr bytes.Buffer

	code := run([]string{"configure", "--config", filepath.Join(t.TempDir(), "c.yml"), "--set", "git.base_branch=trunk"}, &stdout, &stderr, testOptions(home))
	if code != 0 || strings.Contains(stderr.String(), "nerv migrate") {
		t.Errorf("exit %d, stderr %q: want no hint for an explicit --config", code, stderr.String())
	}
}

func TestLegacyConfig_ApplyModelsHints(t *testing.T) {
	home := t.TempDir()
	old := paths.Resolve(home).LegacyUserConfig
	writeLegacy(t, old, "models:\n  aoba: { model: haiku }\n")
	var stdout, stderr bytes.Buffer

	run([]string{"apply-models"}, &stdout, &stderr, testOptions(home))
	if got := hintLines(stderr.String(), old); len(got) != 1 {
		t.Errorf("stderr %q: want exactly one hint line naming %s", stderr.String(), old)
	}
}

func TestLegacyConfig_ShipRefusesAsUnconfiguredWithTheHint(t *testing.T) {
	f := newShipFixture(t, "")
	old := filepath.Join(f.top, ".nerv", "nerv.yaml")
	writeLegacy(t, old, shipMonorepoYAML)

	code, _, stderr := f.run(t, "--yes")
	if code != 1 || !strings.Contains(stderr, "no release: section in .ai-workflow.yml") {
		t.Fatalf("exit %d, stderr %q: want the not-configured refusal", code, stderr)
	}
	if got := hintLines(stderr, old); len(got) != 1 {
		t.Errorf("stderr %q: want exactly one hint line naming %s", stderr, old)
	}
}

func TestLegacyConfig_VersionNextIsTheWholeRepositoryWithTheHint(t *testing.T) {
	f := newNextFixture(t, "")
	old := filepath.Join(f.top, ".nerv", "nerv.yaml")
	writeLegacy(t, old, "release:\n  projects:\n    - name: api\n      path: api\n")

	code, _, stderr := f.run(t, "--project", "api")
	if code != 1 || !strings.Contains(stderr, "no release: section in .ai-workflow.yml") {
		t.Fatalf("exit %d, stderr %q: want the single-project refusal", code, stderr)
	}
	if got := hintLines(stderr, old); len(got) != 1 {
		t.Errorf("stderr %q: want exactly one hint line naming %s", stderr, old)
	}
}

func TestLegacyConfig_PipelineRefusesAsUnconfiguredWithTheHint(t *testing.T) {
	f := newPipelineFixture(t, "")
	old := filepath.Join(f.top, ".nerv", "nerv.yaml")
	writeLegacy(t, old, "pipeline:\n  dotnet:\n    solution: App.slnx\n  images:\n    - name: ghcr.io/acme/app\n")

	code, _, stderr := f.run(t, "validate")
	if code != 1 || !strings.Contains(stderr, "no pipeline: section in .ai-workflow.yml") {
		t.Fatalf("exit %d, stderr %q: want the not-configured refusal", code, stderr)
	}
	if got := hintLines(stderr, old); len(got) != 1 {
		t.Errorf("stderr %q: want exactly one hint line naming %s", stderr, old)
	}
}
