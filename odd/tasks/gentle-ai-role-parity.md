# gentle-ai role parity

Branch `feat/gentle-ai-role-parity` from `develop` (`eb781cb`). Started 2026-10-06.
Delivery strategy: `ask-on-risk`; chain strategy: stacked to `develop` (user, 2026-10-06). Slices: PR1 = T1+T2, PR2 = T3 stacked on PR1.
Forecast ~560 authored changed lines (T1 ~60, T2 ~300, T3 ~200).
TDD: strict (global setting). Runner: `go test ./...` (plus `gofmt -l .` and `go vet ./...`).
RDD: on (decided by global).
Engram mirror: topic `odd/gentle-ai-role-parity/tasks`.

## Objective

Every capability of the gentle-ai v4 agents has an equivalent NERV role, so running NERV
loses no gentle-ai functionality.

## Problem / why

User request 2026-10-06: "necesito que todos los roles de nerv sean equivalentes con los de
gentle-ai, no se debe perder funcionalidad por parte de gentle-ai".

gentle-ai v4.0.0 ships 8 Claude agents (`internal/assets/claude/agents/` at tag `v4.0.0`):
`jd-fix-agent`, `jd-judge-a`, `jd-judge-b`, `review-readability`, `review-refuter`,
`review-reliability`, `review-resilience`, `review-risk`. The `sdd-*` files present in
`~/.claude/agents` do not come from gentle-ai v4 and are out of scope.

A capability-level comparison (2026-10-06) found:
- MISSING: `review-resilience` (fallbacks, retry/backoff, graceful degradation, rollback or
  fix-forward, latency/load/SLO, observability review) and the performance lens of the
  `jd-judge-*` agents. No audit pass covers them.
- PARTIAL: `review-readability` (misleading names, unexplained constants, complexity, review
  size and context); `review-reliability` (kaji-coverage is bound to the test plan: no invalid
  inputs, failure paths, contracts or regressions outside it); `jd-judge-a/b` (no general
  correctness and edge-case lens).
- COVERED: `review-risk` (kaji-security), `review-refuter` (kaji-refuter), `jd-fix-agent`
  (fix routing through the owning pilot, kaworu and aoba).

Today the gentle-ai `review-*` agents only run through the RDD relay when a review is due;
with RDD off or under budget nothing replaces them.

## Scope and constraints

- Decided approach (user, 2026-10-06): hybrid. One new audit pass `kaji-resilience` covering
  resilience and performance; every other gap closes by extending an existing role's
  `## Role contract`.
- `RoleInfo.GentleAIEquivalent` is a single string, so it cannot record that one role covers
  two gentle-ai agents (for example balthasar: `jd-judge-a` and `review-readability`). The
  catalogue moves to a list, and a guard test asserts each of the 8 gentle-ai v4 agents is
  claimed by at least one role.
- `FromPhase` semantics stay unchanged (only `jd-*` keys, never `review-*`).
- English artifacts; no change to how models resolve.

## Tasks

- [x] T1 Extend existing audit passes (route: delegated direct, one writer; prompt-only).
  balthasar gains the readability lens (misleading names, unexplained business constants,
  complexity, review size and context). kaji-coverage gains reliability beyond the test plan
  (invalid inputs, failure paths, contracts, regressions) plus correctness and edge cases.
  Purpose strings and the docs roles table stay in sync. Commit `0b90206` (+154/-29).
- [x] T2 New audit pass `kaji-resilience` (route: delegated direct, one writer). Agent file
  modeled on kaji-security; resilience plus performance lens. Add to the catalogue (19
  roles, `kaji-passes` group), the five-to-six pass contract in kaji, `pipeline-full.md`,
  `nerv-artifacts.md`, `SKILL.md`, `init.md` models block, status and integration docs, and
  every count test. RDD rule: always full scope (native review runs only when due). Commit
  `b826c89` (+326/-91).
- [x] T3 Catalogue parity guard (route: delegated direct, one writer). `GentleAIEquivalent`
  becomes a list; update `print.go`, the wizard, the docs table and its parser, and pinned
  tests. New test: every gentle-ai v4 agent is claimed by at least one role. Also folds the
  PR1 review advisories that belong to this feature: R2-equivalent-column-understates-coverage,
  R3-parity-guard-deferred, R3-plugin-default-kaji-resilience-unpinned (pin its default model
  and effort), R2-hardcoded-role-count-duplicated (stop repeating the role count in comments),
  R2-kaji-coverage-determinism-category-drift (align the description with `flaky-risk`).
  Branch `feat/gentle-ai-role-parity-guard`, stacked on PR1. Commit `8dc7ad3` (+191/-77).

