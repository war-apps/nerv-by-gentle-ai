package nerv_test

import (
	"os"
	"strings"
	"testing"
)

// rddNarrowingFile holds the fail-closed RDD narrowing rule for the Phase 3
// audit passes. The rule lives only in prose, so this guard pins it.
const rddNarrowingFile = "plugin/skills/nerv-orchestrator/references/pipeline-full.md"

// rddNarrowingParagraph returns the "**RDD narrowing.**" paragraph (anchor
// line up to the next blank line) with all whitespace runs collapsed to a
// single space, so the checks survive rewrapping.
func rddNarrowingParagraph(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(rddNarrowingFile)
	if err != nil {
		t.Fatal(err)
	}
	region := proseRegion(t, string(data), auditPassList{
		file:   rddNarrowingFile,
		region: "RDD narrowing paragraph",
		anchor: "**RDD narrowing.**",
	})
	return strings.Join(strings.Fields(region), " ")
}

// sentenceWith returns the sentence of paragraph that contains token, bounded
// by the previous and next ". " (or the paragraph edges).
func sentenceWith(t *testing.T, paragraph, token string) string {
	t.Helper()
	i := strings.Index(paragraph, token)
	if i < 0 {
		t.Fatalf("%s (RDD narrowing paragraph): %q not found", rddNarrowingFile, token)
	}
	start := strings.LastIndex(paragraph[:i], ". ")
	if start < 0 {
		start = 0
	} else {
		start += 2
	}
	end := strings.Index(paragraph[i:], ". ")
	if end < 0 {
		end = len(paragraph)
	} else {
		end += i + 1
	}
	return paragraph[start:end]
}

func TestRDDNarrowing_FailsClosed(t *testing.T) {
	paragraph := rddNarrowingParagraph(t)

	// The decision rests on the native assess verdict over committed history.
	for _, token := range []string{
		"gentle-ai review assess",
		"--committed-only",
		"`RDD scope: full`",
		"missing evidence never narrows",
	} {
		if !strings.Contains(paragraph, token) {
			t.Errorf("%s (RDD narrowing paragraph): required %q missing", rddNarrowingFile, token)
		}
	}

	// The clause that launches cross-commit scope admits only the two
	// evidence-backed reasons and narrows exactly melchor (whose audit pass
	// also carries the security lens) and balthasar.
	narrow := sentenceWith(t, paragraph, "`RDD scope: cross-commit`")
	for _, token := range []string{
		"`review_due: false`",
		"`already_reviewed`",
		"`passive`",
		"`melchor`",
		"`balthasar`",
	} {
		if !strings.Contains(narrow, token) {
			t.Errorf("%s (cross-commit clause): required %q missing", rddNarrowingFile, token)
		}
	}
	for _, token := range []string{
		"under_budget",
		"review_due: true",
		"`casper`",
		"`kaji-coverage`",
		"`kaji-resilience`",
		"kaji-security",
	} {
		if strings.Contains(narrow, token) {
			t.Errorf("%s (cross-commit clause): forbidden %q present", rddNarrowingFile, token)
		}
	}

	// under_budget is a fail-to-full outcome.
	full := sentenceWith(t, paragraph, "`RDD scope: full`")
	for _, token := range []string{"`under_budget`", "`review_due: true`"} {
		if !strings.Contains(full, token) {
			t.Errorf("%s (fail-to-full clause): required %q missing", rddNarrowingFile, token)
		}
	}

	// The remaining three passes never narrow.
	always := sentenceWith(t, paragraph, "always keep full NERV scope")
	for _, token := range []string{"`casper`", "`kaji-coverage`", "`kaji-resilience`"} {
		if !strings.Contains(always, token) {
			t.Errorf("%s (always-full clause): required %q missing", rddNarrowingFile, token)
		}
	}
}
