package release_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/release"
)

// ---------------------------------------------------------------------------
// Tags — thin wrapper around `git -C <repo> tag -l <pattern>`, scripted
// against a fake runner.
// ---------------------------------------------------------------------------

func TestTags_ParsesLines(t *testing.T) {
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"git -C /repo tag -l v*": {Stdout: "v0.1.0\nv0.2.0\nv1.0.0-rc.1\n", ExitCode: 0},
		},
	}

	tags, err := release.Tags(context.Background(), runner, "/repo", "v*")
	if err != nil {
		t.Fatalf("Tags() error = %v", err)
	}
	want := []string{"v0.1.0", "v0.2.0", "v1.0.0-rc.1"}
	if len(tags) != len(want) {
		t.Fatalf("Tags() = %v, want %v", tags, want)
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Errorf("Tags()[%d] = %q, want %q", i, tags[i], want[i])
		}
	}
}

func TestTags_EmptyOutputIsEmptySlice(t *testing.T) {
	runner := &envtest.FakeRunner{}
	tags, err := release.Tags(context.Background(), runner, "/repo", "v*")
	if err != nil {
		t.Fatalf("Tags() error = %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("Tags() = %v, want empty", tags)
	}
}

func TestTags_NonZeroExitIsError(t *testing.T) {
	runner := &envtest.FakeRunner{
		Default: envtest.Response{ExitCode: 128, Stderr: "fatal: not a git repository"},
	}
	if _, err := release.Tags(context.Background(), runner, "/repo", "v*"); err == nil {
		t.Fatal("expected an error for a non-zero git exit code")
	}
}

func TestTags_LaunchFailureIsError(t *testing.T) {
	runner := &envtest.FakeRunner{
		Default: envtest.Response{Err: exec.ErrNotFound},
	}
	if _, err := release.Tags(context.Background(), runner, "/repo", "v*"); err == nil {
		t.Fatal("expected an error when git cannot be launched")
	}
}

// ---------------------------------------------------------------------------
// CommitsSince — `git log --no-merges --reverse --pretty=format:... [<tag>..HEAD]`.
// ---------------------------------------------------------------------------

func TestCommitsSince_WholeHistoryWhenTagEmpty(t *testing.T) {
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"git -C /repo log --no-merges --reverse --pretty=format:%H%x1f%h%x1f%s%x1f%b%x1e": {
				Stdout: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x1faaaaaaa\x1ffeat: first\x1f\x1e",
			},
		},
	}
	commits, err := release.CommitsSince(context.Background(), runner, "/repo", "")
	if err != nil {
		t.Fatalf("CommitsSince() error = %v", err)
	}
	if len(commits) != 1 || commits[0].Subject != "feat: first" {
		t.Errorf("CommitsSince() = %+v", commits)
	}
}

func TestCommitsSince_RangeWhenTagGiven(t *testing.T) {
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"git -C /repo log --no-merges --reverse --pretty=format:%H%x1f%h%x1f%s%x1f%b%x1e v0.2.0..HEAD": {
				Stdout: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\x1fbbbbbbb\x1ffix: second\x1f\x1e",
			},
		},
	}
	commits, err := release.CommitsSince(context.Background(), runner, "/repo", "v0.2.0")
	if err != nil {
		t.Fatalf("CommitsSince() error = %v", err)
	}
	if len(commits) != 1 || commits[0].Subject != "fix: second" {
		t.Errorf("CommitsSince() = %+v", commits)
	}
}

func TestCommitsSince_NonZeroExitIsError(t *testing.T) {
	runner := &envtest.FakeRunner{
		Default: envtest.Response{ExitCode: 128},
	}
	if _, err := release.CommitsSince(context.Background(), runner, "/repo", ""); err == nil {
		t.Fatal("expected an error for a non-zero git exit code")
	}
}

// ---------------------------------------------------------------------------
// Real-git integration tests, on t.TempDir() only, per the phase spec.
// ---------------------------------------------------------------------------

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestGitIntegration_TagsAndCommitsSince(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()

	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "chore: initial")
	runGit(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "feat: first feature")
	runGit(t, dir, "tag", "v0.1.0")
	runGit(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "fix: a fix")

	runner := env.ExecRunner{}

	tags, err := release.Tags(context.Background(), runner, dir, "v*")
	if err != nil {
		t.Fatalf("Tags() error = %v", err)
	}
	if len(tags) != 1 || tags[0] != "v0.1.0" {
		t.Fatalf("Tags() = %v, want [v0.1.0]", tags)
	}
	if got := release.LastReleaseTag(tags); got != "v0.1.0" {
		t.Errorf("LastReleaseTag() = %q, want v0.1.0", got)
	}

	commits, err := release.CommitsSince(context.Background(), runner, dir, "v0.1.0")
	if err != nil {
		t.Fatalf("CommitsSince() error = %v", err)
	}
	if len(commits) != 1 || commits[0].Subject != "fix: a fix" {
		t.Fatalf("CommitsSince() = %+v, want one 'fix: a fix' commit", commits)
	}
	if len(commits[0].Sha) != 40 || len(commits[0].ShortSha) < 4 {
		t.Errorf("commit sha/shortsha look wrong: %+v", commits[0])
	}

	allCommits, err := release.CommitsSince(context.Background(), runner, dir, "")
	if err != nil {
		t.Fatalf("CommitsSince() error = %v", err)
	}
	if len(allCommits) != 3 {
		t.Fatalf("CommitsSince(whole history) = %d commits, want 3", len(allCommits))
	}
	if allCommits[0].Subject != "chore: initial" {
		t.Errorf("expected oldest-first order, got %+v", allCommits)
	}
}

func TestGitIntegration_NotAGitRepoErrors(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()

	runner := env.ExecRunner{}
	if _, err := release.Tags(context.Background(), runner, dir, "v*"); err == nil {
		t.Fatal("expected an error for a non-git directory")
	}
}
