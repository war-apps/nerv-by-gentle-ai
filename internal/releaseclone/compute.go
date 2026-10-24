package releaseclone

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

// ComputeOptions select what Compute computes.
type ComputeOptions struct {
	// RepoRoot is any directory inside the repository.
	RepoRoot string
	// Branch is the local branch to compute; empty means the current one.
	Branch string
	// Projects restricts the results to these projects (their dependencies
	// are still computed, for depends_on).
	Projects []string
	// Config is the version engine's configuration.
	Config versioning.Config
	// Offline computes from the local tags and notes, in the repository
	// itself (read only). Otherwise origin's branches, tags and notes are
	// read into a throwaway clone first.
	Offline bool
	// FetchTimeout bounds the read of origin; zero means
	// DefaultFetchTimeout.
	FetchTimeout time.Duration
	// Env is git's environment; nil means os.Environ().
	Env []string
	// Runner runs the clone's git and VersionRunner the engine's; nil means
	// the real ones.
	Runner        proc.Runner
	VersionRunner env.Runner
}

// Compute is `nerv version next`'s computation: the next release of the
// local branch (its unpushed commits count) as CI would compute it after a
// push. Online, origin's branches, release tags and channel notes are read
// into a throwaway clone (origin's branch set, the user's branch at its
// local commit) and the engine runs there, offline; the user's repository is
// never written. Offline, or without an origin, the engine reads the
// repository as it is.
func Compute(ctx context.Context, o ComputeOptions) ([]versioning.Result, error) {
	runner := o.Runner
	if runner == nil {
		runner = proc.ExecRunner{}
	}
	envv := o.Env
	if envv == nil {
		envv = os.Environ()
	}
	w := &Workspace{runner: runner}
	top, err := w.git(ctx, o.RepoRoot, envv, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}
	top = filepath.Clean(top)
	branch := o.Branch
	if branch == "" {
		if branch, err = w.git(ctx, top, envv, "symbolic-ref", "--short", "HEAD"); err != nil {
			return nil, fmt.Errorf("the current checkout is not on a branch; pass a branch: %w", err)
		}
	}
	local, err := w.git(ctx, top, envv, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch+"^{commit}")
	if err != nil || local == "" {
		return nil, fmt.Errorf("branch %q: not a local branch", branch)
	}
	req := versioning.Request{RepoDir: top, Branch: branch, Projects: o.Projects, Config: o.Config, Offline: true, Runner: o.VersionRunner}
	if o.Offline {
		return versioning.Compute(ctx, req)
	}
	ws, err := New(ctx, Options{RepoRoot: top, Branch: branch, Env: envv, FetchTimeout: o.FetchTimeout, Runner: runner, VersionRunner: o.VersionRunner, LocalHead: local})
	if errors.Is(err, ErrNoOrigin) {
		// Nothing to read from: the local tags and notes are all there is.
		return versioning.Compute(ctx, req)
	}
	if err != nil {
		return nil, err
	}
	defer ws.Close()
	req.RepoDir = ws.Dir()
	return versioning.Compute(ctx, req)
}
