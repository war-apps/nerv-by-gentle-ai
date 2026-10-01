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
- The GitHub rename runs before the PR merges (review finding R4-rename-ordering-window): GitHub
  redirects old to new, never new to old, so renaming first keeps both the old `develop`/`main`
  content and the new paths resolving.

## Tasks

- [x] T1 Replace the slug in module path, imports, release config, scripts, tests and docs
  (route: direct inline, mechanical `war-apps/nerv-gentle-ai` → `war-apps/nerv-by-gentle-ai`
  replacement plus the `cd` line in CONTRIBUTING).
- [x] T2 Rename the GitHub repository and update the local `origin` remote (reordered before
  delivery after review).
- [x] T3 Deliver: PR to `develop`, CI green, merge; refresh the plugin cache from `develop`.

## Acceptance criteria

- No `nerv-gentle-ai` slug remains outside dated history.
- All checks above are clean; CI on the PR is green.
- `gh repo view war-apps/nerv-by-gentle-ai` resolves and `git remote -v` points at it.

## Progress

- 2026-10-01: branch created, document written.
- 2026-10-01: T1 done — 84 files, 174 lines replaced (imports, `go.mod`, goreleaser owner/name
  and ldflag, `internal/version` doc, `install.sh`, install test fixtures, README, CONTRIBUTING,
  `/nerv:configure`). No `nerv-gentle-ai` remains outside `odd/tasks/`. `go build`, `go vet`,
  `go test ./...` green, `gofmt -l .` empty. `bash tests/install-sh.test.sh`: 0 passed, 17 failed
  (exit 127, a missing command on this machine), identical on the base before the change, so
  environmental; CI's "Test suites" job covers it. Committed `3228875`.
- 2026-10-01: native review of `868440d..3228875` (high risk, four lenses, lineage
  `review-276f54ee4bba1537`) approved and acknowledged. Advisory, non-blocking: R4 rename
  ordering window (adopted: rename before merge), R4 old `go install` path (covered by the
  `BREAKING CHANGE` footer in the release notes), R3 install-sh unverified locally (merge only
  on green CI).
- 2026-10-01: T2 done — `gh repo rename nerv-by-gentle-ai`; `origin` now
  `https://github.com/war-apps/nerv-by-gentle-ai.git`; the API resolves the old name to the new
  one. The repository is private, so unauthenticated URLs return 404 for both names.
- 2026-10-01: T3 — PR #40 to `develop`, CI green ("Go checks", "Test suites", the latter also
  covering `install-sh.test.sh`), merged; plugin cache refreshed from `develop` with
  `go run ./cmd/nerv install --no-configure`.

## Next step

Feature complete. The next release publishes the new module path; until then the installed
`nerv` 0.2.0 binary keeps working, and `go install` must use the new path once that release is out.
