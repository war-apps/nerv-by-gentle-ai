package paths_test

import (
	"path/filepath"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
)

func TestResolve_JoinsEveryPathUnderHome(t *testing.T) {
	home := filepath.Join("C:", "Users", "walter")
	p := paths.Resolve(home)

	want := paths.Paths{
		Home:             home,
		UserConfig:       filepath.Join(home, ".claude", "nerv", "nerv.yaml"),
		State:            filepath.Join(home, ".gentle-ai", "state.json"),
		SkillsDir:        filepath.Join(home, ".claude", "skills"),
		CommandsDir:      filepath.Join(home, ".claude", "commands", "task"),
		Settings:         filepath.Join(home, ".claude", "settings.json"),
		Marketplace:      filepath.Join(home, ".nerv", "marketplace"),
		InstalledPlugins: filepath.Join(home, ".claude", "plugins", "installed_plugins.json"),
	}
	if p != want {
		t.Errorf("Resolve(%q) = %+v, want %+v", home, p, want)
	}
}

func TestCacheAgentsDir(t *testing.T) {
	home := filepath.Join("C:", "Users", "walter")
	got := paths.Paths{Home: home}.CacheAgentsDir("0.4.0")
	want := filepath.Join(home, ".claude", "plugins", "cache", "nerv", "nerv", "0.4.0", "agents")
	if got != want {
		t.Errorf("CacheAgentsDir() = %q, want %q", got, want)
	}
}
