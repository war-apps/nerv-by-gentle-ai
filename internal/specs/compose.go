// Package specs composes a change's delta spec into the canonical spec it
// amends, for the archive step of a change.
//
// This file is a port of gentle-ai v3.7.0, internal/sddstatus/
// openspec_archive_compose.go (gentle-ai removed that merge subcommand
// in v4.0.0). Merge behavior is unchanged; names and the
// gentle-ai-specific schema constant were adapted.
//
// # Copyright (c) 2025 Gentleman Programming
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
package specs

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// UnappliedDeltaError names exactly one delta requirement that
// Compose could not apply. Callers MUST refuse -- never report success on a
// partial composition.
type UnappliedDeltaError struct {
	// Section is ADDED, MODIFIED, REMOVED, RENAMED, DELTA, or CANONICAL (the
	// last two describe a malformed input, not one named requirement).
	Section     string
	Requirement string
	Reason      string
}

func (e *UnappliedDeltaError) Error() string {
	if e.Requirement == "" {
		return fmt.Sprintf("spec-compose: %s: %s", e.Section, e.Reason)
	}
	return fmt.Sprintf("spec-compose: unapplied %s delta for requirement %q: %s", e.Section, e.Requirement, e.Reason)
}

var specReqHeadingLine = regexp.MustCompile(`(?m)^### Requirement:[ \t]*(.+?)[ \t\r]*$`)
var specH2Heading = regexp.MustCompile(`(?m)^## \S`)
var specDeltaSectionHeading = regexp.MustCompile(`(?m)^## (ADDED|MODIFIED|REMOVED|RENAMED) Requirements[ \t\r]*$`)
var specRenameArrow = regexp.MustCompile(`[ \t]*(?:\x{2192}|->)[ \t]*`)
var specReasonNote = regexp.MustCompile(`(?m)^\(Reason:.+\)[ \t\r]*$`)

type specRequirementBlock struct {
	Name, Text string
}

// specSegment is one contiguous slice of a canonical spec: either a
// "### Requirement:" block or a non-requirement span (preamble, an
// interstitial "## Section", or trailing content). Segments partition the
// whole document with no gaps, so re-emitting every segment in order
// reproduces the input byte-for-byte.
type specSegment struct {
	IsRequirement bool
	Name, Text    string
}

type specDocument struct {
	Segments []specSegment
}

func (d specDocument) hasRequirement() bool {
	for _, s := range d.Segments {
		if s.IsRequirement {
			return true
		}
	}
	return false
}

// fenceMarker reports whether line opens or closes a ``` / ~~~ fence: the
// fence character, its run length, and whether trailing content follows the
// run (an info string, which only a valid opener may carry).
func fenceMarker(line string) (ch byte, length int, hasInfo, ok bool) {
	if len(line) < 3 || (line[0] != '`' && line[0] != '~') {
		return 0, 0, false, false
	}
	ch = line[0]
	for length < len(line) && line[length] == ch {
		length++
	}
	if length < 3 {
		return 0, 0, false, false
	}
	return ch, length, strings.TrimSpace(line[length:]) != "", true
}

// fencedRanges scans text line-by-line and returns the byte ranges that lie
// inside a fenced code block, so a "## " or "### Requirement:" line found
// only inside a fence (e.g. as sample content) is never treated as a
// document boundary. A close only matches the same fence character with a
// run at least as long as the opener, and carries no info string. An
// unterminated fence is treated as fenced through end of document.
func fencedRanges(text string) [][2]int {
	var ranges [][2]int
	inFence, fenceChar, fenceLen, fenceStart := false, byte(0), 0, 0
	for pos := 0; pos <= len(text); {
		nl := strings.IndexByte(text[pos:], '\n')
		lineEnd := len(text)
		next := len(text) + 1
		if nl != -1 {
			lineEnd, next = pos+nl, pos+nl+1
		}
		line := text[pos:lineEnd]
		if !inFence {
			if ch, n, _, ok := fenceMarker(line); ok {
				inFence, fenceChar, fenceLen, fenceStart = true, ch, n, pos
			}
		} else if ch, n, hasInfo, ok := fenceMarker(line); ok && ch == fenceChar && n >= fenceLen && !hasInfo {
			ranges, inFence = append(ranges, [2]int{fenceStart, lineEnd}), false
		}
		pos = next
	}
	if inFence {
		ranges = append(ranges, [2]int{fenceStart, len(text)})
	}
	return ranges
}

