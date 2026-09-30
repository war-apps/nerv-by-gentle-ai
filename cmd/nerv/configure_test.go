package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	nerv "github.com/war-apps/nerv-gentle-ai"
	"github.com/war-apps/nerv-gentle-ai/internal/env/envtest"
)

func testOptions(home string) options {
	return options{
		PluginFS:   nerv.PluginFS(),
		Runner:     &envtest.FakeRunner{},
		Now:        func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) },
		LookPath:   func(string) (string, error) { return "", os.ErrNotExist },
		Stdin:      strings.NewReader(""),
		Home:       home,
		IsTerminal: func() bool { return false },
	}
}

// ---------------------------------------------------------------------------
// No mode flag, no --answers, stdin not a terminal: refusal notice, exit 1.
// ---------------------------------------------------------------------------

func TestRunConfigure_NoModeFlag_NotATerminal_PrintsNoticeAndExits1(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"configure"}, &stdout, &stderr, testOptions(home))

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "stdin is not a terminal") {
		t.Errorf("expected the not-a-terminal notice, got: %q", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// No mode flag, stdin IS a terminal: the wizard runs interactively.
// ---------------------------------------------------------------------------

func TestRunConfigure_NoModeFlag_Terminal_RunsWizard(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()
	opts := testOptions(home)
	opts.IsTerminal = func() bool { return true }
	// Real blank lines (not an exhausted reader — see wizard.ErrInputClosed)
	// for every user-config prompt, so every value keeps its current
	// default and the wizard completes instead of aborting.
	opts.Stdin = strings.NewReader(strings.Repeat("\n", 27))

	code := run([]string{
		"configure",
		"--skip-skills", "--skip-models", "--skip-repos", "--skip-commands", "--no-refresh",
	}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q, stdout=%q)", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "NERV Setup Wizard") {
		t.Errorf("expected the wizard banner, got: %q", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// --answers <file> drives the wizard end to end, regardless of terminal-ness.
// ---------------------------------------------------------------------------

func TestRunConfigure_Answers_DrivesWizard(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".claude", "nerv", "nerv.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("enabled: true\ngit:\n  base_branch: develop\n  worktree: ask\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	answersPath := filepath.Join(home, "answers.txt")
	answers := "release\n" + strings.Repeat("\n", 30)
	if err := os.WriteFile(answersPath, []byte(answers), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"configure", "--answers", answersPath,
		"--skip-skills", "--skip-models", "--skip-repos", "--skip-commands", "--no-refresh",
	}, &stdout, &stderr, testOptions(home))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q, stdout=%q)", code, stderr.String(), stdout.String())
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "base_branch: release") {
		t.Errorf("expected base_branch updated, got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// --answers pointing at a missing file behaves like an empty answers file:
// openAnswersFile still treats "file not found" as "no answers" rather
// than an error, but an empty answers file is now genuine EOF at the
// first prompt (P3.1.2), so the wizard aborts with exit 1 instead of
// silently keeping every default.
// ---------------------------------------------------------------------------

func TestRunConfigure_Answers_MissingFileActsEmpty(t *testing.T) {
	home := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"configure", "--answers", filepath.Join(home, "nonexistent-answers.txt"),
		"--skip-skills", "--skip-models", "--skip-repos", "--skip-commands", "--no-refresh",
	}, &stdout, &stderr, testOptions(home))

	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr=%q, stdout=%q)", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "input ended before the wizard finished; nothing was written") {
		t.Errorf("expected the ErrInputClosed notice, got: %q", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// Mutually exclusive modes -> usage error, exit 2.
// ---------------------------------------------------------------------------

func TestRunConfigure_MultipleModes_UsageErrorExit2(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"configure", "--print", "--install-commands"}, &stdout, &stderr, testOptions(home))

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

	code := run([]string{"configure", "--print"}, &stdout, &stderr, testOptions(home))

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
	code := run([]string{"configure", "--set", "git.worktree=always", "--json"}, &stdout1, &stderr1, testOptions(home))
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
	code = run([]string{"configure", "--set", "git.worktree=always", "--json"}, &stdout2, &stderr2, testOptions(home))
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

	code := run([]string{"configure", "--set", "bogus.key=1"}, &stdout, &stderr, testOptions(home))

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

// ---------------------------------------------------------------------------
// P3.1.3: "--home" is no longer a public flag on "nerv configure" — a fake
// home never sandboxes the global commands it can still reach (claude
// plugin, npx -g); the home directory comes only from options.Home
// (tests) or env.HomeDir() (production). Passing it is now an unknown
// flag, which fails the flag.ContinueOnError parse and exits 2.
// ---------------------------------------------------------------------------

func TestRunConfigure_HomeFlag_NoLongerAccepted(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"configure", "--home", home, "--print"}, &stdout, &stderr, testOptions(home))

	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (a rejected unknown flag); stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
