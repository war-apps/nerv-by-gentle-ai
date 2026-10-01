package release_test

import (
	"reflect"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/release"
)

func c(subject, body string) release.Commit {
	return release.Commit{Sha: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", ShortSha: "deadbee", Subject: subject, Body: body}
}

// ---------------------------------------------------------------------------
// ConventionalInfo — ports Get-NervConventionalCommitInfo.
// ---------------------------------------------------------------------------

func TestConventionalInfo(t *testing.T) {
	cases := []struct {
		name    string
		subject string
		body    string
		want    release.Info
	}{
		{"plain-feat", "feat: add thing", "", release.Info{Conventional: true, Type: "feat", Description: "add thing"}},
		{"scoped-fix", "fix(parser): handle null", "", release.Info{Conventional: true, Type: "fix", Scope: "parser", Description: "handle null"}},
		{"bang-breaking", "feat!: drop old api", "", release.Info{Conventional: true, Type: "feat", Breaking: true, Description: "drop old api"}},
		{"scope-bang-breaking", "fix(parser)!: change contract", "", release.Info{Conventional: true, Type: "fix", Scope: "parser", Breaking: true, Description: "change contract"}},
		{"breaking-change-footer", "fix: adjust parsing", "Body text.\n\nBREAKING CHANGE: input format changed", release.Info{Conventional: true, Type: "fix", Breaking: true, Description: "adjust parsing"}},
		{"type-lowercased", "Feat: Add Thing", "", release.Info{Conventional: true, Type: "feat", Description: "Add Thing"}},
		{"non-conventional", "update the readme quickly", "", release.Info{Description: "update the readme quickly"}},
		{"empty-subject", "", "", release.Info{Description: ""}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := release.ConventionalInfo(tc.subject, tc.body)
			if got != tc.want {
				t.Errorf("ConventionalInfo(%q, %q) = %+v, want %+v", tc.subject, tc.body, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BumpKind — ports the 15 bumpkind-* cases from tests/release.test.ps1
// group C.
// ---------------------------------------------------------------------------

func TestBumpKind(t *testing.T) {
	cases := []struct {
		name    string
		commits []release.Commit
		want    string
	}{
		{"feat-is-minor", []release.Commit{c("feat: add thing", "")}, "minor"},
		{"fix-is-patch", []release.Commit{c("fix: fix thing", "")}, "patch"},
		{"perf-is-patch", []release.Commit{c("perf: speed up thing", "")}, "patch"},
		{"feat-bang-is-major", []release.Commit{c("feat!: drop old api", "")}, "major"},
		{"fix-scope-bang-is-major", []release.Commit{c("fix(parser)!: change contract", "")}, "major"},
		{"breaking-change-footer-is-major", []release.Commit{c("fix: adjust parsing", "Body text.\n\nBREAKING CHANGE: input format changed")}, "major"},
		{"docs-is-none", []release.Commit{c("docs: some change", "")}, "none"},
		{"chore-is-none", []release.Commit{c("chore: some change", "")}, "none"},
		{"test-is-none", []release.Commit{c("test: some change", "")}, "none"},
		{"refactor-is-none", []release.Commit{c("refactor: some change", "")}, "none"},
		{"ci-is-none", []release.Commit{c("ci: some change", "")}, "none"},
		{"build-is-none", []release.Commit{c("build: some change", "")}, "none"},
		{"style-is-none", []release.Commit{c("style: some change", "")}, "none"},
		{"revert-is-none", []release.Commit{c("revert: some change", "")}, "none"},
		{"non-conventional-is-none", []release.Commit{c("update the readme quickly", "")}, "none"},
		{"empty-commits-is-none", nil, "none"},
		{"highest-wins-feat-and-fix", []release.Commit{c("feat: a", ""), c("fix: b", "")}, "minor"},
		{"highest-wins-feat-and-breaking", []release.Commit{c("feat: a", ""), c("feat!: b", "")}, "major"},
		{"highest-wins-docs-and-fix", []release.Commit{c("docs: a", ""), c("fix: b", "")}, "patch"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := release.BumpKind(tc.commits); got != tc.want {
				t.Errorf("BumpKind() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ParseCommits — the record/field splitting half of Get-NervCommitsSince.
// ---------------------------------------------------------------------------

func TestParseCommits(t *testing.T) {
	t.Run("empty-input-is-empty", func(t *testing.T) {
		if got := release.ParseCommits(""); len(got) != 0 {
			t.Errorf("ParseCommits(\"\") = %+v, want empty", got)
		}
	})

	t.Run("single-record", func(t *testing.T) {
		raw := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x1faaaaaaa\x1ffeat: first\x1f\x1e"
		want := []release.Commit{{Sha: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ShortSha: "aaaaaaa", Subject: "feat: first", Body: ""}}
		got := release.ParseCommits(raw)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ParseCommits() = %+v, want %+v", got, want)
		}
	})

	t.Run("multiple-records-oldest-first-order-preserved", func(t *testing.T) {
		raw := "" +
			"1111111111111111111111111111111111111111\x1f1111111\x1ffeat: first\x1f\x1e\n" +
			"2222222222222222222222222222222222222222\x1f2222222\x1ffix: second\x1fBody line 1\nBody line 2\x1e\n"
		got := release.ParseCommits(raw)
		if len(got) != 2 {
			t.Fatalf("ParseCommits() returned %d commits, want 2 (%+v)", len(got), got)
		}
		if got[0].Subject != "feat: first" || got[1].Subject != "fix: second" {
			t.Errorf("order not preserved: %+v", got)
		}
		if got[1].Body != "Body line 1\nBody line 2" {
			t.Errorf("Body = %q, want %q", got[1].Body, "Body line 1\nBody line 2")
		}
	})
}