func insideAnyRange(ranges [][2]int, pos int) bool {
	for _, r := range ranges {
		if pos >= r[0] && pos < r[1] {
			return true
		}
	}
	return false
}

// parseSpecDocument partitions a spec at every "### Requirement:" and "## "
// heading boundary that sits outside a fenced code block. Every byte of the
// input belongs to exactly one segment.
func parseSpecDocument(text string) specDocument {
	fences := fencedRanges(text)

	var reqMatches [][]int
	for _, m := range specReqHeadingLine.FindAllStringSubmatchIndex(text, -1) {
		if !insideAnyRange(fences, m[0]) {
			reqMatches = append(reqMatches, m)
		}
	}
	if len(reqMatches) == 0 {
		if text == "" {
			return specDocument{}
		}
		return specDocument{Segments: []specSegment{{Text: text}}}
	}

	reqAt := make(map[int]int, len(reqMatches))
	boundaries := make([]int, 0, len(reqMatches)+2)
	for i, m := range reqMatches {
		reqAt[m[0]] = i
		boundaries = append(boundaries, m[0])
	}
	for _, m := range specH2Heading.FindAllStringIndex(text, -1) {
		if !insideAnyRange(fences, m[0]) {
			boundaries = append(boundaries, m[0])
		}
	}
	boundaries = append(boundaries, 0, len(text))
	sort.Ints(boundaries)

	var doc specDocument
	for i := 0; i < len(boundaries)-1; i++ {
		start, end := boundaries[i], boundaries[i+1]
		if start == end {
			continue
		}
		if m, ok := reqAt[start]; ok {
			name := strings.TrimSpace(text[reqMatches[m][2]:reqMatches[m][3]])
			doc.Segments = append(doc.Segments, specSegment{IsRequirement: true, Name: name, Text: text[start:end]})
			continue
		}
		doc.Segments = append(doc.Segments, specSegment{Text: text[start:end]})
	}
	return doc
}

type specDelta struct {
	Added, Modified, Removed, Renamed []specRequirementBlock
}

// parseDelta splits a delta spec into its ADDED/MODIFIED/REMOVED/
// RENAMED requirement blocks (the OpenSpec delta format).
func parseDelta(text string) specDelta {
	sections := specDeltaSectionHeading.FindAllStringSubmatchIndex(text, -1)
	var delta specDelta
	for i, m := range sections {
		kind, start, end := text[m[2]:m[3]], m[1], len(text)
		if i+1 < len(sections) {
			end = sections[i+1][0]
		}
		blocks := parseDeltaRequirementBlocks(text[start:end])
		switch kind {
		case "ADDED":
			delta.Added = append(delta.Added, blocks...)
		case "MODIFIED":
			delta.Modified = append(delta.Modified, blocks...)
		case "REMOVED":
			delta.Removed = append(delta.Removed, blocks...)
		case "RENAMED":
			delta.Renamed = append(delta.Renamed, blocks...)
		}
	}
	return delta
}

func parseDeltaRequirementBlocks(text string) []specRequirementBlock {
	matches := specReqHeadingLine.FindAllStringSubmatchIndex(text, -1)
	blocks := make([]specRequirementBlock, 0, len(matches))
	for i, m := range matches {
		end := len(text)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		name := strings.TrimSpace(text[m[2]:m[3]])
		blocks = append(blocks, specRequirementBlock{Name: name, Text: text[m[0]:end]})
	}
	return blocks
}

func splitRenamePair(name string) (oldName, newName string, ok bool) {
	parts := specRenameArrow.Split(name, 2)
	if len(parts) != 2 {
		return "", "", false
	}
	oldName, newName = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	return oldName, newName, oldName != "" && newName != ""
}

func renameRequirementHeading(text, newName string) string {
	replaced := false
	return specReqHeadingLine.ReplaceAllStringFunc(text, func(line string) string {
		if replaced {
			return line
		}
		replaced = true
		heading := "### Requirement: " + newName
		if strings.HasSuffix(line, "\r") {
			heading += "\r" // keep the heading's own line ending
		}
		return heading
	})
}

func ensureTrailingNewline(text string) string {
	if strings.HasSuffix(text, "\n") {
		return text
	}
	return text + "\n"
}

