//go:build semrel_integration

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
)

// TestIntegration_PipelineSyncMergesBackAfterAMainRelease: real git and a
// local bare origin. After a release on main, sync merges main into
// develop and into release/a (pushed) and aborts the conflicting merge into
// release/b (reported, nothing pushed, exit 2), with dim's messages and the
// user's own identity. A dry run first changes nothing.
func TestIntegration_PipelineSyncMergesBackAfterAMainRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	shipEnv(t)
	// The identity must come from the user's repository, not from GIT_*.
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
		os.Unsetenv(k)
	}
	origin, work := newShipRepos(t)
	work.git("config", "user.name", "Sync User")
	work.git("config", "user.email", "sync@example.com")
	work.commit("feat: base", "app.txt", "CHANGELOG.md")
	for _, b := range []string{"develop", "release/a", "release/b"} {
		work.git("branch", b)
	}
	work.git("checkout", "-q", "develop")
	work.commit("feat: next", "next.txt")
	work.git("checkout", "-q", "release/b")
	work.commit("fix: rc notes", "CHANGELOG.md")
	work.git("checkout", "-q", "main")
	work.commit("chore(release): 1.1.0 [skip ci]", "CHANGELOG.md")
	work.git("tag", "v1.1.0")
	work.git("push", "-q", "origin", "main", "develop", "release/a", "release/b", "--tags")
	head := origin.git("rev-parse", "main")
	work.write(".nerv/nerv.yaml", "pipeline:\n  dotnet:\n    solution: App.slnx\n  images:\n    - name: ghcr.io/acme/app\n")
	if _, err := pipeline.WriteReleaseOutputs(work.dir, pipeline.ReleaseOutputs{Step: "release", Status: "passed", Published: true,
		Branch: "main", Version: "1.1.0", GitTag: "v1.1.0", GitHead: head}); err != nil {
		t.Fatal(err)
	}
	refs := func() string { return origin.git("for-each-ref", "--format=%(refname) %(objectname)") }
	before := refs()

	sync := func(args ...string) (int, pipeline.SyncResult, string) {
		t.Helper()
		opts := defaultOptions()
		opts.Stdin = strings.NewReader("")
		opts.IsTerminal = func() bool { return false }
		var stdout, stderr bytes.Buffer
		start := time.Now()
		code := run(append([]string{"pipeline", "sync", "--cwd", work.dir, "--json"}, args...), &stdout, &stderr, opts)
		t.Logf("sync %v: exit %d in %s", args, code, time.Since(start).Round(time.Millisecond))
		var res pipeline.SyncResult
		if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
			t.Fatalf("%v\nstdout %s\nstderr %s", err, stdout.String(), stderr.String())
		}
		return code, res, stderr.String()
	}
	status := func(res pipeline.SyncResult) map[string]string {
		m := map[string]string{}
		for _, b := range res.Branches {
			m[b.Branch] = b.Status
		}
		return m
	}

	code, res, stderr := sync("--dry-run")
	if code != 0 || refs() != before {
		t.Fatalf("dry run: exit %d, origin changed: %v\n%s", code, refs() != before, stderr)
	}
	if s := status(res); s["develop"] != "would-merge" || s["release/a"] != "would-merge" || s["release/b"] != "would-conflict" || len(s) != 3 {
		t.Errorf("dry run statuses %v", s)
	}

	code, res, stderr = sync("--yes")
	if code != 2 {
		t.Fatalf("exit %d, want 2 (release/b conflicts)\n%s", code, stderr)
	}
	if s := status(res); s["develop"] != "pushed" || s["release/a"] != "pushed" || s["release/b"] != "conflict" || res.Status != "failed" {
		t.Errorf("statuses %v status %s", s, res.Status)
	}
	if res.Identity != "Sync User <sync@example.com>" {
		t.Errorf("identity %q", res.Identity)
	}
	for _, b := range []string{"develop", "release/a"} {
		got := origin.git("log", "-1", "--format=%s|%an <%ae>|%P", b)
		want := "chore(sync): merge main into " + b + " [skip ci]|Sync User <sync@example.com>|"
		parts := strings.Split(got, "|")
		if !strings.HasPrefix(got, want) || len(strings.Fields(parts[2])) != 2 || !strings.Contains(parts[2], head) {
			t.Errorf("origin's %s tip %q: want %q with main's release commit as a parent", b, got, want)
		}
		for _, r := range res.Branches {
			if r.Branch == b && r.Commit != origin.git("rev-parse", b) {
				t.Errorf("%s: reported commit %s, origin has %s", b, r.Commit, origin.git("rev-parse", b))
			}
		}
	}
	if !strings.Contains(before, "refs/heads/release/b "+origin.git("rev-parse", "release/b")) {
		t.Error("the conflicting merge reached origin's release/b")
	}
	if origin.git("rev-parse", "main") != head {
		t.Error("main moved")
	}
	saved, err := os.ReadFile(pipeline.SyncResultPath(work.dir))
	if err != nil || !strings.Contains(string(saved), `"conflict"`) {
		t.Errorf("sync.json: %v\n%s", err, saved)
	}
	if !strings.Contains(stderr, "[pipeline:sync] release/b: conflict (CHANGELOG.md)") {
		t.Errorf("stderr:\n%s", stderr)
	}

	// Run again: develop and release/a are up to date, release/b still
	// conflicts.
	code, res, _ = sync("--yes")
	if s := status(res); code != 2 || s["develop"] != "up-to-date" || s["release/a"] != "up-to-date" || s["release/b"] != "conflict" {
		t.Errorf("second run: exit %d statuses %v", code, s)
	}
}
