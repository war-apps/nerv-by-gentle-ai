package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
)

func baseInstallRunner() *envtest.FakeRunner {
	return &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"gentle-ai --version":               {Stdout: "gentle-ai version 4.0.0\n", ExitCode: 0},
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

// ---------------------------------------------------------------------------
// The skills step asks before the gentle-ai sync, only in a terminal.
// ---------------------------------------------------------------------------

func TestRunInstall_SkillsStep_Terminal_ConfirmYes_RunsSync(t *testing.T) {
	home := t.TempDir()
	seedInstalledPluginsForCLI(t, home)
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	runner := baseInstallRunner()
	opts.Runner = runner
	opts.IsTerminal = func() bool { return true }
	opts.Stdin = strings.NewReader("y\n")

	code := run([]string{"install", "--no-configure"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if n := countSyncCalls(runner); n != 1 {
		t.Errorf("sync ran %d times, want once; stdout:\n%s", n, stdout.String())
	}
	if !strings.Contains(stdout.String(), "managed files") {
		t.Errorf("stdout lacks the prompt warning about managed files:\n%s", stdout.String())
	}
}

func TestRunInstall_SkillsStep_Terminal_ConfirmNo_SkipsSync(t *testing.T) {
	home := t.TempDir()
	seedInstalledPluginsForCLI(t, home)
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	runner := baseInstallRunner()
	opts.Runner = runner
	opts.IsTerminal = func() bool { return true }
	opts.Stdin = strings.NewReader("\n")

	code := run([]string{"install", "--no-configure"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if n := countSyncCalls(runner); n != 0 {
		t.Errorf("sync ran %d times, want 0", n)
	}
	if !strings.Contains(stdout.String(), "gentle-ai sync skipped") || !strings.Contains(stdout.String(), "Remedy: run 'gentle-ai install'") {
		t.Errorf("stdout lacks the skipped line or the remedy:\n%s", stdout.String())
	}
}

func TestRunInstall_SkillsStep_NotATerminal_NoPromptNoSync(t *testing.T) {
	home := t.TempDir()
	seedInstalledPluginsForCLI(t, home)
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	runner := baseInstallRunner()
	opts.Runner = runner

	code := run([]string{"install", "--no-configure"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if n := countSyncCalls(runner); n != 0 {
		t.Errorf("sync ran %d times without a terminal, want 0", n)
	}
	if strings.Contains(stdout.String(), "managed files") || strings.Contains(stdout.String(), "gentle-ai sync skipped") {
		t.Errorf("stdout has a prompt or skipped line without a terminal:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Remedy: run 'gentle-ai install'") {
		t.Errorf("stdout lacks the remedy:\n%s", stdout.String())
	}
}

// seedExtensionsForCLI records task@nerv and task-teamwork@nerv as cached
// at the embedded version, next to nerv@nerv.
func seedExtensionsForCLI(t *testing.T, home string) {
	t.Helper()
	seedInstalledPluginsForCLI(t, home)
	pv, err := version.PluginVersion(nerv.PluginFS())
	if err != nil {
		t.Fatal(err)
	}
	content := `{"plugins":{"nerv@nerv":[{"installPath":"/c/nerv","version":"` + pv + `"}],` +
		`"task@nerv":[{"installPath":"/c/task","version":"` + pv + `"}],` +
		`"task-teamwork@nerv":[{"installPath":"/c/tw","version":"` + pv + `"}]}}`
	if err := os.WriteFile(filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// seedReleaseFamilyForCLI records every embedded extension as cached at the
// embedded version, next to nerv@nerv.
func seedReleaseFamilyForCLI(t *testing.T, home string) {
	t.Helper()
	seedInstalledPluginsForCLI(t, home)
	pv, err := version.PluginVersion(nerv.PluginFS())
	if err != nil {
		t.Fatal(err)
	}
	content := `{"plugins":{"nerv@nerv":[{"installPath":"/c/nerv","version":"` + pv + `"}]`
	for _, name := range []string{"task", "task-teamwork", "release", "release-conventional-commits", "release-docker-compose", "release-bash", "deploy", "deploy-bash", "deploy-github", "deploy-ssh-compose"} {
		content += `,"` + name + `@nerv":[{"installPath":"/c/` + name + `","version":"` + pv + `"}]`
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), []byte(content+"}}"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunInstall_WithFlag_InstallsTheNamedExtensions(t *testing.T) {
	home := t.TempDir()
	seedExtensionsForCLI(t, home)
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	opts.ExtensionsFS = nerv.ExtensionsFS()
	runner := baseInstallRunner()
	opts.Runner = runner

	code := run([]string{"install", "--no-skills", "--no-configure", "--with", "task"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "-> claude plugin install task@nerv") {
		t.Errorf("task@nerv not installed:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "task-teamwork@nerv") {
		t.Errorf("a provider was installed without being named or confirmed:\n%s", stdout.String())
	}
}

func TestRunInstall_Terminal_SuggestsExtensions(t *testing.T) {
	home := t.TempDir()
	seedExtensionsForCLI(t, home)
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	opts.ExtensionsFS = nerv.ExtensionsFS()
	opts.IsTerminal = func() bool { return true }
	opts.Runner = baseInstallRunner()
	opts.Stdin = strings.NewReader("y\nn\n")

	code := run([]string{"install", "--no-skills", "--no-configure"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Install task@nerv") || !strings.Contains(out, "Install task-teamwork@nerv") {
		t.Errorf("expected both suggestions, got:\n%s", out)
	}
	if !strings.Contains(out, "-> claude plugin install task@nerv") || strings.Contains(out, "-> claude plugin install task-teamwork@nerv") {
		t.Errorf("expected only task@nerv installed, got:\n%s", out)
	}
}

func TestRunInstall_WithUnknownExtension_Exits1(t *testing.T) {
	home := t.TempDir()
	seedInstalledPluginsForCLI(t, home)
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	opts.ExtensionsFS = nerv.ExtensionsFS()
	opts.Runner = baseInstallRunner()

	code := run([]string{"install", "--no-skills", "--no-configure", "--with", "task-jira"}, &stdout, &stderr, opts)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stdout:\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"task-jira"`) {
		t.Errorf("expected the unknown name reported, got:\n%s", stdout.String())
	}
}

// S7: on a terminal, nerv install asks before deleting the old /task:*
// copies and deletes them on yes; elsewhere it only reports them.
func TestRunInstall_LegacyCommands_AskOnTerminalReportOtherwise(t *testing.T) {
	for _, terminal := range []bool{true, false} {
		home := t.TempDir()
		seedInstalledPluginsForCLI(t, home)
		dir := filepath.Join(home, ".claude", "commands", "task")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		copyPath := filepath.Join(dir, "start.md")
		if err := os.WriteFile(copyPath, []byte("old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		opts := testOptions(home)
		opts.Runner = baseInstallRunner()
		opts.IsTerminal = func() bool { return terminal }
		opts.Stdin = strings.NewReader("y\n")

		var stdout, stderr bytes.Buffer
		if code := run([]string{"install", "--no-skills", "--no-configure"}, &stdout, &stderr, opts); code != 0 {
			t.Fatalf("terminal=%v: exit code = %d, want 0; stdout:\n%s", terminal, code, stdout.String())
		}
		_, statErr := os.Stat(copyPath)
		asked := strings.Contains(stdout.String(), "Remove these 1 old copies? [y/N]")
		if terminal && (!asked || !os.IsNotExist(statErr)) {
			t.Errorf("terminal: asked=%v, copy still there=%v; stdout:\n%s", asked, statErr == nil, stdout.String())
		}
		if !terminal && (asked || statErr != nil || !strings.Contains(stdout.String(), "can be removed")) {
			t.Errorf("not a terminal: asked=%v, stat=%v; stdout:\n%s", asked, statErr, stdout.String())
		}
	}
}

// L21: nerv install --help describes the whole suggestion order and the
// requirements.
func TestInstall_HelpDescribesTheSuggestionOrderAndRequirements(t *testing.T) {
	var stdout, stderr bytes.Buffer
	run([]string{"install", "--help"}, &stdout, &stderr, testOptions(t.TempDir()))
	help := stdout.String() + stderr.String()
	order := []string{"task@nerv", "task-<name>@nerv", "release@nerv", "deploy@nerv", "deploy-<name>@nerv"}
	at := 0
	for _, want := range order {
		i := strings.Index(help[at:], want)
		if i < 0 {
			t.Fatalf("help lacks %q after position %d (order %q):\n%s", want, at, order, help)
		}
		at += i + len(want)
	}
	for _, want := range []string{"deploy requires release", "a task provider requires task", "a deploy provider requires deploy"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q:\n%s", want, help)
		}
	}
}
