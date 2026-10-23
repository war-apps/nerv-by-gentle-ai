package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
)

func TestRunInstall_Terminal_DecliningTaskOffersNoProvider(t *testing.T) {
	home := t.TempDir()
	seedExtensionsForCLI(t, home)
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	opts.ExtensionsFS = nerv.ExtensionsFS()
	opts.IsTerminal = func() bool { return true }
	opts.Runner = baseInstallRunner()
	opts.Stdin = strings.NewReader("n\ny\n")

	code := run([]string{"install", "--no-skills", "--no-configure"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s", code, stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Install task@nerv") {
		t.Errorf("task@nerv was not offered:\n%s", out)
	}
	if strings.Contains(out, "Install task-teamwork@nerv") || strings.Contains(out, "claude plugin install task-teamwork@nerv") {
		t.Errorf("a provider was offered or installed after task was declined:\n%s", out)
	}
}

func TestRunInstall_WithProvider_BringsTaskPlugin(t *testing.T) {
	home := t.TempDir()
	seedExtensionsForCLI(t, home)
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	opts.ExtensionsFS = nerv.ExtensionsFS()
	opts.Runner = baseInstallRunner()

	code := run([]string{"install", "--no-skills", "--no-configure", "--with", "task-teamwork"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s", code, stdout.String())
	}
	out := stdout.String()
	for _, want := range []string{"Adding task (required by task-teamwork)", "-> claude plugin install task@nerv", "-> claude plugin install task-teamwork@nerv"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	settings, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if !strings.Contains(string(settings), `"task@nerv": true`) {
		t.Errorf("task@nerv not enabled:\n%s", settings)
	}
}

func TestRunInstall_WithFlag_TrimsBlanksAndDuplicates(t *testing.T) {
	for _, with := range []string{" task , ", "task,task"} {
		home := t.TempDir()
		seedExtensionsForCLI(t, home)
		var stdout, stderr bytes.Buffer
		opts := testOptions(home)
		opts.ExtensionsFS = nerv.ExtensionsFS()
		opts.Runner = baseInstallRunner()

		code := run([]string{"install", "--no-skills", "--no-configure", "--with", with}, &stdout, &stderr, opts)

		if code != 0 {
			t.Fatalf("--with %q: exit code = %d, want 0; stdout:\n%s", with, code, stdout.String())
		}
		if n := strings.Count(stdout.String(), "-> claude plugin install task@nerv"); n != 1 {
			t.Errorf("--with %q: task@nerv installed %d times, want 1:\n%s", with, n, stdout.String())
		}
	}
}

func TestRunInstall_ExtensionMissingFromInstalledPlugins_Exits1(t *testing.T) {
	home := t.TempDir()
	seedInstalledPluginsForCLI(t, home) // nerv@nerv only
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	opts.ExtensionsFS = nerv.ExtensionsFS()
	opts.Runner = baseInstallRunner()

	code := run([]string{"install", "--no-skills", "--no-configure", "--with", "task"}, &stdout, &stderr, opts)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stdout:\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "task@nerv not found in") {
		t.Errorf("refusal does not name task@nerv:\n%s", stdout.String())
	}
}

func TestRunUninstall_KeepsOtherMarketplacesPlugins(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"enabledPlugins":{"nerv@nerv":true,"task@nerv":true,"other@acme":true,"task@acme":true},` +
		`"extraKnownMarketplaces":{"nerv":{"source":{"source":"directory","path":"/x"}},"acme":{"source":{"source":"github","repo":"a/b"}}}}`
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	opts := testOptions(home)
	opts.ExtensionsFS = nerv.ExtensionsFS()
	runner := &envtest.FakeRunner{}
	opts.Runner = runner

	code := run([]string{"uninstall"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s", code, stdout.String())
	}
	settings, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	for _, keep := range []string{`"other@acme": true`, `"task@acme": true`, `"acme"`} {
		if !strings.Contains(string(settings), keep) {
			t.Errorf("settings lost %s:\n%s", keep, settings)
		}
	}
	if strings.Contains(string(settings), "@nerv") {
		t.Errorf("a @nerv id survived:\n%s", settings)
	}
	for _, c := range runner.Calls {
		if strings.Contains(strings.Join(c.Args, " "), "@acme") {
			t.Errorf("uninstall touched another marketplace: %v", c.Args)
		}
	}
}

func TestRunUninstall_RerunAfterExtensionFailure_StillUninstallsIt(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"enabledPlugins":{"nerv@nerv":true,"task@nerv":true},"extraKnownMarketplaces":{"nerv":{"source":{"source":"directory","path":"/x"}}}}`
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	uninstall := func(runner *envtest.FakeRunner) int {
		var stdout, stderr bytes.Buffer
		opts := testOptions(home)
		opts.ExtensionsFS = nerv.ExtensionsFS()
		opts.Runner = runner
		return run([]string{"uninstall"}, &stdout, &stderr, opts)
	}
	failing := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"claude plugin uninstall task@nerv": {Stderr: "boom", ExitCode: 3},
	}}
	if code := uninstall(failing); code == 0 {
		t.Fatal("first uninstall should fail")
	}

	rerun := &envtest.FakeRunner{}
	if code := uninstall(rerun); code != 0 {
		t.Fatalf("rerun exit code = %d, want 0", code)
	}
	for _, c := range rerun.Calls {
		if strings.Join(c.Args, " ") == "plugin uninstall task@nerv" {
			return
		}
	}
	t.Errorf("rerun never ran claude plugin uninstall task@nerv; calls = %v", rerun.Calls)
}

