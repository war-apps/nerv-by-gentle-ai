package versioning

import (
	"regexp"
	"strings"
)

// This file ports conventional-commits-parser 6 (CommitParser.js,
// regex.js, utils.js, options.js) with the parser options of the
// conventionalcommits preset 8 (parser.js) merged over its defaults, as
// commit-analyzer and release-notes-generator merge them.

// defaultReferenceActions are conventional-commits-parser's default
// reference actions (commit-analyzer uses them; release-notes-generator uses
// its host's, see hostReferenceActions).
var defaultReferenceActions = []string{"close", "closes", "closed", "fix", "fixes", "fixed", "resolve", "resolves", "resolved"}

var (
	// headerRe and breakingHeaderRe are the conventionalcommits preset's
	// headerPattern and breakingHeaderPattern.
	headerRe         = regexp.MustCompile(`^(\w*)(?:\((.*)\))?!?: (.*)$`)
	breakingHeaderRe = regexp.MustCompile(`^(\w*)(?:\((.*)\))?!: (.*)$`)
	// revertRe is the preset's revertPattern.
	revertRe = regexp.MustCompile(`(?i)^(?:Revert|revert:)\s"?([\s\S]+?)"?\s*This reverts commit (\w*)\.`)
	// fieldRe is the parser's default fieldPattern.
	fieldRe = regexp.MustCompile(`^-(.*?)-$`)
	gpgRe   = regexp.MustCompile(`^\s*gpg:`)
	lineRe  = regexp.MustCompile(`\r?\n`)
	// urlRe is the parser's url regex: a sentence holding a URL has no
	// references.
	urlRe = regexp.MustCompile(`\bhttps?://(?:www\.)?[-a-zA-Z0-9@:%_+.~#?&/=]+\b`)
)

// reference is an issue reference ("closes owner/repo#12").
type reference struct {
	raw        string
	action     string // "" when none
	hasAction  bool
	owner      string
	repository string
	prefix     string
	issue      string
}

// note is a breaking-change note.
type note struct {
	title string
	text  string
}

// revertInfo is what a revert commit names: the reverted header and hash.
type revertInfo struct {
	header string
	hash   string
}

// commit is one parsed commit: semantic-release's raw fields (hash,
// message, files) and the parser's.
type commit struct {
	hash    string
	message string
	files   []string

	header     string
	typ        string
	scope      string
	subject    string
	body       string
	footer     string
	notes      []note
	references []reference
	revert     *revertInfo
}

// parserOptions are the options that differ between nerv's two parsers.
type parserOptions struct {
	noteKeywords     []string
	referenceActions []string
	issuePrefixes    []string
}

// parser parses commit messages.
type parser struct {
	notesRe       *regexp.Regexp
	actions       []string
	issuePrefixes []string

	lines []string
	index int
	c     *commit
}

func newParser(opts parserOptions) *parser {
	p := &parser{actions: opts.referenceActions, issuePrefixes: opts.issuePrefixes}
	var keys []string
	for _, k := range opts.noteKeywords {
		if k = strings.TrimSpace(k); k != "" {
			keys = append(keys, regexp.QuoteMeta(k))
		}
	}
	if len(keys) > 0 {
		p.notesRe = regexp.MustCompile(`(?i)^[\s|*]*(` + strings.Join(keys, "|") + `)[:\s]+(.*)`)
	}
	return p
}

// trimNewLines removes leading and trailing \r and \n.
func trimNewLines(s string) string {
	return strings.Trim(s, "\r\n")
}

// appendLine is the parser's appendLine: src, a newline and line, or line
// alone when src is empty.
func appendLine(src, line string) string {
	if src != "" {
		return src + "\n" + line
	}
	return line
}

func (p *parser) available() bool { return p.index < len(p.lines) }
func (p *parser) current() string { return p.lines[p.index] }

