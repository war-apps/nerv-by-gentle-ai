# review follow-ups

Branch `fix/review-followups` from `develop` (`3fdcecb`). Started 2026-10-07.
Delivery strategy: `ask-on-risk`. Forecast ~120 authored changed lines: single PR.
TDD: strict (global setting). Runner: `go test ./...` (plus `gofmt -l .` and `go vet ./...`).
RDD: on (decided by global).
Engram mirror: topic `odd/review-followups/tasks`.

## Objective

Close every open follow-up left by the gentle-ai role parity feature: the three advisories of
native review `review-b2a3742af534495c` and the pre-existing RDD narrowing gap.

## Problem / why

User request 2026-10-07: "resolvamos primero todo lo pendiente". Open items:
- R2-role-count-still-hardcoded-in-prose: `plugin/skills/nerv-orchestrator/SKILL.md:125,136`
  repeat the role count (19), which drifts every time a role is added.
- R2-pilots-jd-fix-agent-not-offered-as-from-phase-unexplained: `plugin/commands/configure.md`
  says only the MAGI have a `from_phase`, without saying why the pilots claim `jd-fix-agent`
  but are not offered it.
- R3-001: `.github/workflows/release.yml` captures `nerv release preview` exit codes with
  `&& exit_code=0 || exit_code=$?` because GitHub runs bash with `-e`; nothing guards against a
  regression to the bare `output=$(...)` / `exit_code=$?` form that aborted the job.
- RDD narrowing gap: with RDD on, melchor, balthasar and kaji-security narrow to cross-commit
  concerns assuming native review covered every commit, but native review only runs when due
  and granted, so declined, under-budget or unassessed commits lose those lenses.

## Scope and constraints

- Narrowing becomes evidence-based: `cross-commit` only when native assessment proves the
  round's committed range is already reviewed (or passive); any other result is `full`.
- No change to model resolution, the role catalogue, or the release workflow behavior.
- English artifacts.

## Tasks

- [x] T1 SKILL.md stops hardcoding the role count (route: inline, one mechanical file).
  Commit `267b977`.
- [x] T2 configure.md explains why the pilots have no `from_phase` (route: inline, one file).
  Commit `93835f2`.
- [x] T3 Guard test for the release.yml exit-code capture (route: inline, one new test file).
  `release_workflow_test.go`. Commit `f1d5c7a`.
- [x] T4 Evidence-based RDD narrowing in `pipeline-full.md` and the melchor, balthasar,
  kaji-security scope paragraphs (route: delegated direct, one writer: 4 prose files).
  Also fixed the step-14 table row. Commit `39f19e9`.

- [x] T5 Prose advisories from review `review-bd79021690281844` (route: inline, mechanical):
  status.md role count, docs/configuration.md agent count, kaji.md names melchor.
- [x] T6 Guard-test advisories (route: delegated direct, one writer): narrow the bare
  `exit_code=$?` check to the line after a nerv capture, glob `.yaml`, fix the threshold
  comment; new prose guard that every audit pass list names all six passes.
  `audit_pass_lists_test.go`. Commit `870c4ff`.

## Acceptance criteria

- No role count in orchestrator prose; the pilots' missing `from_phase` is explained.
- A test fails if any `nerv release preview` capture in release.yml loses the errexit-safe form.
- `cross-commit` scope is only launched with native evidence that the range was reviewed.
- `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

## Progress

- 2026-10-07: document written, branch created.
- 2026-10-07: T1, T2 done inline (prose; TDD exception: no meaningful RED; docs guards green).
- 2026-10-07: T3 done inline. Guard passes on current workflows; bite: restoring the bare
  `exit_code=$?` form at release.yml:57 failed with path:line. `gofmt -l .` empty, vet ok.
- 2026-10-07: T4 done by one delegated writer (`39f19e9`). Writer: `go test ./...` ok, vet
  clean, gofmt empty; parent re-ran `go test -count=1 . ./internal/...`: ok. Feature complete.
- 2026-10-07: candidate (base `942774d`, 32 files, 1123 lines) assessed high; user granted.
  Native review `review-bd79021690281844`: approved and acknowledged (authority burned).
  6 non-blocking advisories: R2-status-role-count-still-hardcoded (WARNING),
  R2-docs-agent-count-hardcoded, R2-kaji-third-magi-lens-misnamed,
  R2-release-guard-comment-mismatches-threshold, R3-bare-exit-code-check-overbroad,
  R3-six-pass-batch-unguarded. Folded into T5 and T6; no further review loop on them.
- 2026-10-07: T5 done inline (`0d87150`, prose). T6 done by one delegated writer (`870c4ff`).
  Bites: regressed release.yml:57 failed at :57 and :58; dropping kaji-resilience from each of
  the four pass lists failed once per list. `go test ./...` ok, vet ok, gofmt empty; parent
  re-ran `go test -count=1 .`: ok.
