package release_test

import (
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/release"
)

// ---------------------------------------------------------------------------
// LastReleaseTag — ports the lastreleasetag-* cases from group A. The
// PowerShell version read tags from a real git repo; the pure Go port
// takes the tag list directly (git access itself lives in git.go).
// ---------------------------------------------------------------------------

func TestLastReleaseTag(t *testing.T) {
	t.Run("none-returns-empty", func(t *testing.T) {
		if got := release.LastReleaseTag(nil); got != "" {
			t.Errorf("LastReleaseTag(nil) = %q, want empty", got)
		}
	})

	t.Run("semver-order-not-lexical", func(t *testing.T) {
		tags := []string{"v0.9.0", "v0.10.0"}
		if got := release.LastReleaseTag(tags); got != "v0.10.0" {
			t.Errorf("LastReleaseTag() = %q, want v0.10.0", got)
		}
	})

	t.Run("prerelease-tags-ignored", func(t *testing.T) {
		tags := []string{"v0.9.0", "v0.10.0", "v1.0.0-rc.1"}
		if got := release.LastReleaseTag(tags); got != "v0.10.0" {
			t.Errorf("LastReleaseTag() = %q, want v0.10.0", got)
		}
	})

	t.Run("non-matching-tags-ignored", func(t *testing.T) {
		tags := []string{"not-a-tag", "v1.x", "v1.0.0"}
		if got := release.LastReleaseTag(tags); got != "v1.0.0" {
			t.Errorf("LastReleaseTag() = %q, want v1.0.0", got)
		}
	})
}

// ---------------------------------------------------------------------------
// NextVersion — ports the nextversion-* cases from groups D and D2.
// ---------------------------------------------------------------------------

func TestNextVersion_Bumps(t *testing.T) {
	cases := []struct {
		name string
		cur  string
		bump string
		want string
	}{
		{"major-bump", "1.2.3", "major", "2.0.0"},
		{"minor-bump", "1.2.3", "minor", "1.3.0"},
		{"patch-bump", "1.2.3", "patch", "1.2.4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := release.NextVersion(tc.cur, tc.bump, "", "", nil)
			if err != nil {
				t.Fatalf("NextVersion() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("NextVersion() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNextVersion_NoneBumpIsEmpty(t *testing.T) {
	got, err := release.NextVersion("1.2.3", "none", "", "", nil)
	if err != nil {
		t.Fatalf("NextVersion() error = %v", err)
	}
	if got != "" {
		t.Errorf("NextVersion() = %q, want empty", got)
	}
}

func TestNextVersion_PreRelease(t *testing.T) {
	t.Run("first-number", func(t *testing.T) {
		got, err := release.NextVersion("1.2.3", "minor", "rc", "", nil)
		if err != nil {
			t.Fatalf("NextVersion() error = %v", err)
		}
		if got != "1.3.0-rc.1" {
			t.Errorf("NextVersion() = %q, want 1.3.0-rc.1", got)
		}
	})

	t.Run("next-number-other-labels-do-not-interfere", func(t *testing.T) {
		tags := []string{"v1.3.0-rc.1", "v1.3.0-rc.2", "v1.2.0-rc.5"}
		got, err := release.NextVersion("1.2.3", "minor", "rc", "", tags)
		if err != nil {
			t.Fatalf("NextVersion() error = %v", err)
		}
		if got != "1.3.0-rc.3" {
			t.Errorf("NextVersion() = %q, want 1.3.0-rc.3", got)
		}
	})

	t.Run("base-default-is-next", func(t *testing.T) {
		got, err := release.NextVersion("0.2.0", "minor", "rc", "", nil)
		if err != nil {
			t.Fatalf("NextVersion() error = %v", err)
		}
		if got != "0.3.0-rc.1" {
			t.Errorf("NextVersion() = %q, want 0.3.0-rc.1", got)
		}
	})

	t.Run("base-current-uses-current-as-base", func(t *testing.T) {
		got, err := release.NextVersion("0.2.0", "none", "rc", "current", []string{"v0.1.0"})
		if err != nil {
			t.Fatalf("NextVersion() error = %v", err)
		}
		if got != "0.2.0-rc.1" {
			t.Errorf("NextVersion() = %q, want 0.2.0-rc.1", got)
		}
	})

	t.Run("base-current-numbers-from-existing-rc-tag", func(t *testing.T) {
		got, err := release.NextVersion("0.2.0", "none", "rc", "current", []string{"v0.1.0", "v0.2.0-rc.1"})
		if err != nil {
			t.Fatalf("NextVersion() error = %v", err)
		}
		if got != "0.2.0-rc.2" {
			t.Errorf("NextVersion() = %q, want 0.2.0-rc.2", got)
		}
	})

	t.Run("base-current-other-labels-do-not-interfere", func(t *testing.T) {
		got, err := release.NextVersion("0.2.0", "none", "rc", "current", []string{"v0.2.0-alpha.3"})
		if err != nil {
			t.Fatalf("NextVersion() error = %v", err)
		}
		if got != "0.2.0-rc.1" {
			t.Errorf("NextVersion() = %q, want 0.2.0-rc.1", got)
		}
	})

	t.Run("base-next-other-labels-do-not-interfere", func(t *testing.T) {
		got, err := release.NextVersion("0.1.0", "minor", "rc", "", []string{"v0.2.0-alpha.3"})
		if err != nil {
			t.Fatalf("NextVersion() error = %v", err)
		}
		if got != "0.2.0-rc.1" {
			t.Errorf("NextVersion() = %q, want 0.2.0-rc.1", got)
		}
	})
}

func TestNextVersion_InvalidLabel(t *testing.T) {
	cases := []struct {
		name  string
		label string
	}{
		{"uppercase", "RC"},
		{"digits", "rc1"},
		{"hyphen", "release-candidate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := release.NextVersion("1.0.0", "minor", tc.label, "", nil)
			if err == nil {
				t.Fatalf("NextVersion() error = nil, want an error for invalid label %q", tc.label)
			}
		})
	}
}

func TestNextVersion_InvalidCurrentSemverErrors(t *testing.T) {
	_, err := release.NextVersion("not-a-version", "minor", "", "", nil)
	if err == nil {
		t.Fatal("NextVersion() error = nil, want an error for invalid current version")
	}
}

func TestNextVersion_ErrorMessagesAreLowerNoiseFree(t *testing.T) {
	// Sanity check the error text carries useful context (not a hard
	// contract, just a guard against an empty error string).
	_, err := release.NextVersion("1.0.0", "minor", "RC", "", nil)
	if err == nil || !strings.Contains(err.Error(), "RC") {
		t.Errorf("NextVersion() error = %v, want it to mention the invalid label", err)
	}
}
