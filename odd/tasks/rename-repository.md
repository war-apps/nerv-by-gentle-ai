# Repository rename to `war-apps/nerv-by-gentle-ai`

Branch `feature/rename-repository` from `develop` (`868440d`). Started 2026-10-01.
Delivery strategy: `ask-on-risk` (default). Forecast ~330 authored changed lines, all mechanical
slug replacement, one PR.
TDD: strict (global setting). The change is a rename with no new behavior, so no RED test;
checks: `go build ./...`, `go test ./...`, `go vet ./...`, `gofmt -l .`,
`bash tests/install-sh.test.sh`.
Engram mirror: topic `odd/rename-repository/tasks`.

## Objective

Rename the GitHub repository from `war-apps/nerv-gentle-ai` to `war-apps/nerv-by-gentle-ai`,
matching the "Nerv by Gentle-AI" display name, and move every reference with it.

## Problem / why

- User request 2026-10-01, after the display-name rename (PR #39): "cambia tambien el nombre del
  repositorio", then "ok, cambia todo" for the slug `nerv-by-gentle-ai`, the Go module path and
  every URL.
- GitHub redirects the old repository name for clones, the API and raw URLs, but the Go module
  path has no redirect: after this change, `go install .../nerv-gentle-ai/cmd/nerv@latest` fails
  the module path check until users switch to the new path.

## Scope and constraints

- Changes: `go.mod` module path and every Go import, the version ldflag in `.goreleaser.yaml` and
  `internal/version`, the goreleaser release target, `scripts/install.sh` defaults and comments,
  the install test fixtures, README, CONTRIBUTING and `/nerv:configure` URLs.
- Unchanged: the plugin and marketplace id `nerv@nerv`, the `nerv` binary, the `.nerv`
  directories, the Engram project key `nerv`, and dated history in `odd/tasks/` and the changelog.
- The local clone folder keeps its name; only its `origin` remote URL changes.
- The GitHub rename runs after the PR merges, so `develop` already carries the new paths.

## Tasks

- [ ] T1 Replace the slug in module path, imports, release config, scripts, tests and docs
  (route: direct inline, mechanical `war-apps/nerv-gentle-ai` → `war-apps/nerv-by-gentle-ai`
  replacement plus the `cd` line in CONTRIBUTING).
- [ ] T2 Deliver: PR to `develop`, CI green, merge.
- [ ] T3 Rename the GitHub repository, update the local `origin` remote, refresh the plugin
  cache from `develop`.

## Acceptance criteria

- No `nerv-gentle-ai` slug remains outside dated history.
- All checks above are clean; CI on the PR is green.
- `gh repo view war-apps/nerv-by-gentle-ai` resolves and `git remote -v` points at it.

## Progress

- 2026-10-01: branch created, document written.

## Next step

T1.
