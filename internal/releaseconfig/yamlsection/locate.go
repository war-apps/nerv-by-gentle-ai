// Package yamlsection reads one top-level section of a repository's
// .ai-workflow.yml with gopkg.in/yaml.v3, without depending on the rest of
// the file: the section block is cut out of the text and parsed alone, its
// keys are checked strictly against a Go struct, and paths are checked and
// normalised. It backs the release: section (internal/releaseconfig) and the
// deploy: section (internal/deployconfig).
package yamlsection

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// The rest of .ai-workflow.yml is maintained by internal/config, a line-based
// editor, and may not be strict YAML. A YAML error outside a section must not
// break the commands that read it, so the section block is cut out of the
// file by indentation and parsed alone. The consequence is that an alias
// inside the section cannot refer to an anchor defined outside it. When the
// cut is not reliable (document markers, an indented or flow top level, a
// complex key, the key written twice), the whole document is parsed instead.

// ErrAbsent is returned by Locate when the file has no top-level key.
var ErrAbsent = errors.New("section absent")

// ProblemError is a problem found while locating the section that is not a
// YAML syntax error: a top level that is not a mapping, or an alias to an
// anchor outside the section. Callers turn it into their own validation
// error.
type ProblemError struct{ Problem string }

func (e *ProblemError) Error() string { return e.Problem }

// nodeProperties matches the anchors (&name) and tags (!tag, !!str,
// !<verbatim>) that may precede a top-level key on its line.
const nodeProperties = `(?:(?:&[^\s,\[\]{}]+|![^\s]*)[ \t]+)*`

var (
	propertyKeyRe   = regexp.MustCompile(`^` + nodeProperties + `[^\s#&!].*?:([ \t]|$)`)
	propertiesRe    = regexp.MustCompile(`^` + nodeProperties)
	unknownAnchorRe = regexp.MustCompile(`unknown anchor '([^']*)' referenced`)
)

// blockState is the outcome of looking for the section block by indentation.
type blockState int

const (
	blockUndecided blockState = iota // the text layout defeats the cut
	blockAbsent                      // no top-level key
	blockFound                       // the block was cut out
)

// section holds the per-key patterns.
type section struct {
	key    string
	keyRe  *regexp.Regexp
	wordRe *regexp.Regexp
}

func newSection(key string) section {
	q := regexp.QuoteMeta(key)
	return section{
		key:    key,
		keyRe:  regexp.MustCompile(`^` + nodeProperties + `(` + q + `|"` + q + `"|'` + q + `')[ \t]*:([ \t]|$)`),
		wordRe: regexp.MustCompile(`(^|[^A-Za-z0-9_-])` + q + `([^A-Za-z0-9_-]|$)`),
	}
}

// Locate returns the value node of the top-level key of a .ai-workflow.yml text.
// It returns ErrAbsent when there is none, a *ProblemError for a document
// whose top level is not a mapping or an alias to an anchor outside the
// section, and a plain error prefixed by "<key>: " for a YAML syntax error
// inside it.
func Locate(data []byte, key string) (*yaml.Node, error) {
	s := newSection(key)
	block, state := s.block(data)
	switch state {
	case blockFound:
		var doc yaml.Node
		err := yaml.Unmarshal(block, &doc)
		if err == nil {
			return s.node(&doc)
		}
		if m := unknownAnchorRe.FindStringSubmatch(err.Error()); m != nil {
			return nil, &ProblemError{Problem: s.unknownAnchorProblem(data, block, m[1])}
		}
		// The cut may be wrong (e.g. a flow collection continued at column
		// 0); the whole document decides, and if it is broken too the
		// section's own error is the relevant one.
		if doc, wholeErr := parseDocument(data); wholeErr == nil {
			return s.node(doc)
		}
		return nil, fmt.Errorf("%s: %w", key, err)
	case blockAbsent:
		doc, err := parseDocument(data)
		if err != nil {
			// A line the cut did not recognise may still be the key (e.g. an
			// escaped quoted key); a configured repository must not be read
			// as unconfigured.
			if line := s.possibleKey(data); line > 0 {
				return nil, fmt.Errorf("%s: line %d may be the %s: key, but .ai-workflow.yml is not valid YAML: %w", key, line, key, err)
			}
			// A broken file without the key: not this section's business.
			return nil, ErrAbsent
		}
		return s.node(doc)
	default:
		doc, err := parseDocument(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		return s.node(doc)
	}
}

func parseDocument(data []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// block cuts the top-level section block out of a .ai-workflow.yml text: the key
// line and every following line that is blank, a comment, indented, or a
// column-0 sequence entry (a block sequence may sit at its key's
// indentation). The result keeps the block on its original line numbers by
// prefixing it with empty lines, so parse errors point at the real file.
func (s section) block(data []byte) ([]byte, blockState) {
	lines := strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n")
	start := -1
	firstContent := true
	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if firstContent {
			firstContent = false
			if strings.TrimRight(line, " \t") == "---" {
				continue
			}
			// A first key carrying an anchor or a tag is still a plain
			// top-level key; anything else starting with these characters
			// is not a block mapping the cut understands.
			if strings.ContainsAny(line[:1], " \t[{-?&!*|>") && !propertyKeyRe.MatchString(line) {
				return nil, blockUndecided
			}
		}
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "...") ||
			strings.HasPrefix(line, "%") || strings.HasPrefix(line, "?") {
			return nil, blockUndecided
		}
		if s.keyRe.MatchString(line) {
			if start >= 0 {
				return nil, blockUndecided
			}
			start = i
		}
	}
	if start < 0 {
		return nil, blockAbsent
	}
	end := start + 1
	for end < len(lines) && continuesBlock(strings.TrimRight(lines[end], "\r")) {
		end++
	}
	text := strings.Repeat("\n", start) + strings.Join(lines[start:end], "\n") + "\n"
	return []byte(text), blockFound
}

