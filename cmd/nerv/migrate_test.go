package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-kit-by-gentle-ai/runner/runnertest"
)

// migrateRepo is a repository holding a 4.2.0 fixture of
// internal/migrate/testdata, and the options running nerv against it.
func migrateRepo(t *testing.T, fixture string) (top string, opts options) {
	t.Helper()
	top = filepath.Join(t.TempDir(), "repo")
	src := filepath.Join("..", "..", "internal", "migrate", "testdata", fixture)
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(top, e.Name())
		if e.Name() == "nerv.yaml" {
			dst = filepath.Join(top, ".nerv", "nerv.yaml")
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts = testOptions(t.TempDir())
	opts.Runner = &runnertest.FakeRunner{Responses: map[string]runnertest.Response{
		"git -C . rev-parse --show-toplevel":       {Stdout: top + "\n"},
		"git -C " + top + " remote get-url origin": {Stdout: "git@github.com:dirm-s-a/dim.comprobantes.api.git\n"},
	}}
	return top, opts
}

func runMigrateCLI(t *testing.T, opts options, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(append([]string{"migrate"}, args...), &out, &errOut, opts)
	return code, out.String(), errOut.String()
}

func TestMigrate_DryRunPrintsEveryChangeAndWritesNothing(t *testing.T) {
	top, opts := migrateRepo(t, "dim.comprobantes.api")
	code, out, errOut := runMigrateCLI(t, opts, "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	for _, want := range []string{
		"nerv migrate --dry-run: these are the steps nerv migrate would take; nothing is written:\n",
		"  1. Create .ai-workflow/config.yaml: the new project configuration, from .nerv/nerv.yaml\n",
		". Move .nerv/nerv.yaml to .nerv/nerv.yaml.migrated: a copy of the old configuration, so an older nerv does not read it\n",
		"\nHow the configuration changes:\n  - ",
		"\nNext, install the tools it uses: nerv install --tools nerv-task\n",
		"nerv migrate: dry run: 3 change(s), nothing was written",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"--- ", "+++ ", "@@ ", "\nnote: ", "\nwarning: "} {
		if strings.Contains(out, unwanted) {
			t.Errorf("stdout has %q, the steps carry no diff:\n%s", unwanted, out)
		}
	}
	if _, err := os.Stat(filepath.Join(top, ".ai-workflow/config.yaml")); err == nil {
		t.Error("--dry-run wrote .ai-workflow/config.yaml")
	}
	if _, err := os.Stat(filepath.Join(top, ".nerv", "nerv.yaml")); err != nil {
		t.Errorf("--dry-run moved .nerv/nerv.yaml: %v", err)
	}
}

func TestMigrate_WritesThenASecondRunHasNothingToDo(t *testing.T) {
	top, opts := migrateRepo(t, "legacy-full")
	code, out, errOut := runMigrateCLI(t, opts)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	for _, want := range []string{
		"nerv migrate: these are the steps:\n  1. Create .ai-workflow/config.yaml: the new project configuration, from .nerv/nerv.yaml\n",
		"\nBackups:\n  - " + filepath.Join(top, ".nerv", "backups"),
		"change(s) written",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q", want)
		}
	}
	if strings.Contains(out, "+++ ") {
		t.Errorf("the run prints a diff:\n%s", out)
	}
	code, out, _ = runMigrateCLI(t, opts)
	if code != 0 || !strings.Contains(out, "nerv migrate: nothing to migrate") {
		t.Errorf("second run: exit %d\n%s", code, out)
	}
}

func TestMigrate_OutsideARepositoryRefusesUnlessUser(t *testing.T) {
	opts := testOptions(t.TempDir())
	opts.Runner = &runnertest.FakeRunner{Responses: map[string]runnertest.Response{
		"git -C . rev-parse --show-toplevel": {Stderr: "fatal: not a git repository", ExitCode: 128},
	}}
	if code, out, errOut := runMigrateCLI(t, opts); code != 1 || !strings.Contains(errOut, "not inside a git repository") {
		t.Errorf("exit %d\n%s\n%s", code, out, errOut)
	}
	code, out, errOut := runMigrateCLI(t, opts, "--user")
	if code != 0 || !strings.Contains(out, "nothing to migrate") {
		t.Errorf("--user outside a repository: exit %d\n%s\n%s", code, out, errOut)
	}
}

