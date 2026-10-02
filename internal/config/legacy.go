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
// directly above it). It returns the new bytes and what it removed, in file
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
		} else if rest == "" {
			for k := j + 1; k < to; k++ {
				t := lineText(lines[k])
				if !isContentLine(t) {
					continue
				}
				if leadingSpaces(t) <= provIndent {
					break
				}
				last = k
			}
		}

		first := j
		for first-1 >= from && commentLineRe.MatchString(lineText(lines[first-1])) {
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
