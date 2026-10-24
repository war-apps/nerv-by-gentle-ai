package versioning

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// This file ports @semantic-release/release-notes-generator 14 (index.js,
// lib/hosts-config.js), conventional-changelog-writer 8 (context.js,
// template.js, utils.js) and the conventionalcommits preset 8 (writer.js,
// constants.js, templates/*.hbs), plus nerv's own handling of a repository
// without a public address (internal/semrel/runner/nerv-semrel-lib.mjs
// NO_REMOTE_URL and stripNoRemoteLinks).

// noRemoteURL stands in for the repository URL when there is no public
// address; every link to it is then reduced to its text (see
// stripNoRemoteLinks), exactly as nerv's semantic-release runner does.
const noRemoteURL = "https://nerv-no-remote.invalid/repository/repository"

var noRemoteLinkRe = regexp.MustCompile(`\[([^\]]*)\]\(` + regexp.QuoteMeta(noRemoteURL) + `[^)]*\)`)

// stripNoRemoteLinks replaces every Markdown link to noRemoteURL with its
// text.
func stripNoRemoteLinks(notes string) string {
	return noRemoteLinkRe.ReplaceAllString(notes, "$1")
}

// commitType is one entry of the preset's DEFAULT_COMMIT_TYPES.
type commitType struct {
	typ, section string
	hidden       bool
}

var defaultCommitTypes = []commitType{
	{"feat", "Features", false},
	{"feature", "Features", false},
	{"fix", "Bug Fixes", false},
	{"perf", "Performance Improvements", false},
	{"revert", "Reverts", false},
	{"docs", "Documentation", true},
	{"style", "Styles", true},
	{"chore", "Miscellaneous Chores", true},
	{"refactor", "Code Refactoring", true},
	{"test", "Tests", true},
	{"build", "Build System", true},
	{"ci", "Continuous Integration", true},
}

// commitGroupOrder is the preset's section order.
var commitGroupOrder = func() []string {
	var out []string
	for _, t := range defaultCommitTypes {
		out = append(out, t.section)
	}
	return out
}()

// hostReferenceActions are release-notes-generator's reference actions per
// host (hosts-config.js); other hosts get the default entry.
var hostReferenceActions = map[string][]string{
	"github.com":    {"close", "closes", "closed", "fix", "fixes", "fixed", "resolve", "resolves", "resolved"},
	"bitbucket.org": {"close", "closes", "closed", "closing", "fix", "fixes", "fixed", "fixing", "resolve", "resolves", "resolved", "resolving"},
	"gitlab.com":    {"close", "closes", "closed", "closing", "fix", "fixes", "fixed", "fixing"},
	"":              {"close", "closes", "closed", "closing", "fix", "fixes", "fixed", "fixing", "resolve", "resolves", "resolved", "resolving"},
}

// repoContext is the repository part of the writer's context.
type repoContext struct {
	host       string // "https://github.com"
	hostname   string
	owner      string
	repository string
	ownerSet   bool
	repoSet    bool
}

var (
	scpURLRe     = regexp.MustCompile(`^(?:(.*)@)?(.*?):(.*)$`)
	ownerRepoRe  = regexp.MustCompile(`^/([^/]+)?/?(.+)?$`)
	httpNotSRe   = regexp.MustCompile(`http[^s]`)
	releaseAsRe  = regexp.MustCompile(`(?i)release-as:\s*\w*@?([0-9]+\.[0-9]+\.[0-9a-z]+(-[0-9a-z.]+)?)\s*`)
	subjectIssue = regexp.MustCompile(`(#)([a-z0-9]+)`)
	dotGitRe     = regexp.MustCompile(`(?i)\.git$`)
	subjectUser  = regexp.MustCompile(`\B@([a-z0-9](?:-?[a-z0-9/]){0,38})`)
)

