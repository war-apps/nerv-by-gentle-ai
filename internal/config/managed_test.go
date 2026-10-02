package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
)

// ---------------------------------------------------------------------------
// ParseSetArg (ConvertFrom-NervSetArg). No direct PS unit-test case group
// exists for this function — tests/configure.test.ps1 only exercises it
// indirectly through the -Set/-SetModel e2e cases (group J, a real child
// process, out of P1a scope). Coverage below is derived directly from the
// function's own PowerShell docstring/implementation.
// ---------------------------------------------------------------------------

func TestParseSetArg(t *testing.T) {
	cases := []struct {
		name      string
		arg       string
		wantKey   string
		wantValue string
		wantOK    bool
	}{
		{"simple", "git.worktree=always", "git.worktree", "always", true},
		{"value-contains-equals", "git.branch_pattern=feature/{prefix}={id}", "git.branch_pattern", "feature/{prefix}={id}", true},
		{"empty-value-allowed", "tasks.providers.teamwork.assignee_id=", "tasks.providers.teamwork.assignee_id", "", true},
		{"no-equals-sign", "gitworktree", "", "", false},
		{"equals-at-index-zero-empty-key", "=value", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, value, ok := config.ParseSetArg(tc.arg)
			if ok != tc.wantOK || key != tc.wantKey || value != tc.wantValue {
				t.Errorf("ParseSetArg(%q) = %q, %q, %v; want %q, %q, %v",
					tc.arg, key, value, ok, tc.wantKey, tc.wantValue, tc.wantOK)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ManagedValue / SetManagedValue (Get-/Set-NervManagedConfigValue). Same
// caveat: no direct PS pure-function case group — only exercised through
// the -Set e2e cases in group J. Coverage below follows the documented
// per-key-shape behavior (scalar, teamwork stages, skills block/inline,
// critical_paths) and the exact validation error wording from
// configure.ps1's -Set handler.
// ---------------------------------------------------------------------------

func TestManagedValue(t *testing.T) {
	t.Run("existing-scalar", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		got, ok := config.ManagedValue(doc, "git.base_branch")
		if !ok || got != "develop" {
			t.Errorf("got %q, %v", got, ok)
		}
	})

	t.Run("missing-key-falls-back-to-default", func(t *testing.T) {
		doc := config.Parse("enabled: true\n")
		got, ok := config.ManagedValue(doc, "git.base_branch")
		if !ok || got != "develop" {
			t.Errorf("got %q, %v", got, ok)
		}
	})

	t.Run("teamwork-stage", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		got, ok := config.ManagedValue(doc, "tasks.providers.teamwork.stages.blocked")
		if !ok || got != "BLOQUEA" {
			t.Errorf("got %q, %v", got, ok)
		}
	})

	t.Run("teamwork-stage-falls-back-when-stages-absent", func(t *testing.T) {
		doc := config.Parse("enabled: true\n")
		got, ok := config.ManagedValue(doc, "tasks.providers.teamwork.stages.blocked")
		if !ok || got != "BLOQUEA" {
			t.Errorf("got %q, %v", got, ok)
		}
	})

	t.Run("skills-category-block-form", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		got, ok := config.ManagedValue(doc, "skills.code")
		if !ok || got != "dotnet-best-practices, typescript-best-practices" {
			t.Errorf("got %q, %v", got, ok)
		}
	})

	t.Run("skills-category-inline-form", func(t *testing.T) {
		doc := config.Parse(fixtureInlineLf)
		got, ok := config.ManagedValue(doc, "skills.code")
		if !ok || got != "dotnet-best-practices, typescript-best-practices" {
			t.Errorf("got %q, %v", got, ok)
		}
	})

	t.Run("critical-paths", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		got, ok := config.ManagedValue(doc, "critical_paths")
		if !ok || got != "auth/, payments/, migrations/, infra/" {
			t.Errorf("got %q, %v", got, ok)
		}
	})

	t.Run("worktree-pattern-falls-back-to-default-when-absent", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		got, ok := config.ManagedValue(doc, "git.worktree_pattern")
		if !ok || got != ".claude/worktrees/{slug}" {
			t.Errorf("got %q, %v", got, ok)
		}
	})

	t.Run("unknown-key", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		if _, ok := config.ManagedValue(doc, "does.not.exist"); ok {
			t.Errorf("expected not found")
		}
	})
}

