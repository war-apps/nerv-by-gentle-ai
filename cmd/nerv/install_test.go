package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-gentle-ai"
	"github.com/war-apps/nerv-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-gentle-ai/internal/version"
)

func baseInstallRunner() *envtest.FakeRunner {
	return &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"gentle-ai --version":               {Stdout: "gentle-ai version 3.7.0\n", ExitCode: 0},
			"claude plugin uninstall nerv@nerv": {Stdout: "uninstalled\n", ExitCode: 0},
			"claude plugin install nerv@nerv":   {Stdout: "installed\n", ExitCode: 0},
			"engram projects list":              {Stdout: "nerv\n", ExitCode: 0},
		},
	}
}

func seedInstalledPluginsForCLI(t *testing.T, home string) {
	t.Helper()
	pv, err := version.PluginVersion(nerv.PluginFS())
	if err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(home, ".claude", "plugins", "cache", "nerv", "nerv", pv, "agents")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	installedPath := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	if err := os.MkdirAll(filepath.Dir(installedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"plugins":{"nerv@nerv":[{"installPath":"` + filepath.ToSlash(cacheDir) + `","version":"` + pv + `"}]}}`
	if err := os.WriteFile(installedPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunInstall_HappyPath_Exits0(t *testing.T) {
	home := t.TempDir()
	seedInstalledPluginsForCLI(t, home)
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	opts.Runner = baseInstallRunner()

	code := run([]string{"install", "--no-skills", "--no-configure"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Restart Claude Code") {
		t.Error("expected the restart hint on stdout")
	}
}

func TestRunInstall_RequireGentleAIMissing_Exits1(t *testing.T) {
	home := t.TempDir()
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	opts.Runner = &envtest.FakeRunner{Default: envtest.Response{Err: errors.New("not found")}}

	code := run([]string{"install", "--require-gentle-ai", "--no-skills", "--no-configure"}, &stdout, &stderr, opts)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stdout:\n%s", code, stdout.String())
	}
}

func TestRunUninstall_HappyPath_Exits0(t *testing.T) {
	home := t.TempDir()
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	opts.Runner = &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"claude plugin uninstall nerv@nerv": {Stderr: "not installed\n", ExitCode: 1},
		},
	}

	code := run([]string{"uninstall"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
}

// ---------------------------------------------------------------------------
// nerv install ends by running the wizard unless --no-configure or stdin is
// not a terminal.
// ---------------------------------------------------------------------------

func TestRunInstall_Terminal_NoNoConfigure_RunsWizard(t *testing.T) {
	home := t.TempDir()
	seedInstalledPluginsForCLI(t, home)
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	opts.IsTerminal = func() bool { return true }
	opts.Runner = baseInstallRunner()
	// Real blank lines (not an exhausted reader — see wizard.ErrInputClosed)
	// for every prompt the full wizard asks, so every value keeps its
	// default/no-op answer instead of the wizard aborting on EOF.
	opts.Stdin = strings.NewReader(strings.Repeat("\n", 60))

	code := run([]string{"install", "--no-skills"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "NERV Setup Wizard") {
		t.Errorf("expected the wizard to run, got:\n%s", stdout.String())
	}
}

func TestRunInstall_NotATerminal_NoNoConfigure_PrintsHintNotWizard(t *testing.T) {
	home := t.TempDir()
	seedInstalledPluginsForCLI(t, home)
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	opts.Runner = baseInstallRunner()

	code := run([]string{"install", "--no-skills"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "NERV Setup Wizard") {
		t.Error("expected the wizard NOT to run when stdin is not a terminal")
	}
	if !strings.Contains(stdout.String(), "Run `nerv configure`") {
		t.Errorf("expected the configure hint, got:\n%s", stdout.String())
	}
}

func TestRunInstall_NoConfigure_SkipsHintAndWizard(t *testing.T) {
	home := t.TempDir()
	seedInstalledPluginsForCLI(t, home)
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	opts.IsTerminal = func() bool { return true }
	opts.Runner = baseInstallRunner()

	code := run([]string{"install", "--no-skills", "--no-configure"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "NERV Setup Wizard") || strings.Contains(stdout.String(), "nerv configure") {
		t.Errorf("expected neither the wizard nor the hint, got:\n%s", stdout.String())
	}
}

func TestRunApplyModels_MissingCache_Exits0WithWarning(t *testing.T) {
	home := t.TempDir()
	var stdout, stderr bytes.Buffer

	code := run([]string{"apply-models"}, &stdout, &stderr, testOptions(home))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "not found") {
		t.Errorf("expected a warning about the missing cache dir, got:\n%s", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// P3.1.3: "--home"/"--settings" are no longer public flags on install,
// uninstall, or apply-models — same rationale as configure's own
// TestRunConfigure_HomeFlag_NoLongerAccepted.
// ---------------------------------------------------------------------------

func TestRunInstall_HomeAndSettingsFlags_NoLongerAccepted(t *testing.T) {
	home := t.TempDir()
	var stdout, stderr bytes.Buffer

	code := run([]string{"install", "--home", home, "--no-skills", "--no-configure"}, &stdout, &stderr, testOptions(home))

	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (a rejected unknown flag); stdout=%q", code, stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"install", "--settings", filepath.Join(home, "settings.json"), "--no-skills", "--no-configure"}, &stdout, &stderr, testOptions(home))

	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (a rejected unknown flag); stdout=%q", code, stdout.String())
	}
}

func TestRunUninstall_HomeFlag_NoLongerAccepted(t *testing.T) {
	home := t.TempDir()
	var stdout, stderr bytes.Buffer

	code := run([]string{"uninstall", "--home", home}, &stdout, &stderr, testOptions(home))

	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (a rejected unknown flag); stdout=%q", code, stdout.String())
	}
}

func TestRunApplyModels_HomeFlag_NoLongerAccepted(t *testing.T) {
	home := t.TempDir()
	var stdout, stderr bytes.Buffer

	code := run([]string{"apply-models", "--home", home}, &stdout, &stderr, testOptions(home))

	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (a rejected unknown flag); stdout=%q", code, stdout.String())
	}
}
