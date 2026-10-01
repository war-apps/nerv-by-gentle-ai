package configure_test

import (
	"errors"
	"os"
	"path/filepath"
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
