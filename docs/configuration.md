# Configuration schema

Every Nerv by Gentle-AI setting lives in `nerv.yaml`. There are exactly two
copies, same schema: user scope `~/.claude/nerv/nerv.yaml` (personal
defaults, never committed) and project scope `<repo>/.nerv/nerv.yaml`
(committed). Project overrides user key by key; a missing key falls back
to the user file, then to the built-in default.

```yaml
# ~/.claude/nerv/nerv.yaml (user) and <repo>/.nerv/nerv.yaml (project): same schema
enabled: true                       # project scope only: activates NERV in this repo
skills:                             # stacks per consuming role; names must exist in .atl/skill-registry.md
  testing: [tdd, playwright-best-practices]                        # ritsuko, kaworu, maya
  code: [dotnet-best-practices, typescript-best-practices]         # pilots
  best-practices: [best-practices, solid-principles, clean-code-guard]  # balthasar
  architecture: [hexagonal-architecture, c4-architecture]          # melchor
  audit: [security-review, clean-code-guard]                       # kaji passes, melchor audit
models:                             # per-role model and effort; project overrides user, key by key
  misato: { model: fable, effort: high }
  melchor: { from: jd-judge-b }     # inherit gentle-ai's assignment for that phase (state.json)
  aoba: { model: sonnet, effort: low }
critical_paths: [auth/, payments/, migrations/, infra/]            # Hyuga auto-critical
artifacts:
  commit: at-close                  # with-change | at-close | never (default: at-close)
git:
  base_branch: develop              # default base for the worktree offer
  worktree: ask                     # ask | always | never
  worktree_pattern: ".claude/worktrees/{slug}"   # where task worktrees are created; {slug} {branch} {prefix} {id} {repo}
                                                  # `nerv configure`'s wizard offers default / herdr / custom; herdr
                                                  # reuses its own [worktrees] directory as <directory>/{repo}/{slug}
  branch_pattern: "feature/{prefix}-{id}-{slug}"   # prefix comes from the provider (tw)
  commit_ref_pattern: "({PREFIX}-{id})"
tasks:
  provider: teamwork                # teamwork | none ; "ask" when absent
  ask_when_missing: true            # preflight asks task + worktree + branch if no active task
  subtasks_per_wave: false
  providers:                        # one block per provider, only the enabled one is required
    teamwork:
      task_ref_prefix: tw           # {prefix} in branch_pattern / commit_ref_pattern
      assignee_id: 686035           # user scope
      project_id: 1271726           # project scope
      tasklist_id: 3951970          # project scope
      stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }
  sources:                          # extra work sources for listings (replaces ~/.claude/work/sources.md)
    - { name: erp-proveedores, type: google-sheets, ... }
```

