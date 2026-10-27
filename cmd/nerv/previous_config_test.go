package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/artifact"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

// S9: the project configuration is .ai-workflow/config.yaml and the user
// one config.yaml. A leftover .ai-workflow.yml or user config.yml is never
// read silently: every command refuses (exit 1) with the nerv migrate hint
// and leaves the file unchanged.

const previousText = "enabled: true\nrelease:\n  projects:\n    - name: api\n      path: .\n      build: bash\n"

func writePrevious(t *testing.T, path string) {
	t.Helper()
	writeLegacy(t, path, previousText)
}

func assertPreviousRefused(t *testing.T, code int, stderr, previous, hint string) {
	t.Helper()
	if code != 1 {
		t.Errorf("exit %d, want 1; stderr %s", code, stderr)
	}
	want := previous + " is no longer read (configuration now lives in "
	if !strings.Contains(stderr, want) || !strings.Contains(stderr, hint) {
		t.Errorf("stderr %q: want %q and %s", stderr, want, hint)
	}
	if data, err := os.ReadFile(previous); err != nil || string(data) != previousText {
		t.Errorf("%s = %q, %v; want it unchanged", previous, data, err)
	}
}

const migrateHint = `run "nerv migrate" to move it`

func TestPrevious_VersionNextRefuses(t *testing.T) {
	f := newNextFixture(t, previousText)
	previous := paths.PreviousProjectConfig(f.top)
	writePrevious(t, previous)
	code, _, stderr := f.run(t)
	assertPreviousRefused(t, code, stderr, previous, migrateHint)
}

func TestPrevious_ReleaseRefuses(t *testing.T) {
	f := newReleaseFixture(t, previousText)
	previous := paths.PreviousProjectConfig(f.top)
	writePrevious(t, previous)
	code, _, stderr := f.run(t, "--dry-run")
	assertPreviousRefused(t, code, stderr, previous, migrateHint)
}

func TestPrevious_DeployRefuses(t *testing.T) {
	repo := gitInit(t)
	previous := paths.PreviousProjectConfig(repo)
	writePrevious(t, previous)
	opts := testOptions(t.TempDir())
	opts.Runner = env.ExecRunner{}
	opts.ExtensionsFS = nerv.ExtensionsFS()
	var stdout, stderr bytes.Buffer
	code := run([]string{"deploy", "--cwd", repo, "--artifact", filepath.Join(repo, "x"), "--dry-run"}, &stdout, &stderr, opts)
	assertPreviousRefused(t, code, stderr.String(), previous, migrateHint)
}

func TestPrevious_ConfigureInitRepoRefuses(t *testing.T) {
	repo := gitInit(t)
	previous := paths.PreviousProjectConfig(repo)
	writePrevious(t, previous)
	opts := testOptions(t.TempDir())
	opts.Runner = env.ExecRunner{}
	var stdout, stderr bytes.Buffer
	code := run([]string{"configure", "--init-repo", repo}, &stdout, &stderr, opts)
	assertPreviousRefused(t, code, stdout.String()+stderr.String(), previous, migrateHint)
	if _, err := os.Stat(paths.ProjectConfig(repo)); err == nil {
		t.Error("a refused --init-repo wrote .ai-workflow/config.yaml")
	}
}

// The previous user config.yml is refused by every command that reads the
// user configuration, naming nerv migrate --user.
func TestPrevious_UserConfigIsRefused(t *testing.T) {
	for _, args := range [][]string{
		{"configure", "--set", "git.base_branch=trunk"},
		{"install"},
		{"apply-models"},
	} {
		t.Run(args[0], func(t *testing.T) {
			home := t.TempDir()
			previous := paths.Resolve(home).PreviousUserConfig
			writePrevious(t, previous)
			var stdout, stderr bytes.Buffer
			code := run(args, &stdout, &stderr, testOptions(home))
			assertPreviousRefused(t, code, stderr.String(), previous, `run "nerv migrate --user" to move it`)
			if _, err := os.Stat(paths.Resolve(home).UserConfig); err == nil {
				t.Error("a refused command wrote config.yaml")
			}
		})
	}
}

// configure --init-repo writes .ai-workflow/config.yaml (prior behaviour
// at the new path).
func TestPrevious_ConfigureInitRepoWritesTheFolder(t *testing.T) {
	repo := gitInit(t)
	opts := testOptions(t.TempDir())
	opts.Runner = env.ExecRunner{}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"configure", "--init-repo", repo}, &stdout, &stderr, opts); code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	data, err := os.ReadFile(paths.ProjectConfig(repo))
	if err != nil || !strings.Contains(string(data), "enabled: true") {
		t.Errorf(".ai-workflow/config.yaml = %q, %v", data, err)
	}
	if _, err := os.Stat(paths.PreviousProjectConfig(repo)); err == nil {
		t.Error("--init-repo wrote .ai-workflow.yml")
	}
}

func gitInit(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if resolved, err := filepath.EvalSymlinks(repo); err == nil {
		repo = resolved
	}
	return repo
}

// S9: a project build provider in .ai-workflow/providers/release-<name>/
// fills the release build slot like a shipped one, its script referenced
// through {provider_dir}.
const projectBuildProvider = `name: make
extension: release
slot: build
contract_version: 1
runtime: exec
requires: [sh]
operations:
  validate: not_implemented
  plan:
    - argv: [sh, "{provider_dir}/plan.sh"]
`

