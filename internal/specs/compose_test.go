package specs

import (
	"errors"
	"strings"
	"testing"
)

const canonicalFixture = `# Widgets Specification

## Requirements

### Requirement: Unrelated Listing

The system MUST list widgets.

#### Scenario: List widgets

- GIVEN a widget store
- WHEN a client lists widgets
- THEN the system returns every widget

### Requirement: Widget Expiration

The system MUST expire widgets after 30 days.

#### Scenario: Expire after 30 days

- GIVEN a widget older than 30 days
- WHEN expiration runs
- THEN the widget is removed
`

// composition previously lived only in model Read/Edit instructions,
// which could drop unrelated requirements or skip a delta while reporting
// success. This pins the Go merge now backing the archive step.
func TestComposeAppliesAddedAndModifiedPreservingUnrelated(t *testing.T) {
	delta := `## ADDED Requirements

### Requirement: Widget Tagging

The system MUST support tagging widgets.

#### Scenario: Tag a widget

- GIVEN a widget
- WHEN a client adds a tag
- THEN the tag is stored

## MODIFIED Requirements

### Requirement: Widget Expiration

The system MUST expire widgets after 90 days.
(Previously: 30 days)
`

	composed, err := Compose(canonicalFixture, delta)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}

	// Dropped-requirement bug from the unrelated requirement must survive.
	if !strings.Contains(composed, "### Requirement: Unrelated Listing") || !strings.Contains(composed, "GIVEN a widget store") {
		t.Fatalf("composed spec dropped the unrelated requirement:\n%s", composed)
	}
	// Unapplied-delta bug from the MODIFIED delta must land.
	if !strings.Contains(composed, "expire widgets after 90 days") || strings.Contains(composed, "expire widgets after 30 days") {
		t.Fatalf("composed spec did not apply the MODIFIED delta:\n%s", composed)
	}
	if count := strings.Count(composed, "### Requirement:"); count != 3 {
		t.Fatalf("composed spec has %d requirements, want 3 (2 original + 1 added):\n%s", count, composed)
	}
}

func TestComposeAppliesRemovedAndRenamed(t *testing.T) {
	delta := `## REMOVED Requirements

### Requirement: Unrelated Listing

(Reason: no longer needed)

## RENAMED Requirements

### Requirement: Widget Expiration → Widget Retention

(Reason: clarify naming)
`
	composed, err := Compose(canonicalFixture, delta)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	if strings.Contains(composed, "### Requirement: Unrelated Listing") {
		t.Fatalf("composed spec kept the removed requirement:\n%s", composed)
	}
	if !strings.Contains(composed, "### Requirement: Widget Retention") || strings.Contains(composed, "### Requirement: Widget Expiration") {
		t.Fatalf("composed spec did not apply the rename:\n%s", composed)
	}
}

// archive must refuse (typed error naming the section+requirement)
// rather than compose partially or report success.
func TestComposeRefusesUnapplicableDeltas(t *testing.T) {
	tests := []struct {
		name        string
		canonical   string
		delta       string
		wantSection string
		wantReq     string
	}{
		{
			name:        "MODIFIED references unknown requirement",
			canonical:   canonicalFixture,
			delta:       "## MODIFIED Requirements\n\n### Requirement: Missing\n\nBody.\n",
			wantSection: "MODIFIED",
			wantReq:     "Missing",
		},
		{
			name:        "REMOVED without a Reason note",
			canonical:   canonicalFixture,
			delta:       "## REMOVED Requirements\n\n### Requirement: Widget Expiration\n\nNo reason given.\n",
			wantSection: "REMOVED",
			wantReq:     "Widget Expiration",
		},
		{
			name:        "ADDED duplicates an existing requirement",
			canonical:   canonicalFixture,
			delta:       "## ADDED Requirements\n\n### Requirement: Unrelated Listing\n\nDuplicate.\n",
			wantSection: "ADDED",
			wantReq:     "Unrelated Listing",
		},
		{
			name:        "RENAMED target name already exists",
			canonical:   canonicalFixture,
			delta:       "## RENAMED Requirements\n\n### Requirement: Widget Expiration → Unrelated Listing\n\n(Reason: collide)\n",
			wantSection: "RENAMED",
			wantReq:     "Unrelated Listing",
		},
		{
			name:        "empty delta declares no sections",
			canonical:   canonicalFixture,
			delta:       "No sections here.\n",
			wantSection: "DELTA",
		},
		{
			name:        "empty canonical has no requirements",
			canonical:   "# Widgets Specification\n\nNo requirements yet.\n",
			delta:       "## ADDED Requirements\n\n### Requirement: New One\n\nBody.\n",
			wantSection: "CANONICAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compose(tt.canonical, tt.delta)
			var unapplied *UnappliedDeltaError
			if !errors.As(err, &unapplied) {
				t.Fatalf("error = %v, want *UnappliedDeltaError", err)
			}
			if unapplied.Section != tt.wantSection || (tt.wantReq != "" && unapplied.Requirement != tt.wantReq) {
				t.Fatalf("unapplied = %+v, want Section=%q Requirement=%q", unapplied, tt.wantSection, tt.wantReq)
			}
		})
	}
}