func TestSetManagedValue(t *testing.T) {
	t.Run("changes-existing-scalar", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		changed, err := config.SetManagedValue(doc, "git.base_branch", "main")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !changed {
			t.Errorf("expected changed=true")
		}
		if got, _ := config.ManagedValue(doc, "git.base_branch"); got != "main" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("no-op-when-value-unchanged", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		changed, err := config.SetManagedValue(doc, "git.base_branch", "develop")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if changed {
			t.Errorf("expected changed=false")
		}
	})

	t.Run("unknown-key-rejected-with-managed-keys-listed", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		_, err := config.SetManagedValue(doc, "does.not.exist", "x")
		if err == nil {
			t.Fatalf("expected error")
		}
		want := "Unknown config key 'does.not.exist'. Managed keys: " + strings.Join(config.ManagedKeys(), ", ") + "."
		if err.Error() != want {
			t.Errorf("got %q, want %q", err.Error(), want)
		}
		var unknownKeyErr *config.ErrUnknownKey
		if !errors.As(err, &unknownKeyErr) {
			t.Errorf("expected *config.ErrUnknownKey, got %T", err)
		}
	})

	t.Run("invalid-value-rejected-with-allowed-values-listed", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		_, err := config.SetManagedValue(doc, "git.worktree", "sometimes")
		if err == nil {
			t.Fatalf("expected error")
		}
		want := "Invalid value 'sometimes' for key 'git.worktree'. Allowed: ask, always, never."
		if err.Error() != want {
			t.Errorf("got %q, want %q", err.Error(), want)
		}
		var invalidValueErr *config.ErrInvalidValue
		if !errors.As(err, &invalidValueErr) {
			t.Errorf("expected *config.ErrInvalidValue, got %T", err)
		}
	})

	t.Run("rejected-set-does-not-mutate-document", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		before := doc.String()
		_, _ = config.SetManagedValue(doc, "git.worktree", "sometimes")
		if doc.String() != before {
			t.Errorf("document mutated despite validation error")
		}
	})

	t.Run("sets-teamwork-stage-rewriting-whole-stages-map", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		changed, err := config.SetManagedValue(doc, "tasks.providers.teamwork.stages.blocked", "BLOCKED")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !changed {
			t.Errorf("expected changed=true")
		}
		got := doc.String()
		if !strings.Contains(got, "blocked: BLOCKED") {
			t.Errorf("stage not updated: %s", got)
		}
		// every other stage's current value must survive untouched.
		if !strings.Contains(got, "inDev: DESARROLLO") || !strings.Contains(got, "analysis: ANALISIS") {
			t.Errorf("other stages lost: %s", got)
		}
	})

	t.Run("sets-skills-category-inline-form-without-duplicating-key", func(t *testing.T) {
		doc := config.Parse(fixtureInlineLf)
		changed, err := config.SetManagedValue(doc, "skills.code", "dotnet-best-practices, xunit-testing")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !changed {
			t.Errorf("expected changed=true")
		}
		got := doc.String()
		if strings.Count(got, "skills:") != 1 {
			t.Errorf("expected exactly one skills: key, got %d: %s", strings.Count(got, "skills:"), got)
		}
		if !strings.Contains(got, "xunit-testing") {
			t.Errorf("new value not present: %s", got)
		}
		if !strings.Contains(got, "testing: [tdd, playwright-best-practices]") {
			t.Errorf("other categories lost: %s", got)
		}
	})

	t.Run("sets-skills-category-creating-block-when-absent", func(t *testing.T) {
		doc := config.Parse("enabled: true\n")
		changed, err := config.SetManagedValue(doc, "skills.audit", "security-review")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !changed {
			t.Errorf("expected changed=true")
		}
		got := doc.String()
		if !strings.Contains(got, "audit: [security-review]") {
			t.Errorf("audit category not set: %s", got)
		}
		// unset categories must fall back to their catalogue defaults.
		if !strings.Contains(got, "testing: [tdd, playwright-best-practices]") {
			t.Errorf("unset categories missing default: %s", got)
		}
	})

	t.Run("sets-worktree-pattern-creating-key-next-to-worktree-when-missing", func(t *testing.T) {
		doc := config.Parse("enabled: true\ngit:\n  worktree: ask\n")
		changed, err := config.SetManagedValue(doc, "git.worktree_pattern", ".claude/worktrees/{branch}")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !changed {
			t.Errorf("expected changed=true")
		}
		got := doc.String()
		want := "enabled: true\ngit:\n  worktree: ask\n  worktree_pattern: \".claude/worktrees/{branch}\"\n"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("rejects-empty-worktree-pattern", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		_, err := config.SetManagedValue(doc, "git.worktree_pattern", "")
		if err == nil {
			t.Fatalf("expected error")
		}
		want := "Invalid value '' for key 'git.worktree_pattern': must not be empty."
		if err.Error() != want {
			t.Errorf("got %q, want %q", err.Error(), want)
		}
		var invalidPatternErr *config.ErrInvalidWorktreePattern
		if !errors.As(err, &invalidPatternErr) {
			t.Errorf("expected *config.ErrInvalidWorktreePattern, got %T", err)
		}
	})

	t.Run("rejects-worktree-pattern-with-dotdot-segment", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		_, err := config.SetManagedValue(doc, "git.worktree_pattern", "../{slug}")
		if err == nil {
			t.Fatalf("expected error")
		}
		want := "Invalid value '../{slug}' for key 'git.worktree_pattern': must not contain '..' path segments."
		if err.Error() != want {
			t.Errorf("got %q, want %q", err.Error(), want)
		}
		var invalidPatternErr *config.ErrInvalidWorktreePattern
		if !errors.As(err, &invalidPatternErr) {
			t.Errorf("expected *config.ErrInvalidWorktreePattern, got %T", err)
		}
	})

	t.Run("sets-critical-paths-trims-and-joins", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		changed, err := config.SetManagedValue(doc, "critical_paths", " auth/ , payments/ ,,infra/ ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !changed {
			t.Errorf("expected changed=true")
		}
		got, _ := config.ManagedValue(doc, "critical_paths")
		if got != "auth/, payments/, infra/" {
			t.Errorf("got %q", got)
		}
	})
}

