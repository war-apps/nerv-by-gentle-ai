package semrel

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// minGitPathFilter is the oldest git whose diff-tree accepts
// --diff-merges=first-parent, which the runner's path filter uses to list
// the files of each commit (runner/nerv-semrel-lib.mjs, listCommitFiles).
var minGitPathFilter = [3]int{2, 31, 0}

// gitVersionRe matches `git --version` output: "git version 2.43.0",
// "git version 2.39.3 (Apple Git-146)", "git version 2.45.1.windows.1".
// A missing patch number counts as 0.
var gitVersionRe = regexp.MustCompile(`^git version (\d+)\.(\d+)(?:\.(\d+))?`)

// parseGitVersion returns the major, minor and patch of a `git --version`
// output.
func parseGitVersion(out string) ([3]int, error) {
	m := gitVersionRe.FindStringSubmatch(strings.TrimSpace(out))
	if m == nil {
		return [3]int{}, fmt.Errorf("unrecognised git version %q", strings.TrimSpace(out))
	}
	var v [3]int
	for i := range 3 {
		v[i], _ = strconv.Atoi(m[i+1]) // "" (no patch) -> 0
	}
	return v, nil
}

// versionAtLeast reports whether v >= minimum.
func versionAtLeast(v, minimum [3]int) bool {
	for i := range 3 {
		if v[i] != minimum[i] {
			return v[i] > minimum[i]
		}
	}
	return true
}

// needsPathFilter reports whether any project selects its commits by path.
func needsPathFilter(projects []project) bool {
	for _, p := range projects {
		if p.Filter != nil {
			return true
		}
	}
	return false
}

// checkGitForPathFilter fails, naming the found and the minimum version,
// when git is too old for the runner's path filter.
func (e *Engine) checkGitForPathFilter(ctx context.Context, env []string) error {
	res, err := e.run(ctx, Command{Name: "git", Args: []string{"--version"}, Env: env})
	if err != nil {
		return err
	}
	v, err := parseGitVersion(string(res.Stdout))
	if err != nil {
		return err
	}
	if !versionAtLeast(v, minGitPathFilter) {
		return fmt.Errorf("git %d.%d.%d is too old: selecting a project's commits by path needs git %d.%d.%d or newer (git diff-tree --diff-merges)",
			v[0], v[1], v[2], minGitPathFilter[0], minGitPathFilter[1], minGitPathFilter[2])
	}
	return nil
}
