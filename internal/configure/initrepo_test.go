package configure_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
)

func gitToplevelRunner(repoPath string) *envtest.FakeRunner {
	return &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"git -C " + repoPath + " rev-parse --show-toplevel": {Stdout: repoPath + "\n"},
	}}
}

// ---------------------------------------------------------------------------
// initrepo: creates <repo>/.nerv/nerv.yaml with the given fields; running it
// again reports "already initialized" and writes nothing.
// ---------------------------------------------------------------------------

func TestInitRepo_CreatesProjectConfig(t *testing.T) {
	repoDir := t.TempDir()
	deps := configure.Deps{Runner: gitToplevelRunner(repoDir), Now: func() time.Time { return time.Now() }}

	result, err := configure.InitRepo(deps, configure.InitRepoRequest{
		Path: repoDir, Base: "develop", Provider: "teamwork", ProjectID: "111", TasklistID: "222",
	})
	if err != nil {
		t.Fatalf("InitRepo() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want true")
	}

	repoConfigPath := filepath.Join(repoDir, ".nerv", "nerv.yaml")
	content, err := os.ReadFile(repoConfigPath)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "enabled: true") {
		t.Error("missing enabled: true")
	}
	if !strings.Contains(text, "base_branch: develop") {
		t.Error("missing base_branch")
	}
	if !strings.Contains(text, "project_id: 111") {
		t.Error("missing project_id")
	}
	if !strings.Contains(text, "tasklist_id: 222") {
		t.Error("missing tasklist_id")
	}
	if result.ConfigPath != repoConfigPath {
		t.Errorf("ConfigPath = %q, want %q", result.ConfigPath, repoConfigPath)
	}
	if len(result.Written) != 1 || result.Written[0] != repoConfigPath {
		t.Errorf("Written = %v, want [%s]", result.Written, repoConfigPath)
	}
}

func TestInitRepo_AlreadyInitialized_WarnsNoOverwrite(t *testing.T) {
	repoDir := t.TempDir()
	deps := configure.Deps{Runner: gitToplevelRunner(repoDir), Now: func() time.Time { return time.Now() }}

	if _, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: repoDir, Base: "develop"}); err != nil {
		t.Fatalf("InitRepo() (first) error = %v", err)
	}
	repoConfigPath := filepath.Join(repoDir, ".nerv", "nerv.yaml")
	before, err := os.ReadFile(repoConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	result, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: repoDir, Base: "develop3"})
	if err != nil {
		t.Fatalf("InitRepo() (second) error = %v", err)
	}
	if result.Changed {
		t.Error("Changed = true, want false (already initialized)")
	}
	if len(result.Warnings) == 0 {
		t.Error("Warnings is empty, want a warning naming the already-initialized file")
	}

	after, err := os.ReadFile(repoConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Error("file was overwritten on a second --init-repo")
	}
}

// ---------------------------------------------------------------------------
// initrepo against a non-git path: refused.
// ---------------------------------------------------------------------------

func TestInitRepo_NotGitRepo_Refused(t *testing.T) {
	dir := t.TempDir()
	deps := configure.Deps{
		Runner: &envtest.FakeRunner{}, // no scripted response -> exit 0, empty stdout
		Now:    func() time.Time { return time.Now() },
	}

	_, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: dir})
	if err == nil {
		t.Fatal("InitRepo() error = nil, want a refusal")
	}
	var refusal *configure.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v, want *RefusalError", err)
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("error message %q does not name the path", err.Error())
	}
}

// ---------------------------------------------------------------------------
// initrepo against a missing path: refused.
// ---------------------------------------------------------------------------

func TestInitRepo_PathNotFound_Refused(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	deps := configure.Deps{Runner: &envtest.FakeRunner{}, Now: func() time.Time { return time.Now() }}

	_, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: missing})
	if err == nil {
		t.Fatal("InitRepo() error = nil, want a refusal")
	}
	var refusal *configure.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v, want *RefusalError", err)
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error message %q does not name the path", err.Error())
	}
}

// ---------------------------------------------------------------------------
// initrepo with an invalid --repo-provider: refused.
// ---------------------------------------------------------------------------

func TestInitRepo_InvalidProvider_Refused(t *testing.T) {
	repoDir := t.TempDir()
	deps := configure.Deps{Runner: gitToplevelRunner(repoDir), Now: func() time.Time { return time.Now() }}

	_, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: repoDir, Provider: "bogus-provider"})
	if err == nil {
		t.Fatal("InitRepo() error = nil, want a refusal")
	}
	var refusal *configure.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v, want *RefusalError", err)
	}
	if !strings.Contains(err.Error(), "bogus-provider") {
		t.Errorf("error message %q does not name the provider", err.Error())
	}

	if _, statErr := os.Stat(filepath.Join(repoDir, ".nerv", "nerv.yaml")); statErr == nil {
		t.Error("file was created despite the invalid provider")
	}
}

