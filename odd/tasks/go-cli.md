# Go CLI: replace the PowerShell toolchain with the `nerv` binary

Branch `feature/go-cli` from `develop` (`7afbec5`). Started 2026-09-29.
Delivery strategy: `feature-branch-chain`, one PR per phase, chained in order. Every phase PR
exceeds the 400-line budget because each is a port of an existing script with its suite;
`size:exception` is requested per PR with that rationale. Forecast ~7000 authored changed lines.
TDD: strict (global setting). Runner: `go test ./...` (plus `go vet ./...` and `gofmt -l`), and the
two bash hook suites until they move too. Every PowerShell suite is the executable specification of
the phase that ports it and is deleted in the same PR once the Go suite covers it.
Engram mirror: topic `odd/go-cli/tasks` (project `nerv`).

## Objective

One Go binary, `nerv`, replaces every PowerShell script and is distributed like gentle-ai:
`brew install war-apps/tap/nerv` on macOS, `curl -fsSL .../scripts/install.sh | bash` on Linux and
macOS, `go install github.com/war-apps/nerv-gentle-ai/cmd/nerv@latest` on Windows. The binary embeds
the plugin tree, materializes it under `~/.nerv/marketplace`, registers that directory marketplace in
Claude Code and runs the configuration wizard after installing. PowerShell leaves the requirements.

## Problem / why

- User request 2026-09-29: install methods like gentle-ai (brew, curl/wget, go), remove every
  PowerShell script (bash or Go), remove PowerShell from the requirements, restructure and split
  the README, add CONTRIBUTING.md, state that only Claude Code is supported for now, and apply
  KISS, DRY, YAGNI and SOLID with the refactors that follow.
- Today: ~3000 lines of PowerShell (`configure.ps1` 1200, `install.ps1` 930, `configure-models.ps1`,
  `install-skills.ps1`, `release.ps1`, `release-guard.ps1`, `get-nerv.ps1`) and 6 suites with ~470
  assertions; `install.ps1` alone mixes settings.json edits, cache refresh, model assignment, skills
  and the wizard (SRP violation); YAML scalar handling is duplicated between scripts; two bootstrap
  installers do the same thing; `tools/*.ps1` forwarders exist only because of the plugin cache path.
- A bash wizard cannot run on Windows and "go for Windows" is the request, so the single coherent
  implementation is Go (decision 2026-09-29, user).
- Spike (2026-09-29): `claude plugin marketplace add <non-git dir>` + `claude plugin install`
  succeed; the cache records `version` and `gitCommitSha: null`. A materialized directory needs no git
  (user decision: embedded plugin, not a clone).

## Scope and constraints

- Module `github.com/war-apps/nerv-gentle-ai`, Go 1.27 locally, `go 1.25` minimum in `go.mod`.
  Standard library first; a dependency is added only when it removes real code (YAML: the current
  scripts edit `nerv.yaml` scalar by scalar to preserve comments and layout; keep that behaviour with
  a small format-preserving editor, do not pull a YAML library that reformats the file).
- Layout: `cmd/nerv/main.go` (wiring only); `internal/config` (nerv.yaml read/write, managed key
  catalogue, allowed values, defaults); `internal/models` (role catalogue, gentle-ai phase table from
  `~/.gentle-ai/state.json`, frontmatter apply); `internal/claude` (settings.json registration with
  timestamped backups and atomic writes, `claude plugin` invocations, installed_plugins.json);
  `internal/plugin` (embedded tree, materialization, marketplace.json); `internal/skills` (manifest,
  `npx skills add`); `internal/wizard` (prompts over injected io.Reader/io.Writer); `internal/release`
  (conventional commits, next version, changelog, guard); `internal/gentleai` (version preflight);
  `internal/engram` (knowledge-base project check). Root package embeds `all:plugin` because
  `go:embed` cannot reach a parent directory.
- Each package exposes small interfaces for its external effects (filesystem root, command runner,
  clock) so tests inject fakes; no test touches the real home directory, and every test that runs a
  command does it through the runner fake. This is the lesson of the 2026-09-28 installer incident.
- CLI contract preserved for the plugin commands: `nerv configure --print --json`,
  `nerv configure --set key=value [--set ...] --json`, `--set-model role=model[/effort]`,
  `--init-repo <path> --repo-base --repo-provider [--repo-project-id --repo-tasklist-id]`,
  `--install-commands`, `nerv skills --dry-run --json`, `nerv apply-models`, `nerv install
  [--no-configure]`, `nerv uninstall`, `nerv version`. Repeated `--set` works natively in Go, so
  the `-Command` array workaround disappears from the command docs.
