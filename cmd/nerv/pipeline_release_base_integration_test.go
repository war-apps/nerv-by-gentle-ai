//go:build semrel_integration

package main

import (
	"bytes"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
)

// TestIntegration_PipelineReleaseRecordsItsBaseCommit: with real
// semantic-release (built-in configuration, which makes a release commit),
// release.json's base_commit is origin's branch head the release was made
// from, the parent of git_head; in a dry run (no release commit) both are
// that head.
func TestIntegration_PipelineReleaseRecordsItsBaseCommit(t *testing.T) {
	requireShipToolchain(t)
	shipEnv(t)
	cache := t.TempDir()
	origin, work := newShipRepos(t)
	work.commit("chore: init", "a.txt")
	work.git("tag", "v1.0.0")
	work.commit("feat: add gadgets", "b.txt")
	work.git("push", "-q", "origin", "main", "--tags")
	work.write(".nerv/nerv.yaml", "pipeline:\n  dotnet:\n    solution: App.slnx\n  images:\n    - name: ghcr.io/acme/app\n")
	base := origin.git("rev-parse", "main")
	release := func(args ...string) pipeline.ReleaseOutputs {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if code := run(append([]string{"pipeline", "release", "--cwd", work.dir}, args...), &stdout, &stderr, shipOptions(t, cache)); code != 0 {
			t.Fatalf("release %v: exit %d\n%s", args, code, stderr.String())
		}
		o, err := pipeline.ReadReleaseOutputs(work.dir)
		if err != nil {
			t.Fatal(err)
		}
		return o
	}

	if o := release("--dry-run"); o.BaseCommit != base || o.GitHead != base {
		t.Errorf("dry run: base_commit %s git_head %s, want both %s", o.BaseCommit, o.GitHead, base)
	}
	o := release("--yes")
	head := origin.git("rev-parse", "main")
	if o.BaseCommit != base || o.GitHead != head || head == base || origin.git("rev-parse", head+"^") != base {
		t.Errorf("release: base_commit %s git_head %s; origin main %s, its parent %s; want base %s", o.BaseCommit, o.GitHead, head, origin.git("rev-parse", head+"^"), base)
	}
}
