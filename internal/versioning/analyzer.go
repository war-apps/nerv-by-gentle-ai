package versioning

import (
	"regexp"
	"slices"
	"strings"
)

// This file ports @semantic-release/commit-analyzer 13 (index.js,
// lib/analyze-commit.js, lib/default-release-rules.js,
// lib/compare-release-types.js) and conventional-commits-filter 5
// (RevertedCommitsFilter.js, utils.js).

// releaseTypeOrder is commit-analyzer's RELEASE_TYPES, highest first.
var releaseTypeOrder = []string{"major", "premajor", "minor", "preminor", "patch", "prepatch", "prerelease"}

// releaseTypeRank is the index in releaseTypeOrder; ReleaseNone (false) is
// -1, as indexOf(false) is.
func releaseTypeRank(t string) int { return slices.Index(releaseTypeOrder, t) }

// higherRelease is compareReleaseTypes: whether candidate replaces current.
// current "" is undefined, null or false (all falsy); ReleaseNone ranks -1,
// so it replaces any release type.
func higherRelease(current, candidate string) bool {
	return current == "" || current == ReleaseNone || releaseTypeRank(candidate) < releaseTypeRank(current)
}

// defaultReleaseRules are commit-analyzer's default rules that can match a
// conventionalcommits commit (the Atom, Ember, ESLint and Express rules
// match fields the preset's parser never sets).
var defaultReleaseRules = []ReleaseRule{
	{Breaking: true, Release: ReleaseMajor},
	{Revert: true, Release: ReleasePatch},
	{Type: "feat", Release: ReleaseMinor},
	{Type: "fix", Release: ReleasePatch},
	{Type: "perf", Release: ReleasePatch},
	{Type: "FEAT", Release: ReleaseMinor},
	{Type: "FIX", Release: ReleasePatch},
}

// ruleMatches is the analyze-commit.js filter: breaking needs a note,
// revert needs a revert commit, and type and scope match as micromatch globs
// (a rule field on a commit without that field never matches).
func ruleMatches(r ReleaseRule, c commit) bool {
	if r.Breaking && len(c.notes) == 0 {
		return false
	}
	if r.Revert && c.revert == nil {
		return false
	}
	if r.Type != "" && (c.typ == "" || !globMatch(c.typ, r.Type)) {
		return false
	}
	if r.Scope != "" && (c.scope == "" || !globMatch(c.scope, r.Scope)) {
		return false
	}
	return true
}

// analyzeCommit returns the highest release type of the rules matching c;
// matched is false when no rule matches (undefined).
func analyzeCommit(rules []ReleaseRule, c commit) (release string, matched bool) {
	for _, r := range rules {
		if !ruleMatches(r, c) {
			continue
		}
		matched = true
		if higherRelease(release, r.Release) {
			release = r.Release
			if release == ReleaseMajor {
				break
			}
		}
	}
	return release, matched
}

// analyzeCommits is commit-analyzer's analyzeCommits: the release type of
// commits (newest first), "" for no release.
func analyzeCommits(rules []ReleaseRule, commits []commit) string {
	release := ""
	for _, c := range filterReverted(commits) {
		var t string
		matched := false
		if len(rules) > 0 {
			t, matched = analyzeCommit(rules, c)
		}
		if !matched {
			t, _ = analyzeCommit(defaultReleaseRules, c)
		}
		if t != "" && t != ReleaseNone && higherRelease(release, t) {
			release = t
		}
		if release == ReleaseMajor {
			break
		}
	}
	return release
}

// filterReverted drops every revert commit together with the commit it
// reverts when both are in commits (newest first), keeping the order of the
// others (RevertedCommitsFilter).
func filterReverted(commits []commit) []commit {
	var out []commit
	type held struct {
		c       commit
		dropped bool
	}
	var hold []*held
	reverts := 0
	flush := func() {
		for _, h := range hold {
			if !h.dropped {
				out = append(out, h.c)
			}
		}
		hold = hold[:0]
	}
	for _, c := range commits {
		found := false
		for _, h := range hold {
			if !h.dropped && h.c.revert != nil && revertMatches(c, *h.c.revert) {
				h.dropped = true
				reverts--
				found = true
				break
			}
		}
		if found {
			continue
		}
		if c.revert != nil {
			hold = append(hold, &held{c: c})
			reverts++
			continue
		}
		if reverts > 0 {
			hold = append(hold, &held{c: c})
			continue
		}
		flush()
		out = append(out, c)
	}
	flush()
	return out
}

// revertMatches is conventional-commits-filter's isMatch(commit, revert):
// the trimmed header and hash are equal.
func revertMatches(c commit, r revertInfo) bool {
	return strings.TrimSpace(c.header) == strings.TrimSpace(r.header) && strings.TrimSpace(c.hash) == strings.TrimSpace(r.hash)
}

// raiseReleaseType applies depends_on: the project gets at least minimum
// (the highest bump among its released dependencies); a project with no
// release of its own gets minimum.
func raiseReleaseType(analyzed, minimum string) string {
	order := []string{ReleasePatch, ReleaseMinor, ReleaseMajor}
	if minimum == "" {
		return analyzed
	}
	if analyzed == "" {
		return minimum
	}
	if slices.Index(order, minimum) > slices.Index(order, analyzed) {
		return minimum
	}
	return analyzed
}

// globMatch is micromatch.isMatch for the patterns release rules, branches
// and paths use: * and ? within a path segment, ** across segments, [...]
// classes, {a,b} alternatives and a leading ! negation.
func globMatch(s, pattern string) bool {
	negate := false
	for strings.HasPrefix(pattern, "!") {
		negate = !negate
		pattern = pattern[1:]
	}
	re, err := globRegexp(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(s) != negate
}

// globRegexp compiles a glob to an anchored regular expression.
func globRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	depth := 0
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			b.WriteString("(?:[^/]*/)*")
			i += 2
		case strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i++
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		case c == '\\' && i+1 < len(pattern):
			i++
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		case c == '[':
			end := strings.IndexByte(pattern[i+1:], ']')
			if end < 0 {
				b.WriteString(`\[`)
				continue
			}
			class := pattern[i+1 : i+1+end]
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			b.WriteString("[" + strings.ReplaceAll(class, `\`, `\\`) + "]")
			i += end + 1
		case c == '{':
			depth++
			b.WriteString("(?:")
		case c == '}' && depth > 0:
			depth--
			b.WriteString(")")
		case c == ',' && depth > 0:
			b.WriteString("|")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	for ; depth > 0; depth-- {
		b.WriteString(")")
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
