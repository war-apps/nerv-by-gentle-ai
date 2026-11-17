package ecosystem

import (
	"context"
	"strings"
	"testing"

	"github.com/war-apps/nerv-kit-by-gentle-ai/runner/runnertest"
)

func TestInstallCommand_Posix(t *testing.T) {
	name, args := InstallCommand(testTool(), "linux")
	if name != "sh" {
		t.Errorf("name = %q, want sh", name)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-c") {
		t.Errorf("args = %v, want -c <script>", args)
	}
	if !strings.Contains(joined, "curl -fsSL https://raw.githubusercontent.com/war-apps/nerv-task-by-gentle-ai/main/scripts/install.sh | sh") {
		t.Errorf("args = %v, want the curl|sh one-liner for the tool's repo", args)
	}
}

func TestInstallCommand_Windows(t *testing.T) {
	name, args := InstallCommand(testTool(), "windows")
	if name != "powershell" {
		t.Errorf("name = %q, want powershell", name)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
		"irm https://raw.githubusercontent.com/war-apps/nerv-task-by-gentle-ai/main/scripts/install.ps1 | iex"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args = %v, want it to contain %q", args, want)
		}
	}
}

func TestUpgradeCommand_RunsToolUpgrade(t *testing.T) {
	name, args := UpgradeCommand(testTool(), "/usr/local/bin/nerv-task")
	if name != "/usr/local/bin/nerv-task" {
		t.Errorf("name = %q, want the resolved path", name)
	}
	if len(args) != 1 || args[0] != "upgrade" {
		t.Errorf("args = %v, want [upgrade]", args)
	}
}

func TestUpgradeCommand_FallsBackToBareBinaryNameWithoutAPath(t *testing.T) {
	name, _ := UpgradeCommand(testTool(), "")
	if name != "nerv-task" {
		t.Errorf("name = %q, want nerv-task (PATH-resolved by the runner)", name)
	}
}

func TestInstall_RunsThroughTheRunner(t *testing.T) {
	run := &runnertest.FakeRunner{}
	_, _, _, err := Install(context.Background(), run, testTool(), "linux")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(run.Calls) != 1 || run.Calls[0].Name != "sh" {
		t.Fatalf("Calls = %+v, want exactly one sh call (no real network/script execution)", run.Calls)
	}
}

func TestUpgrade_RunsThroughTheRunner(t *testing.T) {
	run := &runnertest.FakeRunner{}
	_, _, _, err := Upgrade(context.Background(), run, testTool(), "/usr/local/bin/nerv-task")
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}
	if len(run.Calls) != 1 || run.Calls[0].Name != "/usr/local/bin/nerv-task" || len(run.Calls[0].Args) != 1 || run.Calls[0].Args[0] != "upgrade" {
		t.Fatalf("Calls = %+v, want one %q upgrade call", run.Calls, "/usr/local/bin/nerv-task")
	}
}
