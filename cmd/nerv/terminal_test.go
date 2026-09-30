package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/war-apps/nerv-gentle-ai/internal/env/envtest"
)

// TestStdinIsTerminal_SeamFalse_RefusesWithoutTouchingRunner is fix
// P3.1.1: when the stdinIsTerminal seam reports false and no --answers
// file is given, "nerv configure" must refuse before doing anything else
// — in particular it must never call the process runner. The 2026-09-29
// incident ran `claude plugin uninstall/install` and nine `npx skills add
// -g` on the real machine because `nerv configure --home <tmp> </dev/null`
// on Windows git-bash misread the redirected NUL device as a terminal (a
// character device), so the wizard ran for real instead of refusing.
//
// The real golang.org/x/term.IsTerminal check itself is not exercised by
// this test: it depends on a real OS file descriptor/console, which a
// unit test cannot fake, so only the stdinIsTerminal seam's two outcomes
// are covered here and in TestStdinIsTerminal_SeamTrue_ProceedsToWizard.
func TestStdinIsTerminal_SeamFalse_RefusesWithoutTouchingRunner(t *testing.T) {
	home := t.TempDir()
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	runner := &envtest.FakeRunner{}
	opts.Runner = runner

	code := run([]string{"configure"}, &stdout, &stderr, opts)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stdout=%q)", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "stdin is not a terminal") {
		t.Errorf("expected the not-a-terminal notice, got: %q", stdout.String())
	}
	if len(runner.Calls) != 0 {
		t.Errorf("Calls = %+v, want none: a refused wizard must never touch the process runner", runner.Calls)
	}
}

// TestStdinIsTerminal_SeamTrue_ProceedsToWizard is the complementary case:
// when the seam reports true, "nerv configure" proceeds into the
// interactive wizard instead of refusing.
func TestStdinIsTerminal_SeamTrue_ProceedsToWizard(t *testing.T) {
	home := t.TempDir()
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	opts.IsTerminal = func() bool { return true }
	opts.Stdin = strings.NewReader(strings.Repeat("\n", 40))

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
