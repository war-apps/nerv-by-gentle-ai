package ecosystem

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/war-apps/nerv-kit-by-gentle-ai/runner/runnertest"
)

func testTool() Tool {
	return Tool{Name: "nerv-task", Repo: "war-apps/nerv-task-by-gentle-ai", Binary: "nerv-task"}
}

func noLookPath(string) (string, error) { return "", errNotFound }

type notFoundErr struct{}

func (notFoundErr) Error() string { return "executable file not found in $PATH" }

var errNotFound error = notFoundErr{}

func TestDefaultInstallDir_Windows(t *testing.T) {
	got := DefaultInstallDir(testTool(), "windows", `C:\Users\wal1rod`, func(k string) string {
		if k == "LOCALAPPDATA" {
			return `C:\Users\wal1rod\AppData\Local`
		}
		return ""
	})
	want := filepath.Join(`C:\Users\wal1rod\AppData\Local`, "nerv-task", "bin")
	if got != want {
		t.Errorf("DefaultInstallDir() = %q, want %q", got, want)
	}
}

func TestDefaultInstallDir_WindowsFallsBackWhenLocalAppDataUnset(t *testing.T) {
	got := DefaultInstallDir(testTool(), "windows", `C:\Users\wal1rod`, func(string) string { return "" })
	want := filepath.Join(`C:\Users\wal1rod`, "AppData", "Local", "nerv-task", "bin")
	if got != want {
		t.Errorf("DefaultInstallDir() = %q, want %q", got, want)
	}
}

func TestDefaultInstallDir_Posix(t *testing.T) {
	got := DefaultInstallDir(testTool(), "linux", "/home/walter", func(string) string { return "" })
	want := filepath.Join("/home/walter", ".local", "bin")
	if got != want {
		t.Errorf("DefaultInstallDir() = %q, want %q", got, want)
	}
}

func TestDetect_NotFoundAnywhere(t *testing.T) {
	home := t.TempDir()
	run := &runnertest.FakeRunner{}
	state := Detect(context.Background(), testTool(), run, noLookPath, "linux", home, func(string) string { return "" })
	if state.Installed {
		t.Errorf("Installed = true, want false (not on PATH, not in default dir)")
	}
	if len(run.Calls) != 0 {
		t.Errorf("version --json was run %d times, want 0 when not installed", len(run.Calls))
	}
}

func TestDetect_FoundOnPath_ParsesVersionJSON(t *testing.T) {
	home := t.TempDir()
	lookPath := func(name string) (string, error) {
		if name == "nerv-task" {
			return "/usr/local/bin/nerv-task", nil
		}
		return "", errNotFound
	}
	run := &runnertest.FakeRunner{
		Responses: map[string]runnertest.Response{
			"/usr/local/bin/nerv-task version --json": {Stdout: `{"binary":"1.1.0","plugin":"1.1.0"}` + "\n", ExitCode: 0},
		},
	}
	state := Detect(context.Background(), testTool(), run, lookPath, "linux", home, func(string) string { return "" })
	if !state.Installed {
		t.Fatal("Installed = false, want true")
	}
	if state.Path != "/usr/local/bin/nerv-task" {
		t.Errorf("Path = %q, want /usr/local/bin/nerv-task", state.Path)
	}
	if state.Version != "1.1.0" {
		t.Errorf("Version = %q, want 1.1.0", state.Version)
	}
	if state.VersionUnknown {
		t.Error("VersionUnknown = true, want false")
	}
}

func TestDetect_FoundInDefaultDirWhenNotOnPath(t *testing.T) {
	home := t.TempDir()
	dir := DefaultInstallDir(testTool(), "linux", home, func(string) string { return "" })
	binPath := filepath.Join(dir, "nerv-task")
	writeExecutable(t, binPath)

	run := &runnertest.FakeRunner{
		Responses: map[string]runnertest.Response{
			binPath + " version --json": {Stdout: `{"binary":"1.1.0","plugin":"1.1.0"}`, ExitCode: 0},
		},
	}
	state := Detect(context.Background(), testTool(), run, noLookPath, "linux", home, func(string) string { return "" })
	if !state.Installed {
		t.Fatal("Installed = false, want true (found in default dir)")
	}
	if state.Path != binPath {
		t.Errorf("Path = %q, want %q", state.Path, binPath)
	}
}

func TestDetect_OlderBinaryWithoutVersionJSON_IsVersionUnknown(t *testing.T) {
	home := t.TempDir()
	lookPath := func(string) (string, error) { return "/usr/local/bin/nerv-task", nil }
	run := &runnertest.FakeRunner{
		Default: runnertest.Response{ExitCode: 2, Stderr: "unknown flag: --json"},
	}
	state := Detect(context.Background(), testTool(), run, lookPath, "linux", home, func(string) string { return "" })
	if !state.Installed {
		t.Fatal("Installed = false, want true")
	}
	if !state.VersionUnknown {
		t.Error("VersionUnknown = false, want true (older release, no --json support)")
	}
	if state.Version != "" {
		t.Errorf("Version = %q, want empty", state.Version)
	}
}

func TestDetect_UnparseableJSON_IsVersionUnknown(t *testing.T) {
	home := t.TempDir()
	lookPath := func(string) (string, error) { return "/usr/local/bin/nerv-task", nil }
	run := &runnertest.FakeRunner{
		Default: runnertest.Response{Stdout: "not json", ExitCode: 0},
	}
	state := Detect(context.Background(), testTool(), run, lookPath, "linux", home, func(string) string { return "" })
	if !state.VersionUnknown {
		t.Error("VersionUnknown = false, want true")
	}
}