// Two entries with the same name inside one delta section used to fold into
// one (the last MODIFIED silently won). Each repeated name is now refused,
// naming the section and the requirement.
func TestComposeRefusesRequirementRepeatedWithinOneDeltaSection(t *testing.T) {
	tests := []struct {
		name        string
		delta       string
		wantSection string
		wantReq     string
	}{
		{
			name: "MODIFIED repeats a name",
			delta: "## MODIFIED Requirements\n\n### Requirement: Widget Expiration\n\nFirst.\n\n" +
				"### Requirement: Widget Expiration\n\nSecond.\n",
			wantSection: "MODIFIED",
			wantReq:     "Widget Expiration",
		},
		{
			name: "ADDED repeats a new name",
			delta: "## ADDED Requirements\n\n### Requirement: Fresh One\n\nFirst.\n\n" +
				"### Requirement: Fresh One\n\nSecond.\n",
			wantSection: "ADDED",
			wantReq:     "Fresh One",
		},
		{
			name: "REMOVED repeats a name",
			delta: "## REMOVED Requirements\n\n### Requirement: Widget Expiration\n\n(Reason: a)\n\n" +
				"### Requirement: Widget Expiration\n\n(Reason: b)\n",
			wantSection: "REMOVED",
			wantReq:     "Widget Expiration",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Compose(canonicalFixture, tt.delta)
			var unapplied *UnappliedDeltaError
			if !errors.As(err, &unapplied) {
				t.Fatalf("error = %v (output %q), want *UnappliedDeltaError", err, out)
			}
			if unapplied.Section != tt.wantSection || unapplied.Requirement != tt.wantReq {
				t.Fatalf("unapplied = %+v, want Section=%q Requirement=%q", unapplied, tt.wantSection, tt.wantReq)
			}
			if !strings.Contains(unapplied.Reason, "more than once") {
				t.Errorf("Reason = %q, want it to say the name appears more than once", unapplied.Reason)
			}
		})
	}
}

// a "## Section" sitting between two
// requirement blocks belonged to no block under the old preamble/
// requirements/trailing model and was silently dropped. This pins the
// segment model that preserves it byte-for-byte while still applying a
// MODIFIED delta to the requirement that follows it.
func TestComposePreservesInterstitialSectionBetweenRequirements(t *testing.T) {
	const interstitial = "## Notes\n\nSome interstitial notes that are not a requirement.\n\n"
	canonical := "# Widgets Specification\n\n## Requirements\n\n" +
		"### Requirement: Req A\n\nOriginal req A sentinel.\n\n" +
		interstitial +
		"### Requirement: Req B\n\nOriginal req B sentinel.\n"
	delta := "## MODIFIED Requirements\n\n### Requirement: Req B\n\nReplaced req B sentinel.\n"

	composed, err := Compose(canonical, delta)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	if !strings.Contains(composed, interstitial) {
		t.Fatalf("composed spec dropped the interstitial section:\n%s", composed)
	}
	if !strings.Contains(composed, "Original req A sentinel.") {
		t.Fatalf("composed spec altered the unrelated requirement:\n%s", composed)
	}
	if !strings.Contains(composed, "Replaced req B sentinel.") {
		t.Fatalf("composed spec did not apply the MODIFIED delta after the interstitial section:\n%s", composed)
	}
	if strings.Contains(composed, "Original req B sentinel.") {
		t.Fatalf("composed spec kept the stale pre-delta requirement text:\n%s", composed)
	}
}

// Regression: the ADDED insertion anchor was recomputed as
// "last requirement index + 1", which becomes 0 (before the preamble) once
// REMOVED deletes the only requirement. A REMOVE-then-ADD on a
// single-requirement spec must still land the replacement after the
// preamble, not ahead of it.
func TestComposeReplacesTheOnlyRequirementKeepingPreambleFirst(t *testing.T) {
	const preamble = "# Widgets Specification\n\n## Requirements\n\n"
	canonical := preamble + "### Requirement: Only\n\nOriginal only body.\n"
	delta := "## REMOVED Requirements\n\n### Requirement: Only\n\n(Reason: replaced)\n\n" +
		"## ADDED Requirements\n\n### Requirement: Replacement\n\nReplacement body.\n"

	composed, err := Compose(canonical, delta)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	want := preamble + "### Requirement: Replacement\n\nReplacement body.\n"
	if composed != want {
		t.Fatalf("composed = %q, want %q (preamble first, replacement after it)", composed, want)
	}
}

