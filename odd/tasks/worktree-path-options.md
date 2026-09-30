# Worktree path options (default / herdr / custom)

Locator: `odd/tasks/worktree-path-options.md` — Engram mirror topic `odd/worktree-path-options/tasks` (project `nerv`).
Branch: `feature/worktree-path-options` off `develop` (c7c96b8). No Teamwork task (repo rule).

## Objective

When `nerv configure` (Go wizard) and `/nerv:configure` (plugin command) ask for
`git.worktree_pattern`, offer a menu instead of free text, and auto-open every
worktree NERV creates as a herdr workspace when herdr is installed.

## Problem / why

Today the pattern is a bare free-text prompt (`internal/wizard/userconfig.go:130`,
`plugin/commands/configure.md:49-51`). Users who run herdr already have a worktree
root configured there (`[worktrees] directory` in herdr's `config.toml`, convention
`<directory>/<repo>/<branch-slug>`); NERV should reuse it instead of making the user
retype it, and should open the worktree in herdr so the two tools stay in sync.
Observed 2026-09-30: a worktree created by NERV rules was not opened in herdr and had
to be opened by hand.

## Scope (authorized)

1. Menu with three options, shown in this order, each with a rendered example:
   1. **default** — the agent default from the catalogue (`.claude/worktrees/{slug}`).
   2. **herdr** — only when herdr is detected: `<herdr worktrees.directory>/{repo}/{slug}`.
      Example: herdr directory `c:/wt`, repo `pepe` → `c:/wt/pepe/{slug}`.
   3. **custom** — free text using `{repo}`, `{slug}` (existing `{branch}`, `{prefix}`, `{id}` stay valid).
2. herdr detection = `herdr` on PATH + read `worktrees.directory` from herdr's
   `config.toml` (`%APPDATA%\herdr\config.toml` on Windows, `~/.config/herdr/config.toml`
   elsewhere; default `~/.herdr/worktrees` when the key is absent). Minimal TOML
   section/key scan, no new dependency.
3. The chosen value is stored as a plain `git.worktree_pattern` string (the herdr
   option stores the resolved pattern, e.g. `D:\.worktrees\{repo}\{slug}`), so
   consumers keep reading one key. Validation in `internal/config/managed.go` unchanged.
4. Consumers (`plugin/skills/nerv-tasks/providers/teamwork/procedures/start.md`) open the
   worktree in herdr right after `git worktree add`, when herdr is on PATH and
   `herdr status server` reports running:
   `herdr worktree open --workspace $HERDR_WORKSPACE_ID --path "<abs>" --label "<slug>" --no-focus`
   (`--cwd <repo root>` when the env var is empty; `--path` and `--branch` are mutually
   exclusive in herdr 0.9.x). Silent skip when herdr is absent or the server is down;
   `already_open: true` is not an error.
5. Docs: README `git.worktree_pattern` note, `plugin/commands/configure.md` menu.

Out of scope: new config keys (no `herdr_workspace` toggle — YAGNI), evals, changing
`git.worktree` policy, herdr `worktree create` (NERV keeps `git worktree add` for base-branch control).

## Constraints

- Strict TDD (CONTRIBUTING.md): failing test first, then code, then refactor. Runner: `go test ./...`; CI also runs `gofmt -l` and `go vet ./...`.
- Tests mock I/O through `configure.Deps` (fake runner, temp home). Never call the real `herdr`.
- Conventional Commits, one work unit per commit, no AI attribution lines.
- English identifiers and artifacts. Menu example must be rendered, not described.
- Per-task ~400 authored lines heuristic (advisory). Delivery strategy: `ask-on-risk`; forecast ≈ 380 authored lines (single PR expected).

## Tasks

