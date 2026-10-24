package releaseclone

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

// git runs git in dir with an isolated configuration and fixed dates.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2026-01-01T00:00:00+00:00", "GIT_COMMITTER_DATE=2026-01-01T00:00:00+00:00")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// fixture is a bare origin and a user's clone of it. Origin has main with
// v1.0.0 and develop with v1.1.0-alpha.1 and its channel note; the user's
// clone has neither the tags nor the note, and one unpushed feat commit on
// develop.
type fixture struct {
	origin, user string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	root := t.TempDir()
	f := fixture{origin: filepath.Join(root, "origin.git"), user: filepath.Join(root, "widgets")}
	seed := filepath.Join(root, "seed")
	git(t, root, "init", "-q", "--bare", "-b", "main", f.origin)
	git(t, root, "init", "-q", "-b", "main", seed)
	git(t, seed, "commit", "-q", "--allow-empty", "-m", "feat: first")
	git(t, seed, "tag", "-a", "-m", "v1.0.0", "v1.0.0")
	git(t, seed, "checkout", "-q", "-b", "develop")
	git(t, seed, "commit", "-q", "--allow-empty", "-m", "feat: second")
	git(t, seed, "tag", "-a", "-m", "v1.1.0-alpha.1", "v1.1.0-alpha.1")
	git(t, seed, "notes", "--ref", "semantic-release-v1.1.0-alpha.1", "add", "-m", `{"channels":["alpha"]}`, "v1.1.0-alpha.1^{commit}")
	git(t, seed, "push", "-q", f.origin, "main", "develop", "--tags", "refs/notes/*:refs/notes/*")
	git(t, root, "clone", "-q", "--no-tags", f.origin, f.user)
	git(t, f.user, "checkout", "-q", "develop")
	git(t, f.user, "commit", "-q", "--allow-empty", "-m", "feat: third")
	return f
}

// state is everything the user's repository holds: refs, config, index.
func state(t *testing.T, dir string) string {
	t.Helper()
	config, _ := os.ReadFile(filepath.Join(dir, ".git", "config"))
	return git(t, dir, "for-each-ref") + "\n" + string(config) + "\n" + git(t, dir, "status", "--porcelain")
}

// L6, S7: online, origin's tags and channel notes are read into a throwaway
// copy and the version is computed there: the prerelease numbering follows
// origin's note, and the user's repository is not written.
func TestCompute_ReadsOriginIntoACopyAndNeverWritesTheRepository(t *testing.T) {
	f := newFixture(t)
	before := state(t, f.user)
	results, err := Compute(context.Background(), ComputeOptions{RepoRoot: f.user, Config: versioning.Config{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].NewReleaseVersion != "1.1.0-alpha.2" || results[0].NewReleaseChannel != "alpha" || results[0].LastReleaseGitTag != "v1.1.0-alpha.1" {
		t.Fatalf("results = %+v, want 1.1.0-alpha.2 on alpha after v1.1.0-alpha.1", results)
	}
	if results[0].Project != "widgets" || results[0].Branch != "develop" {
		t.Errorf("project %q branch %q", results[0].Project, results[0].Branch)
	}
	if after := state(t, f.user); after != before {
		t.Errorf("the user's repository changed:\n--- before\n%s\n--- after\n%s", before, after)
	}
}

// Offline the repository is read as it is: without origin's tags the
// computation starts over, and nothing is written either.
func TestCompute_OfflineReadsTheRepositoryAsItIs(t *testing.T) {
	f := newFixture(t)
	before := state(t, f.user)
	results, err := Compute(context.Background(), ComputeOptions{RepoRoot: f.user, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].LastReleaseGitTag != "" {
		t.Fatalf("results = %+v, want no last release (the local repository has no tags)", results)
	}
	if after := state(t, f.user); after != before {
		t.Errorf("the user's repository changed offline")
	}
}

// A branch that is not local is refused before origin is read.
func TestCompute_UnknownBranch(t *testing.T) {
	f := newFixture(t)
	if _, err := Compute(context.Background(), ComputeOptions{RepoRoot: f.user, Branch: "nope"}); err == nil || !strings.Contains(err.Error(), `branch "nope": not a local branch`) {
		t.Fatalf("err = %v", err)
	}
}

// New checks out origin's branch (not the user's unpushed commit), reads the
// user's local release: section, computes offline in the clone and Close
// removes everything.
func TestNew_ClonesOriginAndComputesInTheClone(t *testing.T) {
	f := newFixture(t)
	if err := os.WriteFile(filepath.Join(f.user, ".ai-workflow.yml"), []byte("release:\n  tag_format: v${version}\n  projects:\n    - name: app\n      path: .\n      build: { mode: script }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := state(t, f.user)
	ws, err := New(context.Background(), Options{RepoRoot: f.user, Branch: "develop"})
	if err != nil {
		t.Fatal(err)
	}
	if want := git(t, f.origin, "rev-parse", "develop"); ws.OriginHead() != want {
		t.Errorf("origin head %s, want %s", ws.OriginHead(), want)
	}
	if ws.Config() == nil || ws.Config().Projects[0].Name != "app" {
		t.Fatalf("config = %+v", ws.Config())
	}
	if _, err := os.Stat(filepath.Join(ws.Dir(), ".ai-workflow.yml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the clone holds origin's code only, not the local file: %v", err)
	}
	results, err := ws.Compute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// origin's develop has no commit after v1.1.0-alpha.1.
	if len(results) != 1 || results[0].Project != "app" || results[0].NewReleasePublished || results[0].LastReleaseVersion != "1.1.0-alpha.1" {
		t.Errorf("results = %+v", results)
	}
	b, err := ws.Branch(context.Background())
	if err != nil || b.Channel != "alpha" || b.Prerelease != "alpha" {
		t.Errorf("Branch = %+v, %v", b, err)
	}
	top := filepath.Dir(ws.Dir())
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(top); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s left behind", top)
	}
	if after := state(t, f.user); after != before {
		t.Errorf("the user's repository changed")
	}
}

func TestNew_NoOrigin(t *testing.T) {
	f := newFixture(t)
	git(t, f.user, "remote", "remove", "origin")
	if _, err := New(context.Background(), Options{RepoRoot: f.user, Branch: "develop"}); !errors.Is(err, ErrNoOrigin) {
		t.Fatalf("err = %v, want ErrNoOrigin", err)
	}
}

// An unreadable origin is ErrOriginFetch, its credentials never shown, and
// nothing is left behind.
func TestNew_FetchFailureHidesCredentialsAndCleansUp(t *testing.T) {
	f := newFixture(t)
	git(t, f.user, "remote", "set-url", "origin", "https://bot:s3cr3t@127.0.0.1:9/acme/widgets.git")
	// The clone's temporary directory goes under a directory of its own.
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	_, err := New(context.Background(), Options{RepoRoot: f.user, Branch: "develop", FetchTimeout: 20 * time.Second})
	if !errors.Is(err, ErrOriginFetch) {
		t.Fatalf("err = %v, want ErrOriginFetch", err)
	}
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("credentials in the error: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(tmp, "nerv-release-*")); len(left) != 0 {
		t.Errorf("clone left behind: %v", left)
	}
}