// parse parses one commit message (non-empty once trimmed).
func (p *parser) parse(hash, message string) commit {
	c := commit{hash: hash, message: message}
	raw := lineRe.Split(trimNewLines(message), -1)
	p.lines = p.lines[:0]
	for _, l := range raw {
		if !gpgRe.MatchString(l) {
			p.lines = append(p.lines, l)
		}
	}
	p.index = 0
	p.c = &c

	if p.available() {
		c.header = p.current()
		p.index++
		m := breakingHeaderRe.FindStringSubmatch(c.header)
		if m == nil {
			m = headerRe.FindStringSubmatch(c.header)
		}
		if m != nil {
			c.typ, c.scope, c.subject = m[1], m[2], m[3]
		}
	}
	if c.header != "" {
		c.references = p.parseReferences(c.header)
	}
	isBody := true
	for p.available() {
		p.parseMeta()
		if p.parseNotes() {
			isBody = false
		}
		if !p.parseBodyAndFooter(isBody) {
			isBody = false
		}
	}
	if len(c.notes) == 0 && c.header != "" {
		if m := breakingHeaderRe.FindStringSubmatch(c.header); m != nil {
			c.notes = append(c.notes, note{title: "BREAKING CHANGE", text: m[3]})
		}
	}
	if m := revertRe.FindStringSubmatch(message); m != nil {
		c.revert = &revertInfo{header: m[1], hash: m[2]}
	}
	// cleanupCommit
	c.body = trimNewLines(c.body)
	c.footer = trimNewLines(c.footer)
	for i := range c.notes {
		c.notes[i].text = trimNewLines(c.notes[i].text)
	}
	seen := map[string]bool{}
	refs := c.references[:0]
	for _, r := range c.references {
		action := "null"
		if r.hasAction {
			action = r.action
		}
		uid := strings.ToLower(action + " " + r.raw)
		if !seen[uid] {
			seen[uid] = true
			refs = append(refs, r)
		}
	}
	c.references = refs
	p.c = nil
	return c
}

// parseMeta consumes "-field-" blocks (the default fieldPattern): the field
// line and every following line, which the parser stores under the field
// name and nerv drops.
func (p *parser) parseMeta() bool {
	if !p.available() {
		return false
	}
	field := ""
	parsed := false
	for p.available() {
		if m := fieldRe.FindStringSubmatch(p.current()); m != nil {
			field = m[1]
			p.index++
			continue
		}
		if field == "" {
			break
		}
		parsed = true
		p.index++
	}
	return parsed
}

func (p *parser) parseNotes() bool {
	if !p.available() || p.notesRe == nil {
		return false
	}
	m := p.notesRe.FindStringSubmatch(p.current())
	if m == nil {
		return false
	}
	c := p.c
	c.notes = append(c.notes, note{title: m[1], text: m[2]})
	at := len(c.notes) - 1
	c.footer = appendLine(c.footer, p.current())
	p.index++
	for p.available() {
		if p.parseMeta() {
			return true
		}
		if p.parseNotes() {
			return true
		}
		refs := p.parseReferences(p.current())
		if len(refs) > 0 {
			c.references = append(c.references, refs...)
		} else {
			c.notes[at].text = appendLine(c.notes[at].text, p.current())
		}
		c.footer = appendLine(c.footer, p.current())
		p.index++
		if len(refs) > 0 {
			break
		}
	}
	return true
}

func (p *parser) parseBodyAndFooter(isBody bool) bool {
	if !p.available() {
		return isBody
	}
	c := p.c
	refs := p.parseReferences(p.current())
	still := len(refs) == 0 && isBody
	if still {
		c.body = appendLine(c.body, p.current())
	} else {
		c.references = append(c.references, refs...)
		c.footer = appendLine(c.footer, p.current())
	}
	p.index++
	return still
}

