# Role realignment: one gentle-ai equivalent per NERV role

## Objective

No two NERV roles share a gentle-ai v4 equivalent, except the pilots, which
share `jd-fix-agent` by layer split.

## Problem

`casper` and `balthasar` both claim `jd-judge-a`, and `kaji-security` duplicates
the security lens `melchor` already carries.

## Why

Two judges exist in gentle-ai v4 (`jd-judge-a`, `jd-judge-b`): one MAGI member
each. Security belongs to a single owner.

## Scope (authorized by the user, 2026-10-07)

- Remove the `kaji-security` role; `melchor` absorbs its audit pass and its
  `review-risk` equivalent.
- `balthasar`: `jd-judge-a` only (from_phase `jd-judge-a`); drops the
  readability lens.
- `melchor`: `jd-judge-b` and `review-risk` (from_phase `jd-judge-b`).
- `casper`: drops `jd-judge-a` and its from_phase; gains `review-readability`
  and the readability audit lens.
- The audit runs five passes instead of six.

## Constraints

- Every gentle-ai v4 agent stays claimed by at least one role.
- Historical `odd/tasks/*.md` of other features and released CHANGELOG
  entries are not rewritten.

## Tasks

- [x] T1 — config catalogue, Go tests and the role definitions that mirror
  it (route: delegated, writer trigger: 2+ non-trivial files). Commit
  2f686de. Scope grew from the plan: `TestRoles_CatalogueMatchesPluginAgents`
  and `internal/models` pin the agent files, and the root prose tests pin
  the docs roles table, the `/nerv:configure` legend and the derived pass
  count, so the agent deletion, melchor's security absorption, the
  balthasar to casper readability move and those mirrors landed here to
  keep the commit green.
- [x] T2 — orchestration, artifact, command, docs and bench prose plus the
  RDD narrowing prose test and the changelog (route: delegated, same
  writer). Commit e344147.

## Acceptance criteria

- `go test ./...` and `go vet ./...` pass; gofmt is clean.
- No reference to `kaji-security` remains outside history (other features'
  odd docs, released CHANGELOG entries).

## Delivery

Forecast: about 500 authored changed lines, most of them the deletion of
`plugin/agents/kaji-security.md` and moved prose. Strategy: `single-pr`,
stacked on `fix/review-followups` (PR #78) because both touch the audit pass
lists.

## Progress

Branch `feat/role-realignment` created from f59c5e7.

- T1 (2f686de): RED observed with `go test ./internal/...` after updating
  the catalogue tests (roles, print, wizard, group membership); GREEN after
  `internal/config/models.go`. 188 insertions, 320 deletions (most of the
  deletions are `plugin/agents/kaji-security.md`).
- T2 (e344147): RED observed on `TestRDDNarrowing_FailsClosed` (forbidden
  `kaji-security` in the cross-commit clause) before the pipeline prose
  changed; GREEN after. 85 insertions, 72 deletions.
- Total `git diff --shortstat f59c5e7..e344147`: 30 files, 273
  insertions, 392 deletions.

## Verification

- `go test ./...`: all packages ok.
- `go vet ./...`: clean.
- `gofmt -l .`: no output.
- Residual `kaji-security` references: released CHANGELOG entries, the new
  Unreleased removal note, this document, and three absence guards in
  tests (`rdd_narrowing_prose_test.go`, `print_test.go`, `wizard_test.go`).

## Findings

A leftover `models.kaji-security` override in `nerv.yaml` does not error:
`nerv configure --print` and the wizard show it as a row with no purpose,
`nerv apply-models` skips it (no agent file), and `--set-model` for another
role preserves it. `nerv configure --set-model kaji-security=default`
refuses it as an unknown role, so it can only be removed by hand. Left as
a documented gap (CHANGELOG), not changed.

## Next step

Native review of the work-unit commits per RDD, then the user's delivery
decision (PR stacked on `fix/review-followups`).