const projectBuildPlan = `#!/bin/sh
cat >/dev/null
echo '{"status":"ok","data":{"steps":[{"stage":"build","name":"make","exec":["make","all"]}],"image_prefix":"make:"}}'
`

const projectBuildYAML = `release:
  projects:
    - name: core
      path: core
      build: make
`

func writeProjectFiles(t *testing.T, top string, files map[string]string) {
	t.Helper()
	for rel, text := range files {
		path := filepath.Join(top, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProjectProvider_ReleaseDryRunUsesTheProjectBuildProvider(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	f := newReleaseFixture(t, projectBuildYAML)
	writeProjectFiles(t, f.top, map[string]string{
		".ai-workflow/providers/release-make/provider.yaml": projectBuildProvider,
		".ai-workflow/providers/release-make/plan.sh":       projectBuildPlan,
	})
	f.ws.results = []versioning.Result{releaseResult("core", "1.1.0", "core-v1.1.0")}
	code, stdout, stderr := f.run(t, "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	if !slices.Contains(f.events, "core: make all") {
		t.Errorf("events %v: want the project provider's step core: make all", f.events)
	}
}

func TestProjectProvider_InvalidProjectProviderIsRefused(t *testing.T) {
	f := newReleaseFixture(t, projectBuildYAML)
	writeProjectFiles(t, f.top, map[string]string{
		".ai-workflow/providers/release-make/provider.yaml": strings.Replace(projectBuildProvider, "  plan:\n    - argv: [sh, \"{provider_dir}/plan.sh\"]\n", "", 1),
	})
	code, _, stderr := f.run(t, "--dry-run")
	want := ".ai-workflow/providers/release-make/provider.yaml: operations.plan: missing (implement it or declare not_implemented)"
	if code != 1 || !strings.Contains(stderr, want) {
		t.Errorf("exit %d, stderr %q: want 1 and %q", code, stderr, want)
	}
}

func TestProjectProvider_CollisionWithAShippedProviderIsRefused(t *testing.T) {
	f := newReleaseFixture(t, projectBuildYAML)
	writeProjectFiles(t, f.top, map[string]string{
		".ai-workflow/providers/release-bash/provider.yaml": projectBuildProvider,
	})
	code, _, stderr := f.run(t, "--dry-run")
	want := ".ai-workflow/providers/release-bash: nerv already ships an extension or provider named release-bash; give the project's another name"
	if code != 1 || !strings.Contains(stderr, want) {
		t.Errorf("exit %d, stderr %q: want 1 and %q", code, stderr, want)
	}
}

// S9: a project deploy target provider (docs/extensions.md's example) is
// found by nerv deploy, and its preflight script, referenced through
// {provider_dir}, runs from the provider folder during a dry run.
func TestProjectProvider_DeployDryRunUsesTheProjectTargetProvider(t *testing.T) {
	requireReleaseToolchain(t)
	releaseEnv(t)
	origin, work := newReleaseRepos(t)
	work.commit("feat: first", "app.txt")
	work.git("push", "-q", "origin", "main")
	marker := filepath.Join(t.TempDir(), "preflight.log")
	writeProjectFiles(t, work.dir, map[string]string{
		".ai-workflow/config.yaml": "deploy:\n  targets:\n    - name: lab\n      provider: rsync\n      config:\n        host: lab.example\n",
		".ai-workflow/providers/release-rsync/provider.yaml": `name: rsync
extension: release
slot: target
contract_version: 1
runtime: exec
requires: [bash]
config:
  - key: host
    scope: repo
    type: string
    prompt: The host to copy the release to
operations:
  preflight:
    - argv: [bash, "{provider_dir}/preflight.sh"]
  publish: not_implemented
  deploy:
    - argv: [bash, "{provider_dir}/deploy.sh", "{config.host}"]
  verify: not_implemented
`,
		".ai-workflow/providers/release-rsync/preflight.sh": "cat >/dev/null\necho preflight >> \"" + marker + "\"\necho '{\"status\":\"ok\",\"message\":\"rsync ready\"}'\n",
		".ai-workflow/providers/release-rsync/deploy.sh":    "exit 1\n",
	})
	src := filepath.Join(t.TempDir(), "out.txt")
	if err := os.WriteFile(src, []byte("built"), 0o644); err != nil {
		t.Fatal(err)
	}
	head := work.git("rev-parse", "HEAD")
	dir, err := artifact.Write(work.dir, artifact.Input{
		Manifest:         artifact.Manifest{Project: "widgets", Version: "1.0.0", Tag: "v1.0.0", BaseCommit: head, Branch: "main", BuildProvider: "bash"},
		Notes:            "first\n",
		ChangelogSection: "## 1.0.0\n\n* first\n",
		Outputs:          []artifact.OutputSource{{Path: "out.txt", Kind: artifact.KindFile, From: src}},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	opts := testOptions(t.TempDir())
	opts.Runner = env.ExecRunner{}
	opts.ExtensionsFS = nerv.ExtensionsFS()
	var stdout, stderr bytes.Buffer
	code := run([]string{"deploy", "--cwd", work.dir, "--artifact", dir, "--dry-run"}, &stdout, &stderr, opts)
	if code != 0 {
		t.Fatalf("exit %d\nstdout %s\nstderr %s", code, stdout.String(), stderr.String())
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "preflight\n" {
		t.Errorf("the project preflight script did not run once: %q, %v", data, err)
	}
	if out, err := exec.Command("git", "-C", origin.dir, "tag", "--list").CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "" {
		t.Errorf("a dry run pushed tags: %s %v", out, err)
	}
}
