# gentle-ai v4 compatibility, phase 1

Branch `feature/gentle-ai-v4-compat` from `develop` (`5b43239`). Started 2026-10-05.
Delivery strategy: `ask-on-risk`; chain strategy: stacked PRs to `develop` (user choice 2026-10-05).
Forecast: phase 1 266 lines, phase 2 ~700 more (over budget, so chained).

Slices (each branch stacks on the previous one):
1. `feature/gentle-ai-v4-compat`: T1-T2 (phase 1, committed and reviewed).
2. `feature/gentle-ai-v4-spec-compose`: T4 (`nerv spec-compose` port). Likely `size:exception`, because the port is ~350 lines plus tests.
3. `feature/gentle-ai-v4-sdd-retire`: T3, T5, T6, T7.
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

### Phase 2 (user approved 2026-10-05)

- [x] T3 (commit 84e15d9) `/nerv:init` writes `openspec/config.yaml` itself (context, `strict_tdd` with the v3 `sdd-init` rule, testing runner) and runs `gentle-ai skill-registry refresh`; no `sdd-init` delegation. `docs/commands.md:39` follows. (slice 3)
- [x] T4 (commits 20d00bc, a226117) `nerv spec-compose --canonical --delta [--output]`: Go port of gentle-ai v3.7.0 `internal/sddstatus/openspec_archive_compose.go` (MIT, attributed), with an atomic write and tests. Aoba's archive step, `pipeline-full.md`, `troubleshooting.md` and `bench/journeys.md` use it. (slice 2; user chose a subcommand over prompt-level merging)
- [x] T5 (commit 99705c2) Role catalogue equivalents: keep jd-judge-a/b; kaji-security goes to review-risk, kaji-refuter to review-refuter, kaji-coverage to review-reliability; the rest become `none`. Docs, guard tests and the status/configure prose follow. (slice 3)
- [x] T6 (commit ce7c38f) Orchestrator prose: drop the SDD claims (`SKILL.md:25-47`, `docs/integration.md` reuse map); strict TDD becomes nerv-owned, read from `openspec/config.yaml`; "gentle-ai's own" artifact wording becomes nerv-owned. (slice 3)
- [x] T7 (commits 97d6693, 4be1f69, 4843997) Review advisories: a 2.x install case, a wizard warning test, and install prints the same 4.x note as `/nerv:status`; spec-compose rejects a requirement name repeated within one delta section; atomicfile.Write resolves symlinks. (slice 3)

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

- 2026-10-05: The stop hook surfaced the unreviewed slice (base `2db2c5c`, which also contains the earlier `155335d`/`a79d711`/`da75102` commits). The user granted consent. The native review ran one lens, reliability, and was approved. Lineage `review-bac6cb562d314407` was acknowledged and its authority burned. Advisory findings, all non-blocking:
  - R3-v4-gate-activates-unmigrated-paths (WARNING): on 4.x the preflight reports OK even though `/nerv:init` and the Aoba archive still call removed SDD commands. The CLI prints no warning. Phase 2 closes this; an interim install/wizard note is an option.
  - R3-install-gate-cases-incomplete: the install-level table lacks a 2.x case, and the wizard's "3 or 4" warning text is untested.
  - R3-status-4x-note-not-mirrored: install still prints "tested against 3.7.0" for 4.x, while `/nerv:status` prints an "in progress" note.
  - R3-width-assertion-dropped: from the earlier slice (`155335d`, PR #60), not this feature. This was a deliberate change; left as is.
- 2026-10-05: T4 done (route: delegated direct; trigger: 2+ non-trivial files). RED against a stub: `internal/specs` ported cases failed (21 FAIL lines incl. subtests; stub returns empty output and nil error), `internal/atomicfile` Write 3/3 failed, `cmd/nerv` spec-compose tests failed (13 FAIL lines incl. subtests). GREEN after the port. gofmt, go vet, go test ./... clean; hook suites 12/12 and 17/17. `rg sdd-archive-compose` outside odd/ and CHANGELOG now only hits SKILL.md:32 (T6). Slice diff vs bcdaf08: 13 files, +1203/-10 (`size:exception`: faithful port plus tests). Decisions: usage errors exit 2, an unapplied delta or missing input exits 1 (refusal), unwritable output exits 2; the atomic write is a new `atomicfile.Write` (no backup, keeps mode).

- 2026-10-05: Slice 2 delivered 1205 insertions. That is `size:exception`: the faithful port (376) plus its tests (360+226+70) cannot be split cohesively under 400. RDD assess from `8362cd8` returned medium, due (slice_budget_reached). The user granted consent. Native review lineage `review-811ae6f71f92f6a8` ran the reliability lens and was approved and acknowledged. Advisories, carried to T7:
  - R3-duplicate-modified-silently-last-wins (WARNING): two MODIFIED entries with the same name fold into one, silently, and `docs/commands.md` promises an error.
  - R3-atomic-write-replaces-symlink: an `--output` symlink is replaced by a regular file.
  - Writer decision gaps accepted: a missing flag exits 2 (consistent with `skills`); a new output file is created 0644; Aoba assumes `nerv` is on PATH, consistent with `/nerv:configure`.

- 2026-10-05: Slice 3 done (T5, T3, T6, T7; route: delegated direct, one writer). RED: T5 `TestRoles_EveryRoleHasPurposeAndEquivalent` (14 role mismatches), `TestPrint_ModelsRowsCarryPurposeAndEquivalent`, wizard models table test; T7 `TestInstall_GentleAIVersionNote/4.x` lacked "4.x support is in progress"; `TestComposeRefusesRequirementRepeatedWithinOneDeltaSection` (MODIFIED folded silently, ADDED/REMOVED had the wrong reason); `TestWrite_ThroughASymlinkUpdatesTheTargetAndKeepsTheLink` (link replaced by a regular file). The 2.x install case and the wizard "3 or 4" warning test pass on first run (coverage gaps only). gofmt, go vet, go test ./... clean; hook suites 12/12 and 17/17. Remaining `sdd-` hits outside odd/ and CHANGELOG: init.md:94 (historical note that 4.x removed `sdd-init`) and a negative assertion in wizard_test.go. Decisions: "none" equivalent stays the empty string; config.yaml is context, strict_tdd, testing.runner, rules.apply.test_command.

## Next step

Phase 1 is complete locally. Next: PR to `develop` (user decision), then phase 2 (`/nerv:init` without `sdd-init`, Aoba archive without `sdd-archive-compose`, role catalogue `sdd-*` equivalents, orchestrator prose).
