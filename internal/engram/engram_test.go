package engram_test

import (
	"context"
	"os"
	"testing"

	"github.com/war-apps/nerv-gentle-ai/internal/engram"
	"github.com/war-apps/nerv-gentle-ai/internal/env/envtest"
)

func lookPathFound(string) (string, error) { return "/usr/local/bin/engram", nil }
func lookPathMissing(string) (string, error) {
	return "", os.ErrNotExist
}

func TestEnsureKnowledgeBase_EngramNotOnPath_Warns(t *testing.T) {
	runner := &envtest.FakeRunner{}

	result := engram.EnsureKnowledgeBase(context.Background(), runner, lookPathMissing)

	if len(runner.Calls) != 0 {
		t.Errorf("expected no calls when engram is not on PATH, got %+v", runner.Calls)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly one", result.Warnings)
	}
}

func TestEnsureKnowledgeBase_ProjectAlreadyExists(t *testing.T) {
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"engram projects list": {Stdout: "nerv    3 memories\nother-project   1 memory\n", ExitCode: 0},
		},
	}

	result := engram.EnsureKnowledgeBase(context.Background(), runner, lookPathFound)

	if len(result.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", result.Warnings)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("Messages = %v, want exactly one", result.Messages)
	}
	for _, c := range runner.Calls {
		if c.Name == "engram" && len(c.Args) > 0 && c.Args[0] == "save" {
			t.Error("engram save should not run when the project already exists")
		}
	}
}

func TestEnsureKnowledgeBase_ProjectMissing_CreatesIt(t *testing.T) {
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"engram projects list": {Stdout: "other-project   1 memory\n", ExitCode: 0},
		},
	}

	result := engram.EnsureKnowledgeBase(context.Background(), runner, lookPathFound)

	if len(result.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", result.Warnings)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("Messages = %v, want exactly one", result.Messages)
	}

	var sawSave bool
	for _, c := range runner.Calls {
		if c.Name == "engram" && len(c.Args) > 0 && c.Args[0] == "save" {
			sawSave = true
			foundProject := false
			for i, a := range c.Args {
				if a == "--project" && i+1 < len(c.Args) && c.Args[i+1] == "nerv" {
					foundProject = true
				}
			}
			if !foundProject {
				t.Errorf("engram save call missing --project nerv: %+v", c.Args)
			}
		}
	}
	if !sawSave {
		t.Error("expected an 'engram save' call when the project is missing")
	}
}

func TestEnsureKnowledgeBase_ProjectsListFails_Warns(t *testing.T) {
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"engram projects list": {Stderr: "connection refused", ExitCode: 1},
		},
	}

	result := engram.EnsureKnowledgeBase(context.Background(), runner, lookPathFound)

	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly one", result.Warnings)
	}
	if len(result.Messages) != 0 {
		t.Errorf("Messages = %v, want none", result.Messages)
	}
}

func TestEnsureKnowledgeBase_SaveFails_Warns(t *testing.T) {
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"engram projects list": {Stdout: "", ExitCode: 0},
		},
		Default: envtest.Response{Stderr: "write failed", ExitCode: 1},
	}

	result := engram.EnsureKnowledgeBase(context.Background(), runner, lookPathFound)

	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly one", result.Warnings)
	}
}
