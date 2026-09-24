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
- [x] T0.5 Work-unit commits on `feature/phase-0-overlay` (1d74db9 feat, 15b9b76 fix, 2d2e402 docs, 390db21 fix, plus the closing docs commit). RDD receipt: successor lineage `review-60bd7d4e4202af9e-r1` validated by the provider targeted validator → `approved`; acknowledged with the exact token → `gentle-ai.review-acknowledged/v1`, `authority: burned` (2026-09-24). Push pending user OK.
  - Commit 1d74db9 `feat: bootstrap NERV plugin overlay (phase 0)` (773 lines, 11 files).
  - RDD (switch on globally): `review assess --base-ref develop --committed-only` → risk **high** (shell script in the hook), `review_due: high_risk`. Consent envelope relayed; user chose **granted**. Lineage `review-60bd7d4e4202af9e`, 4 lenses, correction budget 200.
  - Lenses: risk, resilience, readability admitted. reliability failed twice (attempt 1: reviewer result rejected by schema, finding without proof reference; attempt 2 and relaunch: model provider safeguards refused the reviewer message: provider issue, not gentle-ai). Declared `unachievable_lens_slot` (`provider_safeguard_refusal`); user chose to retry once; withdrew the declaration; third launch admitted.
  - Closure: `correction_required`, 3 candidate-caused CRITICAL findings: `R3-activation-regex-rejects-documented-config` (hook regex rejects `enabled: true # comment`), `R3-gate-behavior-unproved` (no automated test of the gate), `R4-installer-write-no-rollback` (settings.json rewritten in place, no temp+rename, no parse check, no auto-restore). Correction plan captured: 100 lines. Correction delegated to one bounded writer (hook regex, `tests/hook-session-start.test.sh`, installer atomic write + restore).
  - Correction committed as 15b9b76 `fix: harden phase 0 activation hook and installer` (parent spot check: 8/8 test cases PASS, diffs read back). README docs committed as 2d2e402.
  - Bound STATUS after the correction → `recovery_authorization_required` (disposition `scope_changed`: the new `tests/` path was outside the frozen manifest). User chose maintainer-authorized recovery; `gentle-ai review recover` created successor lineage `review-60bd7d4e4202af9e-r1` (target sha256:7cd2a109...). Four lenses relaunched on the corrected candidate; all four admitted at first try.
  - Successor closure: `correction_required` again, 2 CRITICAL reliability findings: `R3-plugin-root-unset-path-unproved` (hook prints the header without body when `CLAUDE_PLUGIN_ROOT` is unset or the skill file is missing) and `R3-test-harness-swallows-hook-failures` (suite never exercises an error path, so "always exit 0" is asserted, not proved). Correction plan captured: 60 lines. Second bounded writer delegated: plugin-root default from the script location, no header without body, stderr diagnostic; test with pipefail, honest exit capture, error-path cases.

