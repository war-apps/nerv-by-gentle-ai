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
  audit: [security-review, clean-code-guard]                       # kaji passes
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
  branch_pattern: "feature/{prefix}-{id}-{slug}"   # prefix comes from the provider (tw, gh, jira)
  commit_ref_pattern: "({PREFIX}-{id})"
tasks:
  provider: teamwork                # teamwork | github-projects | jira | none ; "ask" when absent
  ask_when_missing: true            # preflight asks task + worktree + branch if no active task
  subtasks_per_wave: false
  providers:                        # one block per provider, only the enabled one is required
    teamwork:
      task_ref_prefix: tw           # {prefix} in branch_pattern / commit_ref_pattern
      assignee_id: 686035           # user scope
      project_id: 1271726           # project scope
      tasklist_id: 3951970          # project scope
      stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }
    github-projects: { task_ref_prefix: gh, owner: "", project_number: 0 }    # later
    jira: { task_ref_prefix: jira, site: "", project_key: "" }               # later
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

### Configuring models and effort

The wizard's **Models** section (see "Setup" in [the README](../README.md))
prints the resolved table (role, model, effort, source — `override`,
`gentle-ai:<phase>`, or `default`), then lets you edit it role by role, or
by group (`magi`, `pilots`, `kaji-passes`, `all`), until you type `done`.
For each role it asks for a model (`sonnet`/`opus`/`haiku`/`fable`/
`inherit`, a custom `claude-...` id, or `from:` a gentle-ai phase listed
from `~/.gentle-ai/state.json`) and an effort (`low`/`medium`/`high`/
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
