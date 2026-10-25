package migrate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
)

// S9: nerv migrate moves the previous .ai-workflow.yml to
// .ai-workflow/config.yaml, transactionally: a dry run writes nothing,
// Apply keeps a backup of the old file, and a second run is a no-op.
func TestPrevious_ProjectFileMovesToTheFolder(t *testing.T) {
	w := world{t: t, root: filepath.Join(t.TempDir(), "repo"), home: t.TempDir()}
	old := "# .ai-workflow.yml -- NERV project configuration\nenabled: true\ngit:\n  base_branch: develop\n"
	w.write(".ai-workflow.yml", old)
	before := treeHash(t, w.root)

	p := w.plan(w.options(false, ""))
	diff := migrate.Render(p)
	contains(t, "the rendered plan", diff, "--- /dev/null\n+++ b/.ai-workflow/config.yaml\n", "remove .ai-workflow.yml\n")
	if treeHash(t, w.root) != before {
		t.Fatal("planning changed the files")
	}

	_, backups := w.migrate(w.options(false, ""))
	want := "# .ai-workflow/config.yaml -- NERV project configuration\nenabled: true\ngit:\n  base_branch: develop\n"
	if got := w.read(".ai-workflow/config.yaml"); got != want {
		t.Errorf(".ai-workflow/config.yaml =\n%s\nwant\n%s", got, want)
	}
	if w.exists(".ai-workflow.yml") {
		t.Error(".ai-workflow.yml is still there")
	}
	backup := filepath.Join(w.root, ".nerv", "backups", "migrate-"+now.Format("20060102-150405"), ".ai-workflow.yml")
	if data, err := os.ReadFile(backup); err != nil || string(data) != old {
		t.Errorf("backup %s = %q, %v; want the old file", backup, data, err)
	}
	found := false
	for _, b := range backups {
		found = found || b == backup
	}
	if !found {
		t.Errorf("backups %q lack %s", backups, backup)
	}
	if _, err := workflowconfig.Load(w.home, w.root); err != nil {
		t.Errorf("the migrated repository does not load: %v", err)
	}

	after := treeHash(t, w.root)
	p = w.plan(w.options(false, ""))
	if !p.Empty() {
		t.Errorf("the second plan has changes:\n%s", migrate.Render(p))
	}
	if _, err := migrate.Apply(p, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if treeHash(t, w.root) != after {
		t.Error("the second run changed files")
	}
}

// A 4.2.0 release: section in the previous file is converted on the way.
func TestPrevious_LegacyReleaseSectionIsConvertedAndMoved(t *testing.T) {
	w := world{t: t, root: filepath.Join(t.TempDir(), "repo"), home: t.TempDir()}
	w.write(".ai-workflow.yml", "enabled: true\nrelease:\n  semantic_release: { version: 24.2.3 }\n  projects:\n    - name: api\n      path: .\n      presets: [dotnet]\n      dotnet: { project_file: Api.csproj }\n")
	w.migrate(w.options(false, ""))
	text := w.read(".ai-workflow/config.yaml")
	contains(t, ".ai-workflow/config.yaml", text, "\nrelease:", "build: bash")
	lacks(t, ".ai-workflow/config.yaml", text, "presets:", "semantic_release:")
	if w.exists(".ai-workflow.yml") {
		t.Error(".ai-workflow.yml is still there")
	}
}

// The previous and the new project file both existing is refused, unless
// they are identical, and nothing is written.
func TestPrevious_BothProjectFiles(t *testing.T) {
	w := world{t: t, root: filepath.Join(t.TempDir(), "repo"), home: t.TempDir()}
	w.write(".ai-workflow.yml", "enabled: true\n")
	w.write(".ai-workflow/config.yaml", "enabled: false\n")
	before := treeHash(t, w.root)
	_, err := migrate.Compute(w.options(false, ""))
	if !refusal.Is(err) || !strings.Contains(err.Error(), "both exist") {
		t.Fatalf("Compute = %v, want the both-exist refusal", err)
	}
	if treeHash(t, w.root) != before {
		t.Error("a refused migration changed files")
	}

	w.write(".ai-workflow/config.yaml", "enabled: true\n")
	w.migrate(w.options(false, ""))
	if w.exists(".ai-workflow.yml") || w.read(".ai-workflow/config.yaml") != "enabled: true\n" {
		t.Error("identical files: want the previous one removed and the new one unchanged")
	}
}

// S9 / L15: with --user, the previous user config.yml becomes config.yaml
// and stays as config.yml.migrated, backed up next to it; a second run is
// a no-op.
func TestPrevious_UserFileMovesToConfigYAML(t *testing.T) {
	w := world{t: t, root: filepath.Join(t.TempDir(), "repo"), home: t.TempDir()}
	up := w.userPaths()
	old := "# ai-workflow/config.yml -- NERV user-scope configuration\ngit:\n  worktree: ask\n"
	w.write(up.PreviousUserConfig, old)
	before := treeHash(t, w.home)
	o := w.options(true, "")
	o.RepoRoot = ""
	p := w.plan(o)
	contains(t, "the rendered plan", migrate.Render(p), "move "+up.PreviousUserConfig+" -> "+up.PreviousUserConfig+migrate.MigratedSuffix)
	if treeHash(t, w.home) != before {
		t.Fatal("planning changed the files")
	}

	_, backups := w.migrate(o)
	if got, want := w.read(up.UserConfig), "# ai-workflow/config.yaml -- NERV user-scope configuration\ngit:\n  worktree: ask\n"; got != want {
		t.Errorf("config.yaml =\n%s\nwant\n%s", got, want)
	}
	if w.exists(up.PreviousUserConfig) || w.read(up.PreviousUserConfig+migrate.MigratedSuffix) != old {
		t.Error("want config.yml kept only as config.yml.migrated")
	}
	backup := up.PreviousUserConfig + "." + migrate.BackupSuffix + now.Format("20060102-150405")
	found := false
	for _, b := range backups {
		found = found || b == backup
	}
	if !found {
		t.Errorf("backups %q lack %s", backups, backup)
	}

	if p := w.plan(o); !p.Empty() {
		t.Errorf("the second plan has changes:\n%s", migrate.Render(p))
	}
}
