package semrel

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestPlanHeads(t *testing.T) {
	const local = "1111111111111111111111111111111111111111"
	refs := map[string]string{
		"refs/heads/main":                "aaaa",
		"refs/heads/develop":             "bbbb",
		"refs/heads/hotfix/local-only":   "cccc",
		"refs/remotes/origin/HEAD":       "aaaa",
		"refs/remotes/origin/main":       "aaaa",
		"refs/remotes/origin/develop":    "b0b0",
		"refs/remotes/origin/release/rc": "dddd",
		"refs/nerv-origin-heads/main":    "a1a1",
		"refs/nerv-origin-heads/develop": "b1b1",
		"refs/nerv-origin-heads/next":    "eeee",
	}
	tests := []struct {
		name          string
		source        string
		authoritative bool
		want          []string
	}{
		{
			name: "online: origin's fetched branches, the requested one local", source: originHeadsRef, authoritative: true,
			want: []string{
				"update refs/heads/develop " + local,
				"delete refs/heads/hotfix/local-only",
				"update refs/heads/main a1a1",
				"create refs/heads/next eeee",
				"delete refs/nerv-origin-heads/develop",
				"delete refs/nerv-origin-heads/main",
				"delete refs/nerv-origin-heads/next",
			},
		},
		{
			name: "offline: the remote-tracking branches, HEAD skipped", source: "refs/remotes/origin/", authoritative: false,
			want: []string{
				"update refs/heads/develop " + local,
				"delete refs/heads/hotfix/local-only",
				"create refs/heads/release/rc dddd",
				"delete refs/nerv-origin-heads/develop",
				"delete refs/nerv-origin-heads/main",
				"delete refs/nerv-origin-heads/next",
			},
		},
		{
			name: "no origin: the local branches", source: "",
			want: []string{
				"update refs/heads/develop " + local,
				"delete refs/nerv-origin-heads/develop",
				"delete refs/nerv-origin-heads/main",
				"delete refs/nerv-origin-heads/next",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Split(strings.TrimSuffix(planHeads(refs, tt.source, tt.authoritative, "develop", local), "\n"), "\n")
			if !slices.Equal(got, tt.want) {
				t.Errorf("plan:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestPlanHeads_OfflineWithoutRemoteTrackingBranchesKeepsTheLocalOnes(t *testing.T) {
	refs := map[string]string{"refs/heads/main": "aaaa", "refs/heads/develop": "bbbb", "refs/remotes/origin/HEAD": "aaaa"}
	if got := planHeads(refs, "refs/remotes/origin/", false, "develop", "bbbb"); got != "" {
		t.Errorf("plan = %q, want nothing to change", got)
	}
}

func TestPlanHeads_OnlineEmptyOriginKeepsOnlyTheRequestedBranch(t *testing.T) {
	refs := map[string]string{"refs/heads/main": "aaaa", "refs/heads/develop": "bbbb"}
	if got := planHeads(refs, originHeadsRef, true, "develop", "bbbb"); got != "delete refs/heads/main\n" {
		t.Errorf("plan = %q", got)
	}
}

// TestCompute_MirrorHeadsFollowOrigin: the fetch brings origin's branches
// into the mirror, and the mirror's heads are rewritten (on the mirror only)
// from what for-each-ref lists, the requested branch at the user's commit.
func TestCompute_MirrorHeadsFollowOrigin(t *testing.T) {
	for _, offline := range []bool{false, true} {
		root := t.TempDir()
		f := originRepo(t, root, "https://github.com/example/repo.git")
		inner := f.handle
		f.handle = func(c Command) (Result, error) {
			switch {
			case isGit(c, "rev-parse", "--verify", "--quiet", "refs/heads/main") && c.Args[1] == root:
				return Result{Stdout: []byte("1111111111111111111111111111111111111111\n")}, nil
			case isGit(c, "for-each-ref"):
				return Result{Stdout: []byte("aaaa refs/heads/main\ncccc refs/heads/hotfix/x\nb0b0 refs/remotes/origin/develop\nb1b1 refs/nerv-origin-heads/develop\n")}, nil
			}
			return inner(c)
		}
		if _, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", Offline: offline, CacheDir: t.TempDir()}); err != nil {
			t.Fatal(err)
		}
		clones := f.find(func(c Command) bool { return isGit(c, "clone") })
		mirror := clones[0].Args[len(clones[0].Args)-1]
		if !offline && !slices.Contains(onlyFetch(t, f).Args, "+refs/heads/*:"+originHeadsRef+"*") {
			t.Errorf("fetch %v does not bring origin's branches", onlyFetch(t, f).Args)
		}
		updates := f.find(func(c Command) bool { return isGit(c, "update-ref", "--stdin") })
		if len(updates) != 1 || updates[0].Args[1] != mirror {
			t.Fatalf("update-ref calls %v, want one on the mirror", updates)
		}
		want := "create refs/heads/develop b1b1\n"
		if offline {
			want = "create refs/heads/develop b0b0\n"
		}
		want += "delete refs/heads/hotfix/x\nupdate refs/heads/main 1111111111111111111111111111111111111111\ndelete refs/nerv-origin-heads/develop\n"
		if got := string(updates[0].Stdin); got != want {
			t.Errorf("offline %v: update-ref stdin:\n%s\nwant:\n%s", offline, got, want)
		}
	}
}

func TestCompute_HeadsRewriteFailureIsAnError(t *testing.T) {
	root := t.TempDir()
	f := originRepo(t, root, "https://github.com/example/repo.git")
	inner := f.handle
	f.handle = func(c Command) (Result, error) {
		switch {
		case isGit(c, "for-each-ref"):
			return Result{Stdout: []byte("cccc refs/heads/hotfix/x\n")}, nil
		case isGit(c, "update-ref"):
			return Result{Stderr: []byte("fatal: cannot lock ref")}, errors.New("exit status 128")
		}
		return inner(c)
	}
	_, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "branches") {
		t.Fatalf("err = %v", err)
	}
}
