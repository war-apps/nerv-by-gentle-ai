package release_test

import (
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/release"
)

// ---------------------------------------------------------------------------
// ChangelogSection — ports the changelogsection-* cases from group F.
// ---------------------------------------------------------------------------

func TestChangelogSection(t *testing.T) {
	commits := []release.Commit{
		c("feat(cli): add --json flag", ""),
		c("fix: handle null path", ""),
		c("docs: update readme", ""),
		c("refactor: simplify parser", ""),
		c("perf: speed up load", ""),
		c("feat!: drop legacy mode", ""),
	}
	section := release.ChangelogSection("1.2.0", "2026-09-28", commits)

	if !strings.HasPrefix(section, "## [1.2.0] - 2026-09-28") {
		t.Fatalf("section does not start with the heading: %q", section)
	}
	breakingIdx := strings.Index(section, "### Breaking")
	addedIdx := strings.Index(section, "### Added")
	changedIdx := strings.Index(section, "### Changed")
	fixedIdx := strings.Index(section, "### Fixed")
	if !(breakingIdx < addedIdx && addedIdx < changedIdx && changedIdx < fixedIdx) {
		t.Errorf("group order wrong: Breaking=%d Added=%d Changed=%d Fixed=%d", breakingIdx, addedIdx, changedIdx, fixedIdx)
	}
	if !strings.Contains(section, "- drop legacy mode (deadbee)") {
		t.Error("missing breaking line")
	}
	if !strings.Contains(section, "- cli: add --json flag (deadbee)") {
		t.Error("missing scoped added line")
	}
	if !strings.Contains(section, "- handle null path (deadbee)") {
		t.Error("missing unscoped fixed line")
	}
	if !strings.Contains(section, "- simplify parser (deadbee)") || !strings.Contains(section, "- speed up load (deadbee)") {
		t.Error("missing changed lines (refactor/perf)")
	}
	if strings.Contains(section, "update readme") {
		t.Error("docs commit should be omitted")
	}

	t.Run("only-empty-groups-omitted-entirely", func(t *testing.T) {
		only := release.ChangelogSection("1.2.1", "2026-09-28", []release.Commit{c("docs: only docs change", "")})
		if !strings.HasPrefix(only, "## [1.2.1] - 2026-09-28") {
			t.Fatalf("section does not start with the heading: %q", only)
		}
		if strings.Contains(only, "###") {
			t.Errorf("expected no group headings, got: %q", only)
		}
	})

	t.Run("nil-commits-no-groups", func(t *testing.T) {
		empty := release.ChangelogSection("2.0.0", "2026-09-28", nil)
		if !strings.HasPrefix(empty, "## [2.0.0] - 2026-09-28") || strings.Contains(empty, "###") {
			t.Errorf("unexpected section for nil commits: %q", empty)
		}
	})
}

// ---------------------------------------------------------------------------
// UpdateChangelog — ports the updatechangelog-* cases from group G.
// content == "" means "the file does not exist yet" (the caller passes the
// empty string when the read failed with not-exist), matching
// Update-NervChangelog's Test-Path branch.
// ---------------------------------------------------------------------------

