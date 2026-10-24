package versioning

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
)

// This file ports semantic-release 24.2's run (index.js) for a dry run:
// lib/branches/get-tags.js, lib/get-last-release.js, lib/get-commits.js and
// lib/get-next-version.js, with nerv's per-project commit filter and
// depends_on raise (the former semantic-release node runner's makeFilter
// and raiseReleaseType, nerv-semrel-lib.mjs).

// Compute returns the next release of each project on the branch, in
// dependency order (only the requested projects when req.Projects is set).
// A branch that matches no configured branch gives results without a
// release, with a reason; configuration and git problems are errors.
func Compute(ctx context.Context, req Request) ([]Result, error) {
	runner := req.Runner
	if runner == nil {
		runner = env.ExecRunner{}
	}
	now := req.Now
	if now == nil {
		now = time.Now
	}
	cfg := req.Config.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if req.Version != "" {
		if _, ok := parseVersion(req.Version); !ok {
			return nil, fmt.Errorf("version %q is not a semantic version", req.Version)
		}
	}

	g := gitReader{runner: runner, dir: req.RepoDir}
	top, err := g.trimmed(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}
	g.dir = filepath.Clean(top)

	projects, wanted, err := planProjects(cfg, filepath.Base(g.dir), req.Projects)
	if err != nil {
		return nil, err
	}

	branch := req.Branch
	if branch == "" {
		if branch, err = g.trimmed(ctx, "symbolic-ref", "--short", "HEAD"); err != nil {
			return nil, fmt.Errorf("the current checkout is not on a branch; pass a branch: %w", err)
		}
	}
	head, err := g.trimmed(ctx, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch+"^{commit}")
	if err != nil || head == "" {
		if head, err = g.trimmed(ctx, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+branch+"^{commit}"); err != nil || head == "" {
			return nil, fmt.Errorf("branch %q: neither a local branch nor one of origin's", branch)
		}
	}

	origin := g.originURL(ctx)
	if !req.Offline && origin != "" {
		if err := g.fetchReleases(ctx); err != nil {
			return nil, err
		}
	}
	repoURL := publicURL(cfg.RepositoryURL)
	if cfg.RepositoryURL == "" {
		repoURL = publicURL(origin)
	}

	names, err := g.branchNames(ctx, branch)
	if err != nil {
		return nil, err
	}
	resolved, err := expandBranches(cfg.Branches, names)
	if err != nil {
		return nil, err
	}
	idx := slices.IndexFunc(resolved, func(b resolvedBranch) bool { return b.name == branch })
	if idx < 0 {
		var out []Result
		for _, p := range projects {
			if wanted[p.Name] {
				out = append(out, Result{
					Project: p.Name, Branch: branch, Changelog: p.Changelog,
					Reason: fmt.Sprintf("branch %s is not a release branch (the configured branches are %s)", branch, branchNames(cfg.Branches)),
				})
			}
		}
		return out, nil
	}
	current := resolved[idx]

	tagsOut, err := g.run(ctx, "tag", "--merged", head)
	if err != nil {
		return nil, err
	}
	var tags []string
	for line := range strings.Lines(tagsOut) {
		if t := strings.TrimSpace(line); t != "" {
			tags = append(tags, t)
		}
	}
	notes, err := g.tagNotes(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p.Path != "." {
			if err := g.checkVersion(ctx); err != nil {
				return nil, err
			}
			break
		}
	}

	date := now().UTC().Format("2006-01-02")
	results := map[string]Result{}
	var out []Result
	for _, p := range projects {
		explicit := ""
		if wanted[p.Name] {
			explicit = req.Version
		}
		r, err := computeProject(ctx, g, cfg, p, current, head, tags, notes, results, repoURL, date, explicit)
		if err != nil {
			return nil, fmt.Errorf("project %s: %w", p.Name, err)
		}
		results[p.Name] = r
		if wanted[p.Name] {
			out = append(out, r)
		}
	}
	return out, nil
}

