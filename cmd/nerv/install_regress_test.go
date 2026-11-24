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
