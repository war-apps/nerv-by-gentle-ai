package install_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/install"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
)

// seedInstalledIDs extends seedInstalledPlugins' installed_plugins.json
// with one cached entry per extension id, all at cachedVersion.
func seedInstalledIDs(t *testing.T, home, cachedVersion string, ids ...string) {
	t.Helper()
	seedInstalledPlugins(t, home, cachedVersion)
	path := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]map[string][]map[string]string
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		doc["plugins"][id] = []map[string]string{{"installPath": "/cache/" + id, "version": cachedVersion}}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeExtensions is a task plugin and one provider at the embedded core
// version, independent of what the real plugins/ tree holds.
func fakeExtensions() fstest.MapFS {
	v, _ := version.PluginVersion(nerv.PluginFS())
	manifest := func(name string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte(`{"name":"` + name + `","version":"` + v + `","description":"` + name + ` plugin"}`)}
	}
	return fstest.MapFS{
		"task/.claude-plugin/plugin.json":          manifest("task"),
		"task-teamwork/.claude-plugin/plugin.json": manifest("task-teamwork"),
		"task-teamwork/provider.yaml":              &fstest.MapFile{Data: []byte("name: teamwork\n")},
	}
}

func runnerLog(calls []envtest.Call) string {
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

func TestInstall_RequestedProviderBringsTheTaskPlugin(t *testing.T) {
	home := t.TempDir()
	runner := baseRunner()
	seedInstalledIDs(t, home, pluginVersion(t), "task@nerv", "task-teamwork@nerv")
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), ExtensionsFS: fakeExtensions(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	err := install.Install(context.Background(), deps, install.Options{NoSkills: true, Extensions: []string{"task-teamwork"}})
	if err != nil {
		t.Fatalf("Install() error = %v; output:\n%s", err, stdout.String())
	}

	enabled := enabledPlugins(t, home)
	for _, id := range []string{"nerv@nerv", "task@nerv", "task-teamwork@nerv"} {
		if !enabled[id] {
			t.Errorf("enabledPlugins[%q] not true: %v", id, enabled)
		}
	}
	joined := runnerLog(runner.Calls)
	for _, want := range []string{"claude plugin install task@nerv", "claude plugin install task-teamwork@nerv"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %s", want, joined)
		}
	}
	if !strings.Contains(stdout.String(), "task (required by task-teamwork)") {
		t.Errorf("the added requirement is not reported:\n%s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".nerv", "marketplace", "plugins", "task", ".claude-plugin", "plugin.json")); err != nil {
		t.Errorf("task plugin not materialized: %v", err)
	}
}

func TestInstall_SuggestsTheTaskPluginThenItsProviders(t *testing.T) {
	for _, tc := range []struct {
		name      string
		answers   map[string]bool
		wantAsked []string
		wantOn    []string
		wantOff   []string
	}{
		{
			name:      "yes to both",
			answers:   map[string]bool{"task": true, "task-teamwork": true},
			wantAsked: []string{"task", "task-teamwork"},
			wantOn:    []string{"task@nerv", "task-teamwork@nerv"},
		},
		{
			name:      "no to the task plugin skips its providers",
			answers:   map[string]bool{"task": false},
			wantAsked: []string{"task"},
			wantOff:   []string{"task@nerv", "task-teamwork@nerv"},
		},
		{
			name:      "the task plugin without providers",
			answers:   map[string]bool{"task": true, "task-teamwork": false},
			wantAsked: []string{"task", "task-teamwork"},
			wantOn:    []string{"task@nerv"},
			wantOff:   []string{"task-teamwork@nerv"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			seedInstalledIDs(t, home, pluginVersion(t), "task@nerv", "task-teamwork@nerv")
			var asked []string
			var stdout bytes.Buffer
			deps := install.Deps{
				Home: home, FS: nerv.PluginFS(), ExtensionsFS: fakeExtensions(), Runner: baseRunner(), Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout,
				ConfirmExtension: func(name, _ string) bool {
					asked = append(asked, name)
					return tc.answers[name]
				},
			}
			if err := install.Install(context.Background(), deps, install.Options{NoSkills: true}); err != nil {
				t.Fatalf("Install() error = %v; output:\n%s", err, stdout.String())
			}
			if !slices.Equal(asked, tc.wantAsked) {
				t.Errorf("asked %v, want %v", asked, tc.wantAsked)
			}
			enabled := enabledPlugins(t, home)
			for _, id := range tc.wantOn {
				if !enabled[id] {
					t.Errorf("%s not enabled", id)
				}
			}
			for _, id := range tc.wantOff {
				if enabled[id] {
					t.Errorf("%s enabled, want off", id)
				}
			}
		})
	}
}

func TestInstall_RefreshesAlreadyEnabledExtensionsWithoutAsking(t *testing.T) {
	home := t.TempDir()
	seedInstalledIDs(t, home, pluginVersion(t), "task@nerv")
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.WriteFile(settings, []byte(`{"enabledPlugins":{"task@nerv":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := baseRunner()
	var asked []string
	var stdout bytes.Buffer
	deps := install.Deps{
		Home: home, FS: nerv.PluginFS(), ExtensionsFS: fakeExtensions(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout,
		ConfirmExtension: func(name, _ string) bool { asked = append(asked, name); return false },
	}
	if err := install.Install(context.Background(), deps, install.Options{NoSkills: true}); err != nil {
		t.Fatalf("Install() error = %v; output:\n%s", err, stdout.String())
	}
	if slices.Contains(asked, "task") {
		t.Errorf("asked again for an enabled plugin: %v", asked)
	}
	if !strings.Contains(runnerLog(runner.Calls), "claude plugin install task@nerv") {
		t.Errorf("enabled extension not refreshed: %s", runnerLog(runner.Calls))
	}
}

func TestInstall_WithoutAnAnswererInstallsNoExtension(t *testing.T) {
	home := t.TempDir()
	seedInstalledPlugins(t, home, pluginVersion(t))
	runner := baseRunner()
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), ExtensionsFS: fakeExtensions(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}
	if err := install.Install(context.Background(), deps, install.Options{NoSkills: true}); err != nil {
		t.Fatalf("Install() error = %v; output:\n%s", err, stdout.String())
	}
	if strings.Contains(runnerLog(runner.Calls), "task@nerv") {
		t.Errorf("an extension was installed without being asked for: %s", runnerLog(runner.Calls))
	}
}

func TestInstall_UnknownExtensionIsRefused(t *testing.T) {
	home := t.TempDir()
	seedInstalledPlugins(t, home, pluginVersion(t))
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), ExtensionsFS: fakeExtensions(), Runner: baseRunner(), Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}
	err := install.Install(context.Background(), deps, install.Options{NoSkills: true, Extensions: []string{"task-jira"}})
	var refused *refusal.Error
	if !errors.As(err, &refused) {
		t.Fatalf("Install() error = %v, want a refusal", err)
	}
	if !strings.Contains(err.Error(), "task-jira") || !strings.Contains(err.Error(), "task-teamwork") {
		t.Errorf("refusal does not name the unknown and the available extensions: %v", err)
	}
}

func TestUninstall_RemovesEnabledExtensions(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"enabledPlugins":{"nerv@nerv":true,"task@nerv":true,"task-teamwork@nerv":true,"other@elsewhere":true}}`
	if err := os.WriteFile(settings, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := baseRunner()
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), ExtensionsFS: fakeExtensions(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}
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
