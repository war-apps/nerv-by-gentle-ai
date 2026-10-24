package config_test

import (
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
)

// ---------------------------------------------------------------------------
// Case group D: Format-* functions (Format-NervSkillsBlock,
// Format-NervCriticalPathsLine, Format-NervProjectFile)
// ---------------------------------------------------------------------------

func TestFormatSkillsBlock_GroupD(t *testing.T) {
	got := config.FormatSkillsBlock(config.SkillsValues{
		Testing:       "tdd, playwright-best-practices",
		Code:          "dotnet-best-practices, typescript-best-practices",
		BestPractices: "best-practices, solid-principles, clean-code-guard",
		Architecture:  "hexagonal-architecture, c4-architecture",
		Audit:         "security-review, clean-code-guard",
	})

	t.Run("format-skills-header", func(t *testing.T) {
		if !strings.HasPrefix(got, "skills:") {
			t.Errorf("got %q", got)
		}
	})
	t.Run("format-skills-testing", func(t *testing.T) {
		if !strings.Contains(got, "[tdd, playwright-best-practices]") {
			t.Errorf("got %q", got)
		}
	})
	t.Run("format-skills-audit", func(t *testing.T) {
		if !strings.Contains(got, "audit: [security-review, clean-code-guard]") {
			t.Errorf("got %q", got)
		}
	})
}

func TestFormatCriticalPathsLine_GroupD(t *testing.T) {
	t.Run("format-critical-paths", func(t *testing.T) {
		got := config.FormatCriticalPathsLine([]string{"auth/", "payments/", "migrations/", "infra/"})
		if !strings.HasPrefix(got, "critical_paths: [auth/, payments/, migrations/, infra/]") {
			t.Errorf("got %q", got)
		}
	})
}

func TestFormatProjectFile_GroupD(t *testing.T) {
	got := config.FormatProjectFile(config.ProjectValues{
		BaseBranch: "develop2",
	})

	t.Run("format-project-enabled", func(t *testing.T) {
		if !strings.HasPrefix(got, "enabled: true") {
			t.Errorf("got %q", got)
		}
	})
	t.Run("format-project-base-branch", func(t *testing.T) {
		if !strings.Contains(got, "base_branch: develop2") {
			t.Errorf("got %q", got)
		}
	})
	t.Run("format-project-no-tasks-block", func(t *testing.T) {
		if strings.Contains(got, "tasks:") {
			t.Errorf("core wrote a tasks: block: %q", got)
		}
	})
	t.Run("format-project-models-hint", func(t *testing.T) {
		if !strings.Contains(got, "# models:") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("format-project-minimal-enabled-only", func(t *testing.T) {
		minimal := config.FormatProjectFile(config.ProjectValues{})
		if !strings.HasPrefix(minimal, "enabled: true") || strings.Contains(minimal, "git:") {
			t.Errorf("got %q", minimal)
		}
	})
}

// ---------------------------------------------------------------------------
// Managed key catalogue: Defaults()/DefaultValue()/ManagedKeys() (from
// Get-NervConfigDefaults) and AllowedValues() (from
// Get-NervConfigAllowedValues). Not their own PS case group — exercised
// only through configure.ps1's non-interactive -Set e2e cases (group J,
// out of P1a scope) — so covered here directly from the catalogue's own
// documented contract.
// ---------------------------------------------------------------------------

func TestDefaults(t *testing.T) {
	defaults := config.Defaults()

	t.Run("has-28-managed-keys", func(t *testing.T) {
		if len(defaults) != 12 {
			t.Errorf("got %d managed keys, want 12", len(defaults))
		}
	})

	wantSample := map[string]string{
		"git.base_branch":        "develop",
		"git.worktree":           "ask",
		"git.worktree_pattern":   ".claude/worktrees/{slug}",
		"git.branch_pattern":     "feature/{prefix}-{id}-{slug}",
		"git.commit_ref_pattern": "({PREFIX}-{id})",
		"skills.testing":         "tdd, playwright-best-practices",
		"skills.code":            "dotnet-best-practices, typescript-best-practices",
		"skills.best-practices":  "best-practices, solid-principles, clean-code-guard",
		"skills.architecture":    "hexagonal-architecture, c4-architecture",
		"skills.audit":           "security-review, clean-code-guard",
		"critical_paths":         "auth/, payments/, migrations/, infra/",
		"artifacts.commit":       "at-close",
	}
	for key, want := range wantSample {
		t.Run("default-"+key, func(t *testing.T) {
			got, ok := config.DefaultValue(key)
			if !ok || got != want {
				t.Errorf("DefaultValue(%q) = %q, %v; want %q", key, got, ok, want)
			}
		})
	}

	t.Run("unknown-key", func(t *testing.T) {
		if _, ok := config.DefaultValue("does.not.exist"); ok {
			t.Errorf("expected not found")
		}
	})

	t.Run("managed-keys-order-matches-defaults", func(t *testing.T) {
		keys := config.ManagedKeys()
		if len(keys) != len(defaults) {
			t.Fatalf("got %d keys, want %d", len(keys), len(defaults))
		}
		for i, kd := range defaults {
			if keys[i] != kd.Key {
				t.Errorf("index %d: got %q, want %q", i, keys[i], kd.Key)
			}
		}
	})
}

func TestAllowedValues(t *testing.T) {
	cases := []struct {
		key  string
		want []string
	}{
		{"git.worktree", []string{"ask", "always", "never"}},
		{"artifacts.commit", []string{"with-change", "at-close", "never"}},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			got, ok := config.AllowedValues(tc.key)
			if !ok || strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, %v; want %v", got, ok, tc.want)
			}
		})
	}

	t.Run("unenumerated-key-has-no-allowed-values", func(t *testing.T) {
		if _, ok := config.AllowedValues("git.base_branch"); ok {
			t.Errorf("expected git.base_branch to have no enumerated allowed values")
		}
	})
}

// S1: core manages no tasks.* key; a tasks.* key is unknown to it.
func TestDefaults_HoldNoTaskKeys(t *testing.T) {
	for _, key := range config.ManagedKeys() {
		if strings.HasPrefix(key, "tasks.") {
			t.Errorf("managed key %q belongs to the task plugin", key)
		}
	}
	if err := config.ValidateManagedKey("tasks.provider"); err == nil {
		t.Error("ValidateManagedKey(tasks.provider) = nil, want an unknown-key error")
	}
}