`models:` assigns a per-role `model` and `effort` override, with project
overriding user key by key like every other section — each entry is an
inline map, one role per line, with `model`, `effort`, or `from` keys;
`from: <gentle-ai-phase>` inherits that phase's `model`/`effort` from
`~/.gentle-ai/state.json`'s `claude_phase_assignments`, and an explicit
`model`/`effort` on the same line wins over the inherited one. A role
absent from both `models:` files keeps the plugin default (see
[integration.md](integration.md#roles)). `model` and `effort` are
applied differently, because Claude Code itself treats them differently:
`model` is a per-call launch parameter, so Ikari reads the resolved
`models:` map (both scopes merged) and passes it to every agent launch —
it takes effect immediately, no reinstall needed. `effort` is honored only
from the launched agent file's own frontmatter, so it can only be applied
by rewriting the cached agent files themselves: `nerv apply-models` reads
`models:` from the **user-scope** file only (`~/.claude/nerv/nerv.yaml`)
and rewrites `model:`/`effort:` in each affected `<role>.md` under the
plugin cache. Project-scope `models:` therefore applies to `model` only —
`effort` is not applicable at project scope, since project config cannot
reach into a user's local plugin cache. `nerv install` runs this same apply
step automatically at the end of its own cache refresh.
`/nerv:status` reports the resolved `models:` table (both scopes merged)
and warns when the cached agent frontmatter has drifted from it, so a
pending `nerv apply-models` run is visible without inspecting the cache by
hand.

### Roles and their gentle-ai equivalents

Every role below can be overridden under `models:`. The gentle-ai
equivalents are the gentle-ai v4 agents whose duties the role covers (the
`jd-*` agents and the native `review-*` agents); roles with none show none.
Every gentle-ai v4 agent (the `GentleAIV4Agents` list in
`internal/config/models.go`) is claimed by at least one role, and a test
fails if one is left unclaimed. No two roles share an equivalent except the
pilots, which claim `jd-fix-agent` because fix routing goes through the
owning pilot, with kaworu writing the RED test first. Equivalents are informational only and never a `from:<phase>` value:
the native review agents (`review-*`) are not keys of
`claude_phase_assignments`. Only the `jd-judge` equivalents (melchor's
`jd-judge-b`, balthasar's `jd-judge-a`) double as a `from:` suggestion,
exposed separately as `from_phase`; only melchor and balthasar carry one.
casper (whose equivalent is the native `review-readability`) and the
pilots get no suggestion. Neither changes how a model is
resolved. The same data is shown in the wizard's models table, in
`/nerv:configure` and `/nerv:status`, and as `purpose` /
`gentle_ai_equivalents` (an array) / `from_phase` on each
`nerv configure --print` models row. `gentle_ai_equivalent` carries the same
list joined with `, ` for readers of the original single-string field. Roles are addressable by group: `magi` (the three voters),
`pilots` (the implementers), `kaji-passes` (the audit passes) and `all`.

| Role | Group | Purpose | gentle-ai equivalent | Default model / effort |
|---|---|---|---|---|
| `misato` | | authors the plan (proposal, design, tasks) | none | fable / high |
| `ritsuko` | | intelligence, test planning, end-of-run docs | none | opus / high |
| `hyuga` | | task criticality, dependency waves, tracking | none | sonnet / medium |
| `melchor` | `magi` | MAGI vote: structure and security; security audit | `jd-judge-b`, `review-risk` | fable / high |
| `balthasar` | `magi` | MAGI vote: software principles | `jd-judge-a` | sonnet / medium |
| `casper` | `magi` | MAGI vote: process and documentation; readability audit | `review-readability` | sonnet / medium |
| `fuyutsuki` | | governance veto on new skills/scripts/commands | none | sonnet / medium |
| `kaworu` | `pilots` | writes the failing tests first | `jd-fix-agent` | sonnet / medium |
| `shinji` | `pilots` | backend pilot | `jd-fix-agent` | sonnet / medium |
| `asuka` | `pilots` | frontend pilot | `jd-fix-agent` | sonnet / medium |
| `rei` | `pilots` | data pilot (persistence, observability) | `jd-fix-agent` | sonnet / medium |
| `toji` | `pilots` | infrastructure pilot (CI/CD, containers) | `jd-fix-agent` | sonnet / medium |
| `maya` | | quality gate (tests, lint, build) | none | sonnet / medium |
| `kaji` | `kaji-passes` | audit compiler | none | opus / high |
| `kaji-coverage` | `kaji-passes` | audit pass: test coverage, reliability, correctness | `review-reliability` | sonnet / medium |
| `kaji-resilience` | `kaji-passes` | audit pass: resilience and performance | `review-resilience` | sonnet / medium |
| `kaji-refuter` | `kaji-passes` | refutes severe audit findings | `review-refuter` | sonnet / medium |
| `aoba` | | commits, PRs and run telemetry | none | sonnet / low |

### Configuring models and effort

The wizard's **Models** section (see "Setup" in [the README](../README.md))
prints the resolved table (role, model, effort, source — `override`,
`gentle-ai:<phase>`, or `default`; `gentle-ai:<phase> (missing; plugin
default)` when `~/.gentle-ai/state.json` has phase assignments but not
that phase — plus what each role does and its gentle-ai equivalent) and a
legend for the group shortcuts, then lets you
edit it role by role, or
by group (`magi`, `pilots`, `kaji-passes`, `all`), until you type `done`.
For each role it asks for a model (`sonnet`/`opus`/`haiku`/`fable`/
`inherit`, a custom `claude-...` id, or `from:` a gentle-ai phase listed
from `~/.gentle-ai/state.json`, with the role's own `from_phase` listed
first and marked `(equivalent)`; roles without one get no mark) and an effort (`low`/`medium`/`high`/
`xhigh`/`max`), with Enter keeping the current value; `reset <role|group>`
clears an override back to the plugin default. Confirming writes the block
to the user-scope `nerv.yaml`, after backing up the file to
`<path>.bak-models-<yyyyMMdd-HHmmss>`, then offers to run
`nerv apply-models` immediately.

Non-interactively, `nerv configure --set-model role=model[/effort]` (or
`role=from:<gentle-ai-phase>`, or `role=default` to clear an override)
applies one role's override at a time to the same user-scope file; pass it
repeatedly for a batch. This always edits the user-scope file — project
scope has no dedicated command, since project-scope `models:` only ever
affects `model` (see above). Edit the project `.nerv/nerv.yaml` `models:`
block by hand for that case, using the same inline-map syntax and `from:`
behavior described above.

You can also skip both the wizard and `--set-model`, and edit the
`models:` block by hand in either `nerv.yaml`, then run
`nerv apply-models` yourself. Either way, restart Claude Code afterwards
for the change to take effect.
