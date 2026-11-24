package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
	"github.com/war-apps/nerv-kit-by-gentle-ai/runner/runnertest"
)

// S6: the extension plugins nerv no longer ships (task, its provider,
// release, its providers and the older deploy ones) are orphans: nerv
// install and nerv upgrade disable them. Without the tool that replaces
// them, they say which were disabled and how to install the tool; with it,
// they only note each removal.

const (
	taskOrphanWarning    = "Warning: disabled task@nerv, task-teamwork@nerv: nerv no longer ships them, so /task:* is unavailable until you install nerv-task: nerv install --tools nerv-task\n"
	releaseOrphanWarning = "Warning: disabled release@nerv, release-github@nerv, deploy@nerv: nerv no longer ships them, so /release:* is unavailable until you install nerv-release: nerv install --tools nerv-release\n"
)

// seedOrphans writes a settings.json enabling nerv@nerv, another
// marketplace's plugin and the orphans task, task-teamwork, release and
// deploy (an older install), and an installed_plugins.json recording
// nerv@nerv at the embedded version plus release-github@nerv, cached but
// no longer enabled.
func seedOrphans(t *testing.T, home string) {
	t.Helper()
	pv, err := version.PluginVersion(nerv.PluginFS())
	if err != nil {
		t.Fatal(err)
	}
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(filepath.Join(claudeDir, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := `{"enabledPlugins":{"nerv@nerv":true,"other@elsewhere":true,"task@nerv":true,"task-teamwork@nerv":true,"release@nerv":true,"deploy@nerv":false}}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	installed := `{"plugins":{"nerv@nerv":[{"installPath":"/c/nerv","version":"` + pv + `"}],"release-github@nerv":[{"installPath":"/c/rg","version":"` + pv + `"}]}}`
	if err := os.WriteFile(filepath.Join(claudeDir, "plugins", "installed_plugins.json"), []byte(installed), 0o644); err != nil {
		t.Fatal(err)
	}
}

// enabledPluginIDs reads the ids settings.json lists under enabledPlugins,
// sorted.
func enabledPluginIDs(t *testing.T, home string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for id := range s.EnabledPlugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// pluginUninstalls lists the ids "claude plugin uninstall" ran for, in order.
func pluginUninstalls(r *runnertest.FakeRunner) []string {
	var ids []string
	for _, c := range r.Calls {
		if c.Name == "claude" && len(c.Args) == 3 && c.Args[0] == "plugin" && c.Args[1] == "uninstall" {
			ids = append(ids, c.Args[2])
		}
	}
	return ids
}

func pluginInstalls(r *runnertest.FakeRunner) []string {
	var ids []string
	for _, c := range r.Calls {
		if c.Name == "claude" && len(c.Args) == 3 && c.Args[0] == "plugin" && c.Args[1] == "install" {
			ids = append(ids, c.Args[2])
		}
	}
	return ids
}

var allOrphanUninstalls = []string{"task@nerv", "task-teamwork@nerv", "release@nerv", "release-github@nerv", "deploy@nerv"}

// assertOrphansGone checks settings.json keeps only nerv@nerv and the other
// marketplace's plugin, every orphan was uninstalled from the plugin cache
// and only nerv@nerv was installed.
func assertOrphansGone(t *testing.T, home string, r *runnertest.FakeRunner) {
	t.Helper()
	if got, want := enabledPluginIDs(t, home), []string{"nerv@nerv", "other@elsewhere"}; !slices.Equal(got, want) {
		t.Errorf("enabledPlugins = %v, want %v", got, want)
	}
	uninstalls := pluginUninstalls(r)
	for _, id := range allOrphanUninstalls {
		if !slices.Contains(uninstalls, id) {
			t.Errorf("claude plugin uninstall %s did not run; uninstalls: %v", id, uninstalls)
		}
	}
	if got := pluginInstalls(r); !slices.Equal(got, []string{"nerv@nerv"}) {
		t.Errorf("claude plugin install ran for %v, want only nerv@nerv", got)
	}
}

func TestOrphans_InstallWithoutTerminal_DisablesThemAndSaysHowToInstallTheTools(t *testing.T) {
	home := t.TempDir()
	seedOrphans(t, home)
	opts := testOptions(home)
	r := baseInstallRunner()
	opts.Runner = r
	var stdout, stderr bytes.Buffer

	code := run([]string{"install", "--no-skills", "--no-configure"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{taskOrphanWarning, releaseOrphanWarning} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "replaced by nerv-") {
		t.Errorf("a removal was reported as replaced by a tool that is not installed:\n%s", out)
	}
	assertOrphansGone(t, home, r)
}

func TestOrphans_InstallToolsDeclinedOnTerminal_DisablesThemAndWarns(t *testing.T) {
	home := t.TempDir()
	seedOrphans(t, home)
	opts := testOptions(home)
	r := baseInstallRunner()
	opts.Runner = r
	opts.GOOS = "linux"
	opts.IsTerminal = func() bool { return true }
	opts.Stdin = strings.NewReader("n\nn\n")
	var stdout, stderr bytes.Buffer

	code := run([]string{"install", "--no-skills", "--no-configure"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Install nerv-task? [y/N]:") || !strings.Contains(out, "Install nerv-release? [y/N]:") {
		t.Fatalf("the tools were not offered:\n%s", out)
	}
	for _, want := range []string{taskOrphanWarning, releaseOrphanWarning} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	assertOrphansGone(t, home, r)
}

// A tool installed on this machine, or chosen in this run, replaces its
// family: the orphans go with a one-line note each, without the warning.
func TestOrphans_InstallToolInstalledOrChosen_RemovesItsFamilyWithANote(t *testing.T) {
	home := t.TempDir()
	seedOrphans(t, home)
	opts := testOptions(home)
	r := baseInstallRunner()
	opts.Runner = r
	opts.GOOS = "linux"
	opts.LookPath = func(name string) (string, error) {
		if name == "nerv-release" {
			return "/usr/bin/nerv-release", nil
		}
		return "", errNotFoundForTest
	}
	var stdout, stderr bytes.Buffer

	code := run([]string{"install", "--no-skills", "--no-configure", "--tools", "nerv-task"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"Removed task@nerv from settings.json (replaced by nerv-task)\n",
		"Removed task-teamwork@nerv from settings.json (replaced by nerv-task)\n",
		"Removed deploy@nerv from settings.json (replaced by nerv-release)\n",
		"Removed release@nerv from settings.json (replaced by nerv-release)\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Warning: disabled") {
		t.Errorf("warned although the tools are installed:\n%s", out)
	}
	assertOrphansGone(t, home, r)
}

// nerv upgrade disables the orphans after its own tools step, and tells
// the install it runs to leave them to it.
func TestOrphans_UpgradeDisablesThemAfterItsToolsStep(t *testing.T) {
	w := newUpgradeWorld(t, "v5.1.0", defaultReleases...)
	seedOrphans(t, w.opts.Home)
	r := &runnertest.FakeRunner{}
	w.opts.Runner = r

	code, out, errOut := w.upgrade()

	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
	}
	toolsHint := strings.Index(out, "Run `nerv upgrade --tools <name,...>`")
	warning := strings.Index(out, taskOrphanWarning)
	if toolsHint < 0 || warning < 0 || warning < toolsHint {
		t.Errorf("want the tools hint, then the orphan warnings:\n%s", out)
	}
	if !strings.Contains(out, releaseOrphanWarning) {
		t.Errorf("stdout lacks %q:\n%s", releaseOrphanWarning, out)
	}
	if got, want := enabledPluginIDs(t, w.opts.Home), []string{"nerv@nerv", "other@elsewhere"}; !slices.Equal(got, want) {
		t.Errorf("enabledPlugins = %v, want %v", got, want)
	}
	if got := pluginUninstalls(r); !slices.Equal(got, allOrphanUninstalls) {
		t.Errorf("uninstalls = %v, want %v", got, allOrphanUninstalls)
	}
}

// --skip-orphan-cleanup (nerv upgrade's own install run) leaves the
// orphans to the upgrade.
func TestOrphans_InstallSkipOrphanCleanup_LeavesThem(t *testing.T) {
	home := t.TempDir()
	seedOrphans(t, home)
	opts := testOptions(home)
	r := baseInstallRunner()
	opts.Runner = r
	var stdout, stderr bytes.Buffer

	code := run([]string{"install", "--no-skills", "--no-configure", "--skip-orphan-cleanup"}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "Warning: disabled") {
		t.Errorf("warned about orphans:\n%s", stdout.String())
	}
	if got, want := enabledPluginIDs(t, home), []string{"deploy@nerv", "nerv@nerv", "other@elsewhere", "release@nerv", "task-teamwork@nerv", "task@nerv"}; !slices.Equal(got, want) {
		t.Errorf("enabledPlugins = %v, want %v", got, want)
	}
	if got := pluginUninstalls(r); !slices.Equal(got, []string{"nerv@nerv"}) {
		t.Errorf("uninstalls = %v, want only nerv@nerv's cache refresh", got)
	}
}

// S6, S8: nerv install --help says the extension plugins are disabled and
// how to get their commands back; --with (extension selection) is gone and,
// like any unknown flag, exits 2 with the usage.
func TestInstall_HelpDescribesTheOrphansAndWithIsGone(t *testing.T) {
	var stdout, stderr bytes.Buffer
	run([]string{"install", "--help"}, &stdout, &stderr, testOptions(t.TempDir()))
	help := strings.Join(strings.Fields(stdout.String()+stderr.String()), " ")
	for _, want := range []string{"The extension plugins nerv no longer ships", "are disabled", "nerv install --tools <tool>", "/task:* or /release:* commands are unavailable until then"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q:\n%s", want, help)
		}
	}
	if strings.Contains(help, "--with") {
		t.Errorf("help still documents --with:\n%s", help)
	}

	home := t.TempDir()
	stdout.Reset()
	stderr.Reset()
	opts := testOptions(home)
	opts.Runner = baseInstallRunner()
	if code := run([]string{"install", "--no-skills", "--no-configure", "--with", "task"}, &stdout, &stderr, opts); code != 2 {
		t.Errorf("--with: exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "Usage: nerv install") {
		t.Errorf("--with: stderr lacks the usage:\n%s", stderr.String())
	}
	if len(opts.Runner.(*runnertest.FakeRunner).Calls) != 0 {
		t.Errorf("--with ran commands: %v", opts.Runner.(*runnertest.FakeRunner).Calls)
	}
}
