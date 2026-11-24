package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/war-apps/nerv-kit-by-gentle-ai/runner/runnertest"
)

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
	runner := &runnertest.FakeRunner{}
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
	uninstall := func(runner *runnertest.FakeRunner) int {
		var stdout, stderr bytes.Buffer
		opts := testOptions(home)
		opts.Runner = runner
		return run([]string{"uninstall"}, &stdout, &stderr, opts)
	}
	failing := &runnertest.FakeRunner{Responses: map[string]runnertest.Response{
		"claude plugin uninstall task@nerv": {Stderr: "boom", ExitCode: 3},
	}}
	if code := uninstall(failing); code == 0 {
		t.Fatal("first uninstall should fail")
	}

	rerun := &runnertest.FakeRunner{}
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

func TestMaintainerReleaseApply_ExtensionWithoutVersion_WritesNothing(t *testing.T) {
	repo := maintainerReleaseFixtureRepo(t, "0.1.0")
	ext := filepath.Join(repo, "plugins", "task", ".claude-plugin", "plugin.json")
	if err := os.MkdirAll(filepath.Dir(ext), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ext, []byte("{\n  \"name\": \"task\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &runnertest.FakeRunner{Responses: map[string]runnertest.Response{
		gitTagsKey(repo, "v*"): {Stdout: ""},
		gitLogKey(repo, ""):    {Stdout: commitRecord(3, "feat: add applied feature", "")},
	}}
	var stdout, stderr bytes.Buffer

	code := run([]string{"maintainer-release", "apply", "--repo", repo}, &stdout, &stderr, maintainerReleaseOptions(t, t.TempDir(), runner))

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

func TestMaintainerReleaseApply_ListsExtensionsAfterChangelog(t *testing.T) {
	repo := maintainerReleaseFixtureRepo(t, "0.1.0")
	ext := filepath.Join(repo, "plugins", "task", ".claude-plugin", "plugin.json")
	if err := os.MkdirAll(filepath.Dir(ext), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ext, []byte("{\n  \"name\": \"task\",\n  \"version\": \"0.1.0\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &runnertest.FakeRunner{Responses: map[string]runnertest.Response{
		gitTagsKey(repo, "v*"): {Stdout: ""},
		gitLogKey(repo, ""):    {Stdout: commitRecord(3, "feat: add applied feature", "")},
	}}
	var stdout, stderr bytes.Buffer

	code := run([]string{"maintainer-release", "apply", "--repo", repo}, &stdout, &stderr, maintainerReleaseOptions(t, t.TempDir(), runner))

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
