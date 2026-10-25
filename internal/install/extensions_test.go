package install_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/extension/extensiontest"
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
		"task/extension.yaml":                      &fstest.MapFile{Data: extensiontest.Extension("task", 1)},
		"task-teamwork/provider.yaml":              &fstest.MapFile{Data: extensiontest.Provider("task", "teamwork")},
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

// A requested provider whose requirement is not shipped is refused instead
// of enabling a nameless "@nerv" plugin.
func TestInstall_RequirementNotShippedIsRefused(t *testing.T) {
	home := t.TempDir()
	seedInstalledPlugins(t, home, pluginVersion(t))
	ext := fakeExtensions()
	v, _ := version.PluginVersion(nerv.PluginFS())
	ext["deploy-acme/.claude-plugin/plugin.json"] = &fstest.MapFile{Data: []byte(`{"name":"deploy-acme","version":"` + v + `"}`)}
	ext["deploy-acme/provider.yaml"] = &fstest.MapFile{Data: extensiontest.Provider("deploy", "acme")}
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), ExtensionsFS: ext, Runner: baseRunner(), Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}
	err := install.Install(context.Background(), deps, install.Options{NoSkills: true, Extensions: []string{"deploy-acme"}})
	if err == nil || !strings.Contains(err.Error(), `"deploy"`) {
		t.Fatalf("Install() error = %v, want a refusal naming the missing deploy plugin", err)
	}

	// An extension whose extension.yaml requires an extension this nerv
	// does not ship is a defect of the tree, found when it is read, before
	// anything is requested or installed.
	ext = fakeExtensions()
	ext["deploy/.claude-plugin/plugin.json"] = &fstest.MapFile{Data: []byte(`{"name":"deploy","version":"` + v + `"}`)}
	ext["deploy/extension.yaml"] = &fstest.MapFile{Data: extensiontest.Extension("deploy", 3, "release")}
	deps.ExtensionsFS = ext
	for _, requested := range [][]string{{"deploy"}, {"task"}} {
		stdout.Reset()
		err = install.Install(context.Background(), deps, install.Options{NoSkills: true, Extensions: requested})
		if err == nil || !strings.Contains(err.Error(), "deploy/extension.yaml: requires: release is not an extension of this tree") {
			t.Fatalf("Install(%v) error = %v, want the tree refused naming the missing release extension", requested, err)
		}
	}
}

// bundleExtensions is an extension rel that bundles its providers rel-a
// and rel-b, and an extension sug whose provider sug-x is suggested.
func bundleExtensions() fstest.MapFS {
	v, _ := version.PluginVersion(nerv.PluginFS())
	manifest := func(name string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte(`{"name":"` + name + `","version":"` + v + `","description":"` + name + ` plugin"}`)}
	}
	ext := func(name string, order int, mode string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte(fmt.Sprintf("name: %s\ncontract_version: 1\nsuggest_order: %d\nproviders: %s\nruntime: agent\nagent: %s:entry\nslots:\n  - name: engine\n    cardinality: one\n    operations: { ping: optional }\n", name, order, mode, name))}
	}
	provider := func(owner, name string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("name: " + name + "\nextension: " + owner + "\nslot: engine\ncontract_version: 1\nruntime: agent\nmcp: { server: x, tools: [x_ping] }\noperations:\n  ping: not_implemented\n")}
	}
	return fstest.MapFS{
		"sug/.claude-plugin/plugin.json":   manifest("sug"),
		"sug/extension.yaml":               ext("sug", 1, "suggested"),
		"sug-x/.claude-plugin/plugin.json": manifest("sug-x"),
		"sug-x/provider.yaml":              provider("sug", "x"),
		"rel/.claude-plugin/plugin.json":   manifest("rel"),
		"rel/extension.yaml":               ext("rel", 2, "bundled"),
		"rel-a/.claude-plugin/plugin.json": manifest("rel-a"),
		"rel-a/provider.yaml":              provider("rel", "a"),
		"rel-b/.claude-plugin/plugin.json": manifest("rel-b"),
		"rel-b/provider.yaml":              provider("rel", "b"),
	}
}