// Regression: the emit loop wrote requirement segments
// verbatim, so a canonical whose final requirement lacked a trailing newline
// got an ADDED requirement concatenated onto its last line.
func TestComposeAddsMissingNewlineBeforeAddedRequirement(t *testing.T) {
	canonical := "## Requirements\n\n### Requirement: Only\n\nBody without trailing newline."
	delta := "## ADDED Requirements\n\n### Requirement: New One\n\nNew body.\n"

	composed, err := Compose(canonical, delta)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	if strings.Contains(composed, "trailing newline.### Requirement: New One") {
		t.Fatalf("new requirement was concatenated onto the previous requirement's last line:\n%s", composed)
	}
	if !strings.Contains(composed, "trailing newline.\n### Requirement: New One") {
		t.Fatalf("new requirement does not start on its own line:\n%s", composed)
	}
}

// an empty-effect delta (a declared section with no actual
// requirement blocks under it) must never silently lose canonical bytes. The
// contract refuses rather than compose a no-op, so verify the refusal and
// that the canonical text itself was never touched to produce it.
func TestComposeRefusesEmptyEffectDeltaWithoutLosingBytes(t *testing.T) {
	composed, err := Compose(canonicalFixture, "## ADDED Requirements\n\nNo requirement heading follows.\n")
	var unapplied *UnappliedDeltaError
	if !errors.As(err, &unapplied) || unapplied.Section != "DELTA" {
		t.Fatalf("error = %v, want *UnappliedDeltaError{Section: DELTA}", err)
	}
	if composed != "" {
		t.Fatalf("refused compose returned non-empty output: %q", composed)
	}

	// The refusal must not have left any mutated/shared state behind: the
	// same canonical text must still compose correctly on a real delta,
	// with every original requirement intact in the returned document.
	recomposed, err := Compose(canonicalFixture, "## ADDED Requirements\n\n### Requirement: Widget Tagging\n\nBody.\n")
	if err != nil {
		t.Fatalf("Compose() after a refusal error = %v", err)
	}
	if !strings.Contains(recomposed, "### Requirement: Unrelated Listing") || !strings.Contains(recomposed, "### Requirement: Widget Expiration") {
		t.Fatalf("canonical content lost after an earlier refusal:\n%s", recomposed)
	}
}

// Regression: a "## " line inside a fenced code block was
// still treated as a hard segment boundary, splitting a requirement body in
// two. MODIFIED then replaced only the truncated head, leaving the fenced
// sample's tail as an orphan segment with an unterminated fence.
func TestComposeKeepsFencedHeadingInsideRequirementBody(t *testing.T) {
	const fencedBlock = "```text\n## Not a heading\n```\n\n"
	canonical := "## Requirements\n\n" +
		"### Requirement: Req A\n\n" + fencedBlock + "More body text.\n\n" +
		"### Requirement: Req B\n\nBody B.\n"
	delta := "## MODIFIED Requirements\n\n### Requirement: Req A\n\nReplaced req A body.\n"

	composed, err := Compose(canonical, delta)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	if strings.Contains(composed, "## Not a heading") || strings.Contains(composed, "More body text.") {
		t.Fatalf("fenced content leaked past the MODIFIED replacement as an orphan tail:\n%s", composed)
	}
	if !strings.Contains(composed, "Replaced req A body.") {
		t.Fatalf("MODIFIED delta was not applied:\n%s", composed)
	}
	if !strings.Contains(composed, "### Requirement: Req B\n\nBody B.\n") {
		t.Fatalf("unrelated requirement B was altered or dropped:\n%s", composed)
	}
	if strings.Count(composed, "```")%2 != 0 {
		t.Fatalf("composed spec has an unterminated fence:\n%s", composed)
	}
}

