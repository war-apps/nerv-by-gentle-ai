# Audit consolidation: fewer kaji agents, independent equivalents

## Objective

Cut the `kaji-*` audit agents from four to two and map gentle-ai v4
equivalents so no role audits its own work and the compiler stays neutral.

## Why

User decision (2026-10-08), after rejecting mappings that made plan or
test-plan authors judge their own output (misato, ritsuko) or made the
compiler judge the findings it compiles (kaji).

## Scope

| Role | Equivalents | from_phase | Change |
|---|---|---|---|
| melchor | `jd-judge-b`, `review-risk` | `jd-judge-b` | unchanged |
| balthasar | `jd-judge-a`, `review-readability` | `jd-judge-a` | readability audit lens moves back from casper |
| casper | none | none | drops readability; process lens only |
| fuyutsuki | `review-refuter` | none | gains a read-only refute mode; absorbs `kaji-refuter` |
| kaji-audit (new) | `review-reliability`, `review-resilience` | none | merges `kaji-coverage` and `kaji-resilience` |
| kaji | none | none | neutral compiler, unchanged duty |

Removed roles: `kaji-coverage`, `kaji-resilience`, `kaji-refuter`. The audit
runs four passes: `kaji-audit`, `melchor`, `balthasar`, `casper`.

## Constraints

- Every gentle-ai v4 agent stays claimed by at least one role.
- `fuyutsuki`'s refute mode is read-only by contract: no writes, no
  `mem_save`, no new findings (same rules as `kaji-refuter`).
- `kaji-audit` keeps every check of both merged passes and always runs at
  full RDD scope.
- History (other features' odd docs, released CHANGELOG entries) is not
  rewritten.

## Tasks

- [x] T1 — catalogue, role definitions (agent files) and every test that
  pins them (route: delegated, writer trigger: 2+ non-trivial files).
  Commit `868da54`. Also carries the prose the root tests pin: the docs
  roles table, the `/nerv:configure` legend, the init models template,
  the step-14 row and pass-batch paragraph, the RDD narrowing paragraph,
  the audit-pass launch paragraph in `nerv-artifacts.md`, and Kaji's
  compile paragraph.
- [x] T2 — orchestration, artifact, command, docs, bench prose and
  CHANGELOG (route: delegated, same writer). Commit `f8962a3`.

## Acceptance criteria

- `go test ./...`, `go vet ./...` pass; `gofmt -l .` clean.
- No live reference to the three removed roles outside history.

## Delivery

Branch `feat/audit-consolidation` stacked on `feat/role-realignment`
(PR #79). Forecast about 500 authored changed lines (two agent files
deleted, one created from their merge). Strategy: `single-pr`.

## Progress

Branch created from 2354a65.

- T1 RED: after updating the catalogue tests (role count 16, `kaji-passes`
  = `kaji`, `kaji-audit`, equivalents, from_phase, four derived audit
  passes, RDD always-full clause naming `kaji-audit`), `go test ./...`
  failed in the root package (`TestAuditPassLists_NameAllFourPasses`,
  `TestRDDNarrowing_FailsClosed`), `internal/config` (`TestRoles`,
  `TestRoles_EveryRoleHasPurposeAndEquivalent`,
  `TestRoles_FromPhaseIsAGentleAIPhaseKey`, `TestRoles_GroupMembership`),
  `internal/configure`, `internal/models` and `internal/wizard`.
- T1 GREEN: catalogue, agent files and pinned prose updated; `go test
  ./...`, `go vet ./...` pass and `gofmt -l .` is clean.
- T2: `go test ./...`, `go vet ./...` pass, `gofmt -l .` clean; the
  removed role names survive only in CHANGELOG and in the tests' explicit
  absence guards.
- Size: `git diff --shortstat 2354a65..HEAD` (before this document) =
  27 files changed, 491 insertions(+), 696 deletions(-). T1: 409+/633-
  (two agent files deleted, one created from their merge); T2: 82+/63-.

## Next step

Native review assessment of the work-unit commits, then the user decides
on push and PR (stacked on PR #79).
