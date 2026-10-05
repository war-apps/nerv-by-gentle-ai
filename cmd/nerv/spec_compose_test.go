package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	composeCanonical = "## Requirements\n\n" +
		"### Requirement: Unrelated Listing\n\nThe system MUST list widgets.\n\n" +
		"### Requirement: Widget Expiration\n\nThe system MUST expire widgets after 30 days.\n"
	composeDelta = "## ADDED Requirements\n\n" +
		"### Requirement: Widget Tagging\n\nThe system MUST support tagging widgets.\n\n" +
		"## MODIFIED Requirements\n\n### Requirement: Widget Expiration\n\nThe system MUST expire widgets after 90 days.\n"
	composeBadDelta = "## MODIFIED Requirements\n\n### Requirement: Missing\n\nNew body.\n"
)

func writeSpecFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func invokeSpecCompose(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(append([]string{"spec-compose"}, args...), &out, &errOut, testOptions(t.TempDir()))
	return code, out.String(), errOut.String()
}

func TestRunSpecCompose_DefaultsToStdoutAndAppliesTheDelta(t *testing.T) {
	root := t.TempDir()
	canonical, delta := filepath.Join(root, "spec.md"), filepath.Join(root, "delta.md")
	writeSpecFixture(t, canonical, composeCanonical)
	writeSpecFixture(t, delta, composeDelta)

	code, stdout, stderr := invokeSpecCompose(t, "--canonical", canonical, "--delta", delta)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr)
	}
	for _, want := range []string{"### Requirement: Unrelated Listing", "expire widgets after 90 days", "### Requirement: Widget Tagging"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "after 30 days") {
		t.Errorf("stdout still has the pre-delta text:\n%s", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	if got, _ := os.ReadFile(canonical); string(got) != composeCanonical {
		t.Errorf("canonical was modified without --output: %q", got)
	}
}

func TestRunSpecCompose_OutputDashIsStdout(t *testing.T) {
	root := t.TempDir()
	canonical, delta := filepath.Join(root, "spec.md"), filepath.Join(root, "delta.md")
	writeSpecFixture(t, canonical, composeCanonical)
	writeSpecFixture(t, delta, composeDelta)

	code, stdout, _ := invokeSpecCompose(t, "--canonical", canonical, "--delta", delta, "--output", "-")

	if code != 0 || !strings.Contains(stdout, "Widget Tagging") {
		t.Fatalf("exit code = %d, stdout = %q; want 0 and the composed spec", code, stdout)
	}
}

func TestRunSpecCompose_OutputInPlaceKeepsTheFileMode(t *testing.T) {
	root := t.TempDir()
	canonical, delta := filepath.Join(root, "spec.md"), filepath.Join(root, "delta.md")
	writeSpecFixture(t, canonical, composeCanonical)
	writeSpecFixture(t, delta, composeDelta)
	if err := os.Chmod(canonical, 0o640); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := invokeSpecCompose(t, "--canonical", canonical, "--delta", delta, "--output", canonical)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty when --output is a file", stdout)
	}
	got, _ := os.ReadFile(canonical)
	if !strings.Contains(string(got), "Widget Tagging") || !strings.HasSuffix(string(got), "\n") {
		t.Errorf("canonical was not composed in place:\n%s", got)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want the original 0640", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 2 {
		t.Errorf("directory has %d entries, want canonical and delta only: %v", len(entries), entries)
	}
}

func TestRunSpecCompose_UnappliedDeltaExits1NamingSectionAndRequirementWritingNothing(t *testing.T) {
	root := t.TempDir()
	canonical, delta := filepath.Join(root, "spec.md"), filepath.Join(root, "delta.md")
	fresh := filepath.Join(root, "fresh.md")
	writeSpecFixture(t, canonical, composeCanonical)
	writeSpecFixture(t, delta, composeBadDelta)

	// Into stdout, into a new file, and over the canonical itself.
	for _, output := range []string{"-", fresh, canonical} {
		code, stdout, stderr := invokeSpecCompose(t, "--canonical", canonical, "--delta", delta, "--output", output)

		if code != 1 {
			t.Errorf("--output %s: exit code = %d, want 1", output, code)
		}
		if !strings.Contains(stderr, "MODIFIED") || !strings.Contains(stderr, "Missing") {
			t.Errorf("--output %s: stderr = %q, want it to name MODIFIED and %q", output, stderr, "Missing")
		}
		if stdout != "" {
			t.Errorf("--output %s: stdout = %q, want empty on failure", output, stdout)
		}
	}
	if _, err := os.Stat(fresh); !os.IsNotExist(err) {
		t.Errorf("output file was created despite the refusal: err = %v", err)
	}
	if got, _ := os.ReadFile(canonical); string(got) != composeCanonical {
		t.Errorf("canonical changed despite the refusal: %q", got)
	}
}

