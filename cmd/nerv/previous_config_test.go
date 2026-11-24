package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-kit-by-gentle-ai/runner"
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

func TestPrevious_ConfigureInitRepoRefuses(t *testing.T) {
	repo := gitInit(t)
	previous := paths.PreviousProjectConfig(repo)
	writePrevious(t, previous)
	opts := testOptions(t.TempDir())
	opts.Runner = runner.ExecRunner{}
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
	opts.Runner = runner.ExecRunner{}
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
