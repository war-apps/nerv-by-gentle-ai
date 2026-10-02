package config

import (
	"regexp"
	"slices"
	"strings"
)

var tasksHeaderRe = regexp.MustCompile(`^tasks:\s*(#.*)?$`)

// StripRemovedTaskProviders removes the settings of the task providers listed
// in RemovedTaskProviders from a nerv.yaml text: the tasks.provider line when
// its value is a removed provider, and each removed provider's sub-block under
// tasks.providers (inline "jira: { ... }" or multi-line, with the comment lines
// directly above it at its own indentation), or its entry in a one-line flow
// providers map. It returns the new bytes and what it removed, in file
// order ("tasks.provider (jira)", "tasks.providers.jira"). Every other byte,
// including line endings, is kept; a text with nothing to remove is returned
// as is with a nil list.
func StripRemovedTaskProviders(data []byte) (out []byte, removed []string) {
	lines := strings.SplitAfter(string(data), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	tasksAt := slices.IndexFunc(lines, func(l string) bool { return tasksHeaderRe.MatchString(lineText(l)) })
	if tasksAt < 0 {
		return data, nil
	}
	end := childRangeEnd(lines, tasksAt+1, len(lines), 0)
	childIndent := firstContentIndent(lines, tasksAt+1, end)
	if childIndent <= 0 {
		return data, nil
	}

	drop := make([]bool, len(lines))
	for i := tasksAt + 1; i < end; i++ {
		text := lineText(lines[i])
		if !isContentLine(text) || leadingSpaces(text) != childIndent {
			continue
		}
		key, rest := splitKey(text[childIndent:])
		switch key {
		case "provider":
			value := trimQuotes(rest)
			if slices.Contains(RemovedTaskProviders, value) {
				drop[i] = true
				removed = append(removed, "tasks.provider ("+value+")")
			}
		case "providers":
			if rest == "" {
				pend := childRangeEnd(lines, i+1, end, childIndent)
				removed = append(removed, stripProviderBlocks(lines, drop, i+1, pend)...)
			} else if strings.HasPrefix(rest, "{") && flowDepth(rest) == 0 {
				// Known limit: only a flow map that opens and closes on this
				// one line is edited. A multi-line flow map is left as written
				// (and reports nothing), since cutting entries out of it safely
				// would mean parsing arbitrary YAML.
				text := lineText(lines[i])
				if newText, names := stripFlowProviders(text); len(names) > 0 {
					lines[i] = newText + lines[i][len(text):]
					removed = append(removed, names...)
				}
			}
		}
	}
	if len(removed) == 0 {
		return data, nil
	}

	var b strings.Builder
	for i, l := range lines {
		if !drop[i] {
			b.WriteString(l)
		}
	}
	return []byte(b.String()), removed
}

// stripProviderBlocks marks, within lines[from:to] (the children of
// tasks.providers), every removed provider's block and the comment lines right
// above it, and returns their tasks.providers.<name> keys.
func stripProviderBlocks(lines []string, drop []bool, from, to int) []string {
	provIndent := firstContentIndent(lines, from, to)
	if provIndent <= 0 {
		return nil
	}
	var removed []string
	for j := from; j < to; j++ {
		text := lineText(lines[j])
		if !isContentLine(text) || leadingSpaces(text) != provIndent {
			continue
		}
		key, rest := splitKey(text[provIndent:])
		if !slices.Contains(RemovedTaskProviders, key) {
			continue
		}

		last := j
		if strings.HasPrefix(rest, "{") || strings.HasPrefix(rest, "[") {
			// A flow value that spills over several lines.
			for depth := flowDepth(rest); depth > 0 && last+1 < to; {
				last++
				depth += flowDepth(lineText(lines[last]))
			}
		} else {
			// Whatever the value form (nothing, a block scalar, an anchor, a
			// tag, a plain value that continues), its lines are the ones
			// indented deeper than the key. Blank lines are part of the run
			// only when more of it follows, so the blank that separates it from
			// the next sibling stays.
			for k := j + 1; k < to; k++ {
				t := lineText(lines[k])
				if strings.TrimSpace(t) == "" {
					continue
				}
				if leadingSpaces(t) <= provIndent {
					break
				}
				last = k
			}
		}

		// Only the comment lines at the key's own indentation, directly above
		// it, belong to it; a deeper one closes the previous block.
		first := j
		for first-1 >= from {
			prev := lineText(lines[first-1])
			if !commentLineRe.MatchString(prev) || leadingSpaces(prev) != provIndent {
				break
			}
			first--
		}
		for k := first; k <= last; k++ {
			drop[k] = true
		}
		removed = append(removed, "tasks.providers."+key)
		j = last
	}
	return removed
}

// stripFlowProviders removes the removed providers' entries from a line holding
// a one-line flow map ("  providers: { teamwork: {...}, jira: {...} }  # c"),
// together with the separator that goes with each, and returns the new line and
// the tasks.providers.<name> keys it took out. Every other byte is kept; a line
// with nothing to remove (or no complete map) is returned as is.
func stripFlowProviders(line string) (string, []string) {
	open := strings.Index(line, "{")
	if open < 0 {
		return line, nil
	}
	type entry struct {
		key        string
		start, end int // trimmed content span within line
	}
	var entries []entry
	closeAt := -1
	depth := 0
	var quote byte
	segStart := open + 1
	flush := func(to int) {
		a, b := segStart, to
		for a < b && (line[a] == ' ' || line[a] == '\t') {
			a++
		}
		for b > a && (line[b-1] == ' ' || line[b-1] == '\t') {
			b--
		}
		if a == b {
			return
		}
		key, _, _ := strings.Cut(line[a:b], ":")
		entries = append(entries, entry{trimQuotes(strings.TrimSpace(key)), a, b})
	}
scan:
	for i := open; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '{' || c == '[':
			depth++
		case c == '}' || c == ']':
			depth--
			if depth == 0 {
				flush(i)
				closeAt = i
				break scan
			}
		case c == ',' && depth == 1:
			flush(i)
			segStart = i + 1
		}
	}
	if closeAt < 0 {
		return line, nil
	}

	lastKept := -1
	for i, e := range entries {
		if !slices.Contains(RemovedTaskProviders, e.key) {
			lastKept = i
		}
	}
	cut := make([]bool, len(line))
	mark := func(from, to int) {
		for k := from; k < to; k++ {
			cut[k] = true
		}
	}
	var names []string
	for i, e := range entries {
		if slices.Contains(RemovedTaskProviders, e.key) {
			names = append(names, "tasks.providers."+e.key)
			if i < lastKept {
				mark(e.start, entries[i+1].start)
			}
		}
	}
	switch {
	case len(names) == 0:
		return line, nil
	case lastKept < 0:
		mark(open+1, closeAt)
	case lastKept < len(entries)-1:
		mark(entries[lastKept].end, entries[len(entries)-1].end)
	}

	var b strings.Builder
	for i := 0; i < len(line); i++ {
		if !cut[i] {
			b.WriteByte(line[i])
		}
	}
	return b.String(), names
}