// parseRepositoryURL reads a repository URL as release-notes-generator
// does: an scp-like address becomes ssh://, the protocol is http or https,
// and the path gives the owner and the repository.
func parseRepositoryURL(raw string) (repoContext, bool) {
	raw = dotGitRe.ReplaceAllString(raw, "")
	target := raw
	if i := strings.Index(raw, "://"); i < 1 {
		if m := scpURLRe.FindStringSubmatch(raw); m != nil {
			auth := ""
			if m[1] != "" {
				auth = m[1] + "@"
			}
			target = "ssh://" + auth + m[2] + "/" + m[3]
		}
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme == "" {
		return repoContext{}, false
	}
	scheme := u.Scheme + ":"
	port := u.Port()
	if strings.Contains(scheme, "ssh") {
		port = ""
	}
	protocol := "https"
	if httpNotSRe.MatchString(scheme) {
		protocol = "http"
	}
	rc := repoContext{hostname: strings.ToLower(u.Hostname())}
	rc.host = protocol + "://" + rc.hostname
	if port != "" {
		rc.host += ":" + port
	}
	p := u.EscapedPath()
	if p == "" {
		p = "/"
	}
	if m := ownerRepoRe.FindStringSubmatch(p); m != nil {
		rc.owner, rc.repository = m[1], m[2]
		rc.ownerSet, rc.repoSet = m[1] != "", m[2] != ""
	}
	return rc, true
}

// notesInput is everything one release's notes are written from.
type notesInput struct {
	commits     []commit // newest first, already filtered for the project
	version     string
	previousTag string // last release tag; "" for a first release
	currentTag  string
	date        string // yyyy-mm-dd
	repoURL     string // public repository URL; "" means none
}

// noteRef is a transformed note with its commit's scope.
type noteRef struct {
	text  string
	scope string
}

// writtenCommit is a commit after the preset's transform.
type writtenCommit struct {
	typ        string
	scope      string
	subject    string
	header     string
	hash       string
	shortHash  string
	references []reference
	notes      []noteRef
}

// generateNotes writes a release's notes exactly as release-notes-generator
// with the conventionalcommits preset does, then removes the placeholder
// links when the repository has no public address.
func generateNotes(in notesInput, noteKeywords []string) string {
	own := in.repoURL
	public := own != ""
	if !public {
		own = noRemoteURL
	}
	rc, ok := parseRepositoryURL(own)
	if !ok {
		rc, _ = parseRepositoryURL(noRemoteURL)
		public = false
	}
	actions, found := hostReferenceActions[rc.hostname]
	if !found {
		actions = hostReferenceActions[""]
	}
	p := newParser(parserOptions{noteKeywords: noteKeywords, referenceActions: actions, issuePrefixes: []string{"#"}})
	var parsed []commit
	for _, c := range in.commits {
		if strings.TrimSpace(c.message) == "" {
			continue
		}
		pc := p.parse(c.hash, c.message)
		pc.files = c.files
		parsed = append(parsed, pc)
	}
	parsed = filterReverted(parsed)

	var written []writtenCommit
	for _, c := range parsed {
		if w, keep := transformCommit(c, rc); keep {
			written = append(written, w)
		}
	}
	notes := renderNotes(written, in, rc)
	if !public {
		notes = stripNoRemoteLinks(notes)
	}
	return notes
}

// orUndefined renders a missing context value as JavaScript's template
// replacement does.
func orUndefined(s string, set bool) string {
	if !set {
		return "undefined"
	}
	return s
}

// transformCommit is the preset's writer transform.
func transformCommit(c commit, rc repoContext) (writtenCommit, bool) {
	typeKey := strings.ToLower(c.typ)
	if c.revert != nil {
		typeKey = "revert"
	}
	var entry *commitType
	for i := range defaultCommitTypes {
		if defaultCommitTypes[i].typ == typeKey {
			entry = &defaultCommitTypes[i]
			break
		}
	}
	discard := true
	if releaseAsRe.MatchString(c.footer) || releaseAsRe.MatchString(c.body) {
		discard = false
	}
	notes := c.notes
	if len(notes) == 0 {
		if m := breakingHeaderRe.FindStringSubmatch(c.header); m != nil {
			notes = []note{{title: "BREAKING CHANGE", text: m[3]}}
		}
	}
	if len(notes) > 0 {
		discard = false
	}
	if discard && (entry == nil || entry.hidden) {
		return writtenCommit{}, false
	}
	w := writtenCommit{typ: c.typ, scope: c.scope, header: c.header, hash: c.hash}
	if entry != nil {
		w.typ = entry.section
	}
	if w.scope == "*" {
		w.scope = ""
	}
	if len(c.hash) > 7 {
		w.shortHash = c.hash[:7]
	} else {
		w.shortHash = c.hash
	}
	var issues []string
	if c.subject != "" {
		subject := subjectIssue.ReplaceAllStringFunc(c.subject, func(m string) string {
			sm := subjectIssue.FindStringSubmatch(m)
			issues = append(issues, sm[1]+sm[2])
			u := rc.host + "/" + orUndefined(rc.owner, rc.ownerSet) + "/" + orUndefined(rc.repository, rc.repoSet) + "/issues/" + sm[2]
			return "[" + sm[1] + sm[2] + "](" + u + ")"
		})
		subject = replaceUsers(subject, rc)
		w.subject = subject
	}
	for _, r := range c.references {
		if !slices.Contains(issues, r.prefix+r.issue) {
			w.references = append(w.references, r)
		}
	}
	for _, n := range notes {
		w.notes = append(w.notes, noteRef{text: n.text, scope: w.scope})
	}
	return w, true
}

// replaceUsers links @user mentions of a subject (`\B@user`, a user without
// a slash), as the preset's transform does.
func replaceUsers(s string, rc repoContext) string {
	var b strings.Builder
	last := 0
	for _, loc := range subjectUser.FindAllStringSubmatchIndex(s, -1) {
		user := s[loc[2]:loc[3]]
		b.WriteString(s[last:loc[0]])
		if strings.Contains(user, "/") {
			b.WriteString("@" + user)
		} else {
			b.WriteString("[@" + user + "](" + rc.host + "/" + user + ")")
		}
		last = loc[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

// renderNotes renders the preset's template.hbs with its header and commit
// partials, then trims it and ends it with one newline, as the writer does.
func renderNotes(commits []writtenCommit, in notesInput, rc repoContext) string {
	linkReferences := rc.repoSet
	ownerPath := func(owner string) string {
		if owner != "" {
			return owner
		}
		return rc.owner
	}
	repoPath := func(repo string) string {
		if repo != "" {
			return repo
		}
		return rc.repository
	}

	var b strings.Builder
	b.WriteString("## ")
	if in.previousTag != "" && in.currentTag != "" {
		b.WriteString("[" + in.version + "](" + rc.host + "/" + rc.owner + "/" + rc.repository + "/compare/" + in.previousTag + "..." + in.currentTag + ")")
	} else {
		b.WriteString(in.version)
	}
	if in.date != "" {
		b.WriteString(" (" + in.date + ")")
	}
	b.WriteString("\n")

	var notes []noteRef
	for _, c := range commits {
		notes = append(notes, c.notes...)
	}
	if len(notes) > 0 {
		b.WriteString("\n### ⚠ BREAKING CHANGES\n\n")
		for _, n := range notes {
			b.WriteString("* ")
			if n.scope != "" {
				b.WriteString("**" + n.scope + ":** ")
			}
			b.WriteString(n.text + "\n")
		}
	}

	type group struct {
		title   string
		commits []writtenCommit
	}
	var groups []*group
	index := map[string]*group{}
	for _, c := range commits {
		g, ok := index[c.typ]
		if !ok {
			g = &group{title: c.typ}
			index[c.typ] = g
			groups = append(groups, g)
		}
		g.commits = append(g.commits, c)
	}
	for _, g := range groups {
		slices.SortStableFunc(g.commits, func(x, y writtenCommit) int {
			return localeCompare(x.scope+x.subject, y.scope+y.subject)
		})
	}
	slices.SortStableFunc(groups, func(x, y *group) int {
		return slices.Index(commitGroupOrder, x.title) - slices.Index(commitGroupOrder, y.title)
	})
	for _, g := range groups {
		b.WriteString("\n")
		if g.title != "" {
			b.WriteString("### " + g.title + "\n\n")
		}
		for _, c := range g.commits {
			b.WriteString("*")
			if c.scope != "" {
				b.WriteString(" **" + c.scope + ":**")
			}
			b.WriteString(" ")
			if c.subject != "" {
				b.WriteString(c.subject)
			} else {
				b.WriteString(c.header)
			}
			if c.hash != "" {
				b.WriteString(" ")
				if linkReferences {
					b.WriteString("([" + c.shortHash + "](" + rc.host + "/" + rc.owner + "/" + rc.repository + "/commit/" + c.hash + "))")
				} else {
					b.WriteString(c.shortHash)
				}
			}
			if len(c.references) > 0 {
				b.WriteString(", closes")
				for _, r := range c.references {
					b.WriteString(" ")
					text := r.repository + r.prefix + r.issue
					if r.owner != "" {
						text = r.owner + "/" + text
					}
					if linkReferences {
						b.WriteString("[" + text + "](" + rc.host + "/" + ownerPath(r.owner) + "/" + repoPath(r.repository) + "/issues/" + r.issue + ")")
					} else {
						b.WriteString(text)
					}
				}
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String()) + "\n"
}
