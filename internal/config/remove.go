package config

import (
	"regexp"
	"strings"
)

// Remove deletes the key at a dotted path, with every line of its value:
// a scalar line, or a block's key line and the more-indented, blank and
// comment lines under it (trailing blank and comment lines that precede a
// sibling stay, as they may belong to it). Every other byte is kept. It
// reports whether the key was there.
func (d *Document) Remove(path string) bool {
	eol := yamlEOL(d.text)
	lines := yamlSplitLines(d.text)
	start, end := 0, len(lines)
	indent := 0
	segments := strings.Split(path, ".")
	for i, seg := range segments {
		pattern := regexp.MustCompile(`^` + strings.Repeat(" ", indent) + regexp.QuoteMeta(seg) + `:(\s|$)`)
		found := -1
		for j := start; j < end; j++ {
			if pattern.MatchString(lines[j]) {
				found = j
				break
			}
		}
		if found < 0 {
			return false
		}
		childEnd := childrenEnd(lines, found+1, end, indent)
		if i == len(segments)-1 {
			// Keep the blank and comment lines right before the next
			// sibling: they introduce it, not the removed key.
			last := childEnd
			for last > found+1 && (strings.TrimSpace(lines[last-1]) == "" || commentLineRe.MatchString(lines[last-1])) {
				last--
			}
			out := append(append([]string{}, lines[:found]...), lines[last:]...)
			d.text = strings.Join(out, eol)
			return true
		}
		start, end = found+1, childEnd
		indent += 2
	}
	return false
}

// childrenEnd is the index of the first line from start (before end) that
// is neither blank, a comment, nor indented deeper than indent.
func childrenEnd(lines []string, start, end, indent int) int {
	j := start
	for j < end {
		l := lines[j]
		if strings.TrimSpace(l) == "" || commentLineRe.MatchString(l) || leadingSpaces(l) > indent {
			j++
			continue
		}
		break
	}
	return j
}
