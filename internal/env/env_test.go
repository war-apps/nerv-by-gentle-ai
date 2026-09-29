package env_test

import (
	"context"
	"strings"
	"testing"

	"github.com/war-apps/nerv-gentle-ai/internal/env"
)

func TestHomeDir_PrefersHomeOverUserProfile(t *testing.T) {
	t.Setenv("HOME", `C:\home\walter`)
	t.Setenv("USERPROFILE", `C:\Users\walter`)

	got, err := env.HomeDir()
	if err != nil {
		t.Fatalf("HomeDir() error = %v", err)
	}
	if got != `C:\home\walter` {
		t.Fatalf("HomeDir() = %q, want HOME value", got)
	}
}

func TestHomeDir_FallsBackToUserProfile(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", `C:\Users\walter`)

	got, err := env.HomeDir()
	if err != nil {
		t.Fatalf("HomeDir() error = %v", err)
	}
	if got != `C:\Users\walter` {
		t.Fatalf("HomeDir() = %q, want USERPROFILE value", got)
	}
}

func TestHomeDir_ErrorsWhenNeitherSet(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	_, err := env.HomeDir()
	if err == nil {
		t.Fatal("HomeDir() error = nil, want error when neither HOME nor USERPROFILE is set")
	}
}

func TestLookPath_FindsKnownExecutable(t *testing.T) {
	// "go" must be on PATH: the test binary itself was built by it.
	if _, err := env.LookPath("go"); err != nil {
		t.Fatalf("LookPath(go) error = %v, want nil", err)
	}
}

func TestLookPath_ErrorsOnUnknownExecutable(t *testing.T) {
	if _, err := env.LookPath("nerv-definitely-not-a-real-binary-xyz"); err == nil {
		t.Fatal("LookPath(nonexistent) error = nil, want error")
	}
}

func TestExecRunner_CapturesStdoutAndExitCode(t *testing.T) {
	var runner env.ExecRunner
	stdout, _, exitCode, err := runner.Run(context.Background(), "go", "version")
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if exitCode != 0 {
		t.Fatalf("Run() exitCode = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, "go version") {
		t.Fatalf("Run() stdout = %q, want it to contain %q", stdout, "go version")
	}
}

func TestExecRunner_NonZeroExitWithoutLaunchError(t *testing.T) {
	var runner env.ExecRunner
	_, _, exitCode, err := runner.Run(context.Background(), "go", "nerv-bogus-subcommand-xyz")
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (a non-zero exit is not a launch error)", err)
	}
	if exitCode == 0 {
		t.Fatal("Run() exitCode = 0, want non-zero for an unknown go subcommand")
	}
}

func TestExecRunner_LaunchErrorOnUnknownExecutable(t *testing.T) {
	var runner env.ExecRunner
	_, _, _, err := runner.Run(context.Background(), "nerv-definitely-not-a-real-binary-xyz")
	if err == nil {
		t.Fatal("Run() error = nil, want a launch error for a nonexistent executable")
	}
}
