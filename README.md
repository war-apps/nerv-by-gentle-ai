# NERV

NERV is a Claude Code plugin implementing an Evangelion-named multi-agent
governance workflow: Ikari orchestrates, Fuyutsuki holds governance veto,
Misato authors the plan, MAGI (Balthasar / Melchor / Casper) vote per task,
pilots (Rei / Shinji / Asuka / Toji / Kaworu) implement, Kaji compiles a
ranked, multi-pass audit, Maya gates quality, Hyuga owns criticality/waves/
task-tracking, and Aoba handles git operations and the run summary.

NERV brings governance that a plain implementation loop lacks: a per-task
MAGI vote gated by criticality, a governance veto on new skills/scripts/
commands, a mandatory quality gate before any audit, a multi-pass audit
compiler with a ranked user issue gate, an append-only deliberation log, and
a run summary reporting tokens, time, and model per agent.

## Relation to gentle-ai

NERV is an **overlay**, not a fork. It reuses gentle-ai's native engine and
contracts unchanged — the SDD artifact pipeline, RDD (receipt-driven review),
the skill registry and resolver, strict TDD, delivery budgeting with chained
PRs, and the lossless blocking-prompt contract — and expresses NERV's own
governance on top. NERV never modifies any gentle-ai file: nothing is ever
written under `~/.claude/agents` or `~/.claude/skills`, so `gentle-ai sync`
cannot see or touch it.

Tested against: gentle-ai 3.7.0

## Install

1. `git clone https://github.com/war-apps/nerv-gentle-ai.git` (the folder path is registered as a local plugin marketplace, so keep the clone where it will stay).
2. `pwsh tools/install.ps1`
3. Restart Claude Code.
4. In any repo where you want NERV active, create `.nerv/nerv.yaml` with:

   ```yaml
   enabled: true
   ```

Run `pwsh tools/install.ps1 -Uninstall` to remove the marketplace and
plugin registration again.

### Updating after local changes

Claude Code snapshots a directory marketplace from the repository's
**committed HEAD** into `~/.claude/plugins/cache/nerv/nerv/<version>/`
(it records the git commit SHA in `installed_plugins.json`). Uncommitted
edits are invisible to sessions. After changing plugin files:

1. Commit the change.
2. Refresh the cache. `claude plugin update nerv@nerv` only re-snapshots
   when `version` in `plugin/.claude-plugin/plugin.json` changed; with the
   same version run `claude plugin uninstall nerv@nerv` followed by
   `claude plugin install nerv@nerv` (settings.json keeps `nerv@nerv`
   enabled, so nothing else changes).
3. Restart Claude Code.

## Phase 0 status

This is the Phase 0 slice: the smallest proof that the overlay mechanism
works. It ships one agent (`aoba`), one command (`/nerv:status`), one
SessionStart hook that injects the NERV orchestrator protocol only in repos
where `.nerv/nerv.yaml` has `enabled: true`, an installer, and this README.
The full Ikari protocol (classification, MAGI vote, audit, waves) is not
implemented yet — a NERV-enabled repo currently falls back to gentle-ai's
ordinary organic flow for actual work.

## Roadmap

- **Phase 1** — LIGHT path end to end: the Ikari protocol (classification,
  delegation triggers, RDD relay, usage collection), `ritsuko`, `shinji`,
  `kaworu`, `maya`, and `/nerv:init`.
- **Phase 2** — FULL path with MAGI: `misato`, `hyuga`, `balthasar`,
  `melchor`, `casper`, `fuyutsuki`, the vote/veto/waves artifacts, and the
  plan-approval HARD gate.
- **Phase 3** — Audit and closure: `kaji`, `kaji-security`, `kaji-coverage`,
  the frozen-patch audit passes, ranked issue gate, fix routing, and
  archive.
- **Phase 4** — Task-tracking layer (Hyuga) and single config file: the
  provider-agnostic task port, the Teamwork adapter, and the preflight
  question for task/worktree/branch/base.
- **Phase 5** — Hardening: the full `bench/journeys.md` suite, README
  parity, and the resume protocol.

## Roles

| Role | One-line responsibility |
|---|---|
| `fuyutsuki` | Governance veto ruling on new skills/scripts/commands; curates the deliberation log. |
| `ritsuko` | Intel, spec + test plan, and docs — three spawns per run. |
| `misato` | Authors the proposal, design, and tasks; issues binding rulings. |
| `balthasar` | MAGI vote member (per-task vote and audit passes). |
| `melchor` | MAGI vote member — the strong-model side of the MAGI asymmetry. |
| `casper` | MAGI vote member; also runs in AUDIT mode. |
| `rei` | Pilot — implements assigned tasks within a wave. |
| `shinji` | Pilot — implements assigned tasks within a wave. |
| `asuka` | Pilot — implements assigned tasks within a wave. |
| `toji` | Pilot — implements assigned tasks within a wave. |
| `kaworu` | Pilot — commits the failing (RED) test before the pilot's GREEN implementation. |
| `kaji` | Compiles and dedupes the multi-pass audit into `audit-report.md`. |
| `kaji-security` | Audit pass focused on security. |
| `kaji-coverage` | Audit pass focused on test coverage. |
| `maya` | Quality gate — runs tests, lint, and build across phases a-d. |
| `hyuga` | Criticality, waves, issue ranking, and task-tracker dispatch. |
| `aoba` | Git operations and run telemetry — commits, delivery prep, and the run summary. |

Ikari (the orchestrator) is the session itself, not a spawnable agent — see
`plugin/skills/nerv-orchestrator/SKILL.md`.

## Configuration schema

Every NERV setting lives in `nerv.yaml`. There are exactly two copies, same
schema: user scope `~/.claude/nerv/nerv.yaml` (personal defaults, never
committed) and project scope `<repo>/.nerv/nerv.yaml` (committed). Project
overrides user key by key; a missing key falls back to the user file, then
to the built-in default.

```yaml
# ~/.claude/nerv/nerv.yaml (user) and <repo>/.nerv/nerv.yaml (project): same schema
enabled: true                       # project scope only: activates NERV in this repo
skills:                             # stacks per consuming role; names must exist in .atl/skill-registry.md
  testing: [tdd, playwright-best-practices]                        # ritsuko, kaworu, maya
  code: [dotnet-best-practices, typescript-best-practices]         # pilots
  best-practices: [best-practices, solid-principles, clean-code-guard]  # balthasar
  architecture: [hexagonal-architecture, c4-architecture]          # melchor
  audit: [security-review, clean-code-guard]                       # kaji passes
critical_paths: [auth/, payments/, migrations/, infra/]            # Hyuga auto-critical
git:
  base_branch: develop              # default base for the worktree offer
  worktree: ask                     # ask | always | never
  branch_pattern: "feature/{prefix}-{id}-{slug}"   # prefix comes from the provider (tw, gh, jira)
  commit_ref_pattern: "({PREFIX}-{id})"
tasks:
  provider: teamwork                # teamwork | github-projects | jira | none ; "ask" when absent
  ask_when_missing: true            # preflight asks task + worktree + branch if no active task
  subtasks_per_wave: false
  providers:                        # one block per provider, only the enabled one is required
    teamwork:
      assignee_id: 686035           # user scope
      project_id: 1271726           # project scope
      tasklist_id: 3951970          # project scope
      stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }
    github-projects: { owner: "", project_number: 0 }    # later
    jira: { site: "", project_key: "" }                  # later
  sources:                          # extra work sources for listings (replaces ~/.claude/work/sources.md)
    - { name: erp-proveedores, type: google-sheets, ... }
```
