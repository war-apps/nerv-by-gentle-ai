package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-kit-by-gentle-ai/runner/runnertest"
	"gopkg.in/yaml.v3"
)

// fakeNervRelease is where toolsInstalledOpts finds nerv-release.
const fakeNervRelease = "/usr/bin/nerv-release"

// legacyShip is a 5.0.0-alpha.1 project file: its ship: section is an
// older nerv's release configuration.
const legacyShip = "enabled: true\nship:\n  projects:\n    - { name: api, path: api }\n"

// legacyRepo is a repository whose .ai-workflow/config.yaml is project,
// with nerv-release installed (fake, at fakeNervRelease) when withRelease.
func legacyRepo(t *testing.T, project string, withRelease bool) (home, top string, opts options) {
	t.Helper()
	home, top = t.TempDir(), filepath.Join(t.TempDir(), "repo")
	writeFile(t, filepath.Join(top, ".ai-workflow", "config.yaml"), project)
	var tools []string
	if withRelease {
		tools = append(tools, "nerv-release")
	}
	return home, top, toolsInstalledOpts(home, top, tools...)
}

// releaseMigrateCalls are the nerv-release migrate runs opts' fake runner
// saw, each as its argument line.
func releaseMigrateCalls(opts options) []string {
	var calls []string
	for _, c := range opts.Runner.(*runnertest.FakeRunner).Calls {
		if c.Name == fakeNervRelease && len(c.Args) > 0 && c.Args[0] == "migrate" {
			calls = append(calls, strings.Join(c.Args, " "))
		}
	}
	return calls
}

// scriptReleaseMigrate scripts the answer of nerv-release migrate args.
func scriptReleaseMigrate(opts options, r runnertest.Response, args ...string) {
	opts.Runner.(*runnertest.FakeRunner).Responses[fakeNervRelease+" migrate "+strings.Join(args, " ")] = r
}

func yamlSection(t *testing.T, text, section string) any {
	t.Helper()
	var m map[string]any
	if err := yaml.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("not valid YAML: %v\n%s", err, text)
	}
	return m[section]
}

// S2, S5: with nerv-release installed, nerv migrate moves the ship:
// section as it was into .ai-workflow/release.yaml, then runs
// nerv-release migrate --cwd <top>, printing its output.
func TestMigrate_LegacySectionsAreHandedToNervReleaseMigrate(t *testing.T) {
	_, top, opts := legacyRepo(t, legacyShip, true)
	scriptReleaseMigrate(opts, runnertest.Response{Stdout: "nerv-release migrate: ship: converted\n"}, "--cwd", top)

	code, out, errOut := runMigrateCLI(t, opts)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if got, want := releaseMigrateCalls(opts), []string{"migrate --cwd " + top}; !slices.Equal(got, want) {
		t.Errorf("nerv-release calls = %q, want %q", got, want)
	}
	release := readFile(t, filepath.Join(top, ".ai-workflow", "release.yaml"))
	if got, want := yamlSection(t, release, "ship"), yamlSection(t, legacyShip, "ship"); !reflect.DeepEqual(got, want) {
		t.Errorf("release.yaml ship: = %#v, want %#v", got, want)
	}
	if got := readFile(t, filepath.Join(top, ".ai-workflow", "config.yaml")); strings.Contains(got, "ship") {
		t.Errorf("config.yaml kept ship:\n%s", got)
	}
	for _, want := range []string{"nerv-release migrate", "ship:", "nerv-release migrate: ship: converted"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(top, ".nerv", "backups")); err != nil || len(entries) == 0 {
		t.Errorf("no backup was made (%v)", err)
	}
}

