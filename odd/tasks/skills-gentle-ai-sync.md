# Installer provides missing gentle-ai skills; RefreshCache ordering tests

Branch `feature/skills-gentle-ai-sync` from `develop` (`49df98b`). Started 2026-10-02.
Delivery strategy: `ask-on-risk` (default). Forecast ~250 authored changed lines, one PR.
TDD: strict (global setting). Runner: `go test ./...` (plus `gofmt -l .` and `go vet ./...`).
Engram mirror: topic `odd/skills-gentle-ai-sync/tasks`.

## Objective

1. When a skill of kind `gentle-ai` from `plugin/skills-manifest.json` is missing, the skills
   step runs `gentle-ai sync` itself instead of only printing a remedy.
2. Close the advisory review gap: `RefreshCache` tests prove the marketplace add runs before the
   uninstall/install pair and that its failure stops the refresh.

## Problem / why

- 2026-10-01: `nerv install` printed "Remedy: run 'gentle-ai install' (or 'gentle-ai sync') to
  provide gentle-ai skill 'branch-pr'." User request 2026-10-02: "que ya lo resuelva el
  instalador".
- Verified 2026-10-02: the gentle-ai 3.7.0 binary embeds `skills/branch-pr/SKILL.md` (and the
  other manifest gentle-ai skills). gentle-ai has no skills-only command; `gentle-ai sync`
  accepts `--agents` and `--skills` filters but still runs its managed components.
- Review finding R3-refreshcache-order-unasserted (lineages `review-c67eed7e4e5face9`,
  `review-53f835f751a541aa`, `review-7d95d74a622ae2cb`): only `Install` asserts the ordering.

## Scope and constraints

- `skills.Run` (shared by `nerv install`, the wizard and `nerv skills`): after the npx installs,
  when gentle-ai skills are missing and not DryRun, run once
  `gentle-ai sync --agents claude-code --skills <comma-separated missing names>` through the
  runner, then recompute the status of those skills. Remedies stay only for skills still missing
  afterwards (sync failed, gentle-ai absent, or the skill not provided). A sync launch error or
  non-zero exit never fails the run; it leaves the remedies in place.
- DryRun reports the planned sync instead of running it.
- Report carries whether the sync ran and its outcome so callers can print one line about it.
- Docs that describe the skills step follow the behavior.
- Out of scope: `go install` hint window (next release), making the repository public.

## Tasks

- [x] T3 (added 2026-10-02, user request) herdr auto-open focuses the new worktree's workspace:
  `--focus` instead of `--no-focus`, plus `herdr workspace focus <workspace_id>` when
  `already_open` (route: direct inline, one procedure file).
- [x] T4 (added 2026-10-02, review hardening) refresh gentle-ai statuses after the sync, surface
  the sync diagnostic, test caller rendering (route: delegated direct).

- [x] T1 RefreshCache tests assert marketplace add before uninstall/install and fail-stop on an
  add failure (route: delegated direct, part of the same writer; test-only, characterization of
  existing behavior, so GREEN on first run is expected and recorded).
- [x] T2 Skills step runs `gentle-ai sync` for missing gentle-ai skills (route: delegated direct,
  writer trigger: `internal/skills`, `internal/install`, `internal/wizard`, `cmd/nerv` and tests).
  RED commit, then GREEN commit.

- [x] T5 (added 2026-10-02, review R1-001/R4-001 + user decision "Preguntar si es interactivo")
  the sync asks first: interactive callers (stdin is a terminal: `nerv install`, the wizard,
  `nerv skills`) confirm before running it, naming the skills and warning that gentle-ai sync
  rewrites its managed files (e.g. `~/.claude/CLAUDE.md`); without a terminal it never runs and
  the remedy stays. Also: one shared renderer for the sync result (R2-001), a wizard rendering
  test (R3), document the positional refresh assumption (R2-002), and docs describe the prompt
  and its side effect (R3-sync-side-effects-undocumented). Route: delegated direct.

