//go:build semrel_integration

package semrel

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// TestIntegration_WorkspaceMergesBranchesAndPushes runs the sync helpers
// with real git against a local bare origin: develop takes main cleanly,
// release/b conflicts (aborted, nothing pushed), release/a is up to date
// after a first sync.
func TestIntegration_WorkspaceMergesBranchesAndPushes(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	r, origin := newOriginRepo(t)
	r.commit("feat: base", "app.txt")
	r.git("push", "-q", "origin", "main")
	for _, b := range []string{"develop", "release/a", "release/b"} {
		r.git("branch", b)
	}
	r.git("checkout", "-q", "release/b")
	r.commit("fix: rc change", "app.txt")
	r.git("checkout", "-q", "main")
	r.commit("chore(release): 1.1.0 [skip ci]", "app.txt")
	r.git("push", "-q", "origin", "main", "develop", "release/a", "release/b")
	r.git("config", "user.name", "Local User")
	r.git("config", "user.email", "local@example.com")

	ctx := context.Background()
	ws, err := NewWorkspace(ctx, WorkspaceOptions{RepoRoot: r.dir, Branch: "main", Env: syncEnv()})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	ident, err := ws.UseCommitIdentity(ctx)
	if err != nil || ident != "Local User <local@example.com>" {
		t.Fatalf("UseCommitIdentity = %q, %v", ident, err)
	}
	branches, err := ws.Branches(ctx, "release/*")
	if err != nil || !slices.Equal(branches, []string{"release/a", "release/b"}) {
		t.Fatalf("Branches = %q, %v", branches, err)
	}

	pre, err := ws.PreviewMerge(ctx, "release/b", "main")
	if err != nil || !pre.Conflict || !slices.Equal(pre.Conflicts, []string{"app.txt"}) {
		t.Errorf("PreviewMerge(release/b) = %+v, %v", pre, err)
	}
	if pre, err := ws.PreviewMerge(ctx, "develop", "main"); err != nil || pre.Conflict || pre.UpToDate {
		t.Errorf("PreviewMerge(develop) = %+v, %v", pre, err)
	}

	msg := "chore(sync): merge main into develop [skip ci]"
	m, err := ws.Merge(ctx, "develop", "main", msg)
	if err != nil || m.Commit == "" || m.Conflict || m.UpToDate {
		t.Fatalf("Merge(develop) = %+v, %v", m, err)
	}
	if err := ws.PushBranch(ctx, "develop"); err != nil {
		t.Fatal(err)
	}
	if got := origin.git("rev-parse", "develop"); got != m.Commit {
		t.Errorf("origin's develop %s, want the merge %s", got, m.Commit)
	}
	if got := origin.git("log", "-1", "--format=%s|%an|%P", "develop"); !strings.HasPrefix(got, msg+"|Local User|") || len(strings.Fields(strings.Split(got, "|")[2])) != 2 {
		t.Errorf("merge commit %q: want dim's message, the user's identity and two parents", got)
	}

	before := origin.git("rev-parse", "release/b")
	c, err := ws.Merge(ctx, "release/b", "main", "chore(sync): merge main into release/b [skip ci]")
	if err != nil || !c.Conflict || !slices.Equal(c.Conflicts, []string{"app.txt"}) || c.Commit != "" {
		t.Fatalf("Merge(release/b) = %+v, %v", c, err)
	}
	if out, err := exec.Command("git", "-C", ws.Dir(), "rev-parse", "-q", "--verify", "MERGE_HEAD").CombinedOutput(); err == nil {
		t.Errorf("merge not aborted: MERGE_HEAD %s", out)
	}
	if origin.git("rev-parse", "release/b") != before {
		t.Error("a conflicting merge reached origin")
	}

	a, err := ws.Merge(ctx, "release/a", "main", "chore(sync): merge main into release/a [skip ci]")
	if err != nil || a.Commit == "" {
		t.Fatalf("Merge(release/a) = %+v, %v", a, err)
	}
	if err := ws.PushBranch(ctx, "release/a"); err != nil {
		t.Fatal(err)
	}
	if up, err := ws.PreviewMerge(ctx, "develop", "main"); err != nil || !up.UpToDate {
		t.Errorf("after the push, develop should be up to date with main: %+v, %v", up, err)
	}
	if up, err := ws.Merge(ctx, "release/a", "main", "x"); err != nil || !up.UpToDate || up.Commit != "" {
		t.Errorf("second merge into release/a = %+v, %v", up, err)
	}

	if _, err := ws.Merge(ctx, "release/zz", "main", "x"); !errors.Is(err, ErrBranchNotOnOrigin) {
		t.Errorf("missing target: err = %v", err)
	}
	if _, err := ws.PreviewMerge(ctx, "develop", "nope"); !errors.Is(err, ErrBranchNotOnOrigin) {
		t.Errorf("missing source: err = %v", err)
	}
}

// syncEnv is testEnv without the GIT_AUTHOR_*/GIT_COMMITTER_* overrides,
// so the commit identity comes from the user's repository configuration.
func syncEnv() []string {
	return slices.DeleteFunc(testEnv(), func(kv string) bool {
		return strings.HasPrefix(kv, "GIT_AUTHOR_") || strings.HasPrefix(kv, "GIT_COMMITTER_")
	})
}
