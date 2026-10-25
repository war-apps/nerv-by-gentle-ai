package workflowconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_ReadsTheTwoNewFiles(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	write(t, paths.Resolve(home).UserConfig, "git:\n  base_branch: develop\n")
	write(t, paths.ProjectConfig(repo), "enabled: true\n")

	cfg, err := workflowconfig.Load(home, repo)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UserPath != paths.Resolve(home).UserConfig || cfg.ProjectPath != paths.ProjectConfig(repo) {
		t.Errorf("paths = %q, %q", cfg.UserPath, cfg.ProjectPath)
	}
	if !cfg.UserExists || !cfg.ProjectExists {
		t.Errorf("exists = %v, %v; want both true", cfg.UserExists, cfg.ProjectExists)
	}
	if v, _ := cfg.Scalar("enabled"); v != "true" {
		t.Errorf("enabled = %q, want true (from the project file)", v)
	}
	if v, _ := cfg.Scalar("git.base_branch"); v != "develop" {
		t.Errorf("git.base_branch = %q, want develop (from the user file)", v)
	}
}

func TestScalar_MergesKeyByKeyWithTheProjectWinning(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	write(t, paths.Resolve(home).UserConfig, "git:\n  base_branch: develop\n  remote: upstream\n")
	write(t, paths.ProjectConfig(repo), "git:\n  base_branch: main\n")

	cfg, err := workflowconfig.Load(home, repo)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := cfg.Scalar("git.base_branch"); v != "main" {
		t.Errorf("git.base_branch = %q, want main (the project wins)", v)
	}
	if v, _ := cfg.Scalar("git.remote"); v != "upstream" {
		t.Errorf("git.remote = %q, want upstream (a key only the user sets)", v)
	}
	if _, found := cfg.Scalar("git.missing"); found {
		t.Error("git.missing found, want not found")
	}
}

func TestLoad_OldFilesOnlyIsUnconfiguredWithHints(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	p := paths.Resolve(home)
	write(t, p.LegacyUserConfig, "git:\n  base_branch: develop\n")
	write(t, filepath.Join(repo, ".nerv", "nerv.yaml"), "enabled: true\n")

	cfg, err := workflowconfig.Load(home, repo)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UserExists || cfg.ProjectExists {
		t.Errorf("exists = %v, %v; want both false (old files are never read)", cfg.UserExists, cfg.ProjectExists)
	}
	if _, found := cfg.Scalar("enabled"); found {
		t.Error("enabled found in an old-layout repo, want unconfigured")
	}
	if len(cfg.Hints) != 2 {
		t.Fatalf("hints = %q, want 2", cfg.Hints)
	}
	for _, h := range cfg.Hints {
		if strings.Contains(h, "\n") || !strings.Contains(h, "nerv migrate") {
			t.Errorf("hint %q: want one line naming nerv migrate", h)
		}
	}
	if !strings.Contains(cfg.Hints[0], p.LegacyUserConfig) || !strings.Contains(cfg.Hints[1], filepath.Join(repo, ".nerv", "nerv.yaml")) {
		t.Errorf("hints %q do not name the old files", cfg.Hints)
	}
}

func TestLegacyHint(t *testing.T) {
	dir := t.TempDir()
	oldPath, newPath := filepath.Join(dir, "old.yaml"), filepath.Join(dir, "new.yml")
	if h := workflowconfig.LegacyHint(oldPath, newPath); h != "" {
		t.Errorf("no files: hint %q, want none", h)
	}
	write(t, oldPath, "x: 1\n")
	want := "nerv: " + oldPath + " is no longer read (configuration now lives in " + newPath + "); run \"nerv migrate\" to move it"
	if h := workflowconfig.LegacyHint(oldPath, newPath); h != want {
		t.Errorf("old only: hint %q, want %q", h, want)
	}
	write(t, newPath, "x: 1\n")
	if h := workflowconfig.LegacyHint(oldPath, newPath); h != "" {
		t.Errorf("both files: hint %q, want none", h)
	}
}

func TestLoad_NoRepoReadsOnlyTheUserFile(t *testing.T) {
	home := t.TempDir()
	cfg, err := workflowconfig.Load(home, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProjectPath != "" || cfg.ProjectExists || len(cfg.Hints) != 0 {
		t.Errorf("cfg = %+v, want no project", cfg)
	}
}

// S9: a leftover .ai-workflow.yml or user config.yml is refused with the
// nerv migrate hint, even next to the new file, and is left unchanged.
func TestLoad_RefusesThePreviousFiles(t *testing.T) {
	for _, tc := range []struct {
		name, hint string
		file       func(home, repo string) string
	}{
		{"project", `run "nerv migrate" to move it`, func(_, repo string) string { return paths.PreviousProjectConfig(repo) }},
		{"user", `run "nerv migrate --user" to move it`, func(home, _ string) string { return paths.Resolve(home).PreviousUserConfig }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, repo := t.TempDir(), t.TempDir()
			write(t, paths.ProjectConfig(repo), "enabled: true\n")
			previous := tc.file(home, repo)
			write(t, previous, "enabled: false\n")
			_, err := workflowconfig.Load(home, repo)
			if err == nil || !refusal.Is(err) || !strings.Contains(err.Error(), previous) || !strings.Contains(err.Error(), tc.hint) {
				t.Fatalf("err = %v, want a refusal naming %s and %s", err, previous, tc.hint)
			}
			if data, _ := os.ReadFile(previous); string(data) != "enabled: false\n" {
				t.Errorf("%s changed: %q", previous, data)
			}
		})
	}
}
