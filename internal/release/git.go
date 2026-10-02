package release

import (
	"context"
	"fmt"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
)

const gitLogFormat = "%H%x1f%h%x1f%s%x1f%b%x1e"

// Tags returns the tag names matching pattern (e.g. "v*") in repoDir, one
// per line as git prints them, in the order git reports (no ordering is
// applied here -- LastReleaseTag does the semantic-version ordering). This
// is the thin git access half of Get-NervLastReleaseTag
// ("git tag -l <pattern>").
func Tags(ctx context.Context, runner env.Runner, repoDir, pattern string) ([]string, error) {
	stdout, stderr, exitCode, err := runner.Run(ctx, "git", "-C", repoDir, "tag", "-l", pattern)
	if err != nil {
		return nil, fmt.Errorf("launching git tag in %q: %w", repoDir, err)
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("git tag failed in %q (exit %d): %s", repoDir, exitCode, strings.TrimSpace(stderr))
	}

	var tags []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			tags = append(tags, line)
		}
	}
	return tags, nil
}

// CommitsSince returns the commits reachable from HEAD since tag (or the
// whole history when tag is ""), oldest first, merge commits excluded.
// Ports the git-access half of Get-NervCommitsSince; ParseCommits does the
// record parsing.
func CommitsSince(ctx context.Context, runner env.Runner, repoDir, tag string) ([]Commit, error) {
	args := []string{"-C", repoDir, "log", "--no-merges", "--reverse", "--pretty=format:" + gitLogFormat}
	if tag != "" {
		args = append(args, tag+"..HEAD")
	}

	stdout, stderr, exitCode, err := runner.Run(ctx, "git", args...)
	if err != nil {
		return nil, fmt.Errorf("launching git log in %q: %w", repoDir, err)
	}
	if exitCode != 0 {
		rangeDesc := "HEAD"
		if tag != "" {
			rangeDesc = tag + "..HEAD"
		}
		return nil, fmt.Errorf("git log failed in %q for range %q (exit %d): %s", repoDir, rangeDesc, exitCode, strings.TrimSpace(stderr))
	}

	return ParseCommits(stdout), nil
}