- [x] T1 — `internal/herdr`: `Detect(lookPath, getenv, home, readFile, goos)` → `{Installed bool, WorktreesDir string}` with config-path resolution, TOML `[worktrees] directory` scan, default fallback, `~` expansion; `Pattern(dir)` → `<dir>/{repo}/{slug}` keeping the separator herdr reports. 13 tests. Route: delegated writer (writer trigger: 2+ non-trivial files). Commit `732cef9`.
- [x] T2 — wizard menu: new `internal/wizard/worktreepattern.go` (`worktreePatternField`) wired into `userconfig.go`; rows default / herdr (when detected) / custom, each with an `e.g.` example (`{repo}`→`my-repo`, `{slug}`→`feature-tw-123-add-button`, relative patterns prefixed `<repo-root>/`); custom falls through to the free-text `ask`; blank keeps current via `menuChoice`. `configure.Deps.Getenv` added (nil-safe), wired to `os.Getenv` in `cmd/nerv`. 4 new wizard tests + 1 pre-existing test adjusted to the menu. Commit `07d5f4b`.
- [x] T3 — plugin docs: `plugin/commands/configure.md` menu contract; `start.md` step 6.g herdr auto-open (PATH + `herdr status server` running, `--path` never with `--branch`, `--cwd` fallback, silent skip, `already_open` not an error, no `--trust-repository` unprompted, `workspace_id` in the confirmation); README example comment. Commit `d127aa6`.
- [x] T4 — verification (parent re-run 2026-09-30): `gofmt -l .` → empty; `go vet ./...` → clean; `go test ./...` → all packages ok (incl. `internal/herdr`, `internal/wizard`). Writer reported RED→GREEN per task (T1 build-failed RED, T2 menu-shape RED). Risk assess (`gentle-ai review assess`, untracked declared): `medium`, `review_due: slice_budget_reached`; RDD is **off** globally, so no native review started (ordinary policy). Parent read back the full diff.

## Acceptance criteria

- Wizard with herdr detected shows exactly: default, herdr, custom (in that order), each with an example path.
- Wizard without herdr shows default, custom only.
- Selecting herdr writes `<herdr dir>/{repo}/{slug}` (OS separators preserved as read) to `git.worktree_pattern`.
- `start.md` instructs the herdr open step with the exact flags above and the silent-skip rules.
- `go test ./...`, `go vet ./...`, `gofmt -l .` all clean.

## Progress / evidence

- 2026-09-30: exploration done (mapper report), branch created, this document written and mirrored.
- 2026-09-30: T1–T3 implemented by one delegated writer under strict TDD; T4 verified by the parent. Commits `732cef9`, `07d5f4b`, `d127aa6` on `feature/worktree-path-options`.
- Authored changed lines (additions + deletions, tests included): **~737**, above the ~380 forecast and the ~400 delivery budget. Nothing was trimmed to fit. Delivery strategy `ask-on-risk` → user chose **stacked chain** (2026-09-30).
- Chain slices (each within budget, one slicing pass):
  1. PR #29 `feature/worktree-path-options` → `develop`: `732cef9` (herdr package), 352 lines.
  2. PR #30 `feature/worktree-path-options-wizard` → PR #29 branch: `07d5f4b` (wizard menu), 371 lines.
  3. PR #31 `feature/worktree-path-options-docs` → PR #30 branch: `d127aa6` + this document, 106 lines.
- 2026-09-30: branches pushed and the three PRs opened with explicit user OK.

## Decisions

- herdr option stores the **resolved** pattern (e.g. `D:\.worktrees\{repo}\{slug}`) rather than a `herdr` sentinel: consumers keep reading one plain key and the user sees the exact value. Drift if herdr's directory changes later is accepted; re-run the wizard.
- No new config key for the auto-open (YAGNI): detection + running server is the whole gate.
- `Pattern` infers the separator from herdr's directory string (backslash present → `\`, else `/`) so the stored value matches what herdr shows.

## Next step

Push the three branches and open the stacked PRs in order (PR bodies prepared with Chain Context) — push and PR creation remain the user's call. After PR 1 merges, retarget PR 2 to `develop`; after PR 2 merges, retarget PR 3.
