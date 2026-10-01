package config_test

import (
	"strings"
	"testing"

	"github.com/war-apps/nerv-gentle-ai/internal/config"
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
		Provider:   "teamwork",
		ProjectID:  "111",
		TasklistID: "222",
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
	t.Run("format-project-provider", func(t *testing.T) {
		if !strings.Contains(got, "provider: teamwork") {
			t.Errorf("got %q", got)
		}
	})
	t.Run("format-project-project-id", func(t *testing.T) {
		if !strings.Contains(got, "project_id: 111") {
			t.Errorf("got %q", got)
		}
	})
	t.Run("format-project-tasklist-id", func(t *testing.T) {
		if !strings.Contains(got, "tasklist_id: 222") {
			t.Errorf("got %q", got)
		}
	})
	t.Run("format-project-models-hint", func(t *testing.T) {
		if !strings.Contains(got, "# models:") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("format-project-minimal-enabled-only", func(t *testing.T) {
		minimal := config.FormatProjectFile(config.ProjectValues{})
		if !strings.HasPrefix(minimal, "enabled: true") || strings.Contains(minimal, "project_id") {
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
		if len(defaults) != 28 {
			t.Errorf("got %d managed keys, want 28", len(defaults))
		}
	})

	wantSample := map[string]string{
		"git.base_branch":                              "develop",
		"git.worktree":                                 "ask",
		"git.worktree_pattern":                         ".claude/worktrees/{slug}",
		"git.branch_pattern":                           "feature/{prefix}-{id}-{slug}",
		"git.commit_ref_pattern":                       "({PREFIX}-{id})",
		"tasks.provider":                               "teamwork",
		"tasks.ask_when_missing":                       "true",
		"tasks.subtasks_per_wave":                      "false",
		"tasks.timer_store":                            "~/.claude/work/timers.json",
		"tasks.rounding_minutes":                       "15",
		"tasks.providers.teamwork.task_ref_prefix":     "tw",
		"tasks.providers.teamwork.assignee_id":         "",
		"tasks.providers.teamwork.default_project_id":  "",
		"tasks.providers.teamwork.default_tasklist_id": "",
		"tasks.providers.teamwork.stages.inDev":        "DESARROLLO",
		"tasks.providers.teamwork.stages.testing":      "TESTING",
		"tasks.providers.teamwork.stages.implemented":  "IMPLEMENTA",
		"tasks.providers.teamwork.stages.blocked":      "BLOQUEA",
		"tasks.providers.teamwork.stages.canceled":     "CANCEL",
		"tasks.providers.teamwork.stages.pending":      "PENDIENTE",
		"tasks.providers.teamwork.stages.analysis":     "ANALISIS",
		"skills.testing":                               "tdd, playwright-best-practices",
		"skills.code":                                  "dotnet-best-practices, typescript-best-practices",
		"skills.best-practices":                        "best-practices, solid-principles, clean-code-guard",
		"skills.architecture":                          "hexagonal-architecture, c4-architecture",
		"skills.audit":                                 "security-review, clean-code-guard",
		"critical_paths":                               "auth/, payments/, migrations/, infra/",
		"artifacts.commit":                             "at-close",
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
		{"tasks.provider", []string{"teamwork", "github-projects", "jira", "none"}},
		{"tasks.ask_when_missing", []string{"true", "false"}},
		{"tasks.subtasks_per_wave", []string{"true", "false"}},
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