// The removed stub providers are refused like any other unknown value.
func TestInitRepo_RemovedStubProviders_Refused(t *testing.T) {
	for _, provider := range []string{"github-projects", "jira"} {
		t.Run(provider, func(t *testing.T) {
			repoDir := t.TempDir()
			deps := configure.Deps{Runner: gitToplevelRunner(repoDir), Now: func() time.Time { return time.Now() }}

			_, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: repoDir, Provider: provider})
			var refusal *configure.RefusalError
			if !errors.As(err, &refusal) {
				t.Fatalf("error = %v, want *RefusalError", err)
			}
			if !strings.Contains(err.Error(), provider) {
				t.Errorf("error message %q does not name the provider", err.Error())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// initrepo against an existing project config: the removed task providers are
// cleaned (with a backup and a Removed report); a clean file is untouched.
// ---------------------------------------------------------------------------

const legacyProjectConfig = "enabled: true\n" +
	"tasks:\n" +
	"  provider: jira   # old\n" +
	"  providers:\n" +
	"    teamwork:\n" +
	"      project_id: 111\n" +
	"    github-projects: { owner: x }\n" +
	"    jira: { site: x }\n"

const cleanedProjectConfig = "enabled: true\n" +
	"tasks:\n" +
	"  providers:\n" +
	"    teamwork:\n" +
	"      project_id: 111\n"

func writeProjectConfig(t *testing.T, repoDir, content string) string {
	t.Helper()
	path := filepath.Join(repoDir, ".nerv", "nerv.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInitRepo_ExistingLegacyConfig_IsCleanedWithBackup(t *testing.T) {
	repoDir := t.TempDir()
	path := writeProjectConfig(t, repoDir, legacyProjectConfig)
	deps := configure.Deps{Runner: gitToplevelRunner(repoDir), Now: func() time.Time { return time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC) }}

	result, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: repoDir})
	if err != nil {
		t.Fatalf("InitRepo() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != cleanedProjectConfig {
		t.Errorf("file =\n%s\nwant\n%s", got, cleanedProjectConfig)
	}
	if !result.Changed {
		t.Error("Changed = false, want true")
	}
	if len(result.Written) != 1 || result.Written[0] != path {
		t.Errorf("Written = %v, want [%s]", result.Written, path)
	}
	want := []string{"tasks.provider (jira)", "tasks.providers.github-projects", "tasks.providers.jira"}
	if !slices.Equal(result.Removed, want) {
		t.Errorf("Removed = %v, want %v", result.Removed, want)
	}
	if result.Backup == nil {
		t.Fatal("Backup = nil, want the backup path")
	}
	backup, err := os.ReadFile(*result.Backup)
	if err != nil {
		t.Fatalf("backup not written: %v", err)
	}
	if string(backup) != legacyProjectConfig {
		t.Errorf("backup does not hold the original file:\n%s", backup)
	}
	if !strings.HasPrefix(*result.Backup, path+".bak-configure-") {
		t.Errorf("Backup = %q, want the %q convention", *result.Backup, path+".bak-configure-<timestamp>")
	}
}

func TestInitRepo_ExistingCleanConfig_IsUntouchedWithoutBackup(t *testing.T) {
	repoDir := t.TempDir()
	path := writeProjectConfig(t, repoDir, cleanedProjectConfig)
	deps := configure.Deps{Runner: gitToplevelRunner(repoDir), Now: func() time.Time { return time.Now() }}

	result, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: repoDir})
	if err != nil {
		t.Fatalf("InitRepo() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != cleanedProjectConfig {
		t.Errorf("clean file was rewritten:\n%s", got)
	}
	if result.Changed || len(result.Written) != 0 || len(result.Removed) != 0 || result.Backup != nil {
		t.Errorf("result = %+v, want nothing changed, written, removed or backed up", result)
	}
	if len(result.Warnings) == 0 {
		t.Error("Warnings is empty, want the already-initialized warning")
	}
	backups, _ := filepath.Glob(path + ".bak-*")
	if len(backups) != 0 {
		t.Errorf("backups = %v, want none", backups)
	}
}

func TestInitRepo_UnreadableExistingConfig_WarnsInsteadOfFailing(t *testing.T) {
	repoDir := t.TempDir()
	path := filepath.Join(repoDir, ".nerv", "nerv.yaml")
	// A directory at the config path exists for os.Stat but cannot be read
	// as a file, like an unreadable config.
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	deps := configure.Deps{Runner: gitToplevelRunner(repoDir), Now: func() time.Time { return time.Now() }}

	result, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: repoDir})
	if err != nil {
		t.Fatalf("InitRepo() error = %v, want a warning instead of a failure", err)
	}
	if result.Changed || len(result.Written) != 0 || len(result.Removed) != 0 {
		t.Errorf("result = %+v, want nothing changed, written or removed", result)
	}
	if len(result.Warnings) == 0 {
		t.Error("Warnings is empty, want one naming the unreadable config")
	}
}
