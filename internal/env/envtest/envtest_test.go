package envtest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
)

// Compile-time proof FakeRunner satisfies env.Runner without importing it
// from a variable declaration inside a test (kept here, next to the tests
// that exercise it).
var _ env.Runner = (*envtest.FakeRunner)(nil)

func TestFakeRunner_ReturnsScriptedResponseByExactCommand(t *testing.T) {
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"gentle-ai --version": {Stdout: "gentle-ai 4.0.0\n", ExitCode: 0},
		},
	}

	stdout, _, exitCode, err := runner.Run(context.Background(), "gentle-ai", "--version")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if exitCode != 0 || stdout != "gentle-ai 4.0.0\n" {
		t.Fatalf("Run() = (%q, _, %d), want scripted response", stdout, exitCode)
	}
}

func TestFakeRunner_FallsBackToNameOnlyKey(t *testing.T) {
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"npx": {ExitCode: 0},
		},
	}

	_, _, exitCode, err := runner.Run(context.Background(), "npx", "skills", "add", "mattpocock/skills")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("Run() exitCode = %d, want 0 from the name-only fallback", exitCode)
	}
}

func TestFakeRunner_UsesDefaultWhenNoResponseMatches(t *testing.T) {
	runner := &envtest.FakeRunner{
		Default: envtest.Response{ExitCode: 1, Err: errors.New("boom")},
	}

	_, _, exitCode, err := runner.Run(context.Background(), "unscripted", "cmd")
	if err == nil || exitCode != 1 {
		t.Fatalf("Run() = (_, _, %d, %v), want the Default response", exitCode, err)
	}
}

func TestFakeRunner_RecordsEveryCall(t *testing.T) {
	runner := &envtest.FakeRunner{}

	_, _, _, _ = runner.Run(context.Background(), "gentle-ai", "--version")
	_, _, _, _ = runner.Run(context.Background(), "npx", "skills", "add", "repo")

	if len(runner.Calls) != 2 {
		t.Fatalf("len(Calls) = %d, want 2", len(runner.Calls))
	}
	if runner.Calls[0].Name != "gentle-ai" || len(runner.Calls[0].Args) != 1 || runner.Calls[0].Args[0] != "--version" {
		t.Fatalf("Calls[0] = %+v, unexpected", runner.Calls[0])
	}
	if runner.Calls[1].Name != "npx" || len(runner.Calls[1].Args) != 3 {
		t.Fatalf("Calls[1] = %+v, unexpected", runner.Calls[1])
	}
}
