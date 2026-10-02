package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
)

// These cases port the 10 CLI cases install-skills.test.ps1's case groups
// E and F left for P2 (dry-run output lines, --only tdd exactly one line,
// --json array shapes); the forwarder cases (root tools/install-skills.ps1)
// are N/A now that there is no separate forwarder script.

func TestRunSkills_DryRun_FreshDir_PrintsNineInstallLinesAndExitsZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"skills", "--dry-run"}, &stdout, &stderr, testOptions(home))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	installLines := 0
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.Contains(line, "npx skills add") {
			installLines++
		}
	}
	if installLines != 9 {
		t.Errorf("install lines = %d, want 9; stdout:\n%s", installLines, stdout.String())
	}
}

func TestRunSkills_DryRun_OnlyTDD_PrintsExactlyOneLineAndExitsZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"skills", "--only", "tdd", "--dry-run"}, &stdout, &stderr, testOptions(home))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	installLines := 0
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.Contains(line, "npx skills add") {
			installLines++
		}
	}
	if installLines != 1 {
		t.Errorf("install lines = %d, want 1; stdout:\n%s", installLines, stdout.String())
	}
}

func TestRunSkills_OnlyUnknownName_Exits1AndNamesIt(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"skills", "--only", "no-such-skill", "--dry-run"}, &stdout, &stderr, testOptions(home))

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "no-such-skill") {
		t.Errorf("stdout = %q, want it to name the unknown skill", stdout.String())
	}
	if strings.Contains(stdout.String(), "panic") {
		t.Errorf("stdout = %q, want a clean refusal, not a crash", stdout.String())
	}
}

func TestRunSkills_JSON_SingleEntryIsArray(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"skills", "--only", "tdd", "--json", "--dry-run"}, &stdout, &stderr, testOptions(home))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	trimmed := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(trimmed, "[") {
		t.Errorf("stdout = %q, want it to start with '['", trimmed)
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		t.Fatalf("stdout is not a JSON array: %v", err)
	}
	if len(decoded) != 1 {
		t.Errorf("len(decoded) = %d, want 1", len(decoded))
	}
}

func TestRunSkills_JSON_FullManifestHasAtLeast15EntriesAndParses(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"skills", "--json", "--dry-run"}, &stdout, &stderr, testOptions(home))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	var decoded []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (stdout=%q)", err, stdout.String())
	}
	if len(decoded) < 15 {
		t.Errorf("len(decoded) = %d, want >= 15", len(decoded))
	}
}

// ---------------------------------------------------------------------------
// P3.1.3: "--home" is no longer a public flag on "nerv skills" — same
// rationale as configure's own TestRunConfigure_HomeFlag_NoLongerAccepted.
// ---------------------------------------------------------------------------

func TestRunSkills_HomeFlag_NoLongerAccepted(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"skills", "--home", home, "--dry-run"}, &stdout, &stderr, testOptions(home))

	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (a rejected unknown flag); stdout=%q", code, stdout.String())
	}
}

func TestRunSkills_DryRun_PrintsOneGentleAISyncLine(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"skills", "--dry-run"}, &stdout, &stderr, testOptions(home))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	syncLines := 0
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.HasPrefix(line, "DryRun: gentle-ai sync --agents claude-code --skills ") {
			syncLines++
		}
	}
	if syncLines != 1 {
		t.Errorf("sync lines = %d, want 1; stdout:\n%s", syncLines, stdout.String())
	}
}

func TestRunSkills_Run_PrintsTheGentleAISyncLine(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"skills"}, &stdout, &stderr, testOptions(home))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "\n-> gentle-ai sync --agents claude-code --skills ") {
		t.Errorf("stdout lacks the sync line:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "Warning: gentle-ai sync failed") {
		t.Errorf("stdout has a sync failure warning for a successful sync:\n%s", stdout.String())
	}
}

func TestRunSkills_FailedSync_PrintsWarningWithDiagnostic(t *testing.T) {
	cases := map[string]envtest.Response{
		"non-zero exit": {ExitCode: 1, Stderr: "boom: no such agent\n"},
		"launch error":  {Err: errors.New("boom: no such agent")},
	}
	for name, resp := range cases {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			home := t.TempDir()
			opts := testOptions(home)
			opts.Runner = &envtest.FakeRunner{Responses: map[string]envtest.Response{"gentle-ai": resp}}

			code := run([]string{"skills", "--only", "work-unit-commits"}, &stdout, &stderr, opts)

			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
			}
			warning := "Warning: gentle-ai sync failed; the gentle-ai skills may still be missing.\n  boom: no such agent\n"
			if !strings.Contains(stdout.String(), warning) {
				t.Errorf("stdout lacks the warning with its diagnostic:\n%s", stdout.String())
			}
		})
	}
}
