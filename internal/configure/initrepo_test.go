package configure_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
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
	deps := configure.Deps{Runner: gitToplevelRunner(repoDir), Now: func() time.Time { return time.Now() }, ExtensionsFS: nerv.ExtensionsFS()}

	result, err := configure.InitRepo(deps, configure.InitRepoRequest{
		Path: repoDir, Base: "develop", ProviderConfig: []string{"teamwork.project_id=111", "teamwork.tasklist_id=222"},
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
	doc := config.Parse(text)
	for key, want := range map[string]string{
		"tasks.providers.teamwork.project_id":  "111",
		"tasks.providers.teamwork.tasklist_id": "222",
	} {
		if got, _ := doc.Scalar(key); got != want {
			t.Errorf("%s = %q, want %q:\n%s", key, got, want, text)
		}
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
// initrepo with an invalid provider config entry: refused before anything is
// written, naming the problem.
// ---------------------------------------------------------------------------

func TestInitRepo_InvalidProviderConfig_Refused(t *testing.T) {
	cases := map[string]string{
		"jira.site=x":             "Unknown task provider 'jira' in --repo-provider-config: no embedded manifest (available: teamwork).",
		"teamwork.bogus=1":        "Unknown key 'bogus' for task provider 'teamwork'.",
		"teamwork.project_id=1.5": "Invalid value '1.5' for tasks.providers.teamwork.project_id: expected int.",
		"=1":                      "Invalid --repo-provider-config entry '=1'; expected <provider>.<key>=<value>.",
	}
	for entry, want := range cases {
		t.Run(entry, func(t *testing.T) {
			repoDir := t.TempDir()
			deps := configure.Deps{Runner: gitToplevelRunner(repoDir), Now: func() time.Time { return time.Now() }, ExtensionsFS: nerv.ExtensionsFS()}

			_, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: repoDir, ProviderConfig: []string{entry}})
			var refusal *configure.RefusalError
			if !errors.As(err, &refusal) {
				t.Fatalf("error = %v, want *RefusalError", err)
			}
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), want)
			}
			if _, statErr := os.Stat(filepath.Join(repoDir, ".nerv", "nerv.yaml")); statErr == nil {
				t.Error("file was created despite the invalid entry")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// S7: initrepo against an existing project config leaves it untouched, a
// tasks: block written by an earlier release included.
// ---------------------------------------------------------------------------

const legacyProjectConfig = "enabled: true\n" +
	"tasks:\n" +
	"  provider: jira   # old\n" +
	"  providers:\n" +
	"    teamwork:\n" +
	"      project_id: 111\n" +
	"    github-projects: { owner: x }\n" +
	"    jira: { site: x }\n"

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

func TestInitRepo_ExistingConfigWithTasksBlock_IsUntouchedWithoutBackup(t *testing.T) {
	repoDir := t.TempDir()
	path := writeProjectConfig(t, repoDir, legacyProjectConfig)
	deps := configure.Deps{Runner: gitToplevelRunner(repoDir), Now: func() time.Time { return time.Now() }, ExtensionsFS: nerv.ExtensionsFS()}

	result, err := configure.InitRepo(deps, configure.InitRepoRequest{Path: repoDir, ProviderConfig: []string{"teamwork.project_id=222"}})
	if err != nil {
		t.Fatalf("InitRepo() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != legacyProjectConfig {
		t.Errorf("existing file was rewritten:\n%s", got)
	}
	if result.Changed || len(result.Written) != 0 || result.Backup != nil {
		t.Errorf("result = %+v, want nothing changed, written or backed up", result)
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
	if result.Changed || len(result.Written) != 0 {
		t.Errorf("result = %+v, want nothing changed or written", result)
	}
	if len(result.Warnings) == 0 {
		t.Error("Warnings is empty, want one naming the unreadable config")
	}
}
