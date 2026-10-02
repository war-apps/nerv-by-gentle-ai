package config_test

import (
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
)

// fixtureLf mirrors tests/configure.test.ps1's $fixtureLf: the real
// nerv.yaml shape (models, skills, critical_paths, artifacts, git, tasks
// with providers.teamwork including known_projects/sources/sources_howto
// the wizard has no field for), deliberately out of documented key order,
// with trailing comments on several scalar lines.
const fixtureLf = `enabled: true
skills:                             # stacks per consuming role; names must exist in .atl/skill-registry.md
  testing: [tdd, playwright-best-practices]                        # ritsuko, kaworu, maya
  code: [dotnet-best-practices, typescript-best-practices]         # pilots
  best-practices: [best-practices, solid-principles, clean-code-guard]  # balthasar
  architecture: [hexagonal-architecture, c4-architecture]          # melchor
  audit: [security-review, clean-code-guard]                       # kaji passes
models:                             # per-role model and effort; project overrides user, key by key
  misato: { model: fable, effort: high }
  melchor: { from: jd-judge-b }     # inherit gentle-ai's assignment for that phase (state.json)
critical_paths: [auth/, payments/, migrations/, infra/]            # Hyuga auto-critical
artifacts:
  commit: at-close                  # with-change | at-close | never (default: at-close)
git:
  base_branch: develop              # default base for the worktree offer
  worktree: ask                     # ask | always | never
  branch_pattern: "feature/{prefix}-{id}-{slug}"   # prefix comes from the provider (tw)
  commit_ref_pattern: "({PREFIX}-{id})"
tasks:
  provider: teamwork                # teamwork | none ; "ask" when absent
  ask_when_missing: true            # preflight asks task + worktree + branch if no active task
  subtasks_per_wave: false
  timer_store: ~/.claude/work/timers.json
  rounding_minutes: 15
  providers:                        # one block per provider, only the enabled one is required
    teamwork:
      task_ref_prefix: tw           # {prefix} in branch_pattern / commit_ref_pattern
      assignee_id: 686035           # user scope
      default_project_id: 1271726
      default_tasklist_id: 3951970
      stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }
      known_projects:                 # extra Teamwork projects seen before, for quick lookup
        - id: 1271726
          name: ERP Proveedores
          note: "primary project for this team"
        - id: 1300000
          name: Infra
          note: "shared infra tasks, rarely used"
        - id: 1400000
          name: Docs
          note: "documentation backlog, low priority"
        - id: 1500000
          name: Legacy
          note: "read-only archive, do not assign"
  sources:                          # extra work sources for listings (replaces ~/.claude/work/sources.md)
    - name: erp-proveedores
      type: google-sheets
      sheet_id: abc123
    - name: another-source
      type: csv
      path: /data/x.csv
  sources_howto: |
    How to add a new source:
    1. Pick a name.
    2. Pick a type.
    3. Fill in the fields.
`

// fixtureInlineLf mirrors tests/configure.test.ps1's $fixtureInlineLf:
// skills: as a one-line inline map, no artifacts: block, critical_paths:
// already inline. Regression fixture for group H.
const fixtureInlineLf = `enabled: true
skills: { testing: [tdd, playwright-best-practices], code: [dotnet-best-practices, typescript-best-practices], best-practices: [best-practices, solid-principles, clean-code-guard], architecture: [hexagonal-architecture, c4-architecture], audit: [security-review, clean-code-guard] }   # stacks per consuming role
models:                             # per-role model and effort; project overrides user, key by key
  misato: { model: fable, effort: high }
critical_paths: [auth/, payments/, migrations/, infra/]            # Hyuga auto-critical
git:
  base_branch: develop              # default base for the worktree offer
  worktree: ask                     # ask | always | never
  branch_pattern: "feature/{prefix}-{id}-{slug}"   # prefix comes from the provider (tw)
  commit_ref_pattern: "({PREFIX}-{id})"
tasks:
  provider: teamwork                # teamwork | none ; "ask" when absent
  ask_when_missing: true            # preflight asks task + worktree + branch if no active task
  subtasks_per_wave: false
  timer_store: ~/.claude/work/timers.json
  rounding_minutes: 15
  providers:                        # one block per provider, only the enabled one is required
    teamwork:
      task_ref_prefix: tw           # {prefix} in branch_pattern / commit_ref_pattern
      assignee_id: 686035           # user scope
      default_project_id: 1271726
      default_tasklist_id: 3951970
      stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }
`

