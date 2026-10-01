package gentleai_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/war-apps/nerv-gentle-ai/internal/config"
	"github.com/war-apps/nerv-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-gentle-ai/internal/gentleai"
)

// ---------------------------------------------------------------------------
// Version / CheckPreflight — mirrors the gentle-ai version preflight in
// install.ps1 (~594-631) and Get-NervPrerequisitesStatus's gentle_ai block.
// ---------------------------------------------------------------------------

func TestCheckPreflight_FoundAndOkAtMajor3(t *testing.T) {
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"gentle-ai --version": {Stdout: "gentle-ai 3.7.0\n"},
	}}

	got := gentleai.CheckPreflight(context.Background(), runner)

	want := gentleai.Preflight{Found: true, Version: "3.7.0", OK: true}
	if got != want {
		t.Fatalf("CheckPreflight() = %+v, want %+v", got, want)
	}
}

func TestCheckPreflight_FoundButWrongMajor(t *testing.T) {
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"gentle-ai --version": {Stdout: "gentle-ai 2.1.0\n"},
	}}

	got := gentleai.CheckPreflight(context.Background(), runner)

	want := gentleai.Preflight{Found: true, Version: "2.1.0", OK: false}
	if got != want {
		t.Fatalf("CheckPreflight() = %+v, want %+v", got, want)
	}
}

func TestCheckPreflight_NotFoundWhenLaunchFails(t *testing.T) {
	runner := &envtest.FakeRunner{} // no scripted response, no Default -> empty output, no error

	got := gentleai.CheckPreflight(context.Background(), runner)

	if got.Found {
		t.Fatalf("CheckPreflight() = %+v, want Found=false", got)
	}
}

func TestCheckPreflight_FoundButUnparseableVersion(t *testing.T) {
	// Mirrors the PS rule: any non-blank output means "found", even when
	// no MAJOR.MINOR.PATCH token is present.
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"gentle-ai --version": {Stdout: "not a version string\n"},
	}}

	got := gentleai.CheckPreflight(context.Background(), runner)

	if !got.Found || got.Version != "" || got.OK {
		t.Fatalf("CheckPreflight() = %+v, want Found=true, Version=\"\", OK=false", got)
	}
}

// ---------------------------------------------------------------------------
// PhaseAssignments — reads ~/.gentle-ai/state.json's claude_phase_assignments.
// ---------------------------------------------------------------------------

func TestPhaseAssignments_ReadsMap(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	if err := os.WriteFile(statePath, []byte(`{
  "claude_phase_assignments": {
    "jd-judge-b": { "model": "opus", "effort": "xhigh" }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := gentleai.PhaseAssignments(statePath)
	if err != nil {
		t.Fatalf("PhaseAssignments() error = %v", err)
	}
	want := map[string]config.PhaseAssignment{"jd-judge-b": {Model: "opus", Effort: "xhigh"}}
	if len(got) != len(want) || got["jd-judge-b"] != want["jd-judge-b"] {
		t.Fatalf("PhaseAssignments() = %+v, want %+v", got, want)
	}
}

func TestPhaseAssignments_MissingFileReturnsEmptyMapNoError(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "does-not-exist.json")

	got, err := gentleai.PhaseAssignments(statePath)
	if err != nil {
		t.Fatalf("PhaseAssignments() error = %v, want nil for a missing file", err)
	}
	if len(got) != 0 {
		t.Fatalf("PhaseAssignments() = %+v, want empty map", got)
	}
}

func TestPhaseAssignments_MalformedFileErrors(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	if err := os.WriteFile(statePath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := gentleai.PhaseAssignments(statePath)
	if err == nil {
		t.Fatal("PhaseAssignments() error = nil, want error for malformed JSON")
	}
}

// ---------------------------------------------------------------------------
// Prerequisites — the -Print `prerequisites` shape (gentle_ai, engram,
// claude). Mirrors Get-NervPrerequisitesStatus.
// ---------------------------------------------------------------------------

func TestPrerequisites_ComputesAllThreeTools(t *testing.T) {
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"gentle-ai --version": {Stdout: "gentle-ai 3.7.0\n"},
	}}
	lookPath := func(name string) (string, error) {
		if name == "engram" {
			return "/usr/bin/engram", nil
		}
		return "", os.ErrNotExist // claude: not found
	}

	got := gentleai.Prerequisites(context.Background(), runner, lookPath)

	if !got.GentleAI.Found || got.GentleAI.Version != "3.7.0" || !got.GentleAI.OK {
		t.Fatalf("Prerequisites().GentleAI = %+v, unexpected", got.GentleAI)
	}
	if !got.Engram.Found {
		t.Fatal("Prerequisites().Engram.Found = false, want true")
	}
	if got.Claude.Found {
		t.Fatal("Prerequisites().Claude.Found = true, want false")
	}
}