// planProjects returns the projects to compute in dependency order, and
// which of them were asked for: every project, or the selected ones and
// (computed but not returned) the projects they depend on. With no project
// configured the whole repository is one project named repoName.
func planProjects(cfg Config, repoName string, selected []string) ([]Project, map[string]bool, error) {
	projects := cfg.Projects
	if len(projects) == 0 {
		projects = []Project{{Name: repoName, Path: ".", TagFormat: cfg.TagFormat, Changelog: cfg.Changelog.File}}
	}
	ordered, err := topoOrder(projects)
	if err != nil {
		return nil, nil, err
	}
	wanted := map[string]bool{}
	if len(selected) == 0 {
		for _, p := range ordered {
			wanted[p.Name] = true
		}
		return ordered, wanted, nil
	}
	byName := map[string]Project{}
	for _, p := range ordered {
		byName[p.Name] = p
	}
	keep := map[string]bool{}
	var mark func(name string)
	mark = func(name string) {
		if keep[name] {
			return
		}
		keep[name] = true
		for _, d := range byName[name].DependsOn {
			mark(d.Project)
		}
	}
	for _, name := range selected {
		if _, ok := byName[name]; !ok {
			return nil, nil, fmt.Errorf("unknown project %q", name)
		}
		wanted[name] = true
		mark(name)
	}
	var out []Project
	for _, p := range ordered {
		if keep[p.Name] {
			out = append(out, p)
		}
	}
	return out, wanted, nil
}

// releaseTag is one tag of the project's format reachable from the branch.
type releaseTag struct {
	tag      string
	version  string
	parsed   version
	channels []string
}

// tagRegexp is semantic-release's tag pattern: the format with ${version}
// as (.+), anchored at the start only.
func tagRegexp(format string) *regexp.Regexp {
	prefix, suffix, _ := strings.Cut(format, versionPlaceholder)
	return regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `(.+)` + regexp.QuoteMeta(suffix))
}

// projectTags selects the tags of a format (get-tags.js): a valid version
// in the placeholder; channels from the semantic-release notes, the default
// channel when a tag has none.
func projectTags(format string, tags []string, notes map[string][]string) []releaseTag {
	re := tagRegexp(format)
	var out []releaseTag
	for _, t := range tags {
		m := re.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		v, ok := parseVersion(m[1])
		if !ok {
			continue
		}
		channels, ok := notes[t]
		if !ok {
			channels = []string{""}
		}
		out = append(out, releaseTag{tag: t, version: m[1], parsed: v, channels: channels})
	}
	return out
}

