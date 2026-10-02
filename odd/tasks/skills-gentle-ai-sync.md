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

- [ ] T1 RefreshCache tests assert marketplace add before uninstall/install and fail-stop on an
  add failure (route: delegated direct, part of the same writer; test-only, characterization of
  existing behavior, so GREEN on first run is expected and recorded).
- [ ] T2 Skills step runs `gentle-ai sync` for missing gentle-ai skills (route: delegated direct,
  writer trigger: `internal/skills`, `internal/install`, `internal/wizard`, `cmd/nerv` and tests).
  RED commit, then GREEN commit.

## Acceptance criteria

- Tests prove: sync runs once with the exact args only when gentle-ai skills are missing; a skill
  present after sync has no remedy; a failed sync keeps the remedies and does not fail the run;
  DryRun never runs sync; RefreshCache ordering and fail-stop.
- `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

## Progress

- 2026-10-02: branch created, document written.

## Next step

T1, T2.