- Exit codes as today: 0 ok, 1 refused (validation, nothing to release), 2 environment/git error.
- Versioning: the binary version is injected with `-ldflags -X` by goreleaser from the tag; the
  embedded `plugin/.claude-plugin/plugin.json` keeps its own `version`, and `nerv version` prints
  both; the release guard asserts they agree.
- Release tooling stays in the repo as `nerv release preview|apply|guard` (hidden from the default
  help, documented for maintainers) so CI runs `go run ./cmd/nerv release ...` without a build step.
- Distribution: `.goreleaser.yaml` (linux/darwin, amd64/arm64, `-trimpath`, archives with LICENSE and
  README), `scripts/install.sh` downloads the archive for `uname -s`/`uname -m` from the GitHub
  Release of the requested channel (`NERV_CHANNEL`), verifies the checksum file, installs to
  `~/.local/bin` (or `NERV_INSTALL_DIR`), then runs `nerv install`; brew formula published by
  goreleaser into `war-apps/homebrew-tap` (repository and `HOMEBREW_TAP_TOKEN` secret are user
  actions, recorded under Decisions pending); Windows documented as `go install ...@latest` followed
  by `nerv install`.
- Workflows: `ci.yml` runs `gofmt -l`, `go vet ./...`, `go test ./...`, the bash hook suites and
  `actionlint`; `release.yml` keeps the three channel jobs but calls `go run ./cmd/nerv release`
  and, after the stable tag, runs goreleaser to publish the binaries and the formula.
- Hooks stay bash (`plugin/hooks/*.sh`); they never depended on PowerShell.
- Docs (P6): README order = Relation to gentle-ai, Requirements, Install methods, Setup, Manual
  setup, Configuration schema, then a link block, Releases; `docs/integration.md` = How NERV
  integrates with gentle-ai, Roles, Operations; `docs/troubleshooting.md` = Known limitations,
  Troubleshooting. Status and Roadmap removed. A clear note near the top: Claude Code only for now.
  `CONTRIBUTING.md` for contributors (branching, commits, TDD, tests, PR size, review).
- Artifacts in English. Never push without explicit OK; pushes and PRs are asked for per phase.
- Out of scope: OpenCode/Codex/Pi support; installing prerequisites (`claude`, `gentle-ai`, `git`)
  on the user's behalf.

## Tasks

- [x] P0 DONE 2026-09-29 (writer on sonnet; parent spot check: gofmt empty, vet clean, `go test ./...` 4 packages ok, binary built and `version`/`version --json`/unknown-command exercised, actionlint clean, public API of internal/plugin read). Commit `d02eb0d`. RED observed per unit (undefined symbols / no non-test files), GREEN 19 subtests, 1 Windows-only skip for POSIX mode bits. Deviation accepted: no LICENSE file exists, so goreleaser archives only README and the brew stanza has no license field; add both when a LICENSE lands. Scaffold and embedding. `go.mod`, `cmd/nerv/main.go` with subcommand dispatch (`version`,
  the rest wired as they land), root `assets.go` with `//go:embed all:plugin`, `internal/plugin`
  materialization (`Materialize(root fs.FS, dest string) error` writing the tree plus a
  `.claude-plugin/marketplace.json` for a marketplace named `nerv` with source `./plugin`; idempotent,
  byte-exact, refuses to write outside `dest`), `nerv version` printing binary and plugin versions,
  `.goreleaser.yaml`, `ci.yml` switched to Go checks (the PowerShell suites keep running in CI until
  each is deleted), `.gitignore` for `dist/`. Tests: materialization on a temp dir (tree equality
  against the embedded FS, idempotence, path traversal guard), version output. Route: writer.
