package install_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/install"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
)

func fixedNow() time.Time { return time.Date(2026, 9, 29, 15, 4, 5, 0, time.UTC) }

func lookPathAll(name string) (string, error) { return "/usr/local/bin/" + name, nil }

func lookPathOnly(found ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, f := range found {
			if f == name {
				return "/usr/local/bin/" + name, nil
			}
		}
		return "", os.ErrNotExist
	}
}

func pluginVersion(t *testing.T) string {
	t.Helper()
	v, err := version.PluginVersion(nerv.PluginFS())
	if err != nil {
		t.Fatalf("version.PluginVersion() error = %v", err)
	}
	return v
}

// seedInstalledPlugins writes a fake installed_plugins.json plus a cache
// agents directory copied from the real embedded agents, as if
// "claude plugin install nerv@nerv" had already run and cached the given
// version.
func seedInstalledPlugins(t *testing.T, home, cachedVersion string) {
	t.Helper()
	cacheDir := filepath.Join(home, ".claude", "plugins", "cache", "nerv", "nerv", cachedVersion)
	agentsDir := filepath.Join(cacheDir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(nerv.PluginFS(), "agents")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(nerv.PluginFS(), "agents/"+e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(agentsDir, e.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	installedPath := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	if err := os.MkdirAll(filepath.Dir(installedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"plugins":{"nerv@nerv":[{"installPath":"` + filepath.ToSlash(cacheDir) + `","version":"` + cachedVersion + `"}]}}`
	if err := os.WriteFile(installedPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func baseRunner() *envtest.FakeRunner {
	return &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"gentle-ai --version":               {Stdout: "gentle-ai version 3.7.0\n", ExitCode: 0},
			"claude plugin uninstall nerv@nerv": {Stdout: "uninstalled\n", ExitCode: 0},
			"claude plugin install nerv@nerv":   {Stdout: "installed\n", ExitCode: 0},
			"engram projects list":              {Stdout: "nerv\n", ExitCode: 0},
		},
	}
}

// ---------------------------------------------------------------------------
// Install: happy path
// ---------------------------------------------------------------------------

func TestInstall_HappyPath_RegistersMarketplaceAndVerifiesCache(t *testing.T) {
	home := t.TempDir()
	runner := baseRunner()
	pv := pluginVersion(t)
	// Simulate the cache already holding the right version once "claude
	// plugin install" ran, so verification succeeds.
	seedInstalledPlugins(t, home, pv)

	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	err := install.Install(context.Background(), deps, install.Options{NoSkills: true})
	if err != nil {
		t.Fatalf("Install() error = %v; output:\n%s", err, stdout.String())
	}

	// settings.json was created and registers the marketplace + plugin.
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}
	var decoded struct {
		ExtraKnownMarketplaces struct {
			Nerv struct {
				Source struct {
					Source string `json:"source"`
					Path   string `json:"path"`
				} `json:"source"`
			} `json:"nerv"`
		} `json:"extraKnownMarketplaces"`
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("settings.json invalid JSON: %v", err)
	}
	if decoded.ExtraKnownMarketplaces.Nerv.Source.Source != "directory" {
		t.Errorf("marketplace source = %q, want directory", decoded.ExtraKnownMarketplaces.Nerv.Source.Source)
	}
	wantMarketplaceDir := filepath.Join(home, ".nerv", "marketplace")
	if decoded.ExtraKnownMarketplaces.Nerv.Source.Path != wantMarketplaceDir {
		t.Errorf("marketplace path = %q, want %q", decoded.ExtraKnownMarketplaces.Nerv.Source.Path, wantMarketplaceDir)
	}
	if !decoded.EnabledPlugins["nerv@nerv"] {
		t.Error(`enabledPlugins["nerv@nerv"] not true`)
	}

	// The plugin tree was materialized.
	if _, err := os.Stat(filepath.Join(wantMarketplaceDir, "plugin", ".claude-plugin", "plugin.json")); err != nil {
		t.Errorf("plugin not materialized: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantMarketplaceDir, ".claude-plugin", "marketplace.json")); err != nil {
		t.Errorf("marketplace.json not written: %v", err)
	}

	// Runner call sequence: gentle-ai preflight, then uninstall, then install.
	var names []string
	for _, c := range runner.Calls {
		names = append(names, c.Name+" "+strings.Join(c.Args, " "))
	}
	joined := strings.Join(names, " | ")
	if !strings.Contains(joined, "gentle-ai --version") {
		t.Errorf("expected a gentle-ai --version call, got %s", joined)
	}
	if !strings.Contains(joined, "claude plugin uninstall nerv@nerv") {
		t.Errorf("expected a claude plugin uninstall call, got %s", joined)
	}
	if !strings.Contains(joined, "claude plugin install nerv@nerv") {
		t.Errorf("expected a claude plugin install call, got %s", joined)
	}

	if !strings.Contains(stdout.String(), "Restart Claude Code") {
		t.Error("expected the restart hint in stdout")
	}
}

func TestInstall_NeverPrintsConfigureHint(t *testing.T) {
	home := t.TempDir()
	runner := baseRunner()
	seedInstalledPlugins(t, home, pluginVersion(t))
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	if err := install.Install(context.Background(), deps, install.Options{NoSkills: true}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if strings.Contains(stdout.String(), "nerv configure") {
		t.Errorf("expected no configure hint (cmd/nerv owns it), got:\n%s", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// gentle-ai preflight
// ---------------------------------------------------------------------------

func TestInstall_RequireGentleAI_MissingRefuses(t *testing.T) {
	home := t.TempDir()
	runner := &envtest.FakeRunner{
		Default: envtest.Response{Err: errors.New("executable file not found in $PATH")},
	}
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	err := install.Install(context.Background(), deps, install.Options{RequireGentleAI: true, NoSkills: true})
	if err == nil {
		t.Fatal("Install() error = nil, want a refusal when gentle-ai is required but missing")
	}
	var refusal *install.RefusalError
	if !errors.As(err, &refusal) {
		t.Errorf("error = %v (%T), want *install.RefusalError", err, err)
	}

	// Nothing else should have run: no settings.json written.
	if _, statErr := os.Stat(filepath.Join(home, ".claude", "settings.json")); statErr == nil {
		t.Error("settings.json was written despite the refusal")
	}
}

func TestInstall_GentleAIMissing_WithoutRequire_WarnsAndContinues(t *testing.T) {
	home := t.TempDir()
	runner := baseRunner()
	delete(runner.Responses, "gentle-ai --version")
	runner.Responses["gentle-ai --version"] = envtest.Response{Err: errors.New("not found")}
	seedInstalledPlugins(t, home, pluginVersion(t))
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	err := install.Install(context.Background(), deps, install.Options{NoSkills: true})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !strings.Contains(strings.ToLower(stdout.String()), "gentle-ai") {
		t.Errorf("expected a gentle-ai warning, got:\n%s", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// Cache version verification failure
// ---------------------------------------------------------------------------

func TestInstall_CacheVersionMismatch_Refuses(t *testing.T) {
	home := t.TempDir()
	runner := baseRunner()
	// Cache reports a version that does not match the embedded plugin.
	seedInstalledPlugins(t, home, "0.0.1-stale")
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	err := install.Install(context.Background(), deps, install.Options{NoSkills: true})
	if err == nil {
		t.Fatal("Install() error = nil, want a refusal on cache version mismatch")
	}
	var refusal *install.RefusalError
	if !errors.As(err, &refusal) {
		t.Errorf("error = %v (%T), want *install.RefusalError", err, err)
	}
	if !strings.Contains(err.Error(), "0.0.1-stale") {
		t.Errorf("error = %v, want it to mention the mismatched version", err)
	}
}

func TestInstall_InstalledPluginsMissingAfterInstall_Refuses(t *testing.T) {
	home := t.TempDir()
	runner := baseRunner()
	// No installed_plugins.json seeded at all.
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	err := install.Install(context.Background(), deps, install.Options{NoSkills: true})
	if err == nil {
		t.Fatal("Install() error = nil, want a refusal when installed_plugins.json never appears")
	}
	var refusal *install.RefusalError
	if !errors.As(err, &refusal) {
		t.Errorf("error = %v (%T), want *install.RefusalError", err, err)
	}
}

// ---------------------------------------------------------------------------
// Engram warning path
// ---------------------------------------------------------------------------

func TestInstall_EngramNotOnPath_WarnsButSucceeds(t *testing.T) {
	home := t.TempDir()
	runner := baseRunner()
	seedInstalledPlugins(t, home, pluginVersion(t))
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathOnly("claude", "gentle-ai"), Stdout: &stdout}

	err := install.Install(context.Background(), deps, install.Options{NoSkills: true})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "engram not found on PATH") {
		t.Errorf("expected the Engram PATH warning, got:\n%s", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// Model apply happens as part of Install
// ---------------------------------------------------------------------------

func TestInstall_AppliesModelOverridesToCache(t *testing.T) {
	home := t.TempDir()
	runner := baseRunner()
	pv := pluginVersion(t)
	seedInstalledPlugins(t, home, pv)

	userConfigDir := filepath.Join(home, ".claude", "nerv")
	if err := os.MkdirAll(userConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "models:\n  aoba: { model: haiku, effort: low }\n"
	if err := os.WriteFile(filepath.Join(userConfigDir, "nerv.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	if err := install.Install(context.Background(), deps, install.Options{NoSkills: true}); err != nil {
		t.Fatalf("Install() error = %v; output:\n%s", err, stdout.String())
	}

	aobaPath := filepath.Join(home, ".claude", "plugins", "cache", "nerv", "nerv", pv, "agents", "aoba.md")
	content, err := os.ReadFile(aobaPath)
	if err != nil {
		t.Fatalf("reading cached aoba.md: %v", err)
	}
	if !strings.Contains(string(content), "model: haiku") {
		t.Errorf("aoba.md model not overridden, content:\n%s", content)
	}
	if !strings.Contains(string(content), "effort: low") {
		t.Errorf("aoba.md effort not overridden, content:\n%s", content)
	}
}

// ---------------------------------------------------------------------------
// Marketplace sync: "claude plugin install" resolves marketplaces from
// known_marketplaces.json, not settings.json, so the installer must
// register the materialized directory through the CLI first.
// ---------------------------------------------------------------------------

func callLines(runner *envtest.FakeRunner) []string {
	var lines []string
	for _, c := range runner.Calls {
		lines = append(lines, c.Name+" "+strings.Join(c.Args, " "))
	}
	return lines
}

func indexOf(lines []string, want string) int {
	for i, l := range lines {
		if l == want {
			return i
		}
	}
	return -1
}

func TestInstall_AddsMarketplaceBeforeInstallingPlugin(t *testing.T) {
	home := t.TempDir()
	runner := baseRunner()
	seedInstalledPlugins(t, home, pluginVersion(t))
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	if err := install.Install(context.Background(), deps, install.Options{NoSkills: true}); err != nil {
		t.Fatalf("Install() error = %v; output:\n%s", err, stdout.String())
	}

	lines := callLines(runner)
	add := indexOf(lines, "claude plugin marketplace add "+filepath.Join(home, ".nerv", "marketplace"))
	uninstall := indexOf(lines, "claude plugin uninstall nerv@nerv")
	installed := indexOf(lines, "claude plugin install nerv@nerv")
	if add < 0 {
		t.Fatalf("no marketplace add call; calls: %s", strings.Join(lines, " | "))
	}
	if add > uninstall || add > installed {
		t.Errorf("marketplace add must run before uninstall/install; calls: %s", strings.Join(lines, " | "))
	}
}

func TestInstall_MarketplaceAddFailure_StopsBeforeInstall(t *testing.T) {
	home := t.TempDir()
	runner := baseRunner()
	runner.Responses["claude plugin marketplace add "+filepath.Join(home, ".nerv", "marketplace")] =
		envtest.Response{Stderr: "invalid marketplace\n", ExitCode: 1}
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	err := install.Install(context.Background(), deps, install.Options{NoSkills: true})
	if err == nil {
		t.Fatal("Install() error = nil, want an error when the marketplace add fails")
	}
	if !strings.Contains(err.Error(), "invalid marketplace") {
		t.Errorf("error = %v, want it to carry the CLI output", err)
	}
	if indexOf(callLines(runner), "claude plugin install nerv@nerv") >= 0 {
		t.Error("claude plugin install ran despite the marketplace add failure")
	}
}

func TestRefreshCache_AddsMarketplace(t *testing.T) {
	home := t.TempDir()
	runner := baseRunner()
	seedInstalledPlugins(t, home, pluginVersion(t))
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	if err := install.RefreshCache(context.Background(), deps); err != nil {
		t.Fatalf("RefreshCache() error = %v; output:\n%s", err, stdout.String())
	}
	if indexOf(callLines(runner), "claude plugin marketplace add "+filepath.Join(home, ".nerv", "marketplace")) < 0 {
		t.Errorf("no marketplace add call; calls: %s", strings.Join(callLines(runner), " | "))
	}
}
