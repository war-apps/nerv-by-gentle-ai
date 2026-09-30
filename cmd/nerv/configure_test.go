package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	nerv "github.com/war-apps/nerv-gentle-ai"
	"github.com/war-apps/nerv-gentle-ai/internal/env/envtest"
)

func testOptions(home string) options {
	return options{
		PluginFS: nerv.PluginFS(),
		Runner:   &envtest.FakeRunner{},
		Now:      func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) },
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}
}

// ---------------------------------------------------------------------------
// No mode flag: the interactive wizard is not available yet in this build.
// ---------------------------------------------------------------------------

func TestRunConfigure_NoModeFlag_PrintsNoticeAndExits1(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"configure", "--home", home}, &stdout, &stderr, testOptions(home))

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stdout.Len() == 0 {
		t.Error("expected a notice on stdout")
	}
}

// ---------------------------------------------------------------------------
// Mutually exclusive modes -> usage error, exit 2.
// ---------------------------------------------------------------------------

func TestRunConfigure_MultipleModes_UsageErrorExit2(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"configure", "--home", home, "--print", "--install-commands"}, &stdout, &stderr, testOptions(home))

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stderr.Len() == 0 {
		t.Error("expected usage text on stderr")
	}
}

// ---------------------------------------------------------------------------
// --print always emits JSON, regardless of --json.
// ---------------------------------------------------------------------------

func TestRunConfigure_Print_EmitsJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"configure", "--home", home, "--print"}, &stdout, &stderr, testOptions(home))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (stdout=%q)", err, stdout.String())
	}
	if _, ok := decoded["config_path"]; !ok {
		t.Error("missing config_path in JSON output")
	}
}

// ---------------------------------------------------------------------------
// --set happy path, then a repeated call with the same value is a no-op —
// changed true/backup set, then changed false/backup null.
// ---------------------------------------------------------------------------

func TestRunConfigure_Set_HappyPathThenNoop(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".claude", "nerv", "nerv.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("enabled: true\ngit:\n  worktree: ask\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout1, stderr1 bytes.Buffer
	code := run([]string{"configure", "--home", home, "--set", "git.worktree=always", "--json"}, &stdout1, &stderr1, testOptions(home))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr1.String())
	}
	var first struct {
		Changed bool    `json:"changed"`
		Backup  *string `json:"backup"`
	}
	if err := json.Unmarshal(stdout1.Bytes(), &first); err != nil {
		t.Fatalf("stdout1 not valid JSON: %v (%q)", err, stdout1.String())
	}
	if !first.Changed {
		t.Error("first call: changed = false, want true")
	}
	if first.Backup == nil {
		t.Error("first call: backup = nil, want a path")
	}

	var stdout2, stderr2 bytes.Buffer
	code = run([]string{"configure", "--home", home, "--set", "git.worktree=always", "--json"}, &stdout2, &stderr2, testOptions(home))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr2.String())
	}
	var second struct {
		Changed bool    `json:"changed"`
		Backup  *string `json:"backup"`
	}
	if err := json.Unmarshal(stdout2.Bytes(), &second); err != nil {
		t.Fatalf("stdout2 not valid JSON: %v (%q)", err, stdout2.String())
	}
	if second.Changed {
		t.Error("second call: changed = true, want false")
	}
	if second.Backup != nil {
		t.Errorf("second call: backup = %v, want nil", *second.Backup)
	}
}

// ---------------------------------------------------------------------------
// --set with an unknown key: exit 1, message on stdout.
// ---------------------------------------------------------------------------

func TestRunConfigure_Set_UnknownKeyExit1(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"configure", "--home", home, "--set", "bogus.key=1"}, &stdout, &stderr, testOptions(home))

	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stdout=%q)", code, stdout.String())
	}
	if stdout.Len() == 0 {
		t.Error("expected a rejection message on stdout")
	}
}

// ---------------------------------------------------------------------------
// --home resolution failure (no override, HOME/USERPROFILE unset): exit 2.
// ---------------------------------------------------------------------------

func TestRunConfigure_HomeResolutionFailure_Exit2(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	var stdout, stderr bytes.Buffer
	code := run([]string{"configure", "--print"}, &stdout, &stderr, testOptions(""))

	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (stdout=%q)", code, stdout.String())
	}
}
