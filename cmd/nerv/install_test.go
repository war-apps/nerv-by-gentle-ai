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

	code := run([]string{"install", "--home", home, "--no-skills", "--no-configure"}, &stdout, &stderr, opts)

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

	code := run([]string{"install", "--home", home, "--require-gentle-ai", "--no-skills", "--no-configure"}, &stdout, &stderr, opts)

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

	code := run([]string{"uninstall", "--home", home}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
}

func TestRunApplyModels_MissingCache_Exits0WithWarning(t *testing.T) {
	home := t.TempDir()
	var stdout, stderr bytes.Buffer

	code := run([]string{"apply-models", "--home", home}, &stdout, &stderr, testOptions(home))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "not found") {
		t.Errorf("expected a warning about the missing cache dir, got:\n%s", stdout.String())
	}
}
