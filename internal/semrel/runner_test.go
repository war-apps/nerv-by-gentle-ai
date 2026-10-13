package semrel

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// TestRunnerLib_NodeUnitTests runs the node unit tests of the runner's pure
// logic (runner/nerv-semrel-lib.test.mjs). It skips when node is not in PATH,
// so `go test ./...` stays green without Node.js.
func TestRunnerLib_NodeUnitTests(t *testing.T) {
	if testing.Short() {
		t.Skip("runs node")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// The runner itself needs semantic-release; check its syntax at least.
	if out, err := exec.CommandContext(ctx, node, "--check", "runner/"+runnerMain).CombinedOutput(); err != nil {
		t.Fatalf("node --check %s: %v\n%s", runnerMain, err, out)
	}
	out, err := exec.CommandContext(ctx, node, "--test", "runner/nerv-semrel-lib.test.mjs").CombinedOutput()
	if err != nil {
		t.Fatalf("node --test: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}

func TestWriteRunner_WritesTheRunnerAndItsModule(t *testing.T) {
	dir := t.TempDir()
	if err := writeRunner(dir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if want := []string{"nerv-semrel-lib.mjs", runnerMain}; !slices.Equal(names, want) {
		t.Errorf("runner files %v, want %v (never the tests)", names, want)
	}
}

func TestCompute_ProjectsShareOneCommitFilesCache(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `release:
  projects:
    - { name: api, path: api }
    - { name: web, path: web }
`)
	var runs []runnerRequest
	f := fakeRepo(t, root, func(req runnerRequest) (Result, error) {
		if req.Mode == "plan" {
			return okPlan()
		}
		runs = append(runs, req)
		return respond(map[string]any{"ok": true})
	})
	if _, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].CommitFilesCache == "" || runs[0].CommitFilesCache != runs[1].CommitFilesCache {
		t.Fatalf("commit files caches %q: want one shared file", []string{runs[0].CommitFilesCache, runs[1].CommitFilesCache})
	}
	if filepath.Dir(runs[0].CommitFilesCache) != filepath.Dir(runs[0].RepositoryURL) {
		t.Errorf("commit files cache %s: want it in the disposable checkout next to %s", runs[0].CommitFilesCache, runs[0].RepositoryURL)
	}
}
