package configure_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
)

func TestResolveHome_UsesOverrideWhenGiven(t *testing.T) {
	home, err := configure.ResolveHome(`C:\temp\fake-home`)
	if err != nil {
		t.Fatalf("ResolveHome() error = %v", err)
	}
	if home != `C:\temp\fake-home` {
		t.Errorf("ResolveHome() = %q, want override", home)
	}
}

func TestResolveHome_FallsBackToEnv(t *testing.T) {
	t.Setenv("HOME", `C:\temp\env-home`)
	t.Setenv("USERPROFILE", "")

	home, err := configure.ResolveHome("")
	if err != nil {
		t.Fatalf("ResolveHome() error = %v", err)
	}
	if home != `C:\temp\env-home` {
		t.Errorf("ResolveHome() = %q, want env fallback", home)
	}
}

func TestResolvePaths_Defaults(t *testing.T) {
	home := `C:\Users\walter`
	resolved := configure.ResolvePaths(home, "")

	wantConfig := filepath.Join(paths.DefaultConfigDir(home, runtime.GOOS), "ai-workflow", "config.yml")
	if resolved.Config != wantConfig {
		t.Errorf("Config = %q, want %q", resolved.Config, wantConfig)
	}
	wantState := filepath.Join(home, ".gentle-ai", "state.json")
	if resolved.State != wantState {
		t.Errorf("State = %q, want %q", resolved.State, wantState)
	}
	wantSkills := filepath.Join(home, ".claude", "skills")
	if resolved.SkillsDir != wantSkills {
		t.Errorf("SkillsDir = %q, want %q", resolved.SkillsDir, wantSkills)
	}
}

func TestResolvePaths_ConfigOverride(t *testing.T) {
	paths := configure.ResolvePaths(`C:\Users\walter`, `C:\explicit\config.yml`)
	if paths.Config != `C:\explicit\config.yml` {
		t.Errorf("Config = %q, want override", paths.Config)
	}
}

func TestRefusalError_UnwrapsAndFormats(t *testing.T) {
	inner := &configure.RefusalError{Err: errTest("bogus")}
	if inner.Error() != "bogus" {
		t.Errorf("Error() = %q, want %q", inner.Error(), "bogus")
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