// Compose merges a delta spec into a canonical
// spec's byte content. Every unrelated canonical requirement is preserved
// verbatim; RENAMED, MODIFIED, REMOVED, then ADDED delta requirements are
// applied in that order, so a rename is visible to a same-change MODIFIED
// for its new name.
//
// It never returns a partial composition: an unmatched, duplicate, or
// malformed delta requirement returns *UnappliedDeltaError instead.
func Compose(canonical, delta string) (string, error) {
	canonicalDoc := parseSpecDocument(canonical)
	if !canonicalDoc.hasRequirement() {
		return "", &UnappliedDeltaError{Section: "CANONICAL", Reason: `canonical spec has no "### Requirement:" headings to compose against`}
	}

	deltaDoc := parseDelta(delta)
	if len(deltaDoc.Added) == 0 && len(deltaDoc.Modified) == 0 && len(deltaDoc.Removed) == 0 && len(deltaDoc.Renamed) == 0 {
		return "", &UnappliedDeltaError{Section: "DELTA", Reason: "delta spec declares no ADDED, MODIFIED, REMOVED, or RENAMED requirements"}
	}

	// segments re-emits the whole document, including non-requirement spans
	// between requirement blocks, so nothing but a targeted delta edit ever
	// changes a byte of it.
	segments := append([]specSegment(nil), canonicalDoc.Segments...)
	indexOf := func(name string) int {
		for i, s := range segments {
			if s.IsRequirement && s.Name == name {
				return i
			}
		}
		return -1
	}
	lastRequirementIndex := func() int {
		last := -1
		for i, s := range segments {
			if s.IsRequirement {
				last = i
			}
		}
		return last
	}

	for _, r := range deltaDoc.Renamed {
		oldName, newName, ok := splitRenamePair(r.Name)
		if !ok {
			return "", &UnappliedDeltaError{Section: "RENAMED", Requirement: r.Name, Reason: `heading must state "Old Name → New Name"`}
		}
		i := indexOf(oldName)
		if i == -1 {
			return "", &UnappliedDeltaError{Section: "RENAMED", Requirement: r.Name, Reason: fmt.Sprintf("no canonical requirement named %q", oldName)}
		}
		if !specReasonNote.MatchString(r.Text) {
			return "", &UnappliedDeltaError{Section: "RENAMED", Requirement: r.Name, Reason: `missing required "(Reason: ...)" note`}
		}
		if j := indexOf(newName); j != -1 && j != i {
			return "", &UnappliedDeltaError{Section: "RENAMED", Requirement: newName, Reason: "target name already exists"}
		}
		segments[i].Text = renameRequirementHeading(segments[i].Text, newName)
		segments[i].Name = newName
	}

	for _, r := range deltaDoc.Modified {
		i := indexOf(r.Name)
		if i == -1 {
			return "", &UnappliedDeltaError{Section: "MODIFIED", Requirement: r.Name, Reason: fmt.Sprintf("no canonical requirement named %q", r.Name)}
		}
		segments[i].Text = ensureTrailingNewline(r.Text)
	}

	// addAnchor is "insert the next ADDED requirement right after this
	// index". It starts at the last requirement segment and is adjusted as
	// REMOVED deletes segments, so a REMOVE-then-ADD on the last remaining
	// requirement falls back to the non-requirement segment that preceded
	// it (e.g. the "## Requirements" heading) instead of index 0.
	addAnchor := lastRequirementIndex()

	for _, r := range deltaDoc.Removed {
		if !specReasonNote.MatchString(r.Text) {
			return "", &UnappliedDeltaError{Section: "REMOVED", Requirement: r.Name, Reason: `missing required "(Reason: ...)" note`}
		}
		i := indexOf(r.Name)
		if i == -1 {
			return "", &UnappliedDeltaError{Section: "REMOVED", Requirement: r.Name, Reason: fmt.Sprintf("no canonical requirement named %q", r.Name)}
		}
		switch {
		case i == addAnchor:
			addAnchor = i - 1
		case i < addAnchor:
			addAnchor--
		}
		segments = append(segments[:i], segments[i+1:]...)
	}

	for _, r := range deltaDoc.Added {
		if indexOf(r.Name) != -1 {
			return "", &UnappliedDeltaError{Section: "ADDED", Requirement: r.Name, Reason: fmt.Sprintf("a requirement named %q already exists in the canonical spec", r.Name)}
		}
		insertAt := addAnchor + 1
		newSegment := specSegment{IsRequirement: true, Name: r.Name, Text: ensureTrailingNewline(r.Text)}
		segments = append(segments[:insertAt], append([]specSegment{newSegment}, segments[insertAt:]...)...)
		addAnchor = insertAt
	}

	var b strings.Builder
	for _, s := range segments {
		if s.IsRequirement {
			b.WriteString(ensureTrailingNewline(s.Text))
			continue
		}
		b.WriteString(s.Text)
	}
	return b.String(), nil
}
