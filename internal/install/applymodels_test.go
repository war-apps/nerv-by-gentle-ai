package install_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/install"
)

func TestApplyModels_AppliesOverridesAndRestoresDefaults(t *testing.T) {
	home := t.TempDir()
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
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: &envtest.FakeRunner{}, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	if err := install.ApplyModels(context.Background(), deps); err != nil {
		t.Fatalf("ApplyModels() error = %v; output:\n%s", err, stdout.String())
	}

	aobaPath := filepath.Join(home, ".claude", "plugins", "cache", "nerv", "nerv", pv, "agents", "aoba.md")
	content, err := os.ReadFile(aobaPath)
	if err != nil {
		t.Fatalf("reading cached aoba.md: %v", err)
	}
	if !strings.Contains(string(content), "model: haiku") {
		t.Errorf("aoba.md model not overridden, content:\n%s", content)
	}
	if !strings.Contains(stdout.String(), "Model/effort apply summary") {
		t.Errorf("expected an apply summary line, got:\n%s", stdout.String())
	}
}

// apply-models rewrites the renamed roles' cached agent files from a user
// nerv.yaml that still holds the legacy keys, and when one file holds both
// the legacy and the new key for a role, the new key's entry is applied.
func TestApplyModels_LegacyRoleKeysApplyToRenamedAgents(t *testing.T) {
	cases := []struct {
		name  string
		yaml  string
		agent string
		want  []string
	}{
		{
			name:  "legacy-key-only",
			yaml:  "models:\n  melchor: { model: haiku, effort: low }\n",
			agent: "melchior.md",
			want:  []string{"model: haiku", "effort: low"},
		},
		{
			name:  "legacy-and-new-key-new-wins",
			yaml:  "models:\n  gendo: { model: opus, effort: xhigh }\n  kaji-audit: { model: haiku, effort: low }\n",
			agent: "gendo.md",
			want:  []string{"model: opus", "effort: xhigh"},
		},
		{
			// The new key wins whatever the order: here it comes last.
			name:  "legacy-key-first-new-key-still-wins",
			yaml:  "models:\n  kaji-audit: { model: haiku, effort: low }\n  gendo: { model: opus, effort: xhigh }\n",
			agent: "gendo.md",
			want:  []string{"model: opus", "effort: xhigh"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			pv := pluginVersion(t)
			seedInstalledPlugins(t, home, pv)
			userConfigDir := filepath.Join(home, ".claude", "nerv")
			if err := os.MkdirAll(userConfigDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(userConfigDir, "nerv.yaml"), []byte(tc.yaml), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout bytes.Buffer
			deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: &envtest.FakeRunner{}, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}
			if err := install.ApplyModels(context.Background(), deps); err != nil {
				t.Fatalf("ApplyModels() error = %v; output:\n%s", err, stdout.String())
			}

			content, err := os.ReadFile(filepath.Join(home, ".claude", "plugins", "cache", "nerv", "nerv", pv, "agents", tc.agent))
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range tc.want {
				// A frontmatter line may keep a trailing comment after the value.
				if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(line) + `(\s|$)`).Match(content) {
					t.Errorf("cached %s lacks %q; content:\n%s", tc.agent, line, content)
				}
			}
		})
	}
}

func TestApplyModels_MissingCacheDir_WarnsWithoutError(t *testing.T) {
	home := t.TempDir()
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: &envtest.FakeRunner{}, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	if err := install.ApplyModels(context.Background(), deps); err != nil {
		t.Fatalf("ApplyModels() error = %v, want nil (missing cache is informational)", err)
	}
	if !strings.Contains(stdout.String(), "not found") {
		t.Errorf("expected a warning about the missing cache dir, got:\n%s", stdout.String())
	}
}

func TestApplyModels_NoOverrides_AppliesPluginDefaults(t *testing.T) {
	home := t.TempDir()
	pv := pluginVersion(t)
	seedInstalledPlugins(t, home, pv)
	// Simulate a cache that still carries a stale override.
	aobaPath := filepath.Join(home, ".claude", "plugins", "cache", "nerv", "nerv", pv, "agents", "aoba.md")
	content, err := os.ReadFile(aobaPath)
	if err != nil {
		t.Fatal(err)
	}
	stale := strings.Replace(string(content), "model: sonnet", "model: haiku", 1)
	if stale == string(content) {
		t.Fatalf("fixture did not contain 'model: sonnet' to replace; content:\n%s", content)
	}
	if err := os.WriteFile(aobaPath, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), Runner: &envtest.FakeRunner{}, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	if err := install.ApplyModels(context.Background(), deps); err != nil {
		t.Fatalf("ApplyModels() error = %v", err)
	}

	after, err := os.ReadFile(aobaPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "model: sonnet") {
		t.Errorf("expected the plugin default (sonnet) to be restored, got:\n%s", after)
	}
}
