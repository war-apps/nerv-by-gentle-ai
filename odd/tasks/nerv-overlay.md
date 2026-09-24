# Feature: NERV overlay on gentle-ai

Approved plan: `C:\Users\wal1rod\.claude\plans\entonces-basado-en-todo-toasty-koala.md` (source of truth for architecture, schemas and phases).

## Objective

Rebuild NERV (Evangelion-named multi-agent workflow) as a Claude Code plugin installed on top of gentle-ai 3.7.0, reusing its native engine and contracts unchanged and adding NERV's governance layer (MAGI vote, Fuyutsuki veto, Maya gate, Kaji audit, Hyuga operations, Aoba delivery, deliberation log, run summary) plus a provider-agnostic task-tracking layer.

## Problem / why

NERV existed only as a text spec. gentle-ai has the infrastructure NERV lacks (SDD artifacts, RDD, registry, strict TDD, resumability, proportionality); NERV has the governance gentle-ai lacks. An overlay keeps both without forking Go code.

## Scope and constraints

- Plugin only: never write under `~/.claude/agents` or `~/.claude/skills` (gentle-ai sync territory).
- Activation per repo via `.nerv/nerv.yaml` `enabled: true`. One config schema, two scopes (user `~/.claude/nerv/nerv.yaml`, project `.nerv/nerv.yaml`).
- Ikari is the main session; 17 agents; artifacts under `openspec/changes/{change}/nerv/`.
- Artifacts, agents, skills and docs in English. Conventional commits. Gitflow: work on `feature/*`, never commit to `main`/`develop`. Never push without explicit OK.
- No Teamwork task for this work (user decision 2026-09-24): commits carry no `(TW-id)` suffix.
- TDD: strict mode is enabled globally but this repo is markdown plus one shell script; verification is behavioral through `bench/journeys.md` and the checks listed per task.
- Delivery strategy: `ask-on-risk` (default). Forecast for Phase 0: ~400 authored lines (scaffold + one agent + hook + installer + README).

## Tasks

### Phase 0: prove the overlay
- [x] T0.1 Local repo `D:\projects\nerv-gentle-ai` with gitflow (`main`, `develop`, `feature/phase-0-overlay`) and remote `war-apps/nerv-gentle-ai` (private). Evidence: commit 4afa8e0, `gh repo create` then `gh repo rename nerv-gentle-ai` (user request 2026-09-24), https://github.com/war-apps/nerv-gentle-ai. Plugin and agent namespace stay `nerv` (`nerv:aoba`). Route: inline (git state).
- [x] T0.2 Scaffold plugin (committed 1d74db9; writer verification: JSON parse ok, `bash -n` ok, hook prints only with `enabled: true`, installer parses; parent spot check: hook with CRLF config prints protocol, disabled prints nothing; three fixes applied by the parent: `nerv:*` markers instead of `gentle-ai:*`, Engram tools added to Aoba, ping status `success`): `.claude-plugin/marketplace.json`, `plugin/.claude-plugin/plugin.json`, `plugin/agents/aoba.md`, `plugin/commands/nerv-status.md`, `plugin/hooks/hooks.json`, `plugin/hooks/nerv-session-start.sh`, `plugin/skills/nerv-orchestrator/SKILL.md` (Phase 0 stub with supersession clause), `tools/install.ps1`, `README.md`. Route: delegated writer (writer trigger: 9 non-trivial files). Checks: JSON files parse; shell script passes `bash -n`; hook prints protocol only when `.nerv/nerv.yaml` has `enabled: true`.
- [x] T0.3 Register the plugin in `~/.claude/settings.json` (marketplace `nerv`, `enabledPlugins["nerv@nerv"]`) via `tools/install.ps1` with a backup of settings.json. Route: inline (bounded action). Checks: settings.json still parses; both keys present. Plus `claude plugin uninstall/install nerv@nerv` to snapshot HEAD 1d74db9 into the cache (see (1)/(2) note below).
- [x] T0.4 Verify overlay. OBSERVED after reinstall (fresh `claude -p` sessions): (1) bench repo with marker → `YES` + `nerv:aoba` listed; (2) scratch repo without marker → `NO` + `nerv:aoba` listed (agents are global once installed, protocol is not injected: as designed); (4) `nerv ping` → `nerv:aoba` spawned once (`by_type: {"nerv:aoba": 1}`), agent reported running on Sonnet 5 as its frontmatter says, envelope returned with `status: success`, repo root and branch correct. Frontmatter model honored. Details of (3) and (5) below.
  - Original check list: (1) session in a repo WITH marker shows injected protocol and `nerv:aoba` listed; (2) repo WITHOUT marker shows nothing NERV; (3) `nerv:aoba` launch passes `sdd-preflight-hook` inside a repo with an active openspec change; (4) `model: opus` honored for a plugin agent; (5) `gentle-ai sync` leaves the plugin cache untouched. Route: inline / per-action workers. Record observed results here.
  - (3) OBSERVED 2026-09-24: scratch repo `D:\projects\nerv-bench-repo` with `openspec/changes/bench-change/{state.yaml,proposal.md}`; synthetic PreToolUse payload piped to `gentle-ai sdd-preflight-hook --agent claude-code`: `subagent_type=nerv:aoba` → exit 0, empty output; `aoba` → exit 0, empty; `sdd-apply` → exit 0 with `permissionDecision: deny` ("SDD child dispatch refused..."). The hook gates only SDD child types. PASS.
  - Also observed: `gentle-ai sdd-status bench-change --json` succeeds with an empty `nerv/` subfolder inside the change (artifactStore openspec, proposal detected). PASS for the layout assumption.
  - (5) OBSERVED: `gentle-ai sync --dry-run` lists managed components engram, sdd, skills, context7, permissions, gga, claude-theme, opencode-gentle-logo, persona; no `plugins/` path. Combined with the embedded asset tree in `internal/assets/assets.go` (mapped earlier), the plugin cache is outside sync's reach. PASS by construction (no live sync run against the user's config).
  - (1)/(2) FIRST ATTEMPT FAILED, root cause found: two fresh `claude -p` sessions (bench repo with marker, scratch repo without) both answered "NO / none". `installed_plugins.json` shows `nerv@nerv` cached from `gitCommitSha` 4afa8e0 (the empty initial commit) into `cache/nerv/nerv/0.1.0`, which does not exist. Claude Code snapshots directory marketplaces from the committed HEAD, not the working tree. Fix: commit the scaffold, then `claude plugin update nerv@nerv`, then re-run (1)/(2)/(4). Documented in README "Updating after local changes".
  - `settings.json` after `tools/install.ps1`: semantically identical to the backup plus the two NERV keys (PowerShell object comparison, 57102 chars both). Installer re-run after the folder rename updated the marketplace path idempotently. T0.3 PASS.
