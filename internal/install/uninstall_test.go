package install_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/install"
)

func TestUninstall_RemovesSettingsKeysAndMarketplaceDir(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{
  "theme": "dark",
  "extraKnownMarketplaces": { "nerv": { "source": { "source": "directory", "path": "C:\\home\\.nerv\\marketplace" } } },
  "enabledPlugins": { "nerv@nerv": true }
}`
	if err := os.WriteFile(settingsPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	marketplaceDir := filepath.Join(home, ".nerv", "marketplace")
	if err := os.MkdirAll(marketplaceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"claude plugin uninstall nerv@nerv": {Stdout: "uninstalled\n", ExitCode: 0},
		},
	}
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	if err := install.Uninstall(context.Background(), deps); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("settings.json invalid JSON: %v", err)
	}
	if marketplaces, ok := decoded["extraKnownMarketplaces"].(map[string]any); ok {
		if _, exists := marketplaces["nerv"]; exists {
			t.Error("extraKnownMarketplaces.nerv still present")
		}
	}
	if enabled, ok := decoded["enabledPlugins"].(map[string]any); ok {
		if _, exists := enabled["nerv@nerv"]; exists {
			t.Error(`enabledPlugins["nerv@nerv"] still present`)
		}
	}
	if decoded["theme"] != "dark" {
		t.Errorf("theme = %v, want dark (unrelated key must survive)", decoded["theme"])
	}

	if _, err := os.Stat(marketplaceDir); !os.IsNotExist(err) {
		t.Errorf("marketplace dir still exists: %v", err)
	}

	var sawUninstall bool
	for _, c := range runner.Calls {
		if c.Name == "claude" && len(c.Args) >= 2 && c.Args[0] == "plugin" && c.Args[1] == "uninstall" {
			sawUninstall = true
		}
	}
	if !sawUninstall {
		t.Error("expected a claude plugin uninstall call")
	}
}

func TestUninstall_TolerantWhenNothingWasInstalled(t *testing.T) {
	home := t.TempDir()
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"claude plugin uninstall nerv@nerv": {Stderr: "not installed\n", ExitCode: 1},
		},
	}
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	if err := install.Uninstall(context.Background(), deps); err != nil {
		t.Fatalf("Uninstall() error = %v, want nil when nothing was installed", err)
	}
}