## Acceptance criteria

- Tests prove: sync runs once with the exact args only when gentle-ai skills are missing; a skill
  present after sync has no remedy; a failed sync keeps the remedies and does not fail the run;
  DryRun never runs sync; RefreshCache ordering and fail-stop.
- `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

## Progress

- 2026-10-02: branch created, document written.
- 2026-10-02: T1 `84b44fc` — RefreshCache ordering assertion plus
  `TestRefreshCache_MarketplaceAddFailure_StopsBeforeInstall`; GREEN on first run, as expected for
  a characterization test.
- 2026-10-02: T2 RED `4fe80ef` (observed: build failure, `report.Sync` / `report.Plan.Sync`
  undefined), GREEN `baa9639`: `SyncStep`/`SyncArgs`/`Plan.Sync` computed in `InstallPlan`;
  `Sync()`/`SyncResult` executed by `Run` after the npx installs; remedies recomputed from disk
  after a sync; DryRun plans without running. Install, wizard and `nerv skills` print the sync
  line and a warning on failure; README (install step 7, wizard required skills) and
  `docs/commands.md` updated. `go test ./...`, `go vet ./...`, `gofmt -l .` clean.
- 2026-10-02: smoke test on the dev machine: `go run ./cmd/nerv skills --dry-run` showed the
  `branch-pr` remedy and `DryRun: gentle-ai sync --agents claude-code --skills branch-pr`; the
  real run executed the sync and ended with 0 gentle-ai gaps; `~/.claude/skills/branch-pr/SKILL.md`
  now exists. Side effect, as expected from gentle-ai sync: it rewrote its managed files
  (e.g. `~/.claude/CLAUDE.md`).

- 2026-10-02: native review of `49df98b..1da3e5e` (medium, reliability lens, lineage
  `review-709bffd4b1c13f97` slice) approved and acknowledged; warnings accepted as T4: stale
  `Report.Statuses` after sync, discarded sync output, untested warning rendering (the `Only`
  remedy recompute was already correct; an assertion now proves it).
- 2026-10-02: T3 `bf50ab2` — `plugin/skills/nerv-tasks/providers/teamwork/procedures/start.md`
  step g. Verified flags with `herdr worktree open --help` (`--focus`, `--no-focus`) and
  `herdr workspace focus --help`.
- 2026-10-02: T4 RED `8b35022` (observed: `report.Sync.Output undefined`; caller warnings without
  a diagnostic line), GREEN `ea6eb28`: `refreshGentleAI` refreshes gentle-ai statuses and remedies;
  `SyncResult.Output` (last two non-empty stderr/stdout lines or the launch error) printed by the
  three callers. Wizard rendering untested (its tests skip skills). `go test ./...`,
  `go vet ./...`, `gofmt -l .` clean.

- 2026-10-02: native review of the branch (high, four lenses, lineage `review-bb8c0dd3ef748cc3`)
  approved and acknowledged. R1/R4 flagged the unprompted sync rewriting gentle-ai managed user
  files; the user chose "ask when interactive" → T5.

- 2026-10-02: T5 RED `be14117` (observed: build failures, `ConfirmSync` undefined in
  `skills.Options` and `install.Deps`; caller and wizard tests failing), GREEN `4871a2e`:
  `skills.Options.ConfirmSync`, `SyncResult.Declined`, shared `skills.SyncPrompt` and
  `skills.RenderSync` (three copies removed), `syncConfirmer` in `cmd/nerv/terminal.go` (nil
  without a terminal; byte-wise stdin read, default No), wizard prompt via `yesNo`, positional
  refresh assumption documented, README and `docs/commands.md` updated. Follow-up: `--json`
  never prompts. Route: delegated direct.

## Next step

T5, then PR to `develop`; after the merge, refresh the plugin cache so
the herdr focus change reaches the installed plugin.
