package shipconfig

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// The rest of nerv.yaml is maintained by internal/config, a line-based
// editor, and may not be strict YAML. A YAML error outside release: must not
// break the release commands, so the release: block is cut out of the file
// by indentation and parsed alone. The consequence is that an alias inside
// the section cannot refer to an anchor defined outside it. When the cut is
// not reliable (document markers, an indented or flow top level, a complex
// key, release: written twice), the whole document is parsed instead.

// nodeProperties matches the anchors (&name) and tags (!tag, !!str,
// !<verbatim>) that may precede a top-level key on its line.
const nodeProperties = `(?:(?:&[^\s,\[\]{}]+|![^\s]*)[ \t]+)*`

var (
	releaseKeyRe    = regexp.MustCompile(`^` + nodeProperties + `(release|"release"|'release')[ \t]*:([ \t]|$)`)
	propertyKeyRe   = regexp.MustCompile(`^` + nodeProperties + `[^\s#&!].*?:([ \t]|$)`)
	propertiesRe    = regexp.MustCompile(`^` + nodeProperties)
	releaseWordRe   = regexp.MustCompile(`(^|[^A-Za-z0-9_-])release([^A-Za-z0-9_-]|$)`)
	unknownAnchorRe = regexp.MustCompile(`unknown anchor '([^']*)' referenced`)
)

// blockState is the outcome of looking for the release: block by indentation.
type blockState int

const (
	blockUndecided blockState = iota // the text layout defeats the cut
	blockAbsent                      // no top-level release: key
	blockFound                       // the block was cut out
)

// locateRelease returns the value node of the top-level release: key. It
// returns ErrNotConfigured when there is none, a *ValidationError for a
// document whose top level is not a mapping or an alias to an anchor
// outside the section, and a plain error for a YAML syntax error inside it.
func locateRelease(data []byte) (*yaml.Node, error) {
	block, state := releaseBlock(data)
	switch state {
	case blockFound:
		var doc yaml.Node
		err := yaml.Unmarshal(block, &doc)
		if err == nil {
			return releaseNode(&doc)
		}
		if m := unknownAnchorRe.FindStringSubmatch(err.Error()); m != nil {
			return nil, &ValidationError{Problems: []string{unknownAnchorProblem(data, block, m[1])}}
		}
		// The cut may be wrong (e.g. a flow collection continued at column
		// 0); the whole document decides, and if it is broken too the
		// section's own error is the relevant one.
		if doc, wholeErr := parseDocument(data); wholeErr == nil {
			return releaseNode(doc)
		}
		return nil, fmt.Errorf("release: %w", err)
	case blockAbsent:
		doc, err := parseDocument(data)
		if err != nil {
			// A line the cut did not recognise may still be the release:
			// key (e.g. an escaped quoted key); a configured repository must
			// not be read as unconfigured.
			if line := possibleReleaseKey(data); line > 0 {
				return nil, fmt.Errorf("release: line %d may be the release: key, but nerv.yaml is not valid YAML: %w", line, err)
			}
			// A broken file without a release: key: not this package's
			// business.
			return nil, ErrNotConfigured
		}
		return releaseNode(doc)
	default:
		doc, err := parseDocument(data)
		if err != nil {
			return nil, fmt.Errorf("release: %w", err)
		}
		return releaseNode(doc)
	}
}

func parseDocument(data []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// releaseBlock cuts the top-level release: block out of a nerv.yaml text:
// the key line and every following line that is blank, a comment, indented,
// or a column-0 sequence entry (a block sequence may sit at its key's
// indentation). The result keeps the block on its original line numbers by
// prefixing it with empty lines, so parse errors point at the real file.
func releaseBlock(data []byte) ([]byte, blockState) {
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
		if releaseKeyRe.MatchString(line) {
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

// continuesBlock reports whether a line after release: still belongs to it.
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
func unknownAnchorProblem(data, block []byte, anchor string) string {
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
		return fmt.Sprintf("release: alias *%s%s refers to an anchor outside the release: section; only anchors defined inside the section can be used there", anchor, where)
	}
	return fmt.Sprintf("release: alias *%s%s refers to an undefined anchor &%s (a typo, or an alias used before its anchor is defined)", anchor, where, anchor)
}

// possibleReleaseKey returns the 1-based line of a top-level key the cut
// could not recognise but that may still be release: (an escaped quoted key,
// or one that mentions release), or 0 when there is none.
func possibleReleaseKey(data []byte) int {
	for i, raw := range strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		line := strings.TrimRight(raw, "\r")
		if line == "" || strings.ContainsAny(line[:1], " \t#") {
			continue
		}
		key := propertiesRe.ReplaceAllString(line, "")
		if j := strings.Index(key, ":"); j >= 0 {
			key = key[:j]
		}
		if releaseWordRe.MatchString(key) || (strings.HasPrefix(key, `"`) && strings.Contains(key, `\`)) {
			return i + 1
		}
	}
	return 0
}

// releaseNode returns the value of the top-level release: key of a parsed
// document. An empty document or a missing key is ErrNotConfigured; a top
// level that is not a mapping is a validation error.
func releaseNode(doc *yaml.Node) (*yaml.Node, error) {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, ErrNotConfigured
	}
	root := doc.Content[0]
	if root.ShortTag() == "!!null" {
		return nil, ErrNotConfigured
	}
	if root.Kind != yaml.MappingNode {
		return nil, &ValidationError{Problems: []string{
			fmt.Sprintf("the top level of nerv.yaml must be a mapping (line %d)", root.Line),
		}}
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "release" {
			return root.Content[i+1], nil
		}
	}
	return nil, ErrNotConfigured
}
