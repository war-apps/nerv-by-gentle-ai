package semrel

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
)

// ErrBranchNotOnOrigin is wrapped by Merge and PreviewMerge when origin has
// no such branch.
var ErrBranchNotOnOrigin = errors.New("the branch is not on origin")

// ErrNoCommitIdentity is wrapped by UseCommitIdentity when the user's
// repository has no commit identity configured (user.name and user.email,
// or the GIT_AUTHOR_*/GIT_COMMITTER_* variables): nothing is guessed.
var ErrNoCommitIdentity = errors.New("no git commit identity is configured")

// MergeResult is the outcome of merging origin's source branch into a
// target branch (nerv pipeline sync).
type MergeResult struct {
	// UpToDate: the target already holds the source; nothing was merged.
	UpToDate bool
	// Conflict: the merge conflicts; Merge aborted it.
	Conflict bool
	// Conflicts are the conflicting paths, when known.
	Conflicts []string
	// Commit is the merge commit Merge made (empty otherwise, and always
	// for PreviewMerge).
	Commit string
}

// UseCommitIdentity makes the merges of nerv pipeline sync the user's
// commits. It resolves the identity in the user's repository with the
// user's environment, as git does there (its own configuration, the global
// one and any includeIf for its path, which would not match the clone's
// temporary path; GIT_AUTHOR_*/GIT_COMMITTER_* win), without guessing one
// from the system account (user.useConfigOnly): none configured wraps
// ErrNoCommitIdentity. It copies user.name and user.email into the clone's
// configuration and returns the committer ("Name <email>").
func (w *Workspace) UseCommitIdentity(ctx context.Context) (string, error) {
	var ident string
	for _, v := range []string{"GIT_AUTHOR_IDENT", "GIT_COMMITTER_IDENT"} {
		out, err := w.e.git(ctx, w.top, w.userEnv, "-c", "user.useConfigOnly=true", "var", v)
		if err != nil {
			return "", fmt.Errorf("%w for %s: set user.name and user.email (git config --global user.name \"Your Name\"; git config --global user.email you@example.com): %w", ErrNoCommitIdentity, w.top, err)
		}
		ident = out
	}
	for _, key := range []string{"user.name", "user.email"} {
		// git config --get exits 1 when the key is not set (the identity
		// then comes from the environment): nothing to copy.
		v, _ := w.e.git(ctx, w.top, w.userEnv, "config", "--get", key)
		if v == "" {
			continue
		}
		if _, err := w.e.git(ctx, w.dir, w.env, "config", key, v); err != nil {
			return "", fmt.Errorf("setting the clone's %s: %w", key, err)
		}
	}
	// "Name <email> 1792156082 -0300": drop the timestamp.
	if i := strings.LastIndex(ident, ">"); i >= 0 {
		ident = ident[:i+1]
	}
	return ident, nil
}

