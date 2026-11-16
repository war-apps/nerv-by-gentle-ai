package ecosystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/war-apps/nerv-kit-by-gentle-ai/runner"
)

// Info is one tool's "<tool> version --json" output: version.Info's
// shape, shared across the ecosystem.
type Info struct {
	Binary string `json:"binary"`
	Plugin string `json:"plugin"`
}

// State is one tool's installed state on this machine.
type State struct {
	// Installed reports whether Binary was found, on PATH or in its
	// default install directory.
	Installed bool
	// Path is where it was found ("" when not Installed).
	Path string
	// Version is its binary version, from "<path> version --json"; ""
	// when Installed is false or VersionUnknown is true.
	Version string
	// VersionUnknown is true when Installed is true but "version --json"
	// failed or did not parse: an older release published before S10
	// added the version contract (1.1.0). Never treated as not
	// installed.
	VersionUnknown bool
}

// DefaultInstallDir is where t's official install script places Binary
// when run with no override: "<LOCALAPPDATA>\<tool>\bin" on Windows
// (getenv("LOCALAPPDATA"), falling back to "<home>\AppData\Local" when
// unset), "<home>/.local/bin" everywhere else.
func DefaultInstallDir(t Tool, goos, home string, getenv func(string) string) string {
	if goos == "windows" {
		dir := getenv("LOCALAPPDATA")
		if dir == "" {
			dir = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(dir, t.Name, "bin")
	}
	return filepath.Join(home, ".local", "bin")
}

// binaryFile is Binary's file name for goos: with the .exe suffix on
// Windows, bare everywhere else.
func binaryFile(t Tool, goos string) string {
	if goos == "windows" {
		return t.Binary + ".exe"
	}
	return t.Binary
}

// Detect reports t's installed state: lookPath(t.Binary) first, then its
// default install directory (DefaultInstallDir). Installed is true as
// soon as the binary is found either way; State.Version then comes from
// running "<path> version --json" through run -- a non-zero exit or
// unparseable output only sets VersionUnknown, never Installed to false.
func Detect(ctx context.Context, t Tool, run runner.Runner, lookPath func(string) (string, error), goos, home string, getenv func(string) string) State {
	path, found := resolveBinary(t, lookPath, goos, home, getenv)
	if !found {
		return State{}
	}
	state := State{Installed: true, Path: path}

	stdout, _, exitCode, err := run.Run(ctx, path, "version", "--json")
	if err != nil || exitCode != 0 {
		state.VersionUnknown = true
		return state
	}
	var info Info
	if jsonErr := json.Unmarshal([]byte(stdout), &info); jsonErr != nil || info.Binary == "" {
		state.VersionUnknown = true
		return state
	}
	state.Version = info.Binary
	return state
}

// resolveBinary finds t.Binary on PATH, then in its default install
// directory, reporting the path found and whether either succeeded.
func resolveBinary(t Tool, lookPath func(string) (string, error), goos, home string, getenv func(string) string) (string, bool) {
	if lookPath != nil {
		if p, err := lookPath(t.Binary); err == nil && p != "" {
			return p, true
		}
	}
	candidate := filepath.Join(DefaultInstallDir(t, goos, home, getenv), binaryFile(t, goos))
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate, true
	}
	return "", false
}