func TestRunSpecCompose_EmptyCanonicalExits1(t *testing.T) {
	root := t.TempDir()
	canonical, delta := filepath.Join(root, "spec.md"), filepath.Join(root, "delta.md")
	writeSpecFixture(t, canonical, "# No requirements yet\n")
	writeSpecFixture(t, delta, composeDelta)

	code, _, stderr := invokeSpecCompose(t, "--canonical", canonical, "--delta", delta)

	if code != 1 || !strings.Contains(stderr, "CANONICAL") {
		t.Fatalf("exit code = %d, stderr = %q; want 1 naming CANONICAL", code, stderr)
	}
}

func TestRunSpecCompose_MissingInputFileExits1(t *testing.T) {
	root := t.TempDir()
	delta := filepath.Join(root, "delta.md")
	writeSpecFixture(t, delta, composeDelta)

	code, _, stderr := invokeSpecCompose(t, "--canonical", filepath.Join(root, "absent.md"), "--delta", delta)

	if code != 1 || !strings.Contains(stderr, "canonical") {
		t.Fatalf("exit code = %d, stderr = %q; want 1 mentioning the canonical spec", code, stderr)
	}
}

func TestRunSpecCompose_UnwritableOutputExits2(t *testing.T) {
	root := t.TempDir()
	canonical, delta := filepath.Join(root, "spec.md"), filepath.Join(root, "delta.md")
	writeSpecFixture(t, canonical, composeCanonical)
	writeSpecFixture(t, delta, composeDelta)
	// A non-empty directory at the output path cannot be replaced by a file.
	out := filepath.Join(root, "out")
	if err := os.MkdirAll(filepath.Join(out, "keep"), 0o755); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := invokeSpecCompose(t, "--canonical", canonical, "--delta", delta, "--output", out)

	if code != 2 || stderr == "" {
		t.Fatalf("exit code = %d, stderr = %q; want 2 with a message", code, stderr)
	}
}

func TestRunSpecCompose_UsageErrorsExit2(t *testing.T) {
	cases := map[string][]string{
		"no flags":            {},
		"missing --delta":     {"--canonical", "a.md"},
		"missing --canonical": {"--delta", "b.md"},
		"positional argument": {"--canonical", "a.md", "--delta", "b.md", "extra"},
		"unknown flag":        {"--canonical", "a.md", "--delta", "b.md", "--bogus"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := invokeSpecCompose(t, args...)
			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if stdout != "" || !strings.Contains(stderr, "Usage: nerv spec-compose") {
				t.Errorf("stdout = %q, stderr = %q; want usage on stderr only", stdout, stderr)
			}
		})
	}
}

func TestRunSpecCompose_HelpPrintsUsageToStdoutAndExits0(t *testing.T) {
	code, stdout, stderr := invokeSpecCompose(t, "--help")

	if code != 0 || stderr != "" {
		t.Fatalf("exit code = %d, stderr = %q; want 0 and empty stderr", code, stderr)
	}
	for _, want := range []string{"Usage: nerv spec-compose", "--canonical", "--delta", "--output"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help missing %q:\n%s", want, stdout)
		}
	}
}

func TestRun_UsageListsSpecCompose(t *testing.T) {
	var stdout, stderr bytes.Buffer

	run(nil, &stdout, &stderr, testOptions(t.TempDir()))

	if !strings.Contains(stdout.String(), "spec-compose") {
		t.Errorf("usage does not list spec-compose:\n%s", stdout.String())
	}
}
