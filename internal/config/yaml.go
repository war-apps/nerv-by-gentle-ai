// Package config edits nerv.yaml documents in place while preserving their
// exact formatting (comments, blank lines, indentation, key order, EOL
// style), and holds the managed configuration-key catalogue and per-role
// model catalogue used by "nerv configure". It never touches the
// filesystem: callers read/write bytes and hand this package text.
//
// This package is a line-based, format-preserving editor over nerv.yaml —
// never a general YAML parser/library.
package config

import (
	"regexp"
	"strings"
)

var (
	eolSplitRe        = regexp.MustCompile(`\r\n|\n`)
	commentLineRe     = regexp.MustCompile(`^\s*#`)
	indentedLineRe    = regexp.MustCompile(`^\s`)
	leadingWSRe       = regexp.MustCompile(`^\s*`)
	trailingWSRe      = regexp.MustCompile(`\s*$`)
	trailingCommentRe = regexp.MustCompile(`\s+#.*$`)
	bareIntRe         = regexp.MustCompile(`^-?\d+$`)
	bareScalarRe      = regexp.MustCompile(`^[A-Za-z0-9_.\-/~]+$`)
)

// Document is a nerv.yaml text blob that can be inspected and edited in
// place while preserving every byte it does not touch.
type Document struct {
	text string
}

// Parse wraps text as a Document. text may be empty.
func Parse(text string) *Document {
	return &Document{text: text}
}

// String returns the document's current text.
func (d *Document) String() string {
	return d.text
}

