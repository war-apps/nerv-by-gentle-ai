# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2026-10-02

### Breaking
- config: drop the GitHub Projects and Jira task providers (cdf8cc7)
  - `tasks.provider` accepts only `teamwork` or `none`. Existing configs are cleaned on the next
    write: `nerv` removes `tasks.provider: github-projects|jira` (the default then applies) and
    the `tasks.providers.github-projects` / `tasks.providers.jira` blocks; run
    `nerv configure --init-repo` in a repository to clean its project `nerv.yaml`.

### Added
- config: describe each role and its gentle-ai equivalent (ba9fbce)
- wizard: explain each role and suggest its gentle-ai equivalent (6b7d766)
- config: strip the removed task providers from existing configs (ae0e388)
- configure: strip the removed providers on every config write (4b31fa6)
- configure: clean removed task providers from existing project configs (8228784)

### Fixed
- wizard: size the models table columns to their longest value (51986fa)
- wizard: size the purpose column to the longest role purpose (720477b)
- config: keep foreign comments and strip every legacy provider form (bdb2fc0)
- config: treat in-scalar quotes as text and keep --init-repo non-fatal on unreadable configs (3fcc9e0)

## [0.3.0] - 2026-10-02

### Breaking
- move the repository and Go module to war-apps/nerv-by-gentle-ai (3228875)
  - Install with `go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest`; the old
    `github.com/war-apps/nerv-gentle-ai` module path no longer resolves to new versions.

### Added
- skills: ask before running the gentle-ai sync (4fba6c6)
- skills: run gentle-ai sync for missing gentle-ai skills (8afa5cd)
- tasks: focus the herdr workspace of a newly opened worktree (774004a)

### Fixed
- install: sync the nerv marketplace before refreshing the plugin cache (cb9a1a9)
- install: remove the nerv marketplace registration on uninstall (5da904f)
- install: tolerate only the marketplace's own not-found message on uninstall (5139e65)
- skills: never prompt for the sync when printing --json (88decdc)
- skills: refresh statuses after sync and surface its diagnostics (9737cf8)

## [0.2.0] - 2026-10-01

### Added
- alpha and rc release channels driven by develop and release branches (83689b1)
- get-nerv.sh one-line installer for Linux and macOS with channel selection (b673c22)
- get-nerv.ps1 one-line installer for Windows with channel selection (2297889)
- Go module with the embedded plugin, materialization and nerv version (d02eb0d)
- internal/config ports the nerv.yaml editor, catalogues and model specs to Go (4c7797d)
- environment packages for the nerv binary (runner, gentle-ai, models, skills) (0e477cc)
- nerv configure replaces configure.ps1 and configure-models.ps1 (1b2c944)
- nerv install, uninstall, apply-models and skills replace install.ps1 and install-skills.ps1 (3c94749)
- interactive configuration wizard as nerv configure and the end of nerv install (3a3c044)
- nerv release replaces release.ps1 and release-guard.ps1; goreleaser publishes the binaries (77a73fe)
- plugin commands call the nerv binary and scripts/install.sh installs it (4f6e56e)
- configurable worktree location through git.worktree_pattern (98a700c)
- initial claude plugin eval corpus for the NERV plugin (1a708d1)
- herdr: detect herdr and resolve its worktrees directory (732cef9)
- configure: offer default / herdr / custom worktree path pattern menu (07d5f4b)

### Changed
- release use case, refusal type, atomic writes, paths and store extracted (9081a45)
- remove the port's residue and unify the duplicated helpers (9d99eec)
- skills: split the orchestrator protocol into a core and reference files (14e1d97)

### Fixed
- wizard safety after the spot-check incident (8051784)
- installer suite keeps the interpreter's directory on usrmerge systems (d69747a)
- hooks: print a short activation header instead of the full protocol (c2e0c85)
- evals: tolerate markdown emphasis and grade the header body line (17077cb)

## [0.1.0] - 2026-09-28

### Added

- NERV plugin overlay on gentle-ai: the Ikari orchestrator protocol and 18
  Evangelion-named agents (Misato, Hyuga, Balthasar, Melchor, Casper,
  Fuyutsuki, Ritsuko, Shinji, Asuka, Rei, Toji, Kaworu, Maya, Kaji and the
  kaji-security/kaji-coverage/kaji-refuter audit passes, and Aoba).
- LIGHT and FULL delegation paths, with FULL adding a per-task MAGI vote
  (Balthasar, Melchor, Casper), a governance veto on new
  skills/scripts/commands (Fuyutsuki), a mandatory quality gate before any
  audit (Maya), a multi-pass ranked audit compiler (Kaji), and an
  end-of-run summary reporting tokens, time, and model per agent.
- Per-repository activation via `.nerv/nerv.yaml`, layered over user-scope
  defaults in `~/.claude/nerv/nerv.yaml` with the same schema.
- The setup wizard (`tools/configure.ps1`) and its non-interactive mode
  (`-Print`, `-Set`, `-SetModel`, `-InitRepo`, `-InstallCommands`) for
  scripted or tested configuration.
- Slash commands `/nerv:init`, `/nerv:status`, and `/nerv:configure` for
  guided repository activation, status reporting, and reconfiguration.
- Per-role model and effort assignment (`models:` in `nerv.yaml`,
  `tools/configure-models.ps1`), including inheriting a role's assignment
  from gentle-ai's own phase table.
- A self-contained Teamwork adapter and the `/task:*` command family
  (start, stop, done, close, blocked, take, priority, timers, listings)
  for provider-agnostic task tracking.
- Skills installation and management for the agent stacks consumed by
  NERV's roles (testing, code, best-practices, architecture, audit).
- The Engram project hook (SessionStart) and knowledge base integration
  for cross-session memory of decisions, conventions, and run history.
- Tag-driven release process: `tools/release.ps1` computes the version and
  changelog from Conventional Commits, CI runs every suite on pull requests,
  and the Release workflow tags, publishes the GitHub Release and opens the
  back-merge on each merge to `main`.