// S4 (L-change): an extension that bundles its providers brings every one
// of them, never asked; declining the extension skips them; a suggested
// extension's providers are still asked.
func TestInstall_BundledProvidersComeWithTheirExtension(t *testing.T) {
	for _, tc := range []struct {
		name      string
		opts      install.Options
		answers   map[string]bool
		wantAsked []string
		wantOn    []string
		wantOff   []string
	}{
		{name: "accepted", answers: map[string]bool{"sug": true, "sug-x": true, "rel": true}, wantAsked: []string{"sug", "sug-x", "rel"},
			wantOn: []string{"sug@nerv", "sug-x@nerv", "rel@nerv", "rel-a@nerv", "rel-b@nerv"}},
		{name: "bundling extension declined", answers: map[string]bool{"sug": true}, wantAsked: []string{"sug", "sug-x", "rel"},
			wantOn: []string{"sug@nerv"}, wantOff: []string{"sug-x@nerv", "rel@nerv", "rel-a@nerv", "rel-b@nerv"}},
		{name: "requested provider", opts: install.Options{Extensions: []string{"rel-b"}}, answers: map[string]bool{}, wantAsked: []string{"sug"},
			wantOn: []string{"rel@nerv", "rel-a@nerv", "rel-b@nerv"}, wantOff: []string{"sug@nerv"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			seedInstalledIDs(t, home, pluginVersion(t), "sug@nerv", "sug-x@nerv", "rel@nerv", "rel-a@nerv", "rel-b@nerv")
			var asked []string
			var stdout bytes.Buffer
			deps := install.Deps{
				Home: home, FS: nerv.PluginFS(), ExtensionsFS: bundleExtensions(), Runner: baseRunner(), Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout,
				ConfirmExtension: func(name, _ string) bool {
					asked = append(asked, name)
					return tc.answers[name]
				},
			}
			opts := tc.opts
			opts.NoSkills = true
			if err := install.Install(context.Background(), deps, opts); err != nil {
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
			for _, p := range []string{"rel-a", "rel-b"} {
				if want := slices.Contains(tc.wantOn, p+"@nerv") && !slices.Contains(tc.opts.Extensions, p); strings.Contains(stdout.String(), "Adding "+p+" (bundled with rel)") != want {
					t.Errorf("notice for %s: want %v in\n%s", p, want, stdout.String())
				}
			}
		})
	}
}

// The shipped tree passes discovery's requires checks, and nerv install
// suggests and adds its extensions in the same order and with the same
// output as before them: task and its providers, release with its bundled
// providers, deploy with its own.
func TestInstall_ShippedTreeSelectionOrder(t *testing.T) {
	home := t.TempDir()
	entries, err := fs.ReadDir(nerv.ExtensionsFS(), ".")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range entries {
		ids = append(ids, e.Name()+"@nerv")
	}
	seedInstalledIDs(t, home, pluginVersion(t), ids...)
	var asked []string
	var stdout bytes.Buffer
	deps := install.Deps{
		Home: home, FS: nerv.PluginFS(), ExtensionsFS: nerv.ExtensionsFS(), Runner: baseRunner(), Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout,
		ConfirmExtension: func(name, _ string) bool {
			asked = append(asked, name)
			return true
		},
	}
	if err := install.Install(context.Background(), deps, install.Options{NoSkills: true}); err != nil {
		t.Fatalf("Install() error = %v; output:\n%s", err, stdout.String())
	}
	var adding []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.HasPrefix(line, "Adding ") {
			adding = append(adding, line)
		}
	}
	if want := []string{"task", "task-teamwork", "release", "deploy"}; !slices.Equal(asked, want) {
		t.Errorf("asked %v, want %v", asked, want)
	}
	want := []string{
		"Adding release-bash (bundled with release)",
		"Adding release-conventional-commits (bundled with release)",
		"Adding release-docker-compose (bundled with release)",
		"Adding deploy-bash (bundled with deploy)",
		"Adding deploy-github (bundled with deploy)",
		"Adding deploy-ssh-compose (bundled with deploy)",
	}
	if !slices.Equal(adding, want) {
		t.Errorf("added %q\nwant %q", adding, want)
	}
}
