package versioning

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
)

// ReleaseBranch is how the configured branches treat one branch.
type ReleaseBranch struct {
	// Name is the branch.
	Name string
	// Prerelease is its prerelease identifier, empty on a release branch.
	Prerelease string
	// Channel is its distribution channel, empty for the default channel.
	Channel string
}

// BranchChannel resolves req.Branch against req.Config's branches (expanded
// against the repository's branches, as Compute does) and returns its
// prerelease identifier and channel. A branch that is not a release branch
// is an error naming the configured branches. It never reads origin.
func BranchChannel(ctx context.Context, req Request) (ReleaseBranch, error) {
	runner := req.Runner
	if runner == nil {
		runner = env.ExecRunner{}
	}
	cfg := req.Config.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return ReleaseBranch{}, err
	}
	g := gitReader{runner: runner, dir: req.RepoDir}
	top, err := g.trimmed(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return ReleaseBranch{}, fmt.Errorf("not a git repository: %w", err)
	}
	g.dir = filepath.Clean(top)
	branch := req.Branch
	if branch == "" {
		if branch, err = g.trimmed(ctx, "symbolic-ref", "--short", "HEAD"); err != nil {
			return ReleaseBranch{}, fmt.Errorf("the current checkout is not on a branch; pass a branch: %w", err)
		}
	}
	names, err := g.branchNames(ctx, branch)
	if err != nil {
		return ReleaseBranch{}, err
	}
	resolved, err := expandBranches(cfg.Branches, names)
	if err != nil {
		return ReleaseBranch{}, err
	}
	idx := slices.IndexFunc(resolved, func(b resolvedBranch) bool { return b.name == branch })
	if idx < 0 {
		return ReleaseBranch{}, fmt.Errorf("branch %s is not a release branch (the configured branches are %s)", branch, branchNames(cfg.Branches))
	}
	b := resolved[idx]
	return ReleaseBranch{Name: b.name, Prerelease: b.prerelease, Channel: b.channel}, nil
}