// nerv migrate --user moves the user file's run-time task keys under
// defaults:: the dry run lists the step and writes nothing, the run writes
// it with a backup, and a second run has nothing to do.
func TestMigrate_UserRunTimeKeysMoveUnderDefaults(t *testing.T) {
	home := t.TempDir()
	opts := testOptions(home)
	opts.Runner = &runnertest.FakeRunner{Responses: map[string]runnertest.Response{
		"git -C . rev-parse --show-toplevel": {Stderr: "fatal: not a git repository", ExitCode: 128},
	}}
	user := paths.Resolve(home).UserConfig
	if err := os.MkdirAll(filepath.Dir(user), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(user, []byte("tasks:\n  provider: teamwork\n  providers:\n    teamwork:\n      assignee_id: 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := treeHash(t, home)

	code, out, errOut := runMigrateCLI(t, opts, "--user", "--dry-run")
	if code != 0 || !strings.Contains(out, "1. Update "+user+": your user configuration") || !strings.Contains(out, "user config: tasks.provider moved under defaults.tasks") {
		t.Errorf("dry run: exit %d\n%s\n%s", code, out, errOut)
	}
	if treeHash(t, home) != before {
		t.Error("the dry run wrote the home")
	}
	if code, out, errOut := runMigrateCLI(t, opts, "--user"); code != 0 || !strings.Contains(out, ".bak-migrate-") {
		t.Errorf("run: exit %d\n%s\n%s", code, out, errOut)
	}
	data, _ := os.ReadFile(user)
	if want := "tasks:\n  providers:\n    teamwork:\n      assignee_id: 7\n\ndefaults:\n  tasks:\n    provider: teamwork\n"; string(data) != want {
		t.Errorf("user config:\n%s\nwant:\n%s", data, want)
	}
	if code, out, _ := runMigrateCLI(t, opts, "--user"); code != 0 || !strings.Contains(out, "nothing to migrate") {
		t.Errorf("second run: exit %d\n%s", code, out)
	}
}

func TestMigrate_UsageErrors(t *testing.T) {
	opts := testOptions(t.TempDir())
	for _, args := range [][]string{{"--nope"}, {"extra"}} {
		if code, _, _ := runMigrateCLI(t, opts, args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
	if code, out, _ := runMigrateCLI(t, opts, "--help"); code != 0 || !strings.Contains(out, "Usage: nerv migrate") {
		t.Errorf("--help: exit %d\n%s", code, out)
	}
}

// --install runs nerv install with the tool that replaces the plugins the
// old configuration used (nerv-task for task, task-teamwork): its install
// script runs and no extension plugin is installed.
func TestMigrate_InstallRunsTheInstallStep(t *testing.T) {
	_, opts := migrateRepo(t, "dim.comprobantes.api")
	seedInstalledPluginsForCLI(t, opts.Home)
	opts.GOOS = "linux"
	runner := opts.Runner.(*runnertest.FakeRunner)
	for k, v := range baseInstallRunner().Responses {
		runner.Responses[k] = v
	}
	code, out, errOut := runMigrateCLI(t, opts, "--install")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if !strings.Contains(out, "=== Install nerv-task ===") {
		t.Errorf("nerv-task was not installed:\n%s", out)
	}
	if strings.Contains(out, "=== Install nerv-release ===") {
		t.Errorf("nerv-release was installed although no release plugin was used:\n%s", out)
	}
	if got := pluginInstalls(runner); len(got) != 1 || got[0] != "nerv@nerv" {
		t.Errorf("claude plugin install ran for %v, want only nerv@nerv", got)
	}
}

// S7: a 5.0.0-alpha.1 ship: section is refused (exit 1) naming the
// section to move by hand, and the file is left unchanged.
func TestMigrate_RefusesTheAlpha1ShipSection(t *testing.T) {
	top, opts := migrateRepo(t, "legacy-full")
	if err := os.Remove(filepath.Join(top, ".nerv", "nerv.yaml")); err != nil {
		t.Fatal(err)
	}
	ship := "enabled: true\nship:\n  projects: []\n"
	if err := os.WriteFile(filepath.Join(top, ".ai-workflow.yml"), []byte(ship), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runMigrateCLI(t, opts)
	if code != 1 || !strings.Contains(errOut, ".ai-workflow.yml: ship: is a nerv 5.0.0-alpha.1 section") || !strings.Contains(errOut, "by hand") {
		t.Errorf("exit %d\n%s\n%s", code, out, errOut)
	}
	if got := mustReadFile(t, filepath.Join(top, ".ai-workflow.yml")); got != ship {
		t.Error("a refused migration changed .ai-workflow.yml")
	}
	if _, err := os.Stat(filepath.Join(top, ".ai-workflow", "config.yaml")); err == nil {
		t.Error("a refused migration wrote .ai-workflow/config.yaml")
	}
}