func TestUnappliedDeltaErrorMessageNamesSectionAndRequirement(t *testing.T) {
	named := &UnappliedDeltaError{Section: "MODIFIED", Requirement: "Missing", Reason: "no canonical requirement"}
	if got, want := named.Error(), `spec-compose: unapplied MODIFIED delta for requirement "Missing": no canonical requirement`; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	malformed := &UnappliedDeltaError{Section: "DELTA", Reason: "no sections"}
	if got, want := malformed.Error(), "spec-compose: DELTA: no sections"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestComposeMatchesRequirementNamesExactly(t *testing.T) {
	_, err := Compose(canonicalFixture, "## MODIFIED Requirements\n\n### Requirement: widget expiration\n\nBody.\n")
	var unapplied *UnappliedDeltaError
	if !errors.As(err, &unapplied) || unapplied.Section != "MODIFIED" {
		t.Fatalf("error = %v, want *UnappliedDeltaError{Section: MODIFIED} for a differently cased name", err)
	}
}

func TestComposeAcceptsAsciiArrowInRename(t *testing.T) {
	delta := "## RENAMED Requirements\n\n### Requirement: Widget Expiration -> Widget Retention\n\n(Reason: naming)\n"
	composed, err := Compose(canonicalFixture, delta)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	if !strings.Contains(composed, "### Requirement: Widget Retention\n") {
		t.Fatalf("rename with -> was not applied:\n%s", composed)
	}
}

func TestComposeRenameWithoutReasonIsRefused(t *testing.T) {
	_, err := Compose(canonicalFixture, "## RENAMED Requirements\n\n### Requirement: Widget Expiration → Widget Retention\n\nNo reason.\n")
	var unapplied *UnappliedDeltaError
	if !errors.As(err, &unapplied) || unapplied.Section != "RENAMED" {
		t.Fatalf("error = %v, want *UnappliedDeltaError{Section: RENAMED}", err)
	}
}

func TestComposeAppliesRenamedBeforeModified(t *testing.T) {
	delta := "## MODIFIED Requirements\n\n### Requirement: Widget Retention\n\nKept for 90 days.\n\n" +
		"## RENAMED Requirements\n\n### Requirement: Widget Expiration → Widget Retention\n\n(Reason: naming)\n"
	composed, err := Compose(canonicalFixture, delta)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	if !strings.Contains(composed, "Kept for 90 days.") || strings.Contains(composed, "expire widgets after 30 days") {
		t.Fatalf("MODIFIED did not see the renamed requirement:\n%s", composed)
	}
}

func TestComposeIgnoresRequirementHeadingInsideFence(t *testing.T) {
	canonical := "## Requirements\n\n### Requirement: A\n\n```md\n### Requirement: Fake\n```\n"
	_, err := Compose(canonical, "## MODIFIED Requirements\n\n### Requirement: Fake\n\nBody.\n")
	var unapplied *UnappliedDeltaError
	if !errors.As(err, &unapplied) || unapplied.Requirement != "Fake" {
		t.Fatalf("error = %v, want the fenced heading to be unmatched", err)
	}
}

func TestComposeOutputEndsWithNewline(t *testing.T) {
	composed, err := Compose("## Requirements\n\n### Requirement: A\n\nNo newline.", "## MODIFIED Requirements\n\n### Requirement: A\n\nStill none.")
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	if !strings.HasSuffix(composed, "\n") {
		t.Fatalf("composed output does not end with a newline: %q", composed)
	}
}

func crlf(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }

const crlfCanonical = "## Requirements\n\n### Requirement: A\n\nOld A.\n\n### Requirement: B\n\nOld B.\n"

func TestComposeAcceptsCRLFModified(t *testing.T) {
	got, err := Compose(crlf(crlfCanonical), crlf("## MODIFIED Requirements\n\n### Requirement: A\n\nNew A.\n"))
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	want := strings.Replace(crlf(crlfCanonical), "Old A.\r\n\r\n", "New A.\r\n", 1)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestComposeAcceptsCRLFAdded(t *testing.T) {
	got, err := Compose(crlf(crlfCanonical), crlf("## ADDED Requirements\n\n### Requirement: C\n\nNew C.\n"))
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	want := crlf(crlfCanonical) + crlf("### Requirement: C\n\nNew C.\n")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestComposeAcceptsCRLFRenamedWithReason(t *testing.T) {
	got, err := Compose(crlf(crlfCanonical), crlf("## RENAMED Requirements\n\n### Requirement: A → Z\n\n(Reason: clearer)\n"))
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	want := strings.Replace(crlf(crlfCanonical), "### Requirement: A\r\n", "### Requirement: Z\r\n", 1)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestComposeAcceptsCRLFRemovedWithReason(t *testing.T) {
	got, err := Compose(crlf(crlfCanonical), crlf("## REMOVED Requirements\n\n### Requirement: B\n\n(Reason: gone)\n"))
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	want := crlf("## Requirements\n\n### Requirement: A\n\nOld A.\n\n")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestComposeMatchesNamesAcrossMixedLineEndings(t *testing.T) {
	got, err := Compose(crlf(crlfCanonical), "## MODIFIED Requirements\n\n### Requirement: B\n\nNew B.\n")
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	if !strings.Contains(got, "### Requirement: B\n\nNew B.\n") || !strings.Contains(got, "Old A.\r\n") {
		t.Fatalf("got %q", got)
	}
}

func TestComposeIgnoresFencedHeadingsInsideDeltaBody(t *testing.T) {
	delta := "## MODIFIED Requirements\n\n### Requirement: A\n\nNew A, with a sample:\n\n```md\n### Requirement: B\n\n## REMOVED Requirements\n```\n"
	got, err := Compose(crlfCanonical, delta)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	want := "## Requirements\n\n" + delta[len("## MODIFIED Requirements\n\n"):] + "### Requirement: B\n\nOld B.\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