- [x] P1 DONE 2026-09-29. P1b-2 (`1b2c944`; parent spot check: all packages ok, actionlint clean, binary exercised against a temp home: batch set, atomic refusal on an unknown key, backup, no-wizard notice): internal/configure with Store, Print, Set, SetModel, InitRepo, InstallCommands (45 tests) and `nerv configure` flags (10 tests); configure.ps1, configure-models.ps1, their forwarders and suites deleted, CI steps removed. Accepted deviations: modes are mutually exclusive per invocation; InitRepo uses `rev-parse --show-toplevel` and exit 1 as the script did; a missing user config is seeded with the script's two-line header. P1a DONE 2026-09-29 (writer on sonnet; parent spot check: gofmt/vet clean, 196 subtests, no os/exec imports, API read). Commit `4c7797d`. 86/86 pure PowerShell cases ported by name plus 48 own cases; accepted deviations: Roles() carries no per-role defaults (they come from agent frontmatter, file I/O, supplied by P1b/P2), ReadModelsOverrides is the raw scan (from:<phase> resolution is the caller's), both Resolve-NervRoleTarget (wizard selection) and the -SetModel spec parser ported under distinct names, `inherit` kept as a model alias because the script allows it. P1b-1 DONE 2026-09-29 (`0e477cc`; parent spot check: 10 packages ok, 53 subtests, HOME/USERPROFILE and os/exec confined to internal/env, one interface). 37 PowerShell cases ported by name, 16 CLI cases deferred to the commands, 3 PowerShell-runtime probes not applicable. P1b-2: the `nerv configure` command with file I/O, backups and the deletion of both scripts and suites. `internal/config` and `nerv configure` non-interactive. Port of `configure.ps1`
  (`-Print`, `-Set`, `-SetModel`, `-InitRepo`, `-InstallCommands`, `-Json`, exit codes, backups, no-op
  detection) and the catalogues in `configure-models.ps1`. Specification: `tests/configure.test.ps1`
  (169) and `tests/configure-models.test.ps1` (25), ported case by case; both `.ps1` files and their
  suites are deleted in this PR. Route: writer, possibly two sequential writers (config editor first,
  then the command).
- [x] P2 DONE 2026-09-29 (writer on sonnet; parent spot check: 14 packages ok, 198 subtests, actionlint clean, plugin/tools gone, usage read). Commit `3c94749`. End-to-end install and uninstall exercised by the writer against stubs on a temp home. Accepted deviations: settings.json is edited as a generic map, so unrelated keys keep their values but not their order (Claude Code rewrites that file with its own order anyway); cache verification compares the embedded plugin version, not a commit sha; `--no-configure` suppresses the hint until P3 wires the wizard; no `--manifest` override. `internal/claude`, `internal/models`, `internal/skills`, `internal/gentleai`,
  `internal/engram` and `nerv install|uninstall|apply-models|skills`. Port of `install.ps1` and
  `install-skills.ps1` (settings.json registration with backup and atomic write, materialize +
  register, `claude plugin uninstall/install` refresh with `installed_plugins.json` verification,
  model/effort frontmatter apply to the cached agents, skills manifest with `npx skills add`,
  gentle-ai 3.x preflight, Engram knowledge-base check-or-create). Specification:
  `tests/install-apply-models.test.ps1` (28) and `tests/install-skills.test.ps1` (29); scripts,
  forwarders under `tools/` and suites deleted. Route: writer.
- [x] P3 DONE 2026-09-29: wizard as commit `3a3c044` (writer on sonnet; parent spot check: 15 packages ok, answers-file run against a temp home changed exactly the two answered keys with a backup). TDD deviation, recorded for the user: the writer built internal/wizard together with its tests and proved them load-bearing by mutation (RED on a forced wrong effort, GREEN on revert) instead of writing them first. Incident during the parent spot check: `nerv configure --home <tmp> < /dev/null` was NOT refused (Windows NUL device reads as a console), the wizard took EOF as "accept every default and answer Y", and the real runner executed `claude plugin uninstall/install nerv@nerv` and nine `npx skills add -g` on the developer machine; the plugin cache was restored from develop (7afbec5) by a momentary checkout, global skills were re-fetched (same sources), settings.json untouched. P3.1 DONE as commit `8051784` (two writers on sonnet, RED first for the EOF abort, the flag removal and the custom-id loop; the x/term swap itself has no unit-testable RED, said so): terminal detection with golang.org/x/term (go.mod now 1.26.0, CI reads go-version-file), EOF on any prompt aborts with exit 1 and no side effects, `--home`/`--settings` removed from the public CLI (tests inject them through options). `internal/wizard` and interactive `nerv configure`. Port of the interactive sections of
  `configure.ps1` (git, tasks and Teamwork, skills, models, repo, commands) over injected
  reader/writer, reusing P1's editor and catalogues; `nerv install` ends by running it unless
  `--no-configure` or stdin is not a terminal. Tests drive the prompts with scripted input. Route:
  writer.