func TestReleaseApply_ExtensionWithoutVersion_WritesNothing(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.1.0")
	ext := filepath.Join(repo, "plugins", "task", ".claude-plugin", "plugin.json")
	if err := os.MkdirAll(filepath.Dir(ext), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ext, []byte("{\n  \"name\": \"task\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"): {Stdout: ""},
		gitLogKey(repo, ""):    {Stdout: commitRecord(3, "feat: add applied feature", "")},
	}}
	var stdout, stderr bytes.Buffer

	code := run([]string{"release", "apply", "--repo", repo}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))

	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero; stdout:\n%s", stdout.String())
	}
	if core := mustReadFile(t, filepath.Join(repo, "plugin", ".claude-plugin", "plugin.json")); !strings.Contains(core, `"version": "0.1.0"`) {
		t.Errorf("core plugin.json was bumped before the extension failed:\n%s", core)
	}
	if _, err := os.Stat(filepath.Join(repo, "CHANGELOG.md")); err == nil {
		t.Error("CHANGELOG.md was written before the extension failed")
	}
}

func TestReleaseApply_ListsExtensionsAfterChangelog(t *testing.T) {
	repo := releaseFixtureRepo(t, "0.1.0")
	ext := filepath.Join(repo, "plugins", "task", ".claude-plugin", "plugin.json")
	if err := os.MkdirAll(filepath.Dir(ext), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ext, []byte("{\n  \"name\": \"task\",\n  \"version\": \"0.1.0\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		gitTagsKey(repo, "v*"): {Stdout: ""},
		gitLogKey(repo, ""):    {Stdout: commitRecord(3, "feat: add applied feature", "")},
	}}
	var stdout, stderr bytes.Buffer

	code := run([]string{"release", "apply", "--repo", repo}, &stdout, &stderr, releaseOptions(t, t.TempDir(), runner))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s", code, stdout.String())
	}
	want := "Applied: wrote " + filepath.Join(repo, "plugin", ".claude-plugin", "plugin.json") + " and " + filepath.Join(repo, "CHANGELOG.md") + "\n" +
		"Applied: wrote " + ext + "\n"
	if !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout lacks %q:\n%s", want, stdout.String())
	}
	if got := mustReadFile(t, ext); !strings.Contains(got, `"version": "0.2.0"`) {
		t.Errorf("extension not bumped:\n%s", got)
	}
}