// Branches returns origin's branches (as the clone fetched them) whose
// names match pattern (path.Match, as pipeline.sync.release is validated),
// sorted.
func (w *Workspace) Branches(ctx context.Context, pattern string) ([]string, error) {
	out, err := w.e.git(ctx, w.dir, w.env, "for-each-ref", "--format=%(refname:lstrip=3)", "refs/remotes/origin/")
	if err != nil {
		return nil, fmt.Errorf("listing origin's branches: %w", err)
	}
	var names []string
	for _, name := range strings.Split(out, "\n") {
		if ok, _ := path.Match(pattern, name); ok && name != "HEAD" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}

// Merge checks target out at origin's commit and merges origin's source
// into it with git merge --no-ff --no-edit -m message (dim's sync jobs),
// committing as UseCommitIdentity set. Already merged: UpToDate, no commit.
// A conflicting merge is aborted (git merge --abort) and reported as
// Conflict with its paths, not as an error. Nothing is pushed.
func (w *Workspace) Merge(ctx context.Context, target, source, message string) (MergeResult, error) {
	up, err := w.upToDate(ctx, target, source)
	if err != nil || up {
		return MergeResult{UpToDate: up}, err
	}
	if _, err := w.e.git(ctx, w.dir, w.env, "checkout", "--quiet", "--force", "--no-track", "-B", target, "refs/remotes/origin/"+target); err != nil {
		return MergeResult{}, fmt.Errorf("checking out origin's %s: %w", target, err)
	}
	// user.useConfigOnly: never a guessed identity (UseCommitIdentity set
	// the user's).
	_, merr := w.e.git(ctx, w.dir, w.env, "-c", "user.useConfigOnly=true", "merge", "--quiet", "--no-ff", "--no-edit", "-m", message, "refs/remotes/origin/"+source)
	if merr == nil {
		commit, err := w.e.git(ctx, w.dir, w.env, "rev-parse", "HEAD")
		if err != nil {
			return MergeResult{}, fmt.Errorf("reading the merge commit: %w", err)
		}
		return MergeResult{Commit: commit}, nil
	}
	// A conflict leaves the merge in progress (MERGE_HEAD); any other
	// failure (identity, hooks, git itself) does not.
	if _, err := w.e.git(ctx, w.dir, w.env, "rev-parse", "--quiet", "--verify", "MERGE_HEAD"); err != nil {
		return MergeResult{}, fmt.Errorf("merging %s into %s: %w", source, target, merr)
	}
	files, _ := w.e.git(ctx, w.dir, w.env, "diff", "--name-only", "--diff-filter=U")
	if _, err := w.e.git(ctx, w.dir, w.env, "merge", "--abort"); err != nil {
		return MergeResult{}, fmt.Errorf("aborting the conflicting merge of %s into %s: %w", source, target, err)
	}
	return MergeResult{Conflict: true, Conflicts: lines(files)}, nil
}

// PreviewMerge reports, without changing any branch, what Merge would do:
// UpToDate, or Conflict with its paths from git merge-tree --write-tree
// (exit 1 on conflicts), or a clean merge.
func (w *Workspace) PreviewMerge(ctx context.Context, target, source string) (MergeResult, error) {
	up, err := w.upToDate(ctx, target, source)
	if err != nil || up {
		return MergeResult{UpToDate: up}, err
	}
	out, err := w.e.git(ctx, w.dir, w.env, "merge-tree", "--write-tree", "--name-only", "--no-messages",
		"refs/remotes/origin/"+target, "refs/remotes/origin/"+source)
	if err == nil {
		return MergeResult{}, nil
	}
	if exitCode(err) != 1 {
		return MergeResult{}, fmt.Errorf("previewing the merge of %s into %s: %w", source, target, err)
	}
	// The first line is the tree; the conflicting paths follow.
	paths := lines(out)
	if len(paths) > 0 {
		paths = paths[1:]
	}
	return MergeResult{Conflict: true, Conflicts: paths}, nil
}

// PushBranch pushes the clone's branch to origin's branch of the same name
// with the user's own credentials, as CheckPush checks them; a moved
// origin branch makes it fail (never forced).
func (w *Workspace) PushBranch(ctx context.Context, branch string) error {
	ref := "refs/heads/" + branch
	if _, err := w.e.git(ctx, w.dir, w.env, "push", "--quiet", "origin", ref+":"+ref); err != nil {
		return fmt.Errorf("pushing %s to origin (%s): %w", branch, redact(w.pushURL), err)
	}
	return nil
}

// upToDate reports whether origin's target already holds origin's source,
// after checking both are on origin.
func (w *Workspace) upToDate(ctx context.Context, target, source string) (bool, error) {
	for _, b := range []string{target, source} {
		if _, err := w.e.git(ctx, w.dir, w.env, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+b); err != nil {
			if exitCode(err) == 1 {
				return false, fmt.Errorf("%s: %w", b, ErrBranchNotOnOrigin)
			}
			return false, fmt.Errorf("reading origin's %s: %w", b, err)
		}
	}
	_, err := w.e.git(ctx, w.dir, w.env, "merge-base", "--is-ancestor", "refs/remotes/origin/"+source, "refs/remotes/origin/"+target)
	switch {
	case err == nil:
		return true, nil
	case exitCode(err) == 1:
		return false, nil
	default:
		return false, fmt.Errorf("comparing %s with %s: %w", source, target, err)
	}
}

// exitCode is a process error's exit code; -1 for other errors.
func exitCode(err error) int {
	var coder interface{ ExitCode() int }
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return -1
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
