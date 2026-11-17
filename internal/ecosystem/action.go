package ecosystem

import (
	"context"

	"github.com/war-apps/nerv-kit-by-gentle-ai/runner"
)

// InstallCommand is the command line t's official install script runs
// with, for goos: the curl|sh one-liner on Linux and macOS, the
// irm|iex one-liner (PowerShell) on Windows -- exactly the lines S11
// documents, so a fake runner's recorded call proves the right script
// ran without ever reaching the network.
func InstallCommand(t Tool, goos string) (name string, args []string) {
	if goos == "windows" {
		script := "irm https://raw.githubusercontent.com/" + t.Repo + "/main/scripts/install.ps1 | iex"
		return "powershell", []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script}
	}
	script := "curl -fsSL https://raw.githubusercontent.com/" + t.Repo + "/main/scripts/install.sh | sh"
	return "sh", []string{"-c", script}
}

// UpgradeCommand is "<path> upgrade"; path empty falls back to t.Binary,
// resolved on PATH by the runner.
func UpgradeCommand(t Tool, path string) (name string, args []string) {
	bin := path
	if bin == "" {
		bin = t.Binary
	}
	return bin, []string{"upgrade"}
}

// Install runs t's official install script through run.
func Install(ctx context.Context, run runner.Runner, t Tool, goos string) (stdout, stderr string, exitCode int, err error) {
	name, args := InstallCommand(t, goos)
	return run.Run(ctx, name, args...)
}

// Upgrade runs "<path> upgrade" through run.
func Upgrade(ctx context.Context, run runner.Runner, t Tool, path string) (stdout, stderr string, exitCode int, err error) {
	name, args := UpgradeCommand(t, path)
	return run.Run(ctx, name, args...)
}
