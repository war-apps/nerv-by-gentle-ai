package release_test

import (
	"strings"
	"testing"

	"github.com/war-apps/nerv-gentle-ai/internal/release"
)

// ---------------------------------------------------------------------------
// Readiness — ports Test-NervReleaseReadiness (group B of
// tests/release-guard.test.ps1). Git access (tag existence) and file
// existence are resolved by the caller; Readiness takes the resulting
// tags/changelog inputs directly.
// ---------------------------------------------------------------------------

func TestReadiness_Ok(t *testing.T) {
	text := fullChangelogText()
	got := release.Readiness("0.2.0", nil, true, text)

	if !got.Ok {
		t.Errorf("Ok = false, want true (reason: %q)", got.Reason)
	}
	if got.Version != "0.2.0" {
		t.Errorf("Version = %q, want 0.2.0", got.Version)
	}
	if got.Tag != "v0.2.0" {
		t.Errorf("Tag = %q, want v0.2.0", got.Tag)
	}
	if got.TagExists {
		t.Error("TagExists = true, want false")
	}
	if !got.ChangelogSection {
		t.Error("ChangelogSection = false, want true")
	}
	if got.Body == "" {
		t.Error("Body is empty, want the section body")
	}
}

func TestReadiness_TagAlreadyExists(t *testing.T) {
	text := fullChangelogText()
	got := release.Readiness("0.2.0", []string{"v0.2.0"}, true, text)

	if got.Ok {
		t.Error("Ok = true, want false")
	}
	if !got.TagExists {
		t.Error("TagExists = false, want true")
	}
	if !strings.Contains(got.Reason, "v0.2.0") {
		t.Errorf("Reason = %q, want it to mention v0.2.0", got.Reason)
	}
}

func TestReadiness_MissingChangelog(t *testing.T) {
	got := release.Readiness("0.2.0", nil, false, "")

	if got.Ok {
		t.Error("Ok = true, want false")
	}
	if got.ChangelogSection {
		t.Error("ChangelogSection = true, want false")
	}
	if !strings.Contains(got.Reason, "CHANGELOG.md") {
		t.Errorf("Reason = %q, want it to mention CHANGELOG.md", got.Reason)
	}
}

func TestReadiness_NoMatchingSection(t *testing.T) {
	text := fullChangelogText()
	got := release.Readiness("0.3.0", nil, true, text)

	if got.Ok {
		t.Error("Ok = true, want false")
	}
	if got.ChangelogSection {
		t.Error("ChangelogSection = true, want false")
	}
	if got.TagExists {
		t.Error("TagExists = true, want false")
	}
	if !strings.Contains(got.Reason, "0.3.0") {
		t.Errorf("Reason = %q, want it to mention 0.3.0", got.Reason)
	}
}