// continuesBlock reports whether a line after the key still belongs to it.
func continuesBlock(line string) bool {
	switch {
	case strings.TrimSpace(line) == "":
		return true
	case line[0] == ' ' || line[0] == '\t' || line[0] == '#':
		return true
	case line == "-" || strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "-\t"):
		return true
	}
	return false
}

// unknownAnchorProblem explains an alias to an anchor the cut-out section
// does not define before it, with the line of its first use. The anchor is
// "outside the section" only when some line of the file outside the block
// defines it; otherwise it is undefined (a typo, or used before its anchor).
func (s section) unknownAnchorProblem(data, block []byte, anchor string) string {
	name := regexp.QuoteMeta(anchor)
	use := regexp.MustCompile(`\*` + name + `([\s,\[\]{}]|$)`)
	def := regexp.MustCompile(`(^|[\s\[{,])&` + name + `([\s,\[\]{}]|$)`)
	blockLines := strings.Split(string(block), "\n")
	where := ""
	for i, line := range blockLines {
		if use.MatchString(line) {
			where = fmt.Sprintf(" (line %d)", i+1)
			break
		}
	}
	// The block keeps the file's line numbers and is empty outside the cut.
	outside := false
	for i, line := range strings.Split(string(data), "\n") {
		inBlock := i < len(blockLines) && strings.TrimSpace(blockLines[i]) != ""
		if !inBlock && def.MatchString(line) {
			outside = true
			break
		}
	}
	if outside {
		return fmt.Sprintf("%s: alias *%s%s refers to an anchor outside the %s: section; only anchors defined inside the section can be used there", s.key, anchor, where, s.key)
	}
	return fmt.Sprintf("%s: alias *%s%s refers to an undefined anchor &%s (a typo, or an alias used before its anchor is defined)", s.key, anchor, where, anchor)
}

// possibleKey returns the 1-based line of a top-level key the cut could not
// recognise but that may still be the section key (an escaped quoted key, or
// one that mentions the key), or 0 when there is none.
func (s section) possibleKey(data []byte) int {
	for i, raw := range strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		line := strings.TrimRight(raw, "\r")
		if line == "" || strings.ContainsAny(line[:1], " \t#") {
			continue
		}
		key := propertiesRe.ReplaceAllString(line, "")
		if j := strings.Index(key, ":"); j >= 0 {
			key = key[:j]
		}
		if s.wordRe.MatchString(key) || (strings.HasPrefix(key, `"`) && strings.Contains(key, `\`)) {
			return i + 1
		}
	}
	return 0
}

// node returns the value of the top-level key of a parsed document. An
// empty document or a missing key is ErrAbsent; a top level that is not a
// mapping is a *ProblemError.
func (s section) node(doc *yaml.Node) (*yaml.Node, error) {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, ErrAbsent
	}
	root := doc.Content[0]
	if root.ShortTag() == "!!null" {
		return nil, ErrAbsent
	}
	if root.Kind != yaml.MappingNode {
		return nil, &ProblemError{Problem: fmt.Sprintf("the top level of .ai-workflow.yml must be a mapping (line %d)", root.Line)}
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == s.key {
			return root.Content[i+1], nil
		}
	}
	return nil, ErrAbsent
}

// DecodeProblems turns a yaml.v3 decoding error into one "<key>: ..."
// problem per line.
func DecodeProblems(key string, err error) []string {
	msg := strings.TrimPrefix(err.Error(), "yaml: unmarshal errors:\n")
	var problems []string
	for _, l := range strings.Split(msg, "\n") {
		problems = append(problems, key+": "+strings.TrimSpace(l))
	}
	return problems
}
