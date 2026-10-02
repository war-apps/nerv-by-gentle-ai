# Uninstall removes the `nerv` marketplace registration

Branch `feature/uninstall-marketplace-remove` from `develop` (`66e444a`). Started 2026-10-02.
Delivery strategy: `ask-on-risk` (default). Forecast ~120 authored changed lines, one PR.
TDD: strict (global setting). Runner: `go test ./...` (plus `gofmt -l .` and `go vet ./...`).
Engram mirror: topic `odd/uninstall-marketplace-remove/tasks`.

## Objective

`nerv uninstall` runs `claude plugin marketplace remove nerv`, so no stale `nerv` entry is left
in `~/.claude/plugins/known_marketplaces.json` pointing at the deleted `~/.nerv/marketplace`.

## Problem / why

- Native review finding R3-uninstall-leaves-registered-marketplace (lineage
  `review-c67eed7e4e5face9`, 2026-10-01): since PR #39, install registers the marketplace with
  `claude plugin marketplace add`, but uninstall never removes it. A stale entry of that kind was
  the root cause of the original `Plugin "nerv" not found in marketplace "nerv"` failure.
- User request 2026-10-02: fix it as a new feature.
- Verified 2026-10-02 (Claude Code CLI): `claude plugin marketplace remove <name>` for an
  unknown name exits 1 with `✘ Failed to remove marketplace: Marketplace '<name>' not found`.

## Scope and constraints

- New `claude.PluginCLI.RemoveMarketplace(ctx, name)`: runs `claude plugin marketplace remove
  <name>`; a non-zero exit whose output mentions "not found" (case-insensitively) is tolerated
  as success; any other non-zero exit or launch failure is an error carrying the output.
- `install.Uninstall` calls it after `claude plugin uninstall nerv@nerv` and before removing the
  materialized directory; a real failure stops the uninstall with an error.
- Out of scope: the other advisory findings of that review (go install hint, task-doc note,
  RefreshCache ordering assertions).

## Tasks

- [x] T1 Uninstall removes the marketplace registration (route: delegated direct, writer
  trigger: `internal/claude/plugincli.go`, `internal/install/uninstall.go` and their tests).
  RED commit, then GREEN commit.

## Acceptance criteria

- A test proves uninstall runs `claude plugin marketplace remove nerv` after the plugin
  uninstall, tolerates "not found", and fails on any other error.
- `go test ./...`, `go vet ./...` and `gofmt -l .` are clean.

## Progress

- 2026-10-02: branch created, document written.
- 2026-10-02: T1 done. RED `a8ba9ff` (observed: `cli.RemoveMarketplace undefined`; uninstall
  made no `marketplace remove` call; uninstall returned nil on a remove failure). GREEN `5da904f`:
  `PluginCLI.RemoveMarketplace` (tolerates "not found") and the call in `Uninstall` between the
  plugin uninstall and the directory removal. `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

- 2026-10-02: native review (four lenses, lineage `review-53f835f751a541aa`, preflight range from
  `main`) approved and acknowledged. Three lenses independently flagged the loose "not found"
  match (a `command not found` or missing settings file would pass as success and leave the
  stale entry). Accepted as in-scope hardening: RED `test(install): require the exact marketplace
  not-found message and a re-run hint` (observed: unrelated "not found" returned nil; error lacked
  the re-run hint), GREEN matches `Marketplace '<name>' not found` and the uninstall error says
  to re-run `nerv uninstall`; `uninstall_test.go` reuses `callLines`. `go test ./...`,
  `go vet ./...`, `gofmt -l .` clean. Not taken (advisory): directory removal on a registry
  failure (re-run already finishes the job), the task-doc note and RefreshCache ordering
  assertions from the earlier review.

## Next step

PR to `develop`.
