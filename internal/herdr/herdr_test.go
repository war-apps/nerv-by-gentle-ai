package herdr_test

import (
	"errors"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/herdr"
)

func notFound(string) ([]byte, error) { return nil, errors.New("not found") }

func lookPathFound(string) (string, error) { return "/usr/local/bin/herdr", nil }
func lookPathNone(string) (string, error)  { return "", errors.New("not found") }

func noEnv(string) string { return "" }

func TestDetect_NotOnPath_NotInstalled(t *testing.T) {
	got := herdr.Detect(lookPathNone, noEnv, "/home/walter", notFound, "linux")

	if got.Installed {
		t.Errorf("Installed = true, want false")
	}
	if got.WorktreesDir != "" {
		t.Errorf("WorktreesDir = %q, want empty", got.WorktreesDir)
	}
}

func TestDetect_Installed_NoConfig_DefaultsWorktreesDir(t *testing.T) {
	got := herdr.Detect(lookPathFound, noEnv, "/home/walter", notFound, "linux")

	if !got.Installed {
		t.Fatalf("Installed = false, want true")
	}
	want := "/home/walter/.herdr/worktrees"
	if got.WorktreesDir != want {
		t.Errorf("WorktreesDir = %q, want %q", got.WorktreesDir, want)
	}
}

func TestDetect_Installed_KeySet_ReadsWorktreesDirectory(t *testing.T) {
	readFile := func(string) ([]byte, error) {
		return []byte("[worktrees]\ndirectory = \"c:/wt\"\n"), nil
	}
	got := herdr.Detect(lookPathFound, noEnv, "/home/walter", readFile, "linux")

	if !got.Installed {
		t.Fatalf("Installed = false, want true")
	}
	if got.WorktreesDir != "c:/wt" {
		t.Errorf("WorktreesDir = %q, want %q", got.WorktreesDir, "c:/wt")
	}
}

func TestDetect_Installed_KeyInOtherSection_Ignored(t *testing.T) {
	readFile := func(string) ([]byte, error) {
		return []byte("[other]\ndirectory = \"c:/wt\"\n[worktrees]\n"), nil
	}
	got := herdr.Detect(lookPathFound, noEnv, "/home/walter", readFile, "linux")

	if !got.Installed {
		t.Fatalf("Installed = false, want true")
	}
	want := "/home/walter/.herdr/worktrees"
	if got.WorktreesDir != want {
		t.Errorf("WorktreesDir = %q, want %q (directory key outside [worktrees] must be ignored)", got.WorktreesDir, want)
	}
}

func TestDetect_TildeExpansion(t *testing.T) {
	readFile := func(string) ([]byte, error) {
		return []byte("[worktrees]\ndirectory = \"~/wt\"\n"), nil
	}
	got := herdr.Detect(lookPathFound, noEnv, "/home/walter", readFile, "linux")

	want := "/home/walter/wt"
	if got.WorktreesDir != want {
		t.Errorf("WorktreesDir = %q, want %q", got.WorktreesDir, want)
	}
}

func TestDetect_Windows_ReadsConfigUnderAPPDATA(t *testing.T) {
	var requestedPath string
	readFile := func(path string) ([]byte, error) {
		requestedPath = path
		return nil, errors.New("not found")
	}
	getenv := func(name string) string {
		if name == "APPDATA" {
			return `C:\Users\walter\AppData\Roaming`
		}
		return ""
	}

	got := herdr.Detect(lookPathFound, getenv, `C:\Users\walter`, readFile, "windows")

	wantPath := `C:\Users\walter\AppData\Roaming\herdr\config.toml`
	if requestedPath != wantPath {
		t.Errorf("requested config path = %q, want %q", requestedPath, wantPath)
	}
	wantDir := `C:\Users\walter/.herdr/worktrees`
	if got.WorktreesDir != wantDir {
		t.Errorf("WorktreesDir = %q, want %q", got.WorktreesDir, wantDir)
	}
}

func TestDetect_UnquotedValue_TrailingComment_Stripped(t *testing.T) {
	readFile := func(string) ([]byte, error) {
		return []byte("[worktrees]\ndirectory = c:/wt # comment\n"), nil
	}
	got := herdr.Detect(lookPathFound, noEnv, "/home/walter", readFile, "linux")

	if got.WorktreesDir != "c:/wt" {
		t.Errorf("WorktreesDir = %q, want %q", got.WorktreesDir, "c:/wt")
	}
}

func TestDetect_SingleQuotedValue(t *testing.T) {
	readFile := func(string) ([]byte, error) {
		return []byte("[worktrees]\ndirectory = 'c:/wt'\n"), nil
	}
	got := herdr.Detect(lookPathFound, noEnv, "/home/walter", readFile, "linux")

	if got.WorktreesDir != "c:/wt" {
		t.Errorf("WorktreesDir = %q, want %q", got.WorktreesDir, "c:/wt")
	}
}

func TestDetect_DoubleQuotedValue_UnescapesBackslash(t *testing.T) {
	readFile := func(string) ([]byte, error) {
		return []byte(`[worktrees]` + "\n" + `directory = "D:\\.worktrees"` + "\n"), nil
	}
	got := herdr.Detect(lookPathFound, noEnv, "/home/walter", readFile, "linux")

	want := `D:\.worktrees`
	if got.WorktreesDir != want {
		t.Errorf("WorktreesDir = %q, want %q", got.WorktreesDir, want)
	}
}

func TestPattern_RelativeUnixStyleDirectory(t *testing.T) {
	got := herdr.Pattern("c:/wt")
	want := "c:/wt/{repo}/{slug}"
	if got != want {
		t.Errorf("Pattern() = %q, want %q", got, want)
	}
}

func TestPattern_WindowsBackslashDirectory(t *testing.T) {
	got := herdr.Pattern(`D:\.worktrees`)
	want := `D:\.worktrees\{repo}\{slug}`
	if got != want {
		t.Errorf("Pattern() = %q, want %q", got, want)
	}
}

func TestPattern_TrailingSeparatorNotDoubled(t *testing.T) {
	got := herdr.Pattern(`D:\.worktrees\`)
	want := `D:\.worktrees\{repo}\{slug}`
	if got != want {
		t.Errorf("Pattern() = %q, want %q", got, want)
	}
}

func TestPattern_NoSeparatorPresent_DefaultsToForwardSlash(t *testing.T) {
	got := herdr.Pattern("wt")
	want := "wt/{repo}/{slug}"
	if got != want {
		t.Errorf("Pattern() = %q, want %q", got, want)
	}
}
