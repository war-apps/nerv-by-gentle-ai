# Feature: protocol injection within the hook cap

Branch `feature/protocol-injection-split` off `develop` (`825f97e`). Started 2026-10-01. Engram mirror: `odd/protocol-injection/tasks` (project `nerv`).

## Objective

Make the NERV orchestrator protocol reach the model in full in every opted-in session, within Claude Code's documented hook limits.

## Problem

`plugin/hooks/nerv-session-start.sh` prints the whole `plugin/skills/nerv-orchestrator/SKILL.md` (51,626 bytes, 854 lines) to stdout. Claude Code caps a hook's plain stdout and `additionalContext` at 10,000 characters (https://code.claude.com/docs/en/hooks.md, "JSON output"); the excess is saved to a file and replaced by the path plus a 2,000-character preview. No setting raises the cap. Observed on 2.1.283 (Windows) and 2.1.285 (WSL) during the first real plugin eval run (Engram #405): the protocol is no longer injected verbatim.

## Why

Without the protocol the session is not Ikari: routing, preflight, classification and the pipelines silently degrade to whatever the model reads from the saved file. This affects every NERV session today, before any release.

## Decision (user, 2026-10-01)

Short activation header in the hook plus a split skill (the documented pattern: https://code.claude.com/docs/en/skills.md recommends SKILL.md under 500 lines with detail in reference files; https://code.claude.com/docs/en/features-overview.md says standing knowledge belongs in a skill, not a hook).

- The hook prints only an activation header (well under 10,000 characters). Its first line stays exactly `# NERV orchestrator protocol (active: .nerv/nerv.yaml enabled)` so the existing hook test, the `activation-*` eval cases and the bench journeys keep their contract. The header instructs the session to invoke the skill `nerv:nerv-orchestrator` with the Skill tool before its first response and again after any compaction (the hook matcher already includes `compact`), and to load the protocol's reference files at the phase that names them.
- `SKILL.md` becomes a core under 500 lines: frontmatter, intro, Supersession, Identity, Orchestrator lock, Configuration resolution, Preflight, Classification, Bounded loops, Delegation triggers (with the per-launch template), Gatekeeper, Lossless blocking prompts, Ping, plus a reference index that says which file to load when.
- The run-time sections move verbatim to `plugin/skills/nerv-orchestrator/references/`: LIGHT pipeline, FULL pipeline (with its `###` subsections), RDD relay, Delivery, Usage collection, Deliberation log, Resume, Phase note. Grouping is the writer's call; no sentence of the protocol is dropped or rewritten except the intro paragraph that describes the injection mechanism and the new index.
- The Go CLI embeds `all:plugin` (`assets.go`), so `references/` ships with the installer without code changes; `internal/plugin/materialize_test.go` compares bytes and needs no edit.

Rejected alternative: header plus the unsplit 854-line skill. It works today but ignores the 500-line guidance and leaves only the start of the skill after a compaction.

## Scope

In: hook, hook test, SKILL.md split, a size guard test, eval grader fixes found by the first run, docs that describe the mechanism. Out: the SessionStart Engram hook, the overlay follow-ups, releases.

## Constraints

- Strict TDD (session config): observed RED before each implementation, then GREEN, then refactor. Runners: `bash tests/hook-session-start.test.sh` (hook suite, 11 pass / 1 skip on the base) and `go test ./...`.
- English artifacts; no AI attribution in commits; conventional commits; no push without explicit OK.
- Delivery strategy: `ask-on-risk` with the chain strategy pre-selected as `feature-branch-chain` (precedent: #14–#16, #17–#24, #29–#31). Forecast: about 1,300 authored changed lines, dominated by the verbatim relocation of ~550 protocol lines; the relocation commit is one slice, the rest another.

## Tasks

- [x] T1 DONE 2026-10-01, commit `c2e0c85`. RED observed by the parent (new `assert_active` against the base hook: `enabled-true-*` cases FAIL with `bytes=51689`), GREEN 11 pass / 0 fail / 1 skip; header is 706 bytes; `tests/hook-engram-project.test.sh` 17/17; `go test ./...` and `go vet ./...` clean. Writer was interrupted by a session end after writing hook + test; the parent reproduced the evidence. Original scope: Hook activation header. RED: `assert_active` in `tests/hook-session-start.test.sh` additionally requires stdout under 10,000 bytes, the Skill-tool instruction (`nerv:nerv-orchestrator`), and the absence of the skill body (`name: nerv-orchestrator` must NOT appear); GREEN: `nerv-session-start.sh` prints the header only; the missing-skill-file branch keeps its stderr warning. Route: delegated writer (test + hook + T3 graders, 4 files).
- [x] T2 DONE 2026-10-01, commit `14e1d97`. Parent spot check: fidelity diffs for `pipeline-full.md` and `usage-and-log.md` re-run and empty, no CRLF in the new files, `go test ./...` + `go vet ./...` + hook suite green. RED observed by the writer: new `TestOrchestratorSkillCoreStaysWithinSkillGuidance` in `skill_split_test.go` (root package) failed on both checks against the base SKILL.md (`SKILL.md has 854 lines, want at most 500`; `reading dir skills/nerv-orchestrator/references: open ...: file does not exist`). GREEN: core `plugin/skills/nerv-orchestrator/SKILL.md` is 445 lines; 7 reference files under `plugin/skills/nerv-orchestrator/references/` (`pipeline-light.md` 29, `pipeline-full.md` 255, `rdd-relay.md` 30, `delivery.md` 27, `usage-and-log.md` 56, `resume.md` 27, `phase-note.md` 21 = 445 lines). Fidelity: `diff <(git show develop:...SKILL.md | sed -n '<range>p') references/<file>` empty for all 7 files (byte-for-byte). Kept body (397 lines) + moved body (445 lines) + original rewritten intro (12 lines) = 854, the full original. `go test ./...` and `go vet ./...` clean; `internal/skills` and `internal/plugin` tests (which walk the embedded tree) still pass. Original scope: Split `SKILL.md` into core + `references/`. Route: delegated writer (2+ non-trivial files).
- [x] T3 DONE 2026-10-01, commit `17077cb` (inline: five one-line regex edits; `no-protocol-header` keeps `target: trace` but matches the header body line `This session is the NERV orchestrator (Ikari)`, which no prompt echoes). No local runner for evals; validated by inspection. Original scope: Eval grader fixes from the first run (Engram #405): `answers-yes.md` / `answers-no.md` tolerate markdown emphasis before YES/NO (`^[\s*_]*NO\b`); `no-protocol-header.md` in `activation-absent` and `activation-disabled` stops grading the trace (the prompt itself echoes the header) and grades `last_message` or a body line the prompt does not quote. Route: bundled with T1's writer.
- [x] T4 DONE 2026-10-01, commit `e77ae78`. Edited `docs/integration.md` (Activation mermaid sequence diagram and its prose, plus the Operations/Activation paragraph) and `docs/commands.md` (Automatic-hooks and Not-commands sections) and `plugin/commands/init.md` (summarize step) to describe header → Skill tool → `references/` on demand, and name the 10,000-character hook cap once. `bench/journeys.md` and `README.md` needed no change: journeys' "protocol injected in this session" lines are J0/J1 prompt text instructing the model, never asserting the full body is already in context, and J0's literal check (first line of the header) is still accurate; README.md has no injection-mechanism statement. `odd/tasks/nerv-overlay.md` needed no change: every `inject*`/`verbatim` hit there is historical progress-log text, not a standing how-it-works statement. Final grep `cat .*SKILL.md|injected verbatim|injects the full` over docs/plugin/README/bench: no hits. Original scope: Docs describing the mechanism. Route: delegated writer, bundled with T2.

## Acceptance criteria

- Hook stdout under 10,000 bytes in the enabled case; first line unchanged; no skill body.
- `SKILL.md` ≤ 500 lines; every section of the original protocol present either in the core or in exactly one reference file, verbatim.
- `bash tests/hook-session-start.test.sh` and `go test ./...` green; `go vet ./...` clean.
- Docs describe the new mechanism (header → Skill tool → references on demand).

## Progress log

- 2026-10-01: feature document created after exploration (docs confirmation of the 10,000-character cap; consumer map: hook test, 3 activation eval cases, bench journeys, 3 docs; Go embed covers `references/`). Branch created.
- 2026-10-01: T1 (`c2e0c85`) and T3 (`17077cb`) committed. Running count: 38 + 10 authored changed lines. T2 + T4 writer launched.
- 2026-10-01: T2 + T4 writer returned. SKILL.md split into a 445-line core plus 7 verbatim reference files (fidelity diffs empty); `skill_split_test.go` added as the RED/GREEN guard; docs updated in `docs/integration.md`, `docs/commands.md`, `plugin/commands/init.md`. `go test ./...`, `go vet ./...`, and `bash tests/hook-session-start.test.sh` (11 pass / 1 skip) all green. Committed by the parent as `14e1d97` (split + test) and `e77ae78` (docs).
- 2026-10-01: feature complete. Branch `develop..HEAD`: 4 commits, 19 files, 627 insertions / 487 deletions (1,114 authored changed lines, ~1,000 of them the verbatim relocation in `14e1d97`). RDD: `gentle-ai review mode status` → off (global), no review run; the Phase 10 note in the overlay doc already records the global off. Delivery budget exceeded by the relocation commit alone; recommendation: two-PR chain (`c2e0c85`+`17077cb` → develop; `14e1d97`+`e77ae78` stacked, `size:exception` for the verbatim move). Push and PRs await the user's OK. After merge the installed plugin cache must be refreshed (installer `-RefreshCache`) for the new hook to take effect.

- 2026-10-01: delivered with explicit user OK as a feature-branch chain: PR #34 `feature/protocol-injection-1-hook` → `develop` (`c2e0c85`..`17077cb`, 48 lines, CI green) and PR #35 `feature/protocol-injection-split` → PR #34 branch (`14e1d97`..HEAD, 1,127 lines, `size:exception` requested for the verbatim relocation; no checks until retargeted to `develop` and closed/reopened). Plugin cache refreshed on the dev machine with `claude plugin uninstall nerv@nerv && claude plugin install nerv@nerv` (`claude plugin update` is a no-op for the directory marketplace): cache now at `5a0baa6`, hook prints 706 bytes, `references/` present. Plugin evals rerun in WSL (Claude Code 2.1.286, 151 s, USD 1.32): 6/6 cases pass with the plugin (overall score 1.0, mean ablation delta 0.39; previous run 3/6, 0.63, 0.27); the without-plugin arms fail exactly where they should (`activation-enabled`, both classification cases).

## Next step

Merge #34, retarget #35 to `develop`, close/reopen it to fire CI, merge. The dev machine already runs the new hook from the branch snapshot; after the merge, refresh the cache again from `develop` so the recorded commit matches.
