package claude_test

import (
	"context"
	"strings"
	"testing"

	"github.com/war-apps/nerv-gentle-ai/internal/claude"
	"github.com/war-apps/nerv-gentle-ai/internal/env/envtest"
)

func TestPluginCLI_Uninstall_Success(t *testing.T) {
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"claude plugin uninstall nerv@nerv": {Stdout: "uninstalled\n", ExitCode: 0},
		},
	}
	cli := claude.PluginCLI{Runner: runner}

	if _, err := cli.Uninstall(context.Background(), "nerv@nerv"); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if len(runner.Calls) != 1 || runner.Calls[0].Name != "claude" {
		t.Fatalf("unexpected calls: %+v", runner.Calls)
	}
}

func TestPluginCLI_Uninstall_TolerantOfNotInstalled(t *testing.T) {
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"claude plugin uninstall nerv@nerv": {Stderr: "Error: plugin nerv@nerv is not installed\n", ExitCode: 1},
		},
	}
	cli := claude.PluginCLI{Runner: runner}

	if _, err := cli.Uninstall(context.Background(), "nerv@nerv"); err != nil {
		t.Fatalf("Uninstall() error = %v, want nil (tolerant of 'not installed')", err)
	}
}

func TestPluginCLI_Uninstall_OtherFailureErrors(t *testing.T) {
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"claude plugin uninstall nerv@nerv": {Stderr: "boom\n", ExitCode: 1},
		},
	}
	cli := claude.PluginCLI{Runner: runner}

	_, err := cli.Uninstall(context.Background(), "nerv@nerv")
	if err == nil {
		t.Fatal("Uninstall() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %v, want it to mention the failure output", err)
	}
}

func TestPluginCLI_Install_Success(t *testing.T) {
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"claude plugin install nerv@nerv": {Stdout: "installed\n", ExitCode: 0},
		},
	}
	cli := claude.PluginCLI{Runner: runner}

	if _, err := cli.Install(context.Background(), "nerv@nerv"); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
}

func TestPluginCLI_Install_FailureErrors(t *testing.T) {
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"claude plugin install nerv@nerv": {Stderr: "network error\n", ExitCode: 1},
		},
	}
	cli := claude.PluginCLI{Runner: runner}

	_, err := cli.Install(context.Background(), "nerv@nerv")
	if err == nil {
		t.Fatal("Install() error = nil, want an error")
	}
}

func TestPluginCLI_LaunchFailureErrors(t *testing.T) {
	runner := &envtest.FakeRunner{
		Default: envtest.Response{Err: context.DeadlineExceeded},
	}
	cli := claude.PluginCLI{Runner: runner}

	if _, err := cli.Install(context.Background(), "nerv@nerv"); err == nil {
		t.Fatal("Install() error = nil, want an error on launch failure")
	}
}
