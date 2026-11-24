package install_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/install"
	"github.com/war-apps/nerv-kit-by-gentle-ai/runner/runnertest"
)

func runnerLog(calls []runnertest.Call) string {
	var lines []string
	for _, c := range calls {
		lines = append(lines, c.Name+" "+strings.Join(c.Args, " "))
	}
	return strings.Join(lines, " | ")
}

func enabledPlugins(t *testing.T, home string) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded.EnabledPlugins
}

func writeSettings(t *testing.T, home, content string) {
	t.Helper()
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

var taskFamily = install.OrphanFamily{Tool: "nerv-task", Commands: "/task:*", Plugins: []string{"task", "task-teamwork"}}

// Install disables the orphans of opts.Orphans and uninstalls them,
// keeping every other plugin; nerv@nerv alone is refreshed.
func TestInstall_DisablesTheOrphansAndKeepsTheRest(t *testing.T) {
	home := t.TempDir()
	seedInstalledPlugins(t, home, pluginVersion(t))
	writeSettings(t, home, `{"enabledPlugins":{"task@nerv":true,"task-teamwork@nerv":false,"other@market":true}}`)
	runner := baseRunner()
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	if err := install.Install(context.Background(), deps, install.Options{NoSkills: true, Orphans: []install.OrphanFamily{taskFamily}}); err != nil {
		t.Fatalf("Install() error = %v; output:\n%s", err, stdout.String())
	}
	enabled := enabledPlugins(t, home)
	if len(enabled) != 2 || !enabled["nerv@nerv"] || !enabled["other@market"] {
		t.Errorf("enabledPlugins = %v, want nerv@nerv and other@market", enabled)
	}
	want := "Warning: disabled task@nerv, task-teamwork@nerv: nerv no longer ships them, so /task:* is unavailable until you install nerv-task: nerv install --tools nerv-task\n"
	if !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout lacks %q:\n%s", want, stdout.String())
	}
	log := runnerLog(runner.Calls)
	for _, id := range []string{"task@nerv", "task-teamwork@nerv"} {
		if !strings.Contains(log, "claude plugin uninstall "+id) {
			t.Errorf("%s not uninstalled: %s", id, log)
		}
		if strings.Contains(log, "claude plugin install "+id) {
			t.Errorf("%s was installed: %s", id, log)
		}
	}
}

// An installed tool turns the warning into one note per removed plugin.
func TestDisableOrphans_ToolInstalledNotesEachRemoval(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{"enabledPlugins":{"nerv@nerv":true,"task@nerv":true}}`)
	runner := baseRunner()
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, Runner: runner, Now: fixedNow, Stdout: &stdout}
	family := taskFamily
	family.ToolInstalled = true

	if err := install.DisableOrphans(context.Background(), deps, []install.OrphanFamily{family}); err != nil {
		t.Fatalf("DisableOrphans() error = %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "Removed task@nerv from settings.json (replaced by nerv-task)\n") || strings.Contains(got, "Warning") {
		t.Errorf("stdout = %q, want only the replaced-by note", got)
	}
	if enabled := enabledPlugins(t, home); len(enabled) != 1 || !enabled["nerv@nerv"] {
		t.Errorf("enabledPlugins = %v, want only nerv@nerv", enabled)
	}
	if log := runnerLog(runner.Calls); log != "claude plugin uninstall task@nerv" {
		t.Errorf("calls = %s, want only the task@nerv uninstall", log)
	}
}

// Without orphans DisableOrphans writes, prints and runs nothing.
func TestDisableOrphans_NoneFoundDoesNothing(t *testing.T) {
	home := t.TempDir()
	content := `{"enabledPlugins":{"nerv@nerv":true}}`
	writeSettings(t, home, content)
	runner := baseRunner()
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, Runner: runner, Now: fixedNow, Stdout: &stdout}

	if err := install.DisableOrphans(context.Background(), deps, []install.OrphanFamily{taskFamily}); err != nil {
		t.Fatalf("DisableOrphans() error = %v", err)
	}
	if stdout.Len() != 0 || len(runner.Calls) != 0 {
		t.Errorf("stdout = %q, calls = %v; want nothing", stdout.String(), runner.Calls)
	}
	if data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json")); string(data) != content {
		t.Errorf("settings.json = %s, want it untouched", data)
	}
}

// RefreshCache refreshes nerv@nerv only: an extension plugin still enabled
// is no longer reinstalled.
func TestRefreshCache_RefreshesOnlyNerv(t *testing.T) {
	home := t.TempDir()
	seedInstalledPlugins(t, home, pluginVersion(t))
	writeSettings(t, home, `{"enabledPlugins":{"nerv@nerv":true,"task@nerv":true}}`)
	runner := baseRunner()
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	if err := install.RefreshCache(context.Background(), deps); err != nil {
		t.Fatalf("RefreshCache() error = %v; output:\n%s", err, stdout.String())
	}
	if log := runnerLog(runner.Calls); strings.Contains(log, "task@nerv") || !strings.Contains(log, "claude plugin install nerv@nerv") {
		t.Errorf("calls = %s, want nerv@nerv refreshed and task@nerv left alone", log)
	}
}

// Uninstall still removes every plugin of the nerv marketplace, extension
// plugins included, and keeps the other marketplaces' ones.
func TestUninstall_RemovesEveryNervMarketplacePlugin(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{"enabledPlugins":{"nerv@nerv":true,"task@nerv":true,"task-teamwork@nerv":true,"other@elsewhere":true}}`)
	runner := baseRunner()
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}
	if err := install.Uninstall(context.Background(), deps); err != nil {
		t.Fatalf("Uninstall() error = %v; output:\n%s", err, stdout.String())
	}
	enabled := enabledPlugins(t, home)
	for _, id := range []string{"nerv@nerv", "task@nerv", "task-teamwork@nerv"} {
		if _, ok := enabled[id]; ok {
			t.Errorf("%s still in enabledPlugins", id)
		}
	}
	if !enabled["other@elsewhere"] {
		t.Error("a plugin from another marketplace was removed")
	}
	joined := runnerLog(runner.Calls)
	for _, want := range []string{"claude plugin uninstall task@nerv", "claude plugin uninstall task-teamwork@nerv", "claude plugin uninstall nerv@nerv"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %s", want, joined)
		}
	}
}