## Acceptance criteria

- Each of the 8 gentle-ai v4 agents maps to at least one NERV role whose contract covers its
  duties, and a test fails if one is left unclaimed.
- An audit run produces six passes, including `kaji-resilience`.
- `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

## Progress

- 2026-10-06: gap map done (read-only explorer), sdd scope verified against the gentle-ai
  v4.0.0 tree, branch created, document written.
- 2026-10-06: T1 done by one delegated writer (`0b90206`). balthasar adds categories naming,
  magic-value, complexity, intention, review-context (WARNING cap unless a proven correctness
  failure). kaji-coverage adds invalid-input, failure-path, contract, boundary, regression,
  low-value-test, logic-error, edge-case, concurrency, under the same candidate-causal bar.
  Audit JSON has no category field, so kaji.md is unchanged. TDD exception: prompt prose, no
  meaningful RED; the purpose guard `TestDocsRolesTable_MatchesCatalogue` passes.
  Checks: `go test ./...` ok, `go vet ./...` clean, `gofmt -l .` empty (writer; parent re-ran
  `go test ./...`: ok).
- 2026-10-06: T2 done by one delegated writer, plus a parent fix of four stale "five passes"
  lines in kaji-security/kaji-coverage (outside the writer's surfaces), amended into `b826c89`.
  RED: TestRoles, TestRoles_GroupMembership, TestPrint_ModelsRowsCarryPurposeAndEquivalent,
  TestPluginDefaults_RealEmbeddedFS_19RolesAobaSonnetLow,
  TestRun_ModelsSection_TableShowsPurposeEquivalentAndGroupLegend, then the docs guards.
  GREEN: `go test ./...` ok (22 packages), `go vet` clean, `gofmt -l .` empty (parent re-ran
  all three after the amend).
- Follow-up for the user (pre-existing, not in scope): with RDD on, melchor, balthasar and
  kaji-security narrow to cross-commit concerns, assuming native review covered each commit;
  native review only runs when due and granted.
- 2026-10-06: PR1 slice (T1+T2, base `eb781cb`, 684 lines) assessed `high` (hot_path), review
  due. User granted consent. Native review `review-4eee5199d25dfb73`: four lenses, risk and
  resilience 0 findings, readability 3 and reliability 2 advisories (non-blocking), state
  approved, acknowledged (`gentle-ai.review-acknowledged/v1`). Reviewed boundary is now
  `8e0163c`. Advisories are folded into T3.
- 2026-10-06: T3 done by one delegated writer (`8dc7ad3`). `GentleAIV4Agents` (8 agents) and
  `GentleAIEquivalents []string`; balthasar adds review-readability, the five pilots claim
  jd-fix-agent. `gentleai_parity_test.go`: every agent claimed, every claim real, list exact.
  JSON adds `gentle_ai_equivalents`; `gentle_ai_equivalent` stays joined (read by
  `plugin/commands/status.md`). RED: build failures on the new identifiers plus print/wizard/
  docs guards. Bite: dropping balthasar's review-readability and misspelling review-risk each
  failed with the agent/role named. GREEN: `go test ./...` ok, vet clean, gofmt empty (parent
  re-ran the config and root packages with -count=1: ok). All 8 agents are claimed.
- 2026-10-06: PR2 slice (base `8e0163c`, 290 lines) assessed `medium`, `under_budget`: no
  review due. Feature complete; push and PRs await the user.
- 2026-10-06: the stop hook raised the selectorless candidate (base `942774d`, 29 files, 960
  lines: PR1, T3, and #73/#74 already on develop). The user granted consent. Native review
  `review-b2a3742af534495c`: risk and resilience 0 findings, 3 advisories (non-blocking),
  approved and acknowledged. Advisories left as follow-ups: R2-role-count-still-hardcoded-in-prose
  (`SKILL.md:125`, WARNING), R2-pilots-jd-fix-agent-not-offered-as-from-phase-unexplained
  (`configure.md:109`), R3-001 (`release.yml:56-57`, from #74).
