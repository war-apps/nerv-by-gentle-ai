package versioning

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
)

// gitReader reads the repository through an env.Runner; it never writes to
// it, except for the fetch of origin's tags and notes outside offline mode.
type gitReader struct {
	runner env.Runner
	dir    string
}

func (g gitReader) run(ctx context.Context, args ...string) (string, error) {
	stdout, stderr, code, err := g.runner.Run(ctx, "git", append([]string{"-C", g.dir}, args...)...)
	if err != nil {
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = fmt.Sprintf("exit code %d", code)
		}
		return stdout, &gitError{args: args, code: code, msg: msg}
	}
	return stdout, nil
}

// gitError is a git command that ran and failed.
type gitError struct {
	args []string
	code int
	msg  string
}

func (e *gitError) Error() string { return "git " + e.args[0] + ": " + e.msg }

func (g gitReader) trimmed(ctx context.Context, args ...string) (string, error) {
	out, err := g.run(ctx, args...)
	return strings.TrimSpace(out), err
}

// minGitDiffMerges is the oldest git whose log accepts
// --diff-merges=first-parent, used to list the files of each commit.
var minGitDiffMerges = [3]int{2, 31, 0}

// decoratedTagRe reads the tags of a %d decoration.
var decoratedTagRe = regexp.MustCompile(`tag: ([^,)]+)`)

var gitVersionRe = regexp.MustCompile(`^git version (\d+)\.(\d+)(?:\.(\d+))?`)

func (g gitReader) checkVersion(ctx context.Context) error {
	out, err := g.trimmed(ctx, "--version")
	if err != nil {
		return err
	}
	m := gitVersionRe.FindStringSubmatch(out)
	if m == nil {
		return fmt.Errorf("unrecognised git version %q", out)
	}
	var v [3]int
	for i := range 3 {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	for i := range 3 {
		if v[i] != minGitDiffMerges[i] {
			if v[i] < minGitDiffMerges[i] {
				return fmt.Errorf("git %d.%d.%d is too old: selecting a project's commits by path needs git %d.%d.%d or newer (git log --diff-merges)",
					v[0], v[1], v[2], minGitDiffMerges[0], minGitDiffMerges[1], minGitDiffMerges[2])
			}
			break
		}
	}
	return nil
}

// branchNames lists the branches semantic-release would see: origin's
// (refs/remotes/origin/*) when the repository has any, its local ones
// otherwise, plus the computed branch.
func (g gitReader) branchNames(ctx context.Context, current string) ([]string, error) {
	out, err := g.run(ctx, "for-each-ref", "--format=%(refname)", "refs/heads/", "refs/remotes/origin/")
	if err != nil {
		return nil, err
	}
	var local, remote []string
	for line := range strings.Lines(out) {
		ref := strings.TrimSpace(line)
		if name, ok := strings.CutPrefix(ref, "refs/remotes/origin/"); ok && name != "HEAD" {
			remote = append(remote, name)
		} else if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			local = append(local, name)
		}
	}
	names := local
	if len(remote) > 0 {
		names = remote
	}
	if !slices.Contains(names, current) {
		names = append(names, current)
	}
	slices.Sort(names)
	return names, nil
}

// tagNotes reads semantic-release's channel notes of every tag
// (lib/git.js getTagsNotes): tag -> channels, "" for the default channel.
func (g gitReader) tagNotes(ctx context.Context) (map[string][]string, error) {
	out, err := g.run(ctx, "log", "--tags=*", "--decorate-refs=refs/tags/*", "--no-walk", "--format=%d%x09%N", "--notes=refs/notes/semantic-release*")
	if err != nil {
		return nil, err
	}
	notes := map[string][]string{}
	for line := range strings.Lines(out) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		tagPart, notePart, _ := strings.Cut(line, "\t")
		var parsed struct {
			Channels []*string `json:"channels"`
		}
		if json.Unmarshal([]byte(notePart), &parsed) != nil {
			continue
		}
		channels := make([]string, len(parsed.Channels))
		for i, c := range parsed.Channels {
			if c != nil {
				channels[i] = *c
			}
		}
		for _, m := range decoratedTagRe.FindAllStringSubmatch(tagPart, -1) {
			notes[m[1]] = channels
		}
	}
	return notes, nil
}

// logCommit is one commit of `git log`.
type logCommit struct {
	hash    string
	message string
	files   []string
}

// commits lists the commits of a range (newest first, git log's order),
// with their files when withFiles: a merge commit's files are its diff
// against its first parent.
func (g gitReader) commits(ctx context.Context, rangeSpec string, withFiles bool) ([]logCommit, error) {
	args := []string{"log", "-z", "--format=%x01%H%x02%B%x03"}
	if withFiles {
		args = append(args, "--name-only", "--no-renames", "--diff-merges=first-parent")
	}
	args = append(args, rangeSpec, "--")
	out, err := g.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	var commits []logCommit
	for _, rec := range strings.Split(out, "\x01")[1:] {
		hash, rest, _ := strings.Cut(rec, "\x02")
		message, files, _ := strings.Cut(rest, "\x03")
		c := logCommit{hash: strings.TrimSpace(hash), message: strings.TrimSpace(message)}
		for _, f := range strings.Split(files, "\x00") {
			if f = strings.TrimPrefix(f, "\n"); f != "" {
				c.files = append(c.files, f)
			}
		}
		commits = append(commits, c)
	}
	return commits, nil
}

// originURL returns remote.origin.url, "" when there is none.
func (g gitReader) originURL(ctx context.Context) string {
	out, err := g.trimmed(ctx, "config", "--get", "remote.origin.url")
	if err != nil {
		return ""
	}
	return out
}

// fetchReleases fetches origin's tags and semantic-release notes as
// semantic-release does before computing (lib/git.js fetch and fetchNotes).
func (g gitReader) fetchReleases(ctx context.Context) error {
	_, err := g.run(ctx, "-c", "credential.interactive=never", "fetch", "--quiet", "--tags", "origin", "+refs/notes/semantic-release*:refs/notes/semantic-release*")
	if err != nil {
		return fmt.Errorf("reading origin's release tags and notes (use offline mode to skip): %w", err)
	}
	return nil
}

var (
	urlSchemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)
	scpLikeRe   = regexp.MustCompile(`^[^/\\:]+:`)
	driveRe     = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
)

// publicSchemes are the URL schemes of a remote other people can reach.
var publicSchemes = []string{"http", "https", "ssh", "git", "git+ssh", "ssh+git"}

// publicURL is the address of a remote to name in release-notes links (as
// internal/semrel publicURL): scheme URLs lose their user info, query and
// fragment, scp-like ssh addresses stay as they are, and a local
// repository (a path, a file: URL) gives "".
func publicURL(raw string) string {
	switch {
	case raw == "", driveRe.MatchString(raw), filepath.IsAbs(raw), strings.HasPrefix(raw, "/"), strings.HasPrefix(raw, `\`):
		return ""
	case urlSchemeRe.MatchString(raw):
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || !slices.Contains(publicSchemes, strings.ToLower(u.Scheme)) {
			return ""
		}
		u.User, u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = nil, "", false, "", ""
		return u.String()
	case strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") || strings.HasPrefix(raw, `.\`) || strings.HasPrefix(raw, `..\`):
		return ""
	case scpLikeRe.MatchString(raw):
		return raw
	}
	return ""
}
