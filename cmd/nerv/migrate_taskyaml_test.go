package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-kit-by-gentle-ai/runner/runnertest"
)

// nervTaskInstalledOpts fakes nerv-task as found on PATH (ecosystem.Detect
// resolves any LookPath hit as Installed, regardless of its "version
// --json" response). The global task.yaml goes next to the user config
// nerv migrates (paths.Resolve), so a fake home never reaches the real
// XDG_CONFIG_HOME or %AppData%.
func nervTaskInstalledOpts(home string) options {
	opts := testOptions(home)
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
	taskYAML := filepath.Join(filepath.Dir(user), "task.yaml")
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
	if _, err := os.Stat(filepath.Join(filepath.Dir(user), "task.yaml")); err == nil {
		t.Error("task.yaml was written without nerv-task installed")
	}
}

// catalogueToolInstalled answers per catalogue tool: nerv-release on PATH
// is installed, nerv-task (not found) is not -- so nerv-release alone never
// turns on the task.yaml move -- and an unknown name never is.
func TestCatalogueToolInstalled_PerTool(t *testing.T) {
	home := t.TempDir()
	opts := testOptions(home)
	opts.GOOS = "linux"
	opts.LookPath = func(name string) (string, error) {
		if name == "nerv-release" {
			return "/usr/bin/nerv-release", nil
		}
		return "", os.ErrNotExist
	}
	tests := []struct {
		name string
		want bool
	}{
		{"nerv-release", true},
		{"nerv-task", false},
		{"not-a-tool", false},
	}
	for _, tt := range tests {
		if got := catalogueToolInstalled(context.Background(), opts, home, tt.name); got != tt.want {
			t.Errorf("catalogueToolInstalled(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