// lineText is a line without its terminator.
func lineText(line string) string {
	return strings.TrimRight(line, "\r\n")
}

// isContentLine reports whether text carries YAML content: not blank and not a
// full-line comment.
func isContentLine(text string) bool {
	return strings.TrimSpace(text) != "" && !commentLineRe.MatchString(text)
}

// childRangeEnd returns the index of the first content line in lines[from:to]
// indented at most parentIndent spaces (to when none), i.e. where the block
// opened by a key at parentIndent ends.
func childRangeEnd(lines []string, from, to, parentIndent int) int {
	for i := from; i < to; i++ {
		text := lineText(lines[i])
		if isContentLine(text) && leadingSpaces(text) <= parentIndent {
			return i
		}
	}
	return to
}

// firstContentIndent is the indentation of the first content line in
// lines[from:to], or 0 when there is none.
func firstContentIndent(lines []string, from, to int) int {
	for i := from; i < to; i++ {
		if text := lineText(lines[i]); isContentLine(text) {
			return leadingSpaces(text)
		}
	}
	return 0
}

// splitKey splits "key: value  # comment" into its key and its value with the
// comment and the padding removed. key is empty when text is not a key line.
func splitKey(text string) (key, rest string) {
	idx := strings.Index(text, ":")
	if idx < 1 || strings.ContainsAny(text[:idx], " \t") {
		return "", ""
	}
	rest = strings.TrimSpace(text[idx+1:])
	if strings.HasPrefix(rest, "#") {
		rest = ""
	}
	rest = strings.TrimSpace(trailingCommentRe.ReplaceAllString(rest, ""))
	return text[:idx], rest
}

// flowDepth is the net count of flow-collection openers over closers in text,
// ignoring quoted strings and a trailing comment.
func flowDepth(text string) int {
	depth := 0
	var quote rune
	prevSpace := true
	for _, r := range text {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '#' && prevSpace:
			return depth
		case r == '{' || r == '[':
			depth++
		case r == '}' || r == ']':
			depth--
		}
		prevSpace = r == ' ' || r == '\t'
	}
	return depth
}
