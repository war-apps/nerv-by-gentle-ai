package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-kit-by-gentle-ai/runner/runnertest"
)

// nervTaskInstalledOpts fakes nerv-task as found on PATH (ecosystem.Detect
// resolves any LookPath hit as Installed, regardless of its "version
// --json" response), on a fixed goos so the global task.yaml path stays
// under the test's fake home (aiworkflow.UserConfigPath's POSIX branch
// reads XDG_CONFIG_HOME, never the fake home's Windows AppData env var).
func nervTaskInstalledOpts(home string) options {
	opts := testOptions(home)
	opts.ExtensionsFS = nerv.ExtensionsFS()
	opts.GOOS = "linux"
	opts.LookPath = func(name string) (string, error) {
		if name == "nerv-task" {
			return "/usr/bin/nerv-task", nil
		}
		return "", os.ErrNotExist
	}
	return opts
}

// nerv migrate --user, with nerv-task installed, also moves the user
// config's tasks: personal keys to nerv-task's global task.yaml (S12);
// without nerv-task installed, nothing about tasks: changes (regression:
// TestMigrate_UserRunTimeKeysMoveUnderDefaults keeps covering that path).
func TestMigrate_TaskYAMLMovesWhenNervTaskIsInstalled(t *testing.T) {
	home := t.TempDir()
	opts := nervTaskInstalledOpts(home)
	opts.Runner = &runnertest.FakeRunner{Responses: map[string]runnertest.Response{
		"git -C . rev-parse --show-toplevel": {Stderr: "fatal: not a git repository", ExitCode: 128},
	}}
	user := paths.Resolve(home).UserConfig
	if err := os.MkdirAll(filepath.Dir(user), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(user, []byte("tasks:\n  timer_store: ~/t.json\n  providers:\n    teamwork:\n      assignee_id: 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := runMigrateCLI(t, opts, "--user")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	taskYAML := filepath.Join(home, ".config", "ai-workflow", "task.yaml")
	data, err := os.ReadFile(taskYAML)
	if err != nil {
		t.Fatalf("task.yaml was not written: %v\n%s", err, out)
	}
	if !strings.Contains(string(data), "timer_store: ~/t.json") || !strings.Contains(string(data), "assignee_id: 7") {
		t.Errorf("task.yaml:\n%s", data)
	}
	remaining, _ := os.ReadFile(user)
	if strings.Contains(string(remaining), "timer_store") || strings.Contains(string(remaining), "assignee_id") {
		t.Errorf("the moved keys are still in the user config:\n%s", remaining)
	}
}

// Without nerv-task installed, nerv migrate --user never touches a global
// task.yaml (the existing user-config run-time-keys behavior is
// unaffected -- see TestMigrate_UserRunTimeKeysMoveUnderDefaults).
func TestMigrate_TaskYAMLUntouchedWhenNervTaskNotInstalled(t *testing.T) {
	home := t.TempDir()
	opts := testOptions(home)
	opts.ExtensionsFS = nerv.ExtensionsFS()
	opts.GOOS = "linux"
	opts.Runner = &runnertest.FakeRunner{Responses: map[string]runnertest.Response{
		"git -C . rev-parse --show-toplevel": {Stderr: "fatal: not a git repository", ExitCode: 128},
	}}
	user := paths.Resolve(home).UserConfig
	if err := os.MkdirAll(filepath.Dir(user), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(user, []byte("tasks:\n  timer_store: ~/t.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := runMigrateCLI(t, opts, "--user")
	if code != 0 || !strings.Contains(out, "nothing to migrate") {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ai-workflow", "task.yaml")); err == nil {
		t.Error("task.yaml was written without nerv-task installed")
	}
}
