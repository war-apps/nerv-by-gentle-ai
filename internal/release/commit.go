// Package release computes the next semantic version from Conventional
// Commits, edits plugin.json and CHANGELOG.md the same way tools/release.ps1
// and tools/release-guard.ps1 did, and checks release readiness. Every
// function in this package is pure over its inputs except the thin git
// access helpers in git.go, which run "git" through an injected
// env.Runner.
package release

import (
	"regexp"
	"strings"
)

// Commit is one git commit as consumed by the bump/changelog computations.
// Mirrors the {Sha, ShortSha, Subject, Body} shape Get-NervCommitsSince
// returned.
type Commit struct {
	Sha      string
	ShortSha string
	Subject  string
	Body     string
}

// Info is the Conventional Commits breakdown of one commit's subject/body,
// as returned by Get-NervConventionalCommitInfo.
type Info struct {
	Conventional bool
	Type         string
	Scope        string
	Breaking     bool
	Description  string
}

var conventionalPattern = regexp.MustCompile(`^([a-zA-Z]+)(\(([^)]*)\))?(!)?:\s*(.*)$`)

var breakingChangeFooter = regexp.MustCompile(`(?m)^BREAKING CHANGE:`)

// ConventionalInfo parses subject/body into Conventional Commits parts.
// Ports Get-NervConventionalCommitInfo.
func ConventionalInfo(subject, body string) Info {
	m := conventionalPattern.FindStringSubmatch(subject)
	if m == nil {
		return Info{Description: subject}
	}

	breaking := m[4] == "!"
	if !breaking && body != "" && breakingChangeFooter.MatchString(body) {
		breaking = true
	}

	return Info{
		Conventional: true,
		Type:         strings.ToLower(m[1]),
		Scope:        m[3],
		Breaking:     breaking,
		Description:  m[5],
	}
}

var bumpRank = map[string]int{"none": 0, "patch": 1, "minor": 2, "major": 3}

// BumpKind reduces a list of commits to one bump kind ("major", "minor",
// "patch", or "none"); the highest wins across all commits. Ports
// Get-NervBumpKind.
func BumpKind(commits []Commit) string {
	best := "none"
	for _, c := range commits {
		info := ConventionalInfo(c.Subject, c.Body)

		kind := "none"
		switch {
		case info.Breaking:
			kind = "major"
		case info.Conventional:
			switch info.Type {
			case "feat":
				kind = "minor"
			case "fix", "perf":
				kind = "patch"
			}
		}

		if bumpRank[kind] > bumpRank[best] {
			best = kind
		}
	}
	return best
}

const (
	unitSeparator   = "\x1f"
	recordSeparator = "\x1e"
)

// ParseCommits parses the output of
// `git log --no-merges --reverse --pretty=format:%H%x1f%h%x1f%s%x1f%b%x1e`
// into a slice of Commit, oldest first. Ports the record-splitting half of
// Get-NervCommitsSince.
func ParseCommits(gitLogOutput string) []Commit {
	if gitLogOutput == "" {
		return nil
	}

	records := strings.Split(gitLogOutput, recordSeparator)
	var commits []Commit
	for _, rec := range records {
		clean := strings.TrimLeft(rec, "\r\n")
		if strings.Trim(clean, "\r\n") == "" {
			continue
		}
		parts := strings.SplitN(clean, unitSeparator, 4)
		if len(parts) < 4 {
			continue
		}
		commits = append(commits, Commit{
			Sha:      parts[0],
			ShortSha: parts[1],
			Subject:  parts[2],
			Body:     strings.Trim(parts[3], "\r\n"),
		})
	}
	return commits
}
