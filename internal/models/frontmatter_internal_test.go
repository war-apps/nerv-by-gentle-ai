package models

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// applyFrontmatter — ports tests/install-apply-models.test.ps1's
// Set-NervAgentFrontmatter cases onto a single file's content. Exercised
// here in-package (rather than through a public wrapper) since ApplyToDir
// is the only production caller and it already has its own directory-level
// tests in models_test.go.
// ---------------------------------------------------------------------------

const shinjiFixture = "---\nname: shinji\nmodel: sonnet\neffort: medium\ntools: Read\n---\n\nbody unchanged\n"

func TestApplyFrontmatter_UpdatesModelAndEffort(t *testing.T) {
	out, changed, _ := applyFrontmatter([]byte(shinjiFixture), "haiku", "low")
	if !changed {
		t.Fatal("changed = false, want true")
	}
	text := string(out)
	if !strings.Contains(text, "model: haiku\n") {
		t.Fatalf("output missing updated model line: %q", text)
	}
	if !strings.Contains(text, "effort: low\n") {
		t.Fatalf("output missing updated effort line: %q", text)
	}
	if !strings.Contains(text, "body unchanged") {
		t.Fatalf("output body altered: %q", text)
	}
}

func TestApplyFrontmatter_SameValuesReportUnchanged(t *testing.T) {
	out, changed, _ := applyFrontmatter([]byte(shinjiFixture), "sonnet", "medium")
	if changed {
		t.Fatal("changed = true, want false when values already match")
	}
	if string(out) != shinjiFixture {
		t.Fatalf("output = %q, want the untouched fixture", out)
	}
}

func TestApplyFrontmatter_EmptyModelLeavesModelUntouched(t *testing.T) {
	// Mirrors Case C: an assignment carrying only Model leaves an existing
	// effort: line alone.
	out, changed, _ := applyFrontmatter([]byte(shinjiFixture), "opus", "")
	if !changed {
		t.Fatal("changed = false, want true")
	}
	text := string(out)
	if !strings.Contains(text, "model: opus\n") {
		t.Fatalf("output missing updated model line: %q", text)
	}
	if !strings.Contains(text, "effort: medium\n") {
		t.Fatalf("output effort line changed, want it untouched: %q", text)
	}
}

func TestApplyFrontmatter_InsertsMissingEffortLineAfterModel(t *testing.T) {
	fixture := "---\nname: x\nmodel: sonnet\ntools: Read\n---\nbody\n"
	out, changed, _ := applyFrontmatter([]byte(fixture), "", "high")
	if !changed {
		t.Fatal("changed = false, want true")
	}
	lines := strings.Split(string(out), "\n")
	modelIdx := indexOf(lines, "model: sonnet")
	if modelIdx < 0 {
		t.Fatalf("model line not found in output: %v", lines)
	}
	if lines[modelIdx+1] != "effort: high" {
		t.Fatalf("line after model = %q, want %q", lines[modelIdx+1], "effort: high")
	}
}

func TestApplyFrontmatter_PreservesTrailingCommentAndCRLF(t *testing.T) {
	fixture := "---\r\nname: misato\r\nmodel: fable # a trailing comment kept verbatim\r\neffort: high\r\n---\r\n"
	out, changed, _ := applyFrontmatter([]byte(fixture), "opus", "")
	if !changed {
		t.Fatal("changed = false, want true")
	}
	text := string(out)
	if !strings.Contains(text, "model: opus # a trailing comment kept verbatim\r\n") {
		t.Fatalf("output did not preserve the trailing comment: %q", text)
	}
	if strings.ContainsRune(strings.ReplaceAll(text, "\r\n", ""), '\n') {
		t.Fatalf("output introduced a bare LF: %q", text)
	}
}

func TestApplyFrontmatter_NoModelKeyLeavesFileUnchanged(t *testing.T) {
	fixture := "---\nname: x\ntools: Read\n---\nbody\n"
	out, changed, ok := applyFrontmatter([]byte(fixture), "sonnet", "low")
	if changed {
		t.Fatal("changed = true, want false when there is no model: key")
	}
	if ok {
		t.Fatal("ok = true, want false when there is no model: key")
	}
	if string(out) != fixture {
		t.Fatalf("output = %q, want the untouched fixture", out)
	}
}

func TestApplyFrontmatter_MalformedFrontmatterLeavesFileUnchanged(t *testing.T) {
	fixture := "not frontmatter at all\n"
	out, changed, ok := applyFrontmatter([]byte(fixture), "sonnet", "low")
	if changed {
		t.Fatal("changed = true, want false for malformed frontmatter")
	}
	if ok {
		t.Fatal("ok = true, want false for malformed frontmatter")
	}
	if string(out) != fixture {
		t.Fatalf("output = %q, want the untouched fixture", out)
	}
}

func indexOf(lines []string, prefix string) int {
	for i, l := range lines {
		if strings.TrimRight(l, "\r") == prefix {
			return i
		}
	}
	return -1
}