func hasBareLF(s string) bool {
	b := []byte(s)
	for i, c := range b {
		if c == '\n' && (i == 0 || b[i-1] != '\r') {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Case group A: Set-NervYamlBlock (ported as Document.SetBlock)
// ---------------------------------------------------------------------------

func TestSetBlock_GroupA(t *testing.T) {
	newGitBlock := "git:\n  base_branch: main\n  worktree: always"

	t.Run("block-replace-contains-new_block-replace-drops-old_block-replace-preserves-unrelated_block-replace-ends-with-eol", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		doc.SetBlock("git", newGitBlock)
		got := doc.String()

		if !strings.Contains(got, newGitBlock) {
			t.Errorf("block-replace-contains-new: new block not present:\n%s", got)
		}
		gitBlock, _ := doc.Block("git")
		if strings.Contains(gitBlock, "commit_ref_pattern") {
			t.Errorf("block-replace-drops-old: old git content still present")
		}
		if !strings.Contains(got, "sources_howto: |") || !strings.Contains(got, "known_projects:") {
			t.Errorf("block-replace-preserves-unrelated: unrelated content lost")
		}
		if !strings.HasSuffix(got, "\n") {
			t.Errorf("block-replace-ends-with-eol: missing trailing EOL")
		}
	})

	t.Run("block-append-when-absent", func(t *testing.T) {
		noKeyYaml := "skills: {}\ncritical_paths: [auth/]"
		doc := config.Parse(noKeyYaml)
		doc.SetBlock("git", newGitBlock)
		want := noKeyYaml + "\n\n" + newGitBlock + "\n"
		if got := doc.String(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("block-remove-when-empty_block-remove-preserves-rest", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		doc.SetBlock("artifacts", "")
		got := doc.String()
		for _, line := range strings.Split(got, "\n") {
			if strings.HasPrefix(line, "artifacts:") {
				t.Errorf("block-remove-when-empty: artifacts: block still present")
			}
		}
		if !strings.Contains(got, "git:") || !strings.Contains(got, "tasks:") {
			t.Errorf("block-remove-preserves-rest: unrelated blocks lost")
		}
	})

	t.Run("block-crlf-preserved", func(t *testing.T) {
		fixtureCrlf := strings.ReplaceAll(fixtureLf, "\n", "\r\n")
		doc := config.Parse(fixtureCrlf)
		doc.SetBlock("git", newGitBlock)
		if hasBareLF(doc.String()) {
			t.Errorf("bare LF found in CRLF-only document")
		}
	})
	// block-models-delegates-to-existing-function: ported in models_test.go
	// (TestSetModelsBlock_DelegatesToGenericSetBlock) since it needs
	// config.SetModelsBlock from models.go.
}

// ---------------------------------------------------------------------------
// Case group B: Get-NervYamlBlock (ported as Document.Block)
// ---------------------------------------------------------------------------

func TestBlock_GroupB(t *testing.T) {
	doc := config.Parse(fixtureLf)

	t.Run("get-block-git", func(t *testing.T) {
		gitBlock, found := doc.Block("git")
		if !found || !strings.Contains(gitBlock, "base_branch: develop") || strings.Contains(gitBlock, "tasks:") {
			t.Errorf("unexpected git block: found=%v %q", found, gitBlock)
		}
	})

	tasksBlock, tasksFound := doc.Block("tasks")
	t.Run("get-block-tasks-includes-nested-content", func(t *testing.T) {
		if !tasksFound ||
			!strings.Contains(tasksBlock, "assignee_id: 686035") ||
			!strings.Contains(tasksBlock, "erp-proveedores") ||
			!strings.Contains(tasksBlock, "known_projects:") ||
			!strings.Contains(tasksBlock, "sources_howto: |") {
			t.Errorf("tasks block missing expected nested content: %q", tasksBlock)
		}
	})

	t.Run("get-block-sources-howto-not-top-level", func(t *testing.T) {
		if _, found := doc.Block("sources_howto"); found {
			t.Errorf("sources_howto should not be found as a top-level block")
		}
	})

	t.Run("get-block-missing-returns-null", func(t *testing.T) {
		if _, found := doc.Block("does_not_exist"); found {
			t.Errorf("expected not found")
		}
	})
}

// ---------------------------------------------------------------------------
// Case group C: Read-NervScalar (ported as Document.Scalar)
// ---------------------------------------------------------------------------

func TestScalar_GroupC(t *testing.T) {
	doc := config.Parse(fixtureLf)

	t.Run("scalar-git-base-branch", func(t *testing.T) {
		if v, ok := doc.Scalar("git.base_branch"); !ok || v != "develop" {
			t.Errorf("got %q, %v", v, ok)
		}
	})
	t.Run("scalar-nested-assignee-id", func(t *testing.T) {
		if v, ok := doc.Scalar("tasks.providers.teamwork.assignee_id"); !ok || v != "686035" {
			t.Errorf("got %q, %v", v, ok)
		}
	})
	t.Run("scalar-missing-path", func(t *testing.T) {
		if _, ok := doc.Scalar("tasks.providers.teamwork.nonexistent"); ok {
			t.Errorf("expected not found")
		}
	})
	t.Run("scalar-missing-top-key", func(t *testing.T) {
		if _, ok := doc.Scalar("does.not.exist"); ok {
			t.Errorf("expected not found")
		}
	})
}

// ---------------------------------------------------------------------------
// Case group E: Set-NervYamlScalar (ported as Document.SetScalar)
// ---------------------------------------------------------------------------

func TestSetScalar_GroupE(t *testing.T) {
	t.Run("scalar-set-existing-value-replaced_scalar-set-existing-comment-kept_scalar-set-existing-only-one-line-changed", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		doc.SetScalar("git.base_branch", "develop2")
		got := doc.String()
		if !strings.Contains(got, "  base_branch: develop2") {
			t.Errorf("scalar-set-existing-value-replaced: value not replaced:\n%s", got)
		}
		if !strings.Contains(got, "  base_branch: develop2              # default base for the worktree offer") {
			t.Errorf("scalar-set-existing-comment-kept: comment not preserved:\n%s", got)
		}
		nonEmpty := func(s string) int {
			n := 0
			for _, l := range strings.Split(s, "\n") {
				if l != "" {
					n++
				}
			}
			return n
		}
		if nonEmpty(fixtureLf) != nonEmpty(got) {
			t.Errorf("scalar-set-existing-only-one-line-changed: line count changed")
		}
	})

	t.Run("scalar-set-existing-comment-param-ignored", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		doc.SetScalar("git.base_branch", "develop3", config.WithComment("ignored"))
		got := doc.String()
		if !strings.Contains(got, "# default base for the worktree offer") || strings.Contains(got, "ignored") {
			t.Errorf("existing comment not preserved / new comment leaked: %s", got)
		}
	})

	t.Run("scalar-set-nested-existing-value-replaced_scalar-set-nested-preserves-known-projects_scalar-set-nested-preserves-sources-howto", func(t *testing.T) {
		doc := config.Parse(fixtureLf)
		doc.SetScalar("tasks.providers.teamwork.assignee_id", "999999")
		got := doc.String()
		if !strings.Contains(got, "      assignee_id: 999999           # user scope") {
			t.Errorf("nested value not replaced correctly:\n%s", got)
		}
		if !strings.Contains(got, "ERP Proveedores") || !strings.Contains(got, "Legacy") {
			t.Errorf("known_projects not preserved")
		}
		if !strings.Contains(got, "Pick a name") {
			t.Errorf("sources_howto not preserved")
		}
	})

	minimalWithArtifacts := "artifacts:\n  commit: at-close\ngit:\n  base_branch: develop\n"
	t.Run("scalar-set-missing-leaf-appended_scalar-set-missing-leaf-before-next-top-key_scalar-set-missing-leaf-does-not-disturb-git", func(t *testing.T) {
		doc := config.Parse(minimalWithArtifacts)
		doc.SetScalar("artifacts.new_field", "hello")
		got := doc.String()
		if !strings.Contains(got, "  new_field: hello") {
			t.Errorf("leaf not appended: %s", got)
		}
		if !strings.Contains(got, "commit: at-close\n  new_field: hello\ngit:") {
			t.Errorf("leaf not appended before next top-level key: %s", got)
		}
		if !strings.Contains(got, "  base_branch: develop") {
			t.Errorf("git block disturbed: %s", got)
		}
	})

	minimalTasks := "tasks:\n  provider: teamwork\n"
	t.Run("scalar-set-chain-creates-providers_scalar-set-chain-creates-teamwork_scalar-set-chain-creates-leaf_scalar-set-chain-nested-correctly", func(t *testing.T) {
		doc := config.Parse(minimalTasks)
		doc.SetScalar("tasks.providers.teamwork.new_field", "x")
		got := doc.String()
		if !strings.Contains(got, "  providers:") {
			t.Errorf("providers: not created: %s", got)
		}
		if !strings.Contains(got, "    teamwork:") {
			t.Errorf("teamwork: not created: %s", got)
		}
		if !strings.Contains(got, "      new_field: x") {
			t.Errorf("leaf not created: %s", got)
		}
		// The three Contains checks above already prove correct nesting:
		// each line's indentation (2/4/6 spaces) only matches when
		// providers/teamwork/new_field are nested inside one another in
		// that order.
	})

	minimalGit := "git:\n  base_branch: develop\n"
	t.Run("scalar-set-quotes-special-chars", func(t *testing.T) {
		doc := config.Parse(minimalGit)
		doc.SetScalar("git.new_pattern", "feature/{prefix}")
		if got := doc.String(); !strings.Contains(got, `  new_pattern: "feature/{prefix}"`) {
			t.Errorf("got %s", got)
		}
	})
	t.Run("scalar-set-bare-boolean", func(t *testing.T) {
		doc := config.Parse(minimalGit)
		doc.SetScalar("git.new_flag", "true")
		if got := doc.String(); !strings.Contains(got, "  new_flag: true") {
			t.Errorf("got %s", got)
		}
	})
	t.Run("scalar-set-bare-integer", func(t *testing.T) {
		doc := config.Parse(minimalGit)
		doc.SetScalar("git.new_count", "42")
		if got := doc.String(); !strings.Contains(got, "  new_count: 42") {
			t.Errorf("got %s", got)
		}
	})
	t.Run("scalar-set-bare-null", func(t *testing.T) {
		doc := config.Parse(minimalGit)
		doc.SetScalar("git.new_null", "null")
		if got := doc.String(); !strings.Contains(got, "  new_null: null") {
			t.Errorf("got %s", got)
		}
	})
	t.Run("scalar-set-raw-bypasses-quoting", func(t *testing.T) {
		doc := config.Parse(minimalGit)
		doc.SetScalar("git.new_list", "[a, b]", config.Raw())
		if got := doc.String(); !strings.Contains(got, "  new_list: [a, b]") {
			t.Errorf("got %s", got)
		}
	})
	t.Run("scalar-set-new-line-comment-applied", func(t *testing.T) {
		doc := config.Parse(minimalGit)
		doc.SetScalar("git.new_with_comment", "x", config.WithComment("hello world"))
		if got := doc.String(); !strings.Contains(got, "  new_with_comment: x  # hello world") {
			t.Errorf("got %s", got)
		}
	})
}

// ---------------------------------------------------------------------------
// Case group H (h1 only; h2-h4 are process-level e2e, out of P1a scope):
// inline top-level key detection.
// ---------------------------------------------------------------------------

func TestInlineKeyDetection_GroupH(t *testing.T) {
	doc := config.Parse(fixtureInlineLf)

	t.Run("key-exists-inline-skills", func(t *testing.T) {
		if !doc.KeyExists("skills") {
			t.Errorf("expected skills to exist")
		}
	})
	t.Run("key-is-inline-skills", func(t *testing.T) {
		if !doc.KeyIsInline("skills") {
			t.Errorf("expected skills to be inline")
		}
	})
	t.Run("key-exists-inline-critical-paths", func(t *testing.T) {
		if !doc.KeyExists("critical_paths") {
			t.Errorf("expected critical_paths to exist")
		}
	})
	t.Run("key-is-inline-critical-paths", func(t *testing.T) {
		if !doc.KeyIsInline("critical_paths") {
			t.Errorf("expected critical_paths to be inline")
		}
	})
	t.Run("key-not-exists-artifacts-in-inline-fixture", func(t *testing.T) {
		if doc.KeyExists("artifacts") {
			t.Errorf("expected artifacts to not exist")
		}
	})
	t.Run("key-exists-but-not-inline-git-block-form", func(t *testing.T) {
		if !doc.KeyExists("git") || doc.KeyIsInline("git") {
			t.Errorf("expected git to exist as a block header, not inline")
		}
	})
	t.Run("get-block-returns-null-for-inline-skills", func(t *testing.T) {
		if _, found := doc.Block("skills"); found {
			t.Errorf("expected inline skills to not be returned as a block")
		}
	})
}

// ---------------------------------------------------------------------------
// FormatScalarToken (Format-NervYamlScalarToken) — exercised indirectly by
// group E above through Document.SetScalar; direct unit coverage here.
// ---------------------------------------------------------------------------

func TestFormatScalarToken(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"empty", "", `""`},
		{"null", "null", "null"},
		{"true", "true", "true"},
		{"false", "false", "false"},
		{"bare-int", "42", "42"},
		{"bare-negative-int", "-3", "-3"},
		{"bare-scalar-chars", "feature/x-1.0_a~b", "feature/x-1.0_a~b"},
		{"quoted-spaces", "feature/{prefix}", `"feature/{prefix}"`},
		{"quoted-embedded-quote", `he said "hi"`, `"he said \"hi\""`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := config.FormatScalarToken(tc.value); got != tc.want {
				t.Errorf("FormatScalarToken(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// InlineListField / SetInlineListField (Get-/Set-NervYamlInlineListField) —
// used internally by managed.go's skills handling; not directly PS-tested
// as a standalone case group, so covered here from the docstring contract.
// ---------------------------------------------------------------------------

func TestInlineListField(t *testing.T) {
	inline := `{ testing: [tdd, playwright-best-practices], code: [dotnet-best-practices] }`

	t.Run("found", func(t *testing.T) {
		v, ok := config.InlineListField(inline, "code")
		if !ok || v != "dotnet-best-practices" {
			t.Errorf("got %q, %v", v, ok)
		}
	})
	t.Run("not-found", func(t *testing.T) {
		if _, ok := config.InlineListField(inline, "audit"); ok {
			t.Errorf("expected not found")
		}
	})
	t.Run("empty-input", func(t *testing.T) {
		if _, ok := config.InlineListField("", "code"); ok {
			t.Errorf("expected not found")
		}
	})
}

func TestSetInlineListField(t *testing.T) {
	inline := `{ testing: [tdd], code: [dotnet-best-practices] }`

	t.Run("replaces-existing-field", func(t *testing.T) {
		got := config.SetInlineListField(inline, "code", "dotnet-best-practices, xunit-testing")
		want := `{ testing: [tdd], code: [dotnet-best-practices, xunit-testing] }`
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("appends-missing-field", func(t *testing.T) {
		got := config.SetInlineListField(inline, "audit", "security-review")
		want := `{ testing: [tdd], code: [dotnet-best-practices], audit: [security-review] }`
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