### Phase 1: LIGHT path end to end
Branch `feature/phase-1-light-path`, stacked on `feature/phase-0-overlay` (Phase 0 not merged into `develop` yet; chained PRs later). Started 2026-09-24.
- [x] T1.1 `nerv-orchestrator/SKILL.md` full protocol (config resolution, preflight, classification LIGHT/FULL + ratchet, delegation triggers, lossless prompts, RDD relay, usage collection, resume). Route: delegated writer A (with T1.2 and T1.4: 4 non-trivial files). Parent readback of the full protocol; three additions inline (change-name rule, usage collection section, TDD mode label `strict|standard`). Hook test suite still 11 pass / 1 skip. Commit 020c219.
- [x] T1.2 `_shared/nerv-phase-common.md` (140 lines), `_shared/nerv-artifacts.md` (175 lines). Route: writer A. Commit 020c219.
- [x] T1.3 Agents `ritsuko` (239), `shinji` (204), `kaworu` (194), `maya` (251). Route: delegated writer B, template `plugin/agents/aoba.md`; parent readback of Kaworu's role contract. Zero `gentle-ai:` markers. Commit 8037dab.
- [x] T1.4 `commands/nerv-init.md` (55 lines). Route: writer A. Commit 020c219. Not yet exercised end to end (its grouped question needs an interactive session); the journey pre-creates `.nerv/nerv.yaml` and `openspec/config.yaml` instead.
- [x] T1.5 LIGHT journey J1 (`bench/journeys.md`) in `D:\projects\nerv-bench-repo` (.NET 10 `Calc` library + xUnit, RDD disabled for that clone by the user's choice, config pre-created instead of `/nerv:init`). OBSERVED 2026-09-24 (non-interactive `claude -p`, build 8037dab): classification LIGHT with reason in `state.yaml`; Ritsuko intel-light (opus) → Ikari wrote `exploration-light.md`; Kaworu RED (CS0117, test-only diff) → Aoba `test:` commit 40942b0; Shinji GREEN 2/2 → TRIANGULATE 3/3 → REFACTOR no-op; Maya reduced: phase a pass, b/c skipped with reason, d build 0 warnings + `no-lint-build-configured`, TDD evidence reproduced read-only from `git show`; Aoba `feat:` commit 0a04785; gatekeeper caught an Aoba envelope that misreported a trailer, retried once, readback proved the commit clean; RDD assess run twice following the native untracked-scope continuation → medium, `under_budget`, switch off; `run-summary.md` with 8 usage rows; `sdd-status subtract-method` OK with `nerv/`. All 10 J1 expectations met. Follow-ups: the Agent tool reports one token total per launch (schema column `tokens_out` unusable: switch to `tokens_total`); NERV artifacts stay untracked until the user commits them (decide a policy: commit with the change or at close); the assess command in the protocol should mention `--untracked-scope=exclude` when the change folder is untracked.
- RDD for the Phase 1 range (base 390db21): medium, `slice_budget_reached`. Consent granted → lineage `review-7a79caea8663cefb`, 1 lens (reliability) → `correction_required` with 4 CRITICAL consistency findings (two pilot domain maps; `exploration-light.md` unwritable under openspec; pilots rei/asuka/toji referenced but unshipped, no failure path; two contradictory descriptions of Maya's RED reproduction). Bounded correction committed as b571732 (64 lines). The bound STATUS then failed 5/5 with `operation_timeout` (pre_native, 25 s budget `reviewFacadeOperationTimeout`, no env override); selectorless STATUS 3 s; STATUS with base-ref treats the corrected range as a fresh target. User chose "continue without reporting" and then **declined** the fresh review (`consent: declined_this_candidate`, lineage review-00bd2086a0485313). Phase 1 ships without a receipt under ordinary policy; lineage review-7a79caea8663cefb stays `correction_required` on disk (not abandoned).

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

## Follow-ups from the Phase 0 review (advisory, non-blocking, 37 items)

The approved receipt lists 37 informational findings (16 WARNING, 21 SUGGESTION). None reopens the review. Worth scheduling in Phase 1 or Phase 5:

- Hook: activation regex ignores YAML structure (indented `enabled: true` under another key activates); no end marker after the injected protocol; silent-on-missing-skill only on stderr (`R1-activation-regex-ignores-yaml-structure`, `R4-injection-has-no-end-marker`, `R2-hook-silent-missing-skill`, `R3-indented-activation-pinned-not-fixed`).
- Installer: settings round-trip through ConvertFrom/ConvertTo-Json verified for syntax, not fidelity; backups unbounded and plaintext; lost-update window between read and swap; uninstall of the marketplace not idempotent; `-Uninstall` untested by the suite (`R1-lossy-settings-roundtrip-verified-as-clean`, `R1-unbounded-plaintext-settings-backups`, `R4-installer-lost-update-and-restore-clobber`, `R3-installer-untested`, `R4-verify-checks-syntax-not-fidelity`, `R2-ensure-property-always-true`, `R2-changed-flag-overreports`, `R2-powershell-unapproved-verb`).
- Aoba: unrestricted Bash on a globally visible agent; frozen-patch base interpolation unvalidated; ping mode buried at the end; `next_recommended` tokens not yet backed by a consumer; "400 lines" unexplained (`R1-global-agent-unrestricted-bash`, `R1-frozen-patch-unvalidated-base-interpolation`, `R2-aoba-ping-mode-buried`, `R2-aoba-next-recommended-unbacked`, `R2-magic-400-lines-unexplained`).
- Docs and metadata: absolute Windows paths and account details in this task file; internal Teamwork ids in README; marketplace description duplicated; version string duplicated in `nerv-status`; protocol stub has two Phase 0 sources; `/nerv:status` failure path undefined (`R1-committed-local-path-account-disclosure`, `R2-task-file-absolute-windows-path`, `R1-internal-tracker-ids-in-readme`, `R2-marketplace-description-duplicated`, `R2-nerv-status-version-duplication`, `R2-phase0-stub-two-sources`, `R4-status-command-undefined-failure-path`, `R2-orchestrator-skill-supersedes-claude-md`, `R2-readme-*`).
- Tests: exit capture judged fragile, `set -u` without `-e`, only the happy plugin root pinned (`R2-test-exit-capture-fragile`, `R2-test-set-u-without-e`, `R4-tests-pin-only-the-happy-plugin-root`, `R2-test-case7-pins-surprising-behavior`).
- `.gitignore` should cover installer backups (`R2-gitignore-omits-installer-backups`).

## Progress log

- 2026-09-24: plan approved; repo and remote created; Phase 0 started.
- 2026-09-24: Phase 1 complete (LIGHT path, J1 green in the bench repo; review declined after a native STATUS timeout).
- 2026-09-24: Phase 0 complete. Overlay proven end to end (protocol injected only with the marker, `nerv:aoba` listed and answering the ping on its declared model, `sdd-preflight-hook` passes NERV agents, `sdd-status` tolerates `nerv/`, sync never touches the plugin cache). RDD review granted, one provider-caused lens failure recovered, two bounded corrections, receipt acknowledged. Repo renamed to `war-apps/nerv-gentle-ai`.

## Next step

User OK to push `feature/phase-1-light-path` (stacked on phase 0). Then Phase 2 (FULL path with MAGI): rei/asuka/toji, misato, hyuga, MAGI, fuyutsuki, votes/waves/log artifacts.
