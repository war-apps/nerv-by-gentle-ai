package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/semrel"
)

// Branch sync statuses.
const (
	SyncPushed        = "pushed"
	SyncUpToDate      = "up-to-date"
	SyncConflict      = "conflict"
	SyncFailed        = "failed"
	SyncWouldMerge    = "would-merge"
	SyncWouldConflict = "would-conflict"
	// SyncNotRun is a target an interruption stopped before it started.
	SyncNotRun = "not-run"
)

// SyncRepo is the clone the sync step merges in; *semrel.Workspace is the
// real one.
type SyncRepo interface {
	Branches(ctx context.Context, pattern string) ([]string, error)
	Merge(ctx context.Context, target, source, message string) (semrel.MergeResult, error)
	PreviewMerge(ctx context.Context, target, source string) (semrel.MergeResult, error)
	PushBranch(ctx context.Context, branch string) error
}

// SyncTarget is one merge-back: origin's Source merged into Branch.
type SyncTarget struct {
	Branch  string
	Source  string
	Message string
}

// BranchSync is the outcome of one SyncTarget.
type BranchSync struct {
	Branch string `json:"branch"`
	Source string `json:"source"`
	// Status: pushed, up-to-date, conflict, failed; in a dry run
	// would-merge, would-conflict, up-to-date or failed.
	Status string `json:"status"`
	// Commit is the merge commit (pushed, or made but not pushed).
	Commit    string   `json:"commit,omitempty"`
	Conflicts []string `json:"conflicts,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// SyncResult is the sync step's document (sync.json, --json).
type SyncResult struct {
	Step    string `json:"step"`
	Status  string `json:"status"`
	DryRun  bool   `json:"dry_run"`
	Version string `json:"version,omitempty"`
	// Source is the released branch.
	Source string `json:"source,omitempty"`
	// Identity is the committer of the merge commits.
	Identity string       `json:"identity,omitempty"`
	Branches []BranchSync `json:"branches"`
	Error    string       `json:"error,omitempty"`
	Action   string       `json:"action,omitempty"`
}

// Sync is dim's sync-to-develop and sync-main-to-release jobs, after the
// release in Release, in a clone of origin.
type Sync struct {
	Repo    SyncRepo
	Config  *pipelineconfig.Pipeline
	Release ReleaseOutputs
	// Output receives one line per branch; nil discards them.
	Output io.Writer
	// DryRun previews every merge; nothing is merged or pushed.
	DryRun bool
}

// Targets are the merges dim runs after a release on Release.Branch: the
// main branch (pipeline.branches' first) or a branch matching
// pipeline.sync.release is merged into pipeline.sync.develop; main is also
// merged into every origin branch matching pipeline.sync.release, in name
// order. Any other released branch (develop, hotfix/*) syncs nothing.
func (s Sync) Targets(ctx context.Context) ([]SyncTarget, error) {
	released, develop, pattern := s.Release.Branch, s.Config.Sync.Develop, s.Config.Sync.Release
	main := s.Config.MainBranch()
	isRelease, _ := path.Match(pattern, released)
	if released != main && !isRelease {
		return nil, nil
	}
	targets := []SyncTarget{target(develop, released)}
	if released != main {
		return targets, nil
	}
	branches, err := s.Repo.Branches(ctx, pattern)
	if err != nil {
		return nil, err
	}
	for _, b := range branches {
		if b != main && b != develop {
			targets = append(targets, target(b, main))
		}
	}
	return targets, nil
}

func target(branch, source string) SyncTarget {
	return SyncTarget{Branch: branch, Source: source, Message: fmt.Sprintf("chore(sync): merge %s into %s [skip ci]", source, branch)}
}

// Run merges and pushes every target in order. A conflicting merge is
// aborted and the next target still runs, as dim's loop; the step fails at
// the end when any target conflicted or failed. A dry run only previews.
func (s Sync) Run(ctx context.Context, targets []SyncTarget) SyncResult {
	res := SyncResult{Step: "sync", Status: StatusPassed, DryRun: s.DryRun, Version: s.Release.Version, Source: s.Release.Branch, Branches: []BranchSync{}}
	var failed []string
	for i, t := range targets {
		if ctx.Err() != nil {
			// Record what did not run, so sync.json shows what is left.
			for _, rest := range targets[i:] {
				res.Branches = append(res.Branches, BranchSync{Branch: rest.Branch, Source: rest.Source, Status: SyncNotRun})
			}
			res.Status, res.Error = StatusInterrupted, "interrupted"
			res.Action = "the targets marked not-run were not merged; run nerv pipeline sync again (merged ones are then up-to-date)"
			return res
		}
		b := s.one(ctx, t)
		res.Branches = append(res.Branches, b)
		line := fmt.Sprintf("%s: %s", b.Branch, b.Status)
		switch {
		case b.Commit != "":
			line += " " + short(b.Commit)
		case len(b.Conflicts) > 0:
			line += " (" + strings.Join(b.Conflicts, ", ") + ")"
		}
		if b.Error != "" {
			line += ": " + b.Error
		}
		s.printf("%s\n", line)
		if b.Status == SyncConflict || b.Status == SyncFailed {
			failed = append(failed, b.Branch)
		}
	}
	if len(failed) > 0 {
		res.Status = StatusFailed
		res.Error = "merge-back failed for: " + strings.Join(failed, ", ")
		res.Action = "those branches are behind; for each: git checkout <branch> && git pull, git merge origin/<source>, resolve the conflicts, git push"
	}
	return res
}

func (s Sync) one(ctx context.Context, t SyncTarget) BranchSync {
	b := BranchSync{Branch: t.Branch, Source: t.Source}
	if s.DryRun {
		r, err := s.Repo.PreviewMerge(ctx, t.Branch, t.Source)
		switch {
		case err != nil:
			b.Status, b.Error = SyncFailed, err.Error()
		case r.UpToDate:
			b.Status = SyncUpToDate
		case r.Conflict:
			b.Status, b.Conflicts = SyncWouldConflict, r.Conflicts
		default:
			b.Status = SyncWouldMerge
		}
		return b
	}
	s.printf("merging %s into %s\n", t.Source, t.Branch)
	r, err := s.Repo.Merge(ctx, t.Branch, t.Source, t.Message)
	switch {
	case err != nil:
		b.Status, b.Error = SyncFailed, err.Error()
		return b
	case r.UpToDate:
		b.Status = SyncUpToDate
		return b
	case r.Conflict:
		b.Status, b.Conflicts = SyncConflict, r.Conflicts
		return b
	}
	b.Commit = r.Commit
	if err := s.Repo.PushBranch(ctx, t.Branch); err != nil {
		b.Status, b.Error = SyncFailed, err.Error()
		return b
	}
	b.Status = SyncPushed
	return b
}

func (s Sync) printf(format string, args ...any) {
	if s.Output != nil {
		fmt.Fprintf(s.Output, format, args...)
	}
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// SyncResultFile is the sync step's result.
const SyncResultFile = "sync.json"

// SyncResultPath is <repo>/.nerv/artifacts/pipeline/sync.json.
func SyncResultPath(repo string) string {
	return filepath.Join(repo, ".nerv", "artifacts", "pipeline", SyncResultFile)
}

// WriteSyncResult writes sync.json as WriteReleaseOutputs writes
// release.json.
func WriteSyncResult(repo string, r SyncResult) (string, error) {
	return writeArtifactJSON(repo, SyncResultPath(repo), r)
}

// RemoveSyncResult removes an earlier sync.json.
func RemoveSyncResult(repo string) error { return removeArtifact(repo, SyncResultPath(repo)) }

// ErrNoDockerResult is wrapped by ReadDockerResult when there is no
// docker.json.
var ErrNoDockerResult = errors.New("no docker result")

// ReadDockerResult reads docker.json, after the artifacts' symlink checks.
func ReadDockerResult(repo string) (DockerResult, error) {
	var d DockerResult
	err := readArtifactJSON(repo, DockerResultPath(repo), &d)
	if errors.Is(err, os.ErrNotExist) {
		return d, fmt.Errorf("%s: %w", DockerResultPath(repo), ErrNoDockerResult)
	}
	return d, err
}