func TestUpdateChangelog(t *testing.T) {
	section1 := release.ChangelogSection("1.0.0", "2026-09-28", []release.Commit{c("feat: first release feature", "")})

	after1, inserted1, err := release.UpdateChangelog("", section1)
	if err != nil {
		t.Fatalf("UpdateChangelog() error = %v", err)
	}
	if !inserted1 {
		t.Error("first insert should report inserted = true")
	}
	if !strings.Contains(after1, "## [Unreleased]") {
		t.Error("missing synthesized Unreleased heading")
	}
	if strings.Index(after1, "## [Unreleased]") >= strings.Index(after1, "## [1.0.0]") {
		t.Error("section should be inserted after Unreleased")
	}
	if len(after1) > 0 && after1[0] == 0xEF {
		t.Error("output starts with a BOM")
	}

	section2 := release.ChangelogSection("1.1.0", "2026-09-29", []release.Commit{c("feat: second release feature", "")})
	after2, inserted2, err := release.UpdateChangelog(after1, section2)
	if err != nil {
		t.Fatalf("UpdateChangelog() error = %v", err)
	}
	if !inserted2 {
		t.Error("second insert should report inserted = true")
	}
	unreleasedIdx := strings.Index(after2, "## [Unreleased]")
	v110Idx := strings.Index(after2, "## [1.1.0]")
	v100Idx := strings.Index(after2, "## [1.0.0]")
	if !(unreleasedIdx < v110Idx && v110Idx < v100Idx) {
		t.Errorf("newest section should land on top, under Unreleased: %d %d %d", unreleasedIdx, v110Idx, v100Idx)
	}
	if !strings.Contains(after2, "first release feature") {
		t.Error("older section content should be preserved")
	}

	t.Run("duplicate-version-is-a-no-op", func(t *testing.T) {
		after3, inserted3, err := release.UpdateChangelog(after2, section2)
		if err != nil {
			t.Fatalf("UpdateChangelog() error = %v", err)
		}
		if inserted3 {
			t.Error("duplicate insert should report inserted = false")
		}
		if after3 != after2 {
			t.Error("duplicate insert must not change the content")
		}
	})

	t.Run("existing-content-without-unreleased-heading-errors", func(t *testing.T) {
		_, _, err := release.UpdateChangelog("# Changelog\n\nSome unrelated content.\n", section1)
		if err == nil {
			t.Fatal("expected an error when no Unreleased heading is found")
		}
	})

	t.Run("malformed-section-errors", func(t *testing.T) {
		_, _, err := release.UpdateChangelog(after2, "not a heading at all")
		if err == nil {
			t.Fatal("expected an error for a section without a '## [X.Y.Z]' heading")
		}
	})
}

// ---------------------------------------------------------------------------
// SectionBody — ports the sectionbody-* cases from release-guard's group A.
// ---------------------------------------------------------------------------

func fullChangelogText() string {
	return "# Changelog\n\n" +
		"All notable changes to this project will be documented in this file.\n\n" +
		"The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),\n" +
		"and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).\n\n" +
		"## [Unreleased]\n\n" +
		"## [0.2.0] - 2026-09-28\n\n" +
		"### Added\n" +
		"- cli: add --json flag (abc1234)\n\n" +
		"### Fixed\n" +
		"- handle null path (def5678)\n\n" +
		"## [0.1.0] - 2026-09-01\n\n" +
		"### Added\n" +
		"- initial release (0000001)\n"
}

func TestSectionBody(t *testing.T) {
	text := fullChangelogText()

	t.Run("middle-section", func(t *testing.T) {
		body, ok := release.SectionBody(text, "0.2.0")
		if !ok {
			t.Fatal("expected the section to be found")
		}
		if !strings.HasPrefix(body, "### Added") {
			t.Errorf("body = %q, want it to start with ### Added", body)
		}
		if !strings.Contains(body, "cli: add --json flag (abc1234)") || !strings.Contains(body, "handle null path (def5678)") {
			t.Error("body missing expected lines")
		}
		if strings.Contains(body, "initial release") {
			t.Error("body should not leak the next section")
		}
		if strings.HasPrefix(body, "\n") || strings.HasSuffix(body, "\n") {
			t.Errorf("body has leading/trailing blank lines: %q", body)
		}
	})

	t.Run("last-section-to-eof", func(t *testing.T) {
		body, ok := release.SectionBody(text, "0.1.0")
		if !ok {
			t.Fatal("expected the section to be found")
		}
		if !strings.HasPrefix(body, "### Added") || !strings.Contains(body, "initial release (0000001)") {
			t.Errorf("body = %q", body)
		}
		if strings.HasSuffix(body, "\n") {
			t.Error("body should not have a trailing newline")
		}
	})

	t.Run("missing-version-returns-not-found", func(t *testing.T) {
		_, ok := release.SectionBody(text, "9.9.9")
		if ok {
			t.Error("expected not found for a missing version")
		}
	})
}
