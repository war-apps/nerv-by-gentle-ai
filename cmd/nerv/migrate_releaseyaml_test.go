package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-kit-by-gentle-ai/runner/runnertest"
)

// toolsInstalledOpts fakes the named catalogue tools as found on PATH,
// over a repository at top (a fake git answers its top level). Every
// path is under the fake home or top: the global release.yaml goes next
// to the user config nerv migrate resolves (paths.Resolve), never to the
// real XDG_CONFIG_HOME or %AppData%.
func toolsInstalledOpts(home, top string, tools ...string) options {
	opts := testOptions(home)
	opts.ExtensionsFS = nerv.ExtensionsFS()
	opts.GOOS = "linux"
	opts.LookPath = func(name string) (string, error) {
		if slices.Contains(tools, name) {
			return "/usr/bin/" + name, nil
		}
		return "", os.ErrNotExist
	}
	opts.Runner = &runnertest.FakeRunner{Responses: map[string]runnertest.Response{
		"git -C . rev-parse --show-toplevel":       {Stdout: top + "\n"},
		"git -C " + top + " remote get-url origin": {ExitCode: 2},
	}}
	return opts
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// releaseYAMLFixture writes a user config with a personal ssh-compose key
// and a defaults.deploy seed, and a project file with release:/deploy:.
func releaseYAMLFixture(t *testing.T) (home, top, user string) {
	t.Helper()
	home, top = t.TempDir(), filepath.Join(t.TempDir(), "repo")
	user = paths.Resolve(home).UserConfig
	writeFile(t, user, "deploy:\n  providers:\n    ssh-compose:\n      ssh_auth: key\ndefaults:\n  deploy:\n    host: github\n")
	writeFile(t, filepath.Join(top, ".ai-workflow", "config.yaml"), "enabled: true\nrelease:\n  projects:\n    - name: api\n      path: api\n      build: bash\ndeploy:\n  host: github\n")
	return home, top, user
}

// With nerv-release installed, nerv migrate --user moves the user
// config's personal deploy keys and defaults.deploy to the global
// release.yaml next to it, and the project file's release:/deploy: to
// .ai-workflow/release.yaml, prefixes kept (S2).
func TestMigrate_ReleaseYAMLMovesWhenNervReleaseIsInstalled(t *testing.T) {
	home, top, user := releaseYAMLFixture(t)
	opts := toolsInstalledOpts(home, top, "nerv-release")

	code, out, errOut := runMigrateCLI(t, opts, "--user")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	global := filepath.Join(filepath.Dir(user), "release.yaml")
	if got, want := readFile(t, global), "deploy:\n  providers:\n    ssh-compose:\n      ssh_auth: key\ndefaults:\n  deploy:\n    host: github\n"; got != want {
		t.Errorf("global release.yaml =\n%s\nwant\n%s", got, want)
	}
	repo := filepath.Join(top, ".ai-workflow", "release.yaml")
	if got, want := readFile(t, repo), "release:\n  projects: [{ name: api, path: api, build: bash }]\ndeploy:\n  host: github\n"; got != want {
		t.Errorf("repo release.yaml =\n%s\nwant\n%s", got, want)
	}
	if got := readFile(t, user); strings.TrimSpace(got) != "" {
		t.Errorf("user config still has settings:\n%s", got)
	}
	if got := readFile(t, filepath.Join(top, ".ai-workflow", "config.yaml")); got != "enabled: true\n" {
		t.Errorf("project file =\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(user), "task.yaml")); err == nil {
		t.Error("task.yaml was written without nerv-task installed")
	}
}

// --dry-run lists the release.yaml steps and writes nothing.
func TestMigrate_ReleaseYAMLDryRunWritesNothing(t *testing.T) {
	home, top, user := releaseYAMLFixture(t)
	opts := toolsInstalledOpts(home, top, "nerv-release")
	before := readFile(t, user)

	code, out, errOut := runMigrateCLI(t, opts, "--user", "--dry-run")
	if code != 0 || !strings.Contains(out, "release.yaml: nerv-release's global configuration") || !strings.Contains(out, ".ai-workflow/release.yaml: nerv-release's repository configuration") {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if readFile(t, user) != before {
		t.Error("the user config changed on a dry run")
	}
	for _, path := range []string{filepath.Join(filepath.Dir(user), "release.yaml"), filepath.Join(top, ".ai-workflow", "release.yaml")} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("%s was written on a dry run", path)
		}
	}
}

// Without nerv-release installed, release:/deploy: stay where they are.
func TestMigrate_ReleaseYAMLUntouchedWhenNervReleaseNotInstalled(t *testing.T) {
	home, top, user := releaseYAMLFixture(t)
	opts := toolsInstalledOpts(home, top, "nerv-task")
	project := filepath.Join(top, ".ai-workflow", "config.yaml")
	before := readFile(t, project)

	code, out, errOut := runMigrateCLI(t, opts, "--user")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	for _, path := range []string{filepath.Join(filepath.Dir(user), "release.yaml"), filepath.Join(top, ".ai-workflow", "release.yaml")} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("%s was written without nerv-release installed", path)
		}
	}
	if got := readFile(t, project); got != before {
		t.Errorf("project file changed:\n%s", got)
	}
	if !strings.Contains(readFile(t, user), "ssh_auth: key") {
		t.Error("the personal key left the user config")
	}
}

// With both tools installed, one run moves tasks: to task.yaml and
// release:/deploy: to release.yaml.
func TestMigrate_TaskAndReleaseYAMLMoveTogether(t *testing.T) {
	home, top, user := releaseYAMLFixture(t)
	writeFile(t, user, "tasks:\n  timer_store: ~/t.json\n"+readFile(t, user))
	opts := toolsInstalledOpts(home, top, "nerv-task", "nerv-release")

	code, out, errOut := runMigrateCLI(t, opts, "--user")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if got := readFile(t, filepath.Join(filepath.Dir(user), "task.yaml")); strings.TrimSpace(got) != "timer_store: ~/t.json" {
		t.Errorf("task.yaml =\n%q", got)
	}
	if got := readFile(t, filepath.Join(filepath.Dir(user), "release.yaml")); !strings.Contains(got, "ssh_auth: key") {
		t.Errorf("release.yaml =\n%s", got)
	}
	if got := readFile(t, user); strings.TrimSpace(got) != "" {
		t.Errorf("user config still has settings:\n%s", got)
	}
}