// yamlEOL returns the document's EOL style: CRLF when text contains at
// least one CRLF, LF otherwise. Mirrors every pure function's own
// `$YamlText -match "`r`n"` detection.
func yamlEOL(text string) string {
	if strings.Contains(text, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func yamlSplitLines(text string) []string {
	return eolSplitRe.Split(text, -1)
}

func stripTrailingEOL(s string) string {
	for {
		switch {
		case strings.HasSuffix(s, "\r\n"):
			s = s[:len(s)-2]
		case strings.HasSuffix(s, "\n"):
			s = s[:len(s)-1]
		default:
			return s
		}
	}
}

func yamlKeyLinePattern(key string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `:\s*(\||>)?[+-]?\s*(#.*)?$`)
}

func leadingSpaces(s string) int {
	return len(leadingWSRe.FindString(s))
}

func trimQuotes(s string) string {
	s = strings.Trim(s, `"`)
	s = strings.Trim(s, `'`)
	return s
}

func insertAt(lines []string, idx int, val string) []string {
	lines = append(lines, "")
	copy(lines[idx+1:], lines[idx:])
	lines[idx] = val
	return lines
}

// ---------------------------------------------------------------------------
// Block (Set-NervYamlBlock / Get-NervYamlBlock)
// ---------------------------------------------------------------------------

// Block returns the raw text of a top-level "<key>:" block: the key line
// plus every following blank, full-line-comment, or indented line, up to
// the next column-0 key. found is false when key is not present at column
// 0 — including when key exists only as an inline value on its own line
// (see Document.KeyIsInline). Mirrors Get-NervYamlBlock; the returned text
// always uses LF line endings, matching the PowerShell function.
func (d *Document) Block(key string) (text string, found bool) {
	normalized := strings.ReplaceAll(d.text, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	pattern := yamlKeyLinePattern(key)

	for i := 0; i < len(lines); i++ {
		if pattern.MatchString(lines[i]) {
			j := scanBlockEnd(lines, i+1, indentedLineRe)
			return strings.Join(lines[i:j], "\n"), true
		}
	}
	return "", false
}

// SetBlock replaces, appends, or removes the top-level "<key>:" block.
// When key already exists at column 0, the whole run (key line plus every
// following blank/comment/indented line) is replaced by blockText;
// everything else in the document is preserved verbatim. When key does not
// exist, blockText is appended after one blank line. When blockText is
// empty, the existing block (if any) is removed instead. Mirrors
// Set-NervYamlBlock (which, for key == "models", the PowerShell script
// delegates to the models-specific Set-NervYamlModelsBlock; that function's
// scan is a strict subset of this one's, so the two always produce the
// same result — see SetModelsBlock).
func (d *Document) SetBlock(key, blockText string) {
	eol := yamlEOL(d.text)
	lines := yamlSplitLines(d.text)
	pattern := yamlKeyLinePattern(key)

	startIdx, endIdx := -1, -1
	for i := 0; i < len(lines); i++ {
		if pattern.MatchString(lines[i]) {
			startIdx = i
			endIdx = scanBlockEnd(lines, i+1, indentedLineRe)
			break
		}
	}

	var blockLines []string
	if blockText != "" {
		blockLines = yamlSplitLines(blockText)
	}

	if startIdx >= 0 {
		before := append([]string{}, lines[:startIdx]...)
		var after []string
		if endIdx < len(lines) {
			after = append([]string{}, lines[endIdx:]...)
		}
		newLines := append(before, blockLines...)
		newLines = append(newLines, after...)
		joined := strings.Join(newLines, eol)
		if !strings.HasSuffix(joined, eol) {
			joined += eol
		}
		d.text = joined
		return
	}

	if len(blockLines) == 0 {
		return
	}
	base := stripTrailingEOL(d.text)
	blockJoined := strings.Join(blockLines, eol)
	if len(base) == 0 {
		d.text = blockJoined + eol
		return
	}
	d.text = base + eol + eol + blockJoined + eol
}

// scanBlockEnd scans forward from start, skipping blank lines,
// full-line-comment lines, and lines matching continue (indentation-style
// continuation), and returns the index of the first line that breaks the
// run (or len(lines) when the run reaches the end).
func scanBlockEnd(lines []string, start int, continue_ *regexp.Regexp) int {
	j := start
	for j < len(lines) {
		line := lines[j]
		if strings.TrimSpace(line) == "" {
			j++
			continue
		}
		if commentLineRe.MatchString(line) {
			j++
			continue
		}
		if continue_.MatchString(line) {
			j++
			continue
		}
		break
	}
	return j
}

// ---------------------------------------------------------------------------
// KeyExists / KeyIsInline (Test-NervYamlKeyExists / Test-NervYamlKeyIsInline)
// ---------------------------------------------------------------------------

// KeyExists returns whether a top-level "<key>:" line exists at all, in any
// form: a bare block header (git:), an inline map/list (skills: { ... }),
// or a plain bare scalar (enabled: true). Mirrors Test-NervYamlKeyExists.
func (d *Document) KeyExists(key string) bool {
	normalized := strings.ReplaceAll(d.text, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	pattern := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `:`)
	for _, l := range lines {
		if pattern.MatchString(l) {
			return true
		}
	}
	return false
}

// KeyIsInline returns whether an EXISTING top-level "<key>:" line carries
// its value inline on that same line, rather than being a bare block
// header or a block-scalar header. Returns false when the key does not
// exist at all — callers must check KeyExists separately. Mirrors
// Test-NervYamlKeyIsInline.
func (d *Document) KeyIsInline(key string) bool {
	normalized := strings.ReplaceAll(d.text, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	anyPattern := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `:`)
	blockHeaderPattern := yamlKeyLinePattern(key)
	for _, l := range lines {
		if anyPattern.MatchString(l) {
			return !blockHeaderPattern.MatchString(l)
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// InlineListField / SetInlineListField (Get-/Set-NervYamlInlineListField)
// ---------------------------------------------------------------------------

// InlineListField extracts one "<key>: [ ... ]" entry's raw list content
// out of an inline-map text (e.g. the value of an inline "skills: { ... }"
// line). Mirrors Get-NervYamlInlineListField.
func InlineListField(inlineText, key string) (value string, found bool) {
	if inlineText == "" {
		return "", false
	}
	pattern := regexp.MustCompile(regexp.QuoteMeta(key) + `:\s*\[([^\]]*)\]`)
	m := pattern.FindStringSubmatch(inlineText)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}

// SetInlineListField replaces (or, if absent, appends) one "<key>: [ ... ]"
// entry inside an inline-map text, leaving every other entry's raw text
// untouched. Mirrors Set-NervYamlInlineListField.
func SetInlineListField(inlineText, key, newValue string) string {
	pattern := regexp.MustCompile(regexp.QuoteMeta(key) + `:\s*\[[^\]]*\]`)
	if loc := pattern.FindStringIndex(inlineText); loc != nil {
		return inlineText[:loc[0]] + key + ": [" + newValue + "]" + inlineText[loc[1]:]
	}

	trimmed := strings.TrimRightFunc(inlineText, isSpace)
	if strings.HasSuffix(trimmed, "}") {
		inner := strings.TrimRightFunc(trimmed[:len(trimmed)-1], isSpace)
		sep := ", "
		if strings.HasSuffix(inner, "{") {
			sep = ""
		}
		return inner + sep + key + ": [" + newValue + "] }"
	}
	return inlineText
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}

// ---------------------------------------------------------------------------
// Scalar (Read-NervScalar)
// ---------------------------------------------------------------------------

// Scalar reads one dotted-path scalar (e.g. "git.base_branch" or
// "tasks.providers.teamwork.assignee_id"), returning its trailing scalar
// text (bare or quoted, trailing comment stripped, quotes trimmed) and
// found=true, or found=false when any segment of the path is missing or
// the leaf has no inline value. Mirrors Read-NervScalar.
func (d *Document) Scalar(path string) (value string, found bool) {
	segments := strings.Split(path, ".")
	normalized := strings.ReplaceAll(d.text, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	searchStart, searchEnd := 0, len(lines)-1
	indent := 0

	for segIdx, seg := range segments {
		isLast := segIdx == len(segments)-1
		prefix := strings.Repeat(" ", indent)
		pattern := regexp.MustCompile(`^` + prefix + regexp.QuoteMeta(seg) + `:(\s*)(.*)$`)

		found := -1
		foundEnd := searchEnd
		for i := searchStart; i <= searchEnd && i < len(lines); i++ {
			if pattern.MatchString(lines[i]) {
				found = i
				j := i + 1
				for j <= searchEnd && j < len(lines) {
					l := lines[j]
					if strings.TrimSpace(l) == "" || commentLineRe.MatchString(l) {
						j++
						continue
					}
					if leadingSpaces(l) > indent {
						j++
						continue
					}
					break
				}
				foundEnd = j - 1
				break
			}
		}

		if found < 0 {
			return "", false
		}

		if isLast {
			m := pattern.FindStringSubmatch(lines[found])
			valuePart := m[2]
			valuePart = trailingCommentRe.ReplaceAllString(valuePart, "")
			valuePart = strings.TrimSpace(valuePart)
			if valuePart == "" {
				return "", false
			}
			return trimQuotes(valuePart), true
		}

		searchStart = found + 1
		searchEnd = foundEnd
		indent += 2
	}

	return "", false
}

// ---------------------------------------------------------------------------
// SetScalar (Set-NervYamlScalar)
// ---------------------------------------------------------------------------

type scalarOptions struct {
	raw     bool
	comment string
}

// ScalarOption customizes Document.SetScalar.
type ScalarOption func(*scalarOptions)

// Raw makes SetScalar use the given value verbatim, unquoted — for a
// pre-formatted list ([a, b]) or inline-map ({ a: 1, b: 2 }) literal.
func Raw() ScalarOption {
	return func(o *scalarOptions) { o.raw = true }
}

// WithComment appends "  # comment" when SetScalar appends a genuinely new
// line; ignored when the leaf already exists (its own comment, if any, is
// always preserved untouched).
func WithComment(comment string) ScalarOption {
	return func(o *scalarOptions) { o.comment = comment }
}

// SetScalar updates one existing scalar (or pre-formatted list/inline-map)
// line at the dotted path in place, or appends it as a new line (creating
// any missing intermediate parent blocks along the way) when missing —
// without touching any other content in the document. value is quoted
// (FormatScalarToken) unless Raw() is passed. Mirrors Set-NervYamlScalar.
func (d *Document) SetScalar(path, value string, opts ...ScalarOption) {
	var o scalarOptions
	for _, opt := range opts {
		opt(&o)
	}

	eol := yamlEOL(d.text)
	lines := yamlSplitLines(d.text)
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	formattedValue := value
	if !o.raw {
		formattedValue = FormatScalarToken(value)
	}
	segments := strings.Split(path, ".")

	searchStart, searchEnd := 0, len(lines)
	indent := 0

	for segIdx, seg := range segments {
		isLast := segIdx == len(segments)-1
		prefix := strings.Repeat(" ", indent)
		keyLinePattern := regexp.MustCompile(`^` + prefix + regexp.QuoteMeta(seg) + `:(.*)$`)

		found := -1
		for i := searchStart; i < searchEnd; i++ {
			if keyLinePattern.MatchString(lines[i]) {
				found = i
				break
			}
		}

		if isLast {
			if found >= 0 {
				m := keyLinePattern.FindStringSubmatch(lines[found])
				rest := m[1]
				hashIdx := strings.Index(rest, "#")
				var newRest string
				if hashIdx >= 0 {
					beforeHash := rest[:hashIdx]
					commentPart := rest[hashIdx:]
					leadingWs := leadingWSRe.FindString(beforeHash)
					trailingWs := trailingWSRe.FindString(beforeHash)
					newRest = leadingWs + formattedValue + trailingWs + commentPart
				} else {
					leadingWs := leadingWSRe.FindString(rest)
					if leadingWs == "" {
						leadingWs = " "
					}
					newRest = leadingWs + formattedValue
				}
				lines[found] = prefix + seg + ":" + newRest
			} else {
				newLine := prefix + seg + ": " + formattedValue
				if o.comment != "" {
					newLine += "  # " + o.comment
				}
				lines = insertAt(lines, searchEnd, newLine)
			}
			break
		}

		if found >= 0 {
			childStart := found + 1
			j := childStart
			for j < searchEnd {
				l := lines[j]
				if strings.TrimSpace(l) == "" || commentLineRe.MatchString(l) {
					j++
					continue
				}
				if leadingSpaces(l) > indent {
					j++
					continue
				}
				break
			}
			searchStart = childStart
			searchEnd = j
			indent += 2
		} else {
			lines = insertAt(lines, searchEnd, prefix+seg+":")
			searchStart = searchEnd + 1
			searchEnd = searchEnd + 1
			indent += 2
		}
	}

	joined := strings.Join(lines, eol)
	if !strings.HasSuffix(joined, eol) {
		joined += eol
	}
	d.text = joined
}

// ---------------------------------------------------------------------------
// FormatScalarToken (Format-NervYamlScalarToken)
// ---------------------------------------------------------------------------

// FormatScalarToken formats value for SetScalar: null/true/false and bare
// integers stay unquoted; a value made only of safe bare-scalar characters
// (letters, digits, "_ - . / ~") stays unquoted; anything else (spaces,
// braces, parens, colons, etc.) is double-quoted, escaping any embedded
// double quote. Mirrors Format-NervYamlScalarToken.
func FormatScalarToken(value string) string {
	if value == "" {
		return `""`
	}
	if value == "null" || value == "true" || value == "false" {
		return value
	}
	if bareIntRe.MatchString(value) {
		return value
	}
	if bareScalarRe.MatchString(value) {
		return value
	}
	escaped := strings.ReplaceAll(value, `"`, `\"`)
	return `"` + escaped + `"`
}
