package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// setupRepo is a git repository holding files (name -> content).
func setupRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

func runSetup(t *testing.T, repo string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opts := testOptions(t.TempDir())
	opts.Runner = env.ExecRunner{}
	code := run(append([]string{"ship", "setup", "--cwd", repo}, args...), &stdout, &stderr, opts)
	return code, stdout.String(), stderr.String()
}

const dimPackageJSON = `{
  "name": "dim.api",
  "release": {
    "branches": ["main", {"name": "develop", "prerelease": "alpha", "channel": "alpha"}, {"name": "hotfix/*", "prerelease": true}],
    "plugins": [
      ["@semantic-release/commit-analyzer", {"preset": "conventionalcommits", "releaseRules": [{"type": "docs", "release": "patch"}]}],
      "@semantic-release/release-notes-generator",
      ["@semantic-release/exec", {"prepareCmd": "./bump-csproj.sh"}],
      "@semantic-release/git"
    ]
  }
}
`

// S3: setup --import translates the semantic-release configuration into
// the ship: section, keeps the rest of the file, and prints what it could
// not translate.
func TestShipSetup_ImportWritesTheShipSectionAndPrintsTheWarnings(t *testing.T) {
	repo := setupRepo(t, map[string]string{
		"package.json":     dimPackageJSON,
		".ai-workflow.yml": "# team settings\nenabled: true\nship:\n  projects:\n    - name: api\n      path: .\n      build: { mode: script }\ntasks:\n  provider: none\n",
	})
	code, stdout, stderr := runSetup(t, repo, "--import")
	if code != 0 {
		t.Fatalf("exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
	data, _ := os.ReadFile(filepath.Join(repo, ".ai-workflow.yml"))
	text := string(data)
	for _, want := range []string{"# team settings\nenabled: true\n", "tasks:\n  provider: none\n", "- name: api"} {
		if !strings.Contains(text, want) {
			t.Errorf("the rest of the file was not kept (%q):\n%s", want, text)
		}
	}
	rel, err := shipconfig.Load(repo)
	if err != nil {
		t.Fatalf("the written section does not load: %v\n%s", err, text)
	}
	if len(rel.Branches) != 3 || rel.Branches[1].Channel != "alpha" || len(rel.ReleaseRules) != 1 || rel.ReleaseRules[0].Type != "docs" || rel.Projects[0].Name != "api" {
		t.Errorf("imported section = %+v", rel)
	}
	for _, want := range []string{"@semantic-release/exec", "package.json"} {
		if !strings.Contains(stderr+stdout, want) {
			t.Errorf("output lacks %q:\nstdout %s\nstderr %s", want, stdout, stderr)
		}
	}
}

// --dry-run prints the new section and writes nothing.
func TestShipSetup_DryRunWritesNothing(t *testing.T) {
	repo := setupRepo(t, map[string]string{"package.json": dimPackageJSON})
	code, stdout, stderr := runSetup(t, repo, "--import", "--dry-run")
	if code != 0 || !strings.Contains(stdout, "ship:\n") || !strings.Contains(stdout, "name: develop") {
		t.Fatalf("exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(repo, ".ai-workflow.yml")); !os.IsNotExist(err) {
		t.Errorf("a dry run wrote the file: %v", err)
	}
}

// S3: a JavaScript configuration is reported, never imported; no
// configuration is a refusal too.
func TestShipSetup_RefusesAJavaScriptConfigAndNoConfig(t *testing.T) {
	repo := setupRepo(t, map[string]string{"release.config.js": "module.exports = {}\n"})
	code, _, stderr := runSetup(t, repo, "--import")
	if code != 1 || !strings.Contains(stderr, "release.config.js") || !strings.Contains(stderr, "by hand") {
		t.Errorf("js config: exit %d, stderr %s", code, stderr)
	}
	code, _, stderr = runSetup(t, setupRepo(t, nil), "--import")
	if code != 1 || !strings.Contains(stderr, "no semantic-release configuration") {
		t.Errorf("no config: exit %d, stderr %s", code, stderr)
	}
}

// S9: an old release: section is renamed to ship: by the import.
func TestShipSetup_RenamesTheOldReleaseSection(t *testing.T) {
	repo := setupRepo(t, map[string]string{
		"package.json":     dimPackageJSON,
		".ai-workflow.yml": "release:\n  semantic_release: { version: 24.2.3 }\n  projects:\n    - name: api\n      path: .\n",
	})
	code, stdout, stderr := runSetup(t, repo, "--import")
	if code != 0 || !strings.Contains(stdout+stderr, "renamed to ship:") {
		t.Fatalf("exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
	rel, err := shipconfig.Load(repo)
	if err != nil || rel.Projects[0].Name != "api" {
		t.Fatalf("Load = %+v, %v", rel, err)
	}
}

// --import is required (the only mode for now).
func TestShipSetup_UsageErrors(t *testing.T) {
	if code, _, stderr := runSetup(t, setupRepo(t, nil)); code != 2 || !strings.Contains(stderr, "--import") {
		t.Errorf("no --import: exit %d, stderr %s", code, stderr)
	}
}

// B1 (L21): an existing engine key with a different value is never
// replaced silently.
func TestShipSetup_ExistingEngineKeysAreNotClobberedSilently(t *testing.T) {
	const before = "ship:\n  # hand-tuned\n  tag_format: custom-${version}\n  branches:\n    - main\n    - {name: next, prerelease: beta, channel: next}\n  projects:\n    - name: api\n      path: .\n      build: { mode: script }\n"
	repo := setupRepo(t, map[string]string{
		".releaserc.json":  `{"branches":["main",{"name":"develop","prerelease":"alpha","channel":"alpha"}],"tagFormat":"v${version}","plugins":[["@semantic-release/commit-analyzer",{"preset":"conventionalcommits"}],["@semantic-release/release-notes-generator",{"preset":"conventionalcommits"}]]}`,
		".ai-workflow.yml": before,
	})
	code, stdout, stderr := runSetup(t, repo, "--import")
	data, _ := os.ReadFile(filepath.Join(repo, ".ai-workflow.yml"))
	if code == 0 && string(data) != before && !strings.Contains(stdout+stderr, "tag_format") {
		t.Fatalf("existing tag_format/branches replaced with exit 0 and no notice\nstdout %s\nstderr %s\nfile:\n%s", stdout, stderr, data)
	}
	if code == 0 && !strings.Contains(string(data), "custom-${version}") && !strings.Contains(stdout+stderr, "custom-${version}") {
		t.Errorf("the replaced value is not reported:\n%s%s", stdout, stderr)
	}
}

const conflictingReleaserc = `{"branches":["main",{"name":"develop","prerelease":"alpha","channel":"alpha"}],"tagFormat":"v${version}","plugins":[["@semantic-release/commit-analyzer",{"preset":"conventionalcommits"}],["@semantic-release/release-notes-generator",{"preset":"conventionalcommits"}]]}`

const handTunedShip = "ship:\n  # hand-tuned\n  tag_format: custom-${version}\n  branches:\n    - main\n    - {name: next, prerelease: beta, channel: next}\n  projects:\n    - name: api\n      path: .\n      build: { mode: script }\n"

// B1: without --force the import refuses (exit 1), writes nothing and lists
// every conflicting key with its current and imported values.
func TestShipSetup_ConflictingEngineKeysRefuseAndListBothValues(t *testing.T) {
	repo := setupRepo(t, map[string]string{".releaserc.json": conflictingReleaserc, ".ai-workflow.yml": handTunedShip})
	code, stdout, stderr := runSetup(t, repo, "--import")
	if code != 1 {
		t.Fatalf("exit %d, want 1\nstdout %s\nstderr %s", code, stdout, stderr)
	}
	data, _ := os.ReadFile(filepath.Join(repo, ".ai-workflow.yml"))
	if string(data) != handTunedShip {
		t.Errorf("a refused import wrote the file:\n%s", data)
	}
	for _, want := range []string{"tag_format", "custom-${version}", "v${version}", "branches", "name: next", "name: develop", "--force"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
}

// B1: --force replaces them, prints each replaced value, and keeps the
// comments; an equal value is no conflict.
func TestShipSetup_ForceReplacesAndPrintsTheReplacedValues(t *testing.T) {
	repo := setupRepo(t, map[string]string{".releaserc.json": conflictingReleaserc, ".ai-workflow.yml": handTunedShip})
	code, stdout, stderr := runSetup(t, repo, "--import", "--force")
	if code != 0 {
		t.Fatalf("exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
	data, _ := os.ReadFile(filepath.Join(repo, ".ai-workflow.yml"))
	text := string(data)
	if !strings.Contains(text, "# hand-tuned") || !strings.Contains(text, "tag_format: v${version}") || strings.Contains(text, "name: next") {
		t.Errorf("forced import result:\n%s", text)
	}
	for _, want := range []string{"replaced", "tag_format", "custom-${version}", "name: next"} {
		if !strings.Contains(stdout+stderr, want) {
			t.Errorf("output lacks %q:\nstdout %s\nstderr %s", want, stdout, stderr)
		}
	}
	// A second import finds the same values: no conflict, no --force.
	if code, stdout, stderr := runSetup(t, repo, "--import"); code != 0 {
		t.Errorf("re-import of equal values: exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
}

// L21 advisory: a release: section without a semantic-release config is
// renamed to ship:, saying there was nothing to import; no false
// "semantic_release: dropped" line.
func TestShipSetup_ReleaseSectionWithoutSemanticReleaseConfigIsRenamed(t *testing.T) {
	repo := setupRepo(t, map[string]string{
		".ai-workflow.yml": "enabled: true\nrelease:\n  projects:\n    - name: api\n      path: .\n      build: { mode: script }\n",
	})
	code, stdout, stderr := runSetup(t, repo, "--import")
	if code != 0 || !strings.Contains(stdout+stderr, "renamed to ship:") || !strings.Contains(stdout+stderr, "nothing to import") {
		t.Fatalf("exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
	if strings.Contains(stdout+stderr, "semantic_release: dropped") {
		t.Errorf("false dropped line:\n%s%s", stdout, stderr)
	}
	rel, err := shipconfig.Load(repo)
	if err != nil || rel.Projects[0].Name != "api" {
		t.Fatalf("Load = %+v, %v", rel, err)
	}
}

// L21 advisory: errors name the section that is really in the file.
func TestShipSetup_ErrorsNameTheReleaseSection(t *testing.T) {
	repo := setupRepo(t, map[string]string{
		"package.json":     dimPackageJSON,
		".ai-workflow.yml": "release: [not, a, mapping]\n",
	})
	code, stdout, stderr := runSetup(t, repo, "--import")
	if code != 1 || !strings.Contains(stderr, "release: must be a mapping") {
		t.Errorf("exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
}
