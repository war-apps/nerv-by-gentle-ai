package paths_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
)

func TestResolve_JoinsEveryPathUnderHome(t *testing.T) {
	home := filepath.Join("C:", "Users", "walter")
	p := paths.Resolve(home)

	want := paths.Paths{
		Home:             home,
		UserConfig:       filepath.Join(paths.DefaultConfigDir(home, runtime.GOOS), "ai-workflow", "config.yml"),
		LegacyUserConfig: filepath.Join(home, ".claude", "nerv", "nerv.yaml"),
		State:            filepath.Join(home, ".gentle-ai", "state.json"),
		SkillsDir:        filepath.Join(home, ".claude", "skills"),
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

func TestDefaultConfigDir_FollowsEachPlatformLayout(t *testing.T) {
	home := filepath.Join("h", "walter")
	cases := map[string]string{
		"linux":   filepath.Join(home, ".config"),
		"freebsd": filepath.Join(home, ".config"),
		"darwin":  filepath.Join(home, "Library", "Application Support"),
		"windows": filepath.Join(home, "AppData", "Roaming"),
	}
	for goos, want := range cases {
		if got := paths.DefaultConfigDir(home, goos); got != want {
			t.Errorf("DefaultConfigDir(%q, %q) = %q, want %q", home, goos, got, want)
		}
	}
}

// The real home follows os.UserConfigDir, so XDG_CONFIG_HOME (Linux) or
// %AppData% (Windows) is honoured exactly as the platform defines it.
func TestResolve_RealHomeUsesOSUserConfigDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("no user config directory")
	}
	want := filepath.Join(dir, "ai-workflow", "config.yml")
	if got := paths.Resolve(home).UserConfig; got != want {
		t.Errorf("Resolve(real home).UserConfig = %q, want %q", got, want)
	}
}

func TestProjectPaths(t *testing.T) {
	repo := filepath.Join("r", "repo")
	if got, want := paths.ProjectConfig(repo), filepath.Join(repo, ".ai-workflow.yml"); got != want {
		t.Errorf("ProjectConfig = %q, want %q", got, want)
	}
	if got, want := paths.LegacyProjectConfig(repo), filepath.Join(repo, ".nerv", "nerv.yaml"); got != want {
		t.Errorf("LegacyProjectConfig = %q, want %q", got, want)
	}
	if got, want := paths.RuntimeDir(repo), filepath.Join(repo, ".nerv"); got != want {
		t.Errorf("RuntimeDir = %q, want %q", got, want)
	}
}
