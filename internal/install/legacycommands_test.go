package install_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/install"
)

// seedLegacyCommands writes the named files under <home>/.claude/commands/task
// and returns that directory.
func seedLegacyCommands(t *testing.T, home string, names ...string) string {
	t.Helper()
	dir := filepath.Join(home, ".claude", "commands", "task")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("old copy\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func legacyInstall(t *testing.T, home string, confirm func([]string) bool) string {
	t.Helper()
	seedInstalledPlugins(t, home, pluginVersion(t))
	var stdout bytes.Buffer
	deps := install.Deps{
		Home: home, FS: nerv.PluginFS(), Runner: baseRunner(), Now: fixedNow, LookPath: lookPathAll,
		ConfirmRemoveLegacyCommands: confirm, Stdout: &stdout,
	}
	if err := install.Install(context.Background(), deps, install.Options{NoSkills: true}); err != nil {
		t.Fatalf("Install() error = %v; output:\n%s", err, stdout.String())
	}
	return stdout.String()
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// S7: without a confirmer the old copies are listed with a remedy line, and
// nothing is deleted.
func TestInstall_LegacyCommands_ReportedWithRemedyWhenNotConfirmable(t *testing.T) {
	home := t.TempDir()
	dir := seedLegacyCommands(t, home, "start.md", "stopAll.md")

	out := legacyInstall(t, home, nil)

	for _, want := range []string{
		filepath.Join(dir, "start.md"),
		filepath.Join(dir, "stopAll.md"),
		"shadow the task@nerv plugin's /task:* commands and can be removed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if got := dirNames(t, dir); !slices.Equal(got, []string{"start.md", "stopAll.md"}) {
		t.Errorf("files left = %v, want both copies kept", got)
	}
}

// S7: a confirmed removal deletes exactly the listed copies, keeping any
// other file and therefore the directory.
func TestInstall_LegacyCommands_ConfirmedRemovesOnlyListedCopies(t *testing.T) {
	home := t.TempDir()
	dir := seedLegacyCommands(t, home, "close.md", "timers.md", "mine.md", "start.txt")

	var asked []string
	out := legacyInstall(t, home, func(files []string) bool {
		asked = files
		return true
	})

	want := []string{filepath.Join(dir, "close.md"), filepath.Join(dir, "timers.md")}
	if !slices.Equal(asked, want) {
		t.Errorf("confirmer got %v, want %v", asked, want)
	}
	if got := dirNames(t, dir); !slices.Equal(got, []string{"mine.md", "start.txt"}) {
		t.Errorf("files left = %v, want only the files that are not old copies", got)
	}
	if !strings.Contains(out, "Removed 2 old /task:* command copies") {
		t.Errorf("output missing the removal line:\n%s", out)
	}
}

// S7: removing the last copy removes the emptied directory too.
func TestInstall_LegacyCommands_ConfirmedRemovesEmptiedDirectory(t *testing.T) {
	home := t.TempDir()
	dir := seedLegacyCommands(t, home, "all.md", "me.md")

	legacyInstall(t, home, func([]string) bool { return true })

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("directory %s still exists (err=%v)", dir, err)
	}
}

// S7: a declined removal keeps the copies and prints the remedy line.
func TestInstall_LegacyCommands_DeclinedKeepsCopies(t *testing.T) {
	home := t.TempDir()
	dir := seedLegacyCommands(t, home, "take.md")

	out := legacyInstall(t, home, func([]string) bool { return false })

	if got := dirNames(t, dir); !slices.Equal(got, []string{"take.md"}) {
		t.Errorf("files left = %v, want the copy kept", got)
	}
	if !strings.Contains(out, "can be removed") {
		t.Errorf("output missing the remedy line:\n%s", out)
	}
}

// With no old copies, the confirmer is never asked and nothing is printed.
func TestInstall_LegacyCommands_NoneFoundIsSilent(t *testing.T) {
	home := t.TempDir()
	seedLegacyCommands(t, home, "custom.md")

	out := legacyInstall(t, home, func([]string) bool {
		t.Error("confirmer asked with no old copies")
		return true
	})
	if strings.Contains(out, "/task:*") {
		t.Errorf("output mentions old copies when there are none:\n%s", out)
	}
}

// L8 gap: RefreshCache also refreshes every enabled extension, at its own
// embedded version, and leaves the ones not enabled alone.
func TestRefreshCache_RefreshesEnabledExtensions(t *testing.T) {
	home := t.TempDir()
	seedInstalledIDs(t, home, pluginVersion(t), "task@nerv")
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.WriteFile(settings, []byte(`{"enabledPlugins":{"nerv@nerv":true,"task@nerv":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := baseRunner()
	var stdout bytes.Buffer
	deps := install.Deps{Home: home, FS: nerv.PluginFS(), ExtensionsFS: fakeExtensions(), Runner: runner, Now: fixedNow, LookPath: lookPathAll, Stdout: &stdout}

	if err := install.RefreshCache(context.Background(), deps); err != nil {
		t.Fatalf("RefreshCache() error = %v; output:\n%s", err, stdout.String())
	}
	log := runnerLog(runner.Calls)
	if !strings.Contains(log, "claude plugin install task@nerv") {
		t.Errorf("enabled extension not refreshed: %s", log)
	}
	if strings.Contains(log, "task-teamwork@nerv") {
		t.Errorf("an extension that is not enabled was refreshed: %s", log)
	}
}
