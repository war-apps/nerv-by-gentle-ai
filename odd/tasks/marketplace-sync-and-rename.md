# Marketplace sync fix and "Nerv by Gentle-AI" display name

Branch `feature/marketplace-sync-and-rename` from `develop` (`ce74539`). Started 2026-10-01.
Delivery strategy: `ask-on-risk` (default). Forecast ~250 authored changed lines, one PR.
TDD: strict (global setting). Runner: `go test ./...` (plus `gofmt -l .` and `go vet ./...`).
Engram mirror: topic `odd/marketplace-sync-and-rename/tasks` (project `nerv`).

## Objective

1. Fix the installer so `claude plugin install nerv@nerv` always resolves the plugin from the
   freshly materialized marketplace at `~/.nerv/marketplace`.
2. Rename the project display name from "NERV Gentle-AI" to "Nerv by Gentle-AI" everywhere.

## Problem / why

- User report 2026-10-01: the cache refresh failed with
  `Plugin "nerv" not found in marketplace "nerv". Your local copy may be out of date`.
- The installer materializes the plugin under `~/.nerv/marketplace` and only writes
  `extraKnownMarketplaces` in `settings.json`. `claude plugin install` resolves marketplaces from
  `~/.claude/plugins/known_marketplaces.json`, which the installer never updates. A stale `nerv`
  entry there (an old checkout or a deleted directory) makes the install fail.
- Verified on 2026-10-01 with a scratch directory marketplace: `claude plugin marketplace add
  <dir>` exits 0 when the marketplace is new, when it is already registered at the same path,
  and when a marketplace with the same name is registered at another path (it replaces the
  path in `known_marketplaces.json`). It also declares the marketplace in user settings.
- User request 2026-10-01: "cambia el nombre del proyecto a \"Nerv by Gentle-AI\" en todos lados".

## Scope and constraints

- The fix runs `claude plugin marketplace add <marketplaceDir>` before the uninstall/install
  pair, in both `Install` and the wizard's `RefreshCache`. A non-zero exit fails the refresh.
- The rename changes the human-readable display name only. Identifiers stay unchanged because
  renaming them breaks installs and links: the GitHub repository slug and raw URLs
  (`war-apps/nerv-gentle-ai`), the Go module path, the plugin and marketplace id `nerv@nerv`,
  the `nerv` binary, the `.nerv` directories, and the Engram project key `nerv`.
- Dated history entries in `odd/tasks/*.md` are records of past work and keep their wording.

## Tasks

- [ ] T1 Installer syncs the `nerv` marketplace through the CLI before the cache refresh
  (route: delegated direct, writer trigger: `internal/install/install.go`,
  `internal/claude/plugincli.go` and their tests).
- [ ] T2 Display name "NERV Gentle-AI" becomes "Nerv by Gentle-AI" in docs, manifests, command
  descriptions and Go doc comments (route: direct inline, mechanical replacement).

## Acceptance criteria

- `go test ./...`, `go vet ./...` and `gofmt -l .` are clean.
- A test proves `claude plugin marketplace add <dir>` runs before `claude plugin install nerv@nerv`
  and that its failure stops the refresh.
- No "NERV Gentle-AI" display name remains outside dated history entries.

## Progress

- 2026-10-01: branch created, document written.

## Next step

T1.