// ---------------------------------------------------------------------------
// Legacy provider sub-blocks (github-projects, jira) left in tasks.providers by
// releases before their removal: the document layer edits only the key it is
// asked to and carries them through, including a write that inserts a missing
// teamwork key right next to them. Removing them is the write layer's job
// (StripRemovedTaskProviders, applied by configstore.Store.Save).
// ---------------------------------------------------------------------------

const legacyProviderBlocks = "" +
	"    github-projects: { task_ref_prefix: gh, owner: \"\", project_number: 0 }    # later\n" +
	"    jira: { task_ref_prefix: jira, site: \"\", project_key: \"\" }               # later\n"

const legacyProvidersDoc = "" +
	"tasks:\n" +
	"  provider: jira                    # was a stub provider\n" +
	"  providers:\n" +
	"    teamwork:\n" +
	"      task_ref_prefix: tw\n" +
	"      stages: { inDev: DESARROLLO, blocked: BLOQUEA }\n" +
	legacyProviderBlocks +
	"git:\n" +
	"  base_branch: develop\n"

func TestSetManagedValue_LegacyProviderBlocks_Preserved(t *testing.T) {
	doc := config.Parse(legacyProvidersDoc)

	for _, kv := range [][2]string{
		{"tasks.providers.teamwork.stages.blocked", "BLOCKED"},
		{"tasks.providers.teamwork.assignee_id", "7"}, // missing key: inserted into the teamwork block
		{"git.base_branch", "main"},
	} {
		if _, err := config.SetManagedValue(doc, kv[0], kv[1]); err != nil {
			t.Fatalf("SetManagedValue(%s) error = %v", kv[0], err)
		}
	}

	got := doc.String()
	if !strings.Contains(got, legacyProviderBlocks) {
		t.Errorf("legacy provider blocks changed:\n%s", got)
	}
	if !strings.Contains(got, "  provider: jira                    # was a stub provider\n") {
		t.Errorf("legacy tasks.provider line changed:\n%s", got)
	}
	for _, want := range []string{"blocked: BLOCKED", "assignee_id: 7", "base_branch: main"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if v, ok := config.ManagedValue(doc, "tasks.provider"); !ok || v != "jira" {
		t.Errorf("ManagedValue(tasks.provider) = %q, %v; want jira, true", v, ok)
	}
}
