package configure_test

// richFixtureLF mirrors tests/configure.test.ps1's $fixtureLf: models,
// skills, critical_paths, artifacts, git, and tasks: containing
// providers.teamwork (task_ref_prefix, assignee_id, default_project_id,
// default_tasklist_id, stages, and a known_projects: list the wizard has no
// field for), a sources: list, and a
// sources_howto: literal block nested under tasks: — plus trailing comments
// on several scalar lines. Deliberately out of the documented key order, to
// exercise the scan-based (not order-dependent) block functions. Group G's
// "unknown content survives byte for byte" regression.
const richFixtureLF = "" +
	"enabled: true\n" +
	"skills:                             # stacks per consuming role; names must exist in .atl/skill-registry.md\n" +
	"  testing: [tdd, playwright-best-practices]                        # ritsuko, kaworu, maya\n" +
	"  code: [dotnet-best-practices, typescript-best-practices]         # pilots\n" +
	"  best-practices: [best-practices, solid-principles, clean-code-guard]  # balthasar\n" +
	"  architecture: [hexagonal-architecture, c4-architecture]          # melchor\n" +
	"  audit: [security-review, clean-code-guard]                       # kaji passes, melchor audit\n" +
	"models:                             # per-role model and effort; project overrides user, key by key\n" +
	"  misato: { model: fable, effort: high }\n" +
	"  melchor: { from: jd-judge-b }     # inherit gentle-ai's assignment for that phase (state.json)\n" +
	"critical_paths: [auth/, payments/, migrations/, infra/]            # Hyuga auto-critical\n" +
	"artifacts:\n" +
	"  commit: at-close                  # with-change | at-close | never (default: at-close)\n" +
	"git:\n" +
	"  base_branch: develop              # default base for the worktree offer\n" +
	"  worktree: ask                     # ask | always | never\n" +
	"  branch_pattern: \"feature/{prefix}-{id}-{slug}\"   # prefix comes from the provider (tw)\n" +
	"  commit_ref_pattern: \"({PREFIX}-{id})\"\n" +
	"tasks:\n" +
	"  provider: teamwork                # teamwork | none ; \"ask\" when absent\n" +
	"  ask_when_missing: true            # preflight asks task + worktree + branch if no active task\n" +
	"  subtasks_per_wave: false\n" +
	"  timer_store: ~/.claude/work/timers.json\n" +
	"  rounding_minutes: 15\n" +
	"  providers:                        # one block per provider, only the enabled one is required\n" +
	"    teamwork:\n" +
	"      task_ref_prefix: tw           # {prefix} in branch_pattern / commit_ref_pattern\n" +
	"      assignee_id: 686035           # user scope\n" +
	"      default_project_id: 1271726\n" +
	"      default_tasklist_id: 3951970\n" +
	"      stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }\n" +
	"      known_projects:                 # extra Teamwork projects seen before, for quick lookup\n" +
	"        - id: 1271726\n" +
	"          name: ERP Proveedores\n" +
	"          note: \"primary project for this team\"\n" +
	"        - id: 1300000\n" +
	"          name: Infra\n" +
	"          note: \"shared infra tasks, rarely used\"\n" +
	"        - id: 1400000\n" +
	"          name: Docs\n" +
	"          note: \"documentation backlog, low priority\"\n" +
	"        - id: 1500000\n" +
	"          name: Legacy\n" +
	"          note: \"read-only archive, do not assign\"\n" +
	"  sources:                          # extra work sources for listings (replaces ~/.claude/work/sources.md)\n" +
	"    - name: erp-proveedores\n" +
	"      type: google-sheets\n" +
	"      sheet_id: abc123\n" +
	"    - name: another-source\n" +
	"      type: csv\n" +
	"      path: /data/x.csv\n" +
	"  sources_howto: |\n" +
	"    How to add a new source:\n" +
	"    1. Pick a name.\n" +
	"    2. Pick a type.\n" +
	"    3. Fill in the fields.\n"

// inlineFixtureLF mirrors tests/configure.test.ps1's $fixtureInlineLf: skills:
// as one inline map, no artifacts: block, critical_paths: already inline.
// Group H's regression coverage: an inline-form key must never be treated
// as "block missing" (which used to append a duplicate block next to it),
// and a missing block/key must never be recreated with built-in defaults
// when nothing actually changed.
const inlineFixtureLF = "" +
	"enabled: true\n" +
	"skills: { testing: [tdd, playwright-best-practices], code: [dotnet-best-practices, typescript-best-practices], best-practices: [best-practices, solid-principles, clean-code-guard], architecture: [hexagonal-architecture, c4-architecture], audit: [security-review, clean-code-guard] }   # stacks per consuming role\n" +
	"models:                             # per-role model and effort; project overrides user, key by key\n" +
	"  misato: { model: fable, effort: high }\n" +
	"critical_paths: [auth/, payments/, migrations/, infra/]            # Hyuga auto-critical\n" +
	"git:\n" +
	"  base_branch: develop              # default base for the worktree offer\n" +
	"  worktree: ask                     # ask | always | never\n" +
	"  branch_pattern: \"feature/{prefix}-{id}-{slug}\"   # prefix comes from the provider (tw)\n" +
	"  commit_ref_pattern: \"({PREFIX}-{id})\"\n" +
	"tasks:\n" +
	"  provider: teamwork                # teamwork | none ; \"ask\" when absent\n" +
	"  ask_when_missing: true            # preflight asks task + worktree + branch if no active task\n" +
	"  subtasks_per_wave: false\n" +
	"  timer_store: ~/.claude/work/timers.json\n" +
	"  rounding_minutes: 15\n" +
	"  providers:                        # one block per provider, only the enabled one is required\n" +
	"    teamwork:\n" +
	"      task_ref_prefix: tw           # {prefix} in branch_pattern / commit_ref_pattern\n" +
	"      assignee_id: 686035           # user scope\n" +
	"      default_project_id: 1271726\n" +
	"      default_tasklist_id: 3951970\n" +
	"      stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }\n"

// diffLineCount counts how many lines differ (position by position, after
// splitting on "\n") between a and b — the same byte-exact, order-sensitive
// comparison tests/configure.test.ps1 uses to assert "exactly one line
// changed".
func diffLineCount(a, b string) int {
	aLines := splitLines(a)
	bLines := splitLines(b)
	max := len(aLines)
	if len(bLines) > max {
		max = len(bLines)
	}
	count := 0
	for i := 0; i < max; i++ {
		var av, bv string
		hasA, hasB := i < len(aLines), i < len(bLines)
		if hasA {
			av = aLines[i]
		}
		if hasB {
			bv = bLines[i]
		}
		if !hasA || !hasB || av != bv {
			count++
		}
	}
	return count
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}