// S5: a refusal of nerv-release migrate (exit 1) is nerv migrate's (exit
// 1), saying the sections are in release.yaml; any other failure is an
// environment error (exit 2).
func TestMigrate_NervReleaseMigrateFailureIsPropagated(t *testing.T) {
	cases := []struct {
		name     string
		response runnertest.Response
		code     int
		want     string
	}{
		{"refused", runnertest.Response{Stderr: "nerv-release migrate: ship.x: unknown\n", ExitCode: 1}, 1, "nerv-release migrate refused"},
		{"environment", runnertest.Response{ExitCode: 2}, 2, "failed (exit 2)"},
		{"not launched", runnertest.Response{Err: errors.New("exec format error")}, 2, "exec format error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, top, opts := legacyRepo(t, legacyShip, true)
			scriptReleaseMigrate(opts, c.response, "--cwd", top)
			code, out, errOut := runMigrateCLI(t, opts)
			if code != c.code || !strings.Contains(errOut, c.want) {
				t.Fatalf("exit %d, want %d naming %q\n%s\n%s", code, c.code, c.want, out, errOut)
			}
			if c.code == 1 && (!strings.Contains(errOut, "release.yaml") || !strings.Contains(errOut, "nerv-release migrate --cwd "+top)) {
				t.Errorf("the refusal does not say where the sections are and what to run again:\n%s", errOut)
			}
			if c.response.Stderr != "" && !strings.Contains(errOut, c.response.Stderr) {
				t.Errorf("nerv-release's stderr was not printed:\n%s", errOut)
			}
		})
	}
}

