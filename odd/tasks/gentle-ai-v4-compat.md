# gentle-ai v4 compatibility, phase 1

Branch `feature/gentle-ai-v4-compat` from `develop` (`5b43239`). Started 2026-10-05.
Delivery strategy: `ask-on-risk`. Forecast ~200 authored changed lines, one PR.
TDD: strict (global setting). Runner: `go test ./...` (plus `gofmt -l .` and `go vet ./...`).
Engram mirror: topic `odd/gentle-ai-v4-compat/tasks`.

## Objective

Let NERV run against gentle-ai 4.x without breaking 3.x users (phase 1 of the v4 upgrade plan).

## Problem / why

User request 2026-10-05: check gentle-ai v4.0.0 compatibility and plan the upgrade; phase 1
accepted ("si"). gentle-ai v4.0.0 retires SDD/OpenSpec and its CLI subcommands. Today:
- `internal/gentleai/gentleai.go:64` accepts only major 3, so `nerv install` refuses 4.x and
  `/nerv:status` and the wizard report it as unsupported.
- `gentle-ai sdd-status` is called by `/nerv:status` and the resume procedure; v4 removed it.

Later phases (not in this feature): replace `sdd-init` in `/nerv:init`, the Aoba archive step
(`sdd-archive-compose`), role catalogue `sdd-*` equivalents, orchestrator prose (SKILL.md:32,
docs/integration.md diagram), then upgrade the local gentle-ai and release.

## Scope and constraints

- Accept majors 3 and 4 during the transition. "Tested against" stays 3.7.0 until phase 3
  validates 4.0.0 on this machine.
- Status/resume read NERV's own artifacts directly so they behave the same on 3.x and 4.x.
- No change to RDD relay commands (unchanged in v4).

## Tasks

- [x] T1 (commit 484f00d) Version gate accepts majors 3 and 4: `internal/gentleai`, install refusal/warnings,
  wizard, `cmd/nerv/install.go` help, `plugin/commands/status.md` step, README, CONTRIBUTING,
  `docs/commands.md` (route: delegated direct; trigger: 2+ non-trivial files).
- [x] T2 (commit c38f182) Drop `gentle-ai sdd-status`: `plugin/commands/status.md`, `references/resume.md`,
  `_shared/nerv-artifacts.md`, `bench/journeys.md` read the change artifacts directly
  (route: delegated direct, same writer).

## Acceptance criteria

- `gentle-ai 4.0.0` and `3.7.0` both yield `OK: true`; `2.x` and `5.x` are refused with a message
  naming the supported majors. Tests prove it.
- No `sdd-status` call remains outside `odd/`, `CHANGELOG.md`, and phase-2 prose.
- `go test ./...`, `go vet ./...`, `gofmt -l .` clean; hook test suites unchanged.

## Known environmental failures

- `tests/install-sh.test.sh`: all 17 cases fail with exit 127 on this machine at the base
  commit (`5b43239`). The PATH-stripping helper removes the directory that also holds `bash`.
  Pre-existing; out of scope.

## Progress

- 2026-10-05: branch created, baseline green except the known environmental failure above.
- 2026-10-05: T1 done. RED: `TestCheckPreflight_MajorGate/major_4` got OK:false want OK:true; `TestInstall_RequireGentleAI_MajorGate/4.x_passes` refused 4.0.0. GREEN after the gate change; gofmt, go vet, go test clean.
- 2026-10-05: T2 done. Status, resume, nerv-artifacts and bench journeys read change artifacts directly; remaining `sdd-status` mentions are phase-2 only (SKILL.md:32, docs/integration.md).
- 2026-10-05: RDD assess over 5b43239..8b61a5d (committed-only): risk medium, review_due false (under_budget, 216 authored lines); review stays pending in the slice. Parent spot check: `go test ./internal/gentleai/ ./internal/install/ -count=1` ok.

## Next step

Phase 1 is complete locally. Next: PR to `develop` (user decision), then phase 2 (`/nerv:init` without `sdd-init`, Aoba archive without `sdd-archive-compose`, role catalogue `sdd-*` equivalents, orchestrator prose).