- [x] P4 DONE 2026-09-29 (writer on sonnet; parent spot check: 16 packages ok, actionlint clean, no pwsh in workflows, tools/ gone, preview and guard exercised read-only, goreleaser step read). Commit `77a73fe`. 199 PowerShell cases ported (136 + 63, more than the document estimated). Accepted deviations: pure functions take file contents and an exists flag, file I/O lives in the command; guard adds `binary_consistent` (skipped for dev builds); no actionlint step in CI (run locally). Unverified: goreleaser in append mode after the stable tag and the `--skip=homebrew` conditional have never run on GitHub; the first stable release after this merges is the test, and goreleaser must find exactly one tag on the main HEAD. `internal/release` and `nerv release preview|apply|guard`. Port of `release.ps1` and
  `release-guard.ps1` (labels, `--pre-release-base current|next`, tag/base in JSON, changelog
  section, guard with notes extraction). Specification: `tests/release.test.ps1` (134) and
  `tests/release-guard.test.ps1` (54); scripts and suites deleted. `release.yml` calls
  `go run ./cmd/nerv release ...` and runs goreleaser after the stable tag (brew publish gated on
  the tap secret being present). Route: writer.
- [ ] P5 Plugin commands and installers. `plugin/commands/{configure,init,status}.md` call `nerv`
  instead of `pwsh ... .ps1` (repeated `--set`, no `-Command` workaround); `scripts/install.sh`
  (binary download + checksum + `nerv install`) with a bash suite on fixtures; `get-nerv.sh`,
  `get-nerv.ps1`, `tests/get-nerv.test.*` and any remaining `.ps1` deleted; `plugin/tools/` removed
  from the plugin tree. Route: writer.
- [ ] P6 Docs. README in the requested order and split into `docs/integration.md` and
  `docs/troubleshooting.md`; Status and Roadmap removed; "Claude Code only for now" note;
  `CONTRIBUTING.md`; CHANGELOG `Unreleased` untouched (release tooling writes it). Route: writer.
- [ ] P7 Principles pass. One bounded read-only review of the Go tree against KISS, DRY, YAGNI and
  SOLID (package boundaries, interface size, duplicated helpers, dead flags carried over from
  PowerShell) producing a short list; then one refactor slice for the accepted items, tests green
  before and after. Route: reviewer (read-only) then writer.

## Acceptance criteria

- `go test ./...`, `go vet ./...` and `gofmt -l` clean; no `.ps1` file left in the repository;
  `rg -i powershell README.md` finds only the sentence saying it is no longer required.
- On a machine with `claude` and `gentle-ai`: `curl -fsSL .../scripts/install.sh | bash` ends inside
  the wizard, and `/nerv:configure git` works afterwards through `nerv configure`.
- `brew install war-apps/tap/nerv` and `go install ...@latest` produce a binary whose `nerv version`
  matches the release tag and the embedded plugin version.

## Decisions pending (user)

- Create the `war-apps/homebrew-tap` repository and the `HOMEBREW_TAP_TOKEN` secret before the first
  stable release after P4 (goreleaser skips the formula until then).

## Progress log

- 2026-09-29: P4 complete (`77a73fe`), release tooling in Go, goreleaser in the workflow; only get-nerv.ps1 remains. P5 next.
- 2026-09-29: P3.1 safety fixes (`8051784`); P4 next.
- 2026-09-29: P3 wizard committed (`3a3c044`); orchestrator incident on the dev machine during the spot check, remediated; P3.1 safety fixes next.
- 2026-09-29: P2 complete (`3c94749`), install/uninstall/apply-models/skills; only release*.ps1 and get-nerv.ps1 remain. P3 next.
- 2026-09-29: P1 complete (`1b2c944`), nerv configure non-interactive; first PowerShell deletions. P2 next.
- 2026-09-29: P1b-1 complete (`0e477cc`), env/gentleai/models/skills; P1b-2 (`nerv configure`) next.
- 2026-09-29: P1a complete (`4c7797d`), internal/config; P1b-1 next.
- 2026-09-29: P0 complete (`d02eb0d`); P1 split into P1a (config editor and catalogues) and P1b (`nerv configure` command).
- 2026-09-29: feature document created; branch `feature/go-cli` from `develop` (`7afbec5`); spike
  proved that a non-git directory marketplace installs.

## Next step

P5 (writer): plugin commands and SKILL references call `nerv`; `scripts/install.sh` downloads the binary; delete get-nerv.* and their suites; RED first for the script suite.
