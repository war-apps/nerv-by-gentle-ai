package versioning

import (
	"slices"
	"testing"
)

// The order node 24 (ICU root; LANG en_US.UTF-8 and C.UTF-8) gives with
// localeCompare: typographic punctuation and symbols sort with the ASCII
// punctuation, before digits and letters.
func TestLocaleCompareTypographicPunctuationMatchesNode(t *testing.T) {
	want := []string{
		"—em dash", "¿qué", "… ellipsis", "’apos", "\"plain\" b", "“quoted” b",
		"§ sec", "° grado", "© c", "✨ sparkles", "📝 docs", "$ dollar", "€ euro",
		"1 paso", "1º paso", "a", "ª", "aesir", "Æsir", "n", "ñ", "nandu", "Ñandú",
		"o", "oe", "œ", "u", "ü", "Ü", "z",
	}
	got := slices.Clone(want)
	slices.Reverse(got)
	slices.SortStableFunc(got, localeCompare)
	if !slices.Equal(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

// dim.comprobantes.api writes em dashes in its subjects; semantic-release
// sorts "wave 4 — security" before "wave 4 cleanup".
func TestComputeNotesSortEmDashLikeSemanticRelease(t *testing.T) {
	r := newTestRepo(t)
	r.commit("feat: base")
	r.release("v1.0.0", "")
	cleanup := r.commit("fix(audit): wave 4 cleanup")
	security := r.commit("fix(audit): wave 4 — security")
	got := one(t, r.compute(Request{}))
	want := "## 1.0.1 (" + got.NewReleaseNotes[10:20] + ")\n\n### Bug Fixes\n\n" +
		"* **audit:** wave 4 — security (" + security[:7] + ")\n" +
		"* **audit:** wave 4 cleanup (" + cleanup[:7] + ")\n"
	if got.NewReleaseNotes != want {
		t.Fatalf("notes:\n%s\nwant:\n%s", got.NewReleaseNotes, want)
	}
}