// actionAt returns the first reference action (in the configured order)
// that matches input at i, case-insensitively.
func (p *parser) actionAt(input string, i int, need func(end int) bool) (string, bool) {
	for _, a := range p.actions {
		if i+len(a) <= len(input) && strings.EqualFold(input[i:i+len(a)], a) && need(i+len(a)) {
			return input[i : i+len(a)], true
		}
	}
	return "", false
}

// parseReferences is the parser's parseReferences: the sentences after a
// reference action (`(action)(?:\s+(.*?))(?=(?:action)|$)`, global and
// case-insensitive), or the whole input when no action occurs, each
// scanned for references.
func (p *parser) parseReferences(input string) []reference {
	type sentence struct {
		action    string
		hasAction bool
		text      string
	}
	var sentences []sentence
	keywordAt := func(i int) bool {
		_, ok := p.actionAt(input, i, func(int) bool { return true })
		return ok
	}
	isSpace := func(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v' }
	for i := 0; i < len(input); {
		var end int
		action, ok := p.actionAt(input, i, func(e int) bool {
			if e < len(input) && isSpace(input[e]) {
				end = e
				return true
			}
			return false
		})
		if !ok {
			i++
			continue
		}
		start := end
		for start < len(input) && isSpace(input[start]) {
			start++
		}
		q := start
		for q < len(input) && !keywordAt(q) {
			q++
		}
		sentences = append(sentences, sentence{action: action, hasAction: true, text: input[start:q]})
		i = q
	}
	if len(sentences) == 0 {
		if input == "" {
			return nil
		}
		sentences = []sentence{{text: input}}
	}
	var refs []reference
	for _, s := range sentences {
		if urlRe.MatchString(s.text) {
			continue
		}
		for from := 0; ; {
			r, next, ok := p.referenceAt(s.text, from)
			if !ok {
				break
			}
			r.action, r.hasAction = s.action, s.hasAction
			refs = append(refs, r)
			from = next
		}
	}
	return refs
}

func isRefRepoChar(b byte) bool {
	return b == '_' || b == '-' || b == '.' || b == '/' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isRefIssueChar(b byte) bool {
	return b == '_' || b == '-' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// referenceAt emulates one exec of the referenceParts regex,
// `(?:.*?)??\s*([\w-\.\/]*?)??(prefix)([\w-]+)(?=\s|$|[,;)\]])` with the g
// and i flags, from index from: the shortest skipped prefix first, then the
// shortest repository, then the longest issue followed by a delimiter.
func (p *parser) referenceAt(s string, from int) (reference, int, bool) {
	isSpace := func(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v' }
	delimited := func(e int) bool {
		if e == len(s) {
			return true
		}
		switch b := s[e]; b {
		case ',', ';', ')', ']':
			return true
		default:
			return isSpace(b)
		}
	}
	for n := from; n <= len(s); n++ {
		p2 := n
		for p2 < len(s) && isSpace(s[p2]) {
			p2++
		}
		maxRepo := 0
		for p2+maxRepo < len(s) && isRefRepoChar(s[p2+maxRepo]) {
			maxRepo++
		}
		for repoLen := 0; repoLen <= maxRepo; repoLen++ {
			q := p2 + repoLen
			for _, prefix := range p.issuePrefixes {
				if q+len(prefix) > len(s) || !strings.EqualFold(s[q:q+len(prefix)], prefix) {
					continue
				}
				is := q + len(prefix)
				run := 0
				for is+run < len(s) && isRefIssueChar(s[is+run]) {
					run++
				}
				for l := run; l >= 1; l-- {
					if !delimited(is + l) {
						continue
					}
					r := reference{raw: s[from : is+l], prefix: s[q:is], issue: s[is : is+l]}
					repo := s[p2:q]
					if i := strings.IndexByte(repo, '/'); i >= 0 {
						r.owner, repo = repo[:i], repo[i+1:]
					}
					r.repository = repo
					return r, is + l, true
				}
			}
		}
	}
	return reference{}, 0, false
}