// --dry-run does not run nerv-release migrate; the plan says it will.
func TestMigrate_DryRunDoesNotRunNervReleaseMigrate(t *testing.T) {
	_, top, opts := legacyRepo(t, legacyShip, true)
	code, out, errOut := runMigrateCLI(t, opts, "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if calls := releaseMigrateCalls(opts); len(calls) > 0 {
		t.Errorf("nerv-release ran on a dry run: %q", calls)
	}
	if !strings.Contains(out, "then run nerv-release migrate --cwd "+top) {
		t.Errorf("the dry run does not say nerv-release migrate runs next:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(top, ".ai-workflow", "release.yaml")); err == nil {
		t.Error("release.yaml was written on a dry run")
	}
}

// Today's release: and deploy: move to release.yaml as before, and
// nerv-release migrate is not run.
// Any release:/deploy: moved to release.yaml runs nerv-release migrate:
// a shape nerv does not tell apart (a release: whose engine keys sit at
// its top) is still converted, and a current one leaves it nothing to do.
func TestMigrate_AnyMovedSectionRunsNervReleaseMigrate(t *testing.T) {
	_, top, opts := legacyRepo(t, "enabled: true\nrelease:\n  tag_format: \"v${version}\"\n  projects: []\n", true)
	code, out, errOut := runMigrateCLI(t, opts)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if calls := releaseMigrateCalls(opts); len(calls) != 1 || calls[0] != "migrate --cwd "+top {
		t.Errorf("nerv-release migrate calls = %q, want one for the repository", calls)
	}
}

// With no release:/deploy: to move, nerv-release migrate does not run.
func TestMigrate_NothingMovedNoNervReleaseMigrate(t *testing.T) {
	_, _, opts := legacyRepo(t, "enabled: true\ngit:\n  base_branch: main\n", true)
	code, out, errOut := runMigrateCLI(t, opts)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if calls := releaseMigrateCalls(opts); len(calls) > 0 {
		t.Errorf("nerv-release migrate ran with nothing moved: %q", calls)
	}
}

// With --user, a legacy section of the user config runs nerv-release
// migrate --user (the repository's run only when it has one too).
func TestMigrate_UserLegacySectionRunsNervReleaseMigrateUser(t *testing.T) {
	home, top, opts := legacyRepo(t, "enabled: true\n", true)
	writeFile(t, paths.Resolve(home).UserConfig, "deploy:\n  targets:\n    - { name: staging, provider: bash }\n")
	code, out, errOut := runMigrateCLI(t, opts, "--user")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if got, want := releaseMigrateCalls(opts), []string{"migrate --cwd " + top + " --user"}; !slices.Equal(got, want) {
		t.Errorf("nerv-release calls = %q, want %q", got, want)
	}
}

// S4: without nerv-release, a legacy section is refused naming the
// command that installs it; nothing is written and nothing runs.
func TestMigrate_LegacySectionWithoutNervReleaseIsRefused(t *testing.T) {
	_, top, opts := legacyRepo(t, legacyShip, false)
	code, out, errOut := runMigrateCLI(t, opts)
	if code != 1 || !strings.Contains(errOut, ".ai-workflow/config.yaml: ship:") || !strings.Contains(errOut, "nerv install --tools nerv-release") {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if got := readFile(t, filepath.Join(top, ".ai-workflow", "config.yaml")); got != legacyShip {
		t.Errorf("a refused migration changed config.yaml:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(top, ".ai-workflow", "release.yaml")); err == nil {
		t.Error("a refused migration wrote release.yaml")
	}
	if calls := releaseMigrateCalls(opts); len(calls) > 0 {
		t.Errorf("nerv-release ran: %q", calls)
	}
}

// A 5.x deploy-<name> project provider alone (nothing for nerv migrate to
// write) is still handed to nerv-release migrate, which renames it.
func TestMigrate_LegacyDeployProviderAloneRunsNervReleaseMigrate(t *testing.T) {
	_, top, opts := legacyRepo(t, "enabled: true\n", true)
	writeFile(t, filepath.Join(top, ".ai-workflow", "providers", "deploy-k8s", "provider.yaml"), "name: k8s\nextension: deploy\n")
	code, out, errOut := runMigrateCLI(t, opts)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if got, want := releaseMigrateCalls(opts), []string{"migrate --cwd " + top}; !slices.Equal(got, want) {
		t.Errorf("nerv-release calls = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(top, ".ai-workflow", "providers", "deploy-k8s", "provider.yaml")); err != nil {
		t.Errorf("nerv migrate moved the provider: %v", err)
	}
}

// With --user, the repository's run carries --user too: nerv-release
// migrate then moves the personal keys of a converted pipeline: to the
// global release.yaml instead of dropping them, and converts the global
// file in the same run (one call).
func TestMigrate_UserRunsOneNervReleaseMigrateForBothScopes(t *testing.T) {
	home, top, opts := legacyRepo(t, "enabled: true\nship:\n  projects: []\n", true)
	writeFile(t, paths.Resolve(home).UserConfig, "deploy:\n  targets:\n    - { name: staging, provider: bash }\n")
	code, out, errOut := runMigrateCLI(t, opts, "--user")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if got, want := releaseMigrateCalls(opts), []string{"migrate --cwd " + top + " --user"}; !slices.Equal(got, want) {
		t.Errorf("nerv-release calls = %q, want %q", got, want)
	}
}

// B1 (S5): when nerv-release migrate failed (exit 2) after nerv migrate
// moved the older sections to release.yaml, a second nerv migrate has
// nothing of its own to write but runs nerv-release migrate again on the
// sections release.yaml still holds; the failure names what to rerun.
func TestMigrate_RerunAfterAFailedNervReleaseMigrateRunsItAgain(t *testing.T) {
	_, top, opts := legacyRepo(t, legacyShip, true)
	scriptReleaseMigrate(opts, runnertest.Response{ExitCode: 2}, "--cwd", top, "--user")
	code, out, errOut := runMigrateCLI(t, opts, "--user")
	if want := "nerv-release migrate --cwd " + top + " --user failed (exit 2): fix what it reports, then run nerv-release migrate --cwd " + top + " --user"; code != 2 || !strings.Contains(errOut, want) {
		t.Fatalf("exit %d, want 2 naming %q\n%s\n%s", code, want, out, errOut)
	}

	scriptReleaseMigrate(opts, runnertest.Response{Stdout: "nerv-release migrate: ship: converted\n"}, "--cwd", top, "--user")
	code, out, errOut = runMigrateCLI(t, opts, "--user")
	if code != 0 {
		t.Fatalf("rerun: exit %d\n%s\n%s", code, out, errOut)
	}
	want := []string{"migrate --cwd " + top + " --user", "migrate --cwd " + top + " --user"}
	if got := releaseMigrateCalls(opts); !slices.Equal(got, want) {
		t.Errorf("nerv-release calls = %q, want %q", got, want)
	}
	for _, want := range []string{"still holds", "nerv-release migrate: ship: converted"} {
		if !strings.Contains(out, want) {
			t.Errorf("rerun stdout lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "nothing to migrate") {
		t.Errorf("rerun says nothing to migrate:\n%s", out)
	}
}
