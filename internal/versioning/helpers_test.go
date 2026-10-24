package versioning

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
)

// fixedNow is the release date of every test.
var fixedNow = func() time.Time { return time.Date(2026, 10, 23, 15, 0, 0, 0, time.UTC) }

// testRepo is a real git repository in a temporary directory, isolated from
// the user's and the system's git configuration.
type testRepo struct {
	t     *testing.T
	dir   string
	clock int
}

func newTestRepo(t *testing.T) *testRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	r := &testRepo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	r.git("config", "user.name", "Test")
	r.git("config", "user.email", "test@example.com")
	r.git("config", "commit.gpgsign", "false")
	r.git("config", "tag.gpgsign", "false")
	return r
}

// git runs git in the repository, with a commit date one minute after the
// previous one so the log order is deterministic.
func (r *testRepo) git(args ...string) string {
	r.t.Helper()
	r.clock++
	date := fmt.Sprintf("2026-01-01T00:%02d:%02d+00:00", r.clock/60, r.clock%60)
	cmd := exec.Command("git", append([]string{"-C", r.dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit commits message, writing each file (a new line each time) first;
// with no file the commit is empty. It returns the commit hash.
func (r *testRepo) commit(message string, files ...string) string {
	r.t.Helper()
	for _, f := range files {
		p := filepath.Join(r.dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			r.t.Fatal(err)
		}
		fh, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			r.t.Fatal(err)
		}
		fmt.Fprintf(fh, "%d\n", r.clock)
		fh.Close()
		r.git("add", "--", f)
	}
	args := []string{"commit", "-q", "-m", message}
	if len(files) == 0 {
		args = append(args, "--allow-empty")
	}
	r.git(args...)
	return r.git("rev-parse", "HEAD")
}

// release tags HEAD as semantic-release does: the tag and its note with the
// channel ("" is the default channel).
func (r *testRepo) release(tag, channel string) {
	r.t.Helper()
	r.git("tag", tag)
	ch := "null"
	if channel != "" {
		ch = fmt.Sprintf("%q", channel)
	}
	r.git("notes", "--ref", "semantic-release-"+tag, "add", "-f", "-m", `{"channels":[`+ch+`]}`, tag)
}

func (r *testRepo) compute(req Request) []Result {
	r.t.Helper()
	res, err := r.computeErr(req)
	if err != nil {
		r.t.Fatalf("Compute: %v", err)
	}
	return res
}

func (r *testRepo) computeErr(req Request) ([]Result, error) {
	req.RepoDir = r.dir
	req.Offline = true
	req.Runner = env.ExecRunner{}
	req.Now = fixedNow
	return Compute(context.Background(), req)
}

// one returns the only result.
func one(t *testing.T, results []Result) Result {
	t.Helper()
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d: %+v", len(results), results)
	}
	return results[0]
}

// byProject indexes results by project.
func byProject(results []Result) map[string]Result {
	m := map[string]Result{}
	for _, r := range results {
		m[r.Project] = r
	}
	return m
}