// lastRelease is get-last-release.js: on a prerelease branch the highest
// stable tag or prerelease tag of the branch's channel and identifier; on
// a release branch the highest stable tag.
func lastRelease(b resolvedBranch, tags []releaseTag) (releaseTag, bool) {
	var candidates []releaseTag
	for _, t := range tags {
		ok := !t.parsed.isPrerelease()
		if b.kind == branchPrerelease && slices.ContainsFunc(t.channels, func(c string) bool { return c == b.channel }) && t.parsed.hasPrereleaseID(b.prerelease) {
			ok = true
		}
		if ok {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 0 {
		return releaseTag{}, false
	}
	slices.SortStableFunc(candidates, func(x, y releaseTag) int { return compareVersions(y.parsed, x.parsed) })
	return candidates[0], true
}

// nextVersion is get-next-version.js.
func nextVersion(b resolvedBranch, releaseType string, last releaseTag, hasLast bool, tags []releaseTag) string {
	if !hasLast {
		if b.kind == branchPrerelease {
			return "1.0.0-" + b.prerelease + ".1"
		}
		return "1.0.0"
	}
	if b.kind != branchPrerelease {
		return last.parsed.inc(releaseType).String()
	}
	sameChannel := slices.ContainsFunc(last.channels, func(c string) bool { return c == b.channel })
	if last.parsed.isPrerelease() && sameChannel {
		latest := tags[0].parsed
		for _, t := range tags[1:] {
			if compareVersions(t.parsed, latest) > 0 {
				latest = t.parsed
			}
		}
		return highestVersion(last.parsed.inc("prerelease").String(), latest.inc(releaseType).String()+"-"+b.prerelease+".1")
	}
	base := version{major: last.parsed.major, minor: last.parsed.minor, patch: last.parsed.patch}
	return base.inc(releaseType).String() + "-" + b.prerelease + ".1"
}

// minReleaseType is the lowest release a project gets from its dependencies
// released in this run: the highest bump among them, ignoring none.
func minReleaseType(deps []Dependency, results map[string]Result) string {
	order := []string{ReleasePatch, ReleaseMinor, ReleaseMajor}
	best := -1
	for _, d := range deps {
		if d.Bump == ReleaseNone || !results[d.Project].NewReleasePublished {
			continue
		}
		best = max(best, slices.Index(order, d.Bump))
	}
	if best < 0 {
		return ""
	}
	return order[best]
}

var scopesRe = regexp.MustCompile(`^[\w-]+\(([^)]*)\)!?:`)

// commitScopes reads the scopes of a conventional commit header
// ("type(a, b)!: subject").
func commitScopes(message string) []string {
	header, _, _ := strings.Cut(message, "\n")
	m := scopesRe.FindStringSubmatch(header)
	if m == nil {
		return nil
	}
	var out []string
	for _, s := range strings.Split(m[1], ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// projectFilter keeps the commits of a project: a header scope in scopes,
// or a file under (or matching) one of paths.
func projectFilter(p Project) func(logCommit) bool {
	if p.Path == "." {
		return nil
	}
	paths := append([]string{p.Path}, p.SharedPaths...)
	for i := range paths {
		paths[i] = strings.ReplaceAll(paths[i], `\`, "/")
	}
	touches := func(file string) bool {
		for _, path := range paths {
			if path == "." || file == path || strings.HasPrefix(file, path+"/") || globMatch(file, path) {
				return true
			}
		}
		return false
	}
	return func(c logCommit) bool {
		for _, s := range commitScopes(c.message) {
			if slices.Contains(p.Scopes, s) {
				return true
			}
		}
		return slices.ContainsFunc(c.files, touches)
	}
}

func computeProject(ctx context.Context, g gitReader, cfg Config, p Project, b resolvedBranch, head string,
	tags []string, notes map[string][]string, done map[string]Result, repoURL, date, explicit string) (Result, error) {
	r := Result{Project: p.Name, Branch: b.name, Changelog: p.Changelog}
	ptags := projectTags(p.TagFormat, tags, notes)
	last, hasLast := lastRelease(b, ptags)
	rangeSpec := head
	since := "the start of the history"
	if hasLast {
		lastHead, err := g.trimmed(ctx, "rev-list", "-1", last.tag)
		if err != nil {
			return r, err
		}
		r.LastReleaseVersion, r.LastReleaseGitTag, r.LastReleaseGitHead = last.version, last.tag, lastHead
		rangeSpec = lastHead + ".." + head
		since = last.tag
	}
	filter := projectFilter(p)
	logged, err := g.commits(ctx, rangeSpec, filter != nil)
	if err != nil {
		return r, err
	}
	var commits []commit
	analyzer := newParser(parserOptions{noteKeywords: cfg.NoteKeywords, referenceActions: defaultReferenceActions, issuePrefixes: []string{"#"}})
	for _, lc := range logged {
		if filter != nil && !filter(lc) {
			continue
		}
		if strings.TrimSpace(lc.message) == "" {
			continue
		}
		c := analyzer.parse(lc.hash, lc.message)
		c.files = lc.files
		commits = append(commits, c)
	}
	releaseType := raiseReleaseType(analyzeCommits(cfg.ReleaseRules, commits), minReleaseType(p.DependsOn, done))
	if releaseType == "" && explicit == "" {
		if len(commits) == 0 {
			r.Reason = fmt.Sprintf("no commit of the project since %s", since)
		} else {
			r.Reason = fmt.Sprintf("no commit since %s triggers a release (only feat, fix, perf, reverts and breaking changes do, or the configured release rules)", since)
		}
		return r, nil
	}
	var v string
	if explicit != "" {
		v = explicit
	} else {
		v = nextVersion(b, releaseType, last, hasLast, ptags)
	}
	tag := strings.Replace(p.TagFormat, versionPlaceholder, v, 1)
	if explicit != "" {
		existing, err := g.run(ctx, "tag", "--list", tag)
		if err != nil {
			return r, err
		}
		if strings.TrimSpace(existing) != "" {
			return r, fmt.Errorf("version %s: the tag %s already exists", v, tag)
		}
	}
	parsed, _ := parseVersion(v)
	r.NewReleasePublished = true
	r.NewReleaseVersion = v
	r.NewReleaseMajorVersion = fmt.Sprint(parsed.major)
	r.NewReleaseMinorVersion = fmt.Sprint(parsed.minor)
	r.NewReleasePatchVersion = fmt.Sprint(parsed.patch)
	r.NewReleaseChannel = b.channel
	r.NewReleaseGitHead = head
	r.NewReleaseGitTag = tag
	r.Bump = releaseType
	var raw []commit
	for _, c := range commits {
		raw = append(raw, commit{hash: c.hash, message: c.message, files: c.files})
	}
	r.NewReleaseNotes = generateNotes(notesInput{
		commits: raw, version: v, previousTag: r.LastReleaseGitTag, currentTag: tag, date: date, repoURL: repoURL,
	}, cfg.NoteKeywords)
	r.ChangelogSection = strings.TrimSpace(r.NewReleaseNotes) + "\n"
	return r, nil
}