- [ ] T0.5 Work-unit commits on `feature/phase-0-overlay`; push only after user OK.
  - Commit 1d74db9 `feat: bootstrap NERV plugin overlay (phase 0)` (773 lines, 11 files).
  - RDD (switch on globally): `review assess --base-ref develop --committed-only` → risk **high** (shell script in the hook), `review_due: high_risk`. Consent envelope relayed; user chose **granted**. Lineage `review-60bd7d4e4202af9e`, 4 lenses, correction budget 200.
  - Lenses: risk, resilience, readability admitted. reliability failed twice (attempt 1: reviewer result rejected by schema, finding without proof reference; attempt 2 and relaunch: model provider safeguards refused the reviewer message: provider issue, not gentle-ai). Declared `unachievable_lens_slot` (`provider_safeguard_refusal`); user chose to retry once; withdrew the declaration; third launch admitted.
  - Closure: `correction_required`, 3 candidate-caused CRITICAL findings: `R3-activation-regex-rejects-documented-config` (hook regex rejects `enabled: true # comment`), `R3-gate-behavior-unproved` (no automated test of the gate), `R4-installer-write-no-rollback` (settings.json rewritten in place, no temp+rename, no parse check, no auto-restore). Correction plan captured: 100 lines. Correction delegated to one bounded writer (hook regex, `tests/hook-session-start.test.sh`, installer atomic write + restore).

### Phase 1: LIGHT path end to end
- [ ] T1.1 `nerv-orchestrator/SKILL.md` full protocol (classification, delegation triggers, lossless prompts, RDD relay, usage collection, resume).
- [ ] T1.2 `_shared/nerv-phase-common.md`, `_shared/nerv-artifacts.md` (light subset).
- [ ] T1.3 Agents `ritsuko`, `shinji`, `kaworu`, `maya`.
- [ ] T1.4 `commands/nerv-init.md` (writes `.nerv/nerv.yaml`, delegates `sdd-init`).
- [ ] T1.5 Verify LIGHT journey in a scratch repo (see plan Phase 1).

### Phase 2: FULL path with MAGI
- [ ] T2.1 Agents `misato`, `hyuga`, `balthasar`, `melchor`, `casper` (VOTE), `fuyutsuki`, `rei`, `asuka`, `toji`.
- [ ] T2.2 Artifacts `criticality.md`, `votes.md`, `veto-ruling.md`, `waves.md`, `deliberation-log.md` schemas + gates + bounded loops in the protocol.
- [ ] T2.3 Verify FULL journey (see plan Phase 2).

### Phase 3: audit and closure
- [ ] T3.1 Agents `kaji`, `kaji-security`, `kaji-coverage`; AUDIT mode in MAGI; frozen patch by Aoba; `review-refuter` reuse; ranking; issue gate; re-audit cap; Ritsuko docs + `sdd-archive`.
- [ ] T3.2 Verify audit journey (see plan Phase 3).

### Phase 4: task-tracking layer and single config file
- [ ] T4.1 `skills/nerv-tasks/SKILL.md` port + `providers/teamwork.md` + stubs for github-projects and jira.
- [ ] T4.2 Preflight grouped question + Hyuga dispatch (d) in FULL and LIGHT + tracker close.
- [ ] T4.3 Migrate `~/.claude/commands/task/*.md` to read from `nerv.yaml`; create user `~/.claude/nerv/nerv.yaml`; delete `~/.claude/work/sources.md`.
- [ ] T4.4 Verify tracker journey (see plan Phase 4).

### Phase 5: hardening
- [ ] T5.1 `bench/journeys.md` complete, README, installer parity, memory note.

## Progress log

- 2026-09-24: plan approved; repo and remote created; Phase 0 started.

## Next step

T0.2 scaffold via delegated writer.
