package migrate_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
)

// An Apply that fails part way (here: odd/ is not writable, so the run
// files cannot move after the ODD document and .ai-workflow.yml were
// written) must leave a repository that either still has the old layout or
// that a second nerv migrate, once the cause is fixed, finishes. Today the
// second run refuses: "odd/tasks/<change>.md already exists".
func TestInterrupted_ApplyLeavesARepositoryASecondRunFinishes(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user")
	}
	w := newWorld(t, comprobantes)
	w.openspecFixture(true)
	before := treeHash(t, w.root)

	odd := filepath.Join(w.root, "odd")
	if err := os.MkdirAll(filepath.Join(odd, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(odd, 0o555); err != nil {
		t.Fatal(err)
	}
	p := w.plan(w.options(false, githubOrigin))
	_, err := migrate.Apply(p, now)
	if err := os.Chmod(odd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err == nil {
		t.Fatal("Apply succeeded with a read-only odd/; the probe did not interrupt it")
	}

	os.Remove(filepath.Join(odd, "tasks")) // only when still empty
	os.Remove(odd)
	if treeHash(t, w.root) == before {
		return // rolled back to the old layout: fine
	}
	p2, err := migrate.Compute(w.options(false, githubOrigin))
	if err != nil {
		t.Fatalf("after an interrupted Apply, the second nerv migrate refuses: %v", err)
	}
	if _, err := migrate.Apply(p2, now); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	for _, rel := range []string{"odd/tasks/tw-1-feature.md", "odd/tw-1-feature/nerv/state.yaml", ".nerv/locks/tw-1-feature.lock", ".ai-workflow.yml"} {
		if !w.exists(rel) {
			t.Errorf("%s is missing after the second run", rel)
		}
	}
	if w.exists("openspec/changes/tw-1-feature") {
		t.Error("the in-flight change folder is still there after the second run")
	}
}
