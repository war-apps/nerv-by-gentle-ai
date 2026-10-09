# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- config: give every role its full character name for display (31a6c90)
  - Role IDs stay short lowercase slugs (agent names, `nerv:<id>` subagent types, `models:`
    keys); each role now also has a display name, such as `misato` = Misato Katsuragi and
    `fuyutsuki` = Kōzō Fuyutsuki.
  - The name is shown as `display_name` on each `nerv configure --print` models row, as the
    NAME column of the wizard's models table, in `/nerv:configure` and `/nerv:status`, in the
    roles table of `docs/configuration.md`, and at the start of each agent's description.

### Changed
- config: rename melchor to melchior and kaji-audit to gendo (31a6c90)
  - Breaking for anything that launches the agents directly: the subagent types
    `nerv:melchor` and `nerv:kaji-audit` are now `nerv:melchior` and `nerv:gendo`, and the
    audit pass files are `pass-melchior-round-N.json` and `pass-gendo-round-N.json`.
  - The group `kaji-passes` is now `audit-passes` (`kaji`, `gendo`).
  - The old names stay accepted as legacy aliases: `models.melchor` and `models.kaji-audit`
    in `nerv.yaml` keep applying to the renamed roles, and `nerv configure --set-model` and the
    wizard accept the old role and group names. Every write stores the new ID once; when a
    file holds both keys, the new one wins.

## [3.0.0] - 2026-10-08

### Breaking
- release: re-publish 2.1.0 under a major version, since it removes roles
  - 2.1.0 removed the `kaji-security`, `kaji-coverage`, `kaji-resilience` and `kaji-refuter`
    roles and changed the Phase 3 audit to four passes. That breaks existing `models:` overrides
    and audit artifact names, so it needed a major bump instead of a minor one.
  - 3.0.0 has the same code and plugin content as 2.1.0. Read the 2.1.0 section below for the
    full list of changes and how to migrate leftover overrides.

## [2.1.0] - 2026-10-08

### Breaking
- config: drop the `kaji-security` role and give each MAGI one gentle-ai equivalent (2f686de)
  - The Phase 3 audit runs five passes. Melchor's audit pass now carries the full security lens
    (injection, authz, secrets, data exposure, unsafe defaults, dependency risk, crypto, infra
    hardening) and claims `review-risk` next to `jd-judge-b`.
  - Balthasar keeps `jd-judge-a` only. Casper takes over the readability audit lens and
    `review-readability`, and no longer suggests a `from:` phase.
  - A `models.kaji-security` override left in `nerv.yaml` is not an error, but it no longer
    applies to any agent: `nerv apply-models` skips it, and `nerv configure --print`,
    `/nerv:status` and the wizard show it as a row with no purpose. `nerv configure --set-model
    kaji-security=default` refuses the unknown role, so delete the line by hand (or move the
    value to `melchor`).
- config: merge the kaji audit passes and move the refuter to fuyutsuki (868da54)
  - Supersedes the pass count and the casper readability move above: the Phase 3 audit runs four
    passes (`kaji-audit`, `melchor`, `balthasar`, `casper`), and no role judges work it authored
    or compiles.
  - `kaji-audit` merges `kaji-coverage` and `kaji-resilience` with every check of both, always at
    full RDD scope, and claims `review-reliability` and `review-resilience`. Its findings land in
    `nerv/audit/pass-kaji-audit-round-N.json`.
  - Balthasar takes the readability audit lens back and claims `jd-judge-a` and
    `review-readability`. Casper keeps the process lens only and claims no equivalent.
  - Fuyutsuki gains a read-only `MODE: refute` with the whole `kaji-refuter` contract (no
    writes, no `mem_save`, no new findings) and claims `review-refuter`. The `kaji-passes` group
    is now `kaji` and `kaji-audit`.
  - `models.kaji-coverage`, `models.kaji-resilience` and `models.kaji-refuter` overrides behave
    like a stale `models.kaji-security` one: `nerv apply-models` skips them, `--print`,
    `/nerv:status` and the wizard show them as rows with no purpose, and `--set-model
    <role>=default` refuses them, so delete the lines by hand (or move the values to
    `kaji-audit` or `fuyutsuki`).

### Added
- agents: extend balthasar and kaji-coverage to cover gentle-ai readability and reliability (0b90206)
- agents: add kaji-resilience audit pass for resilience and performance (b826c89)
- config: list every gentle-ai equivalent per role and guard v4 agent parity (8dc7ad3)

## [2.0.1] - 2026-10-05

### Fixed
- specs: refuse a requirement both modified and removed in one delta (d321c9a)
- atomicfile: refuse to replace a dangling symlink (8faa5a0)
- config: suggest only real phase keys as from: hints (3141729)
  - `nerv configure --print` rows gain a `from_phase` field; `gentle_ai_equivalent` is now
    informational only.
- config: flag a from: phase missing from gentle-ai state (66a4bf9)
  - The model source reads `gentle-ai:<phase> (missing; plugin default)`, and `--set-model` warns
    with the available phases.

## [2.0.0] - 2026-10-05

### Breaking
- gentleai: require gentle-ai 4.x (ebdc5ba)
  - gentle-ai 4.0.0 retired SDD/OpenSpec, and NERV no longer depends on it. gentle-ai 3.x is
    refused by `nerv install --require-gentle-ai` and flagged as unsupported elsewhere.
  - To upgrade, run `go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@latest`
    (or `brew upgrade gentle-ai`), then `gentle-ai sync`. On go installs, `gentle-ai upgrade` from
    3.x cannot reach 4.x.
  - The v4 sync leaves the v3 `~/.claude/skills/sdd-*`, `~/.claude/agents/sdd-*.md` and
    `~/.claude/commands/gentle-sdd-*.md` in place. It also keeps a `PreToolUse` hook calling the
    removed `gentle-ai sdd-preflight-hook`. NERV does not use any of them, so they are safe to
    remove.

### Added
- cli: add nerv spec-compose ported from gentle-ai (20d00bc). It replaces
  `gentle-ai sdd-archive-compose` for Aoba's archive merges.
- plugin: bootstrap openspec config in /nerv:init without sdd-init (366fcbd)

### Changed
- config: map role equivalents to gentle-ai v4 agents (dcbdacb)

### Fixed
- plugin: read change artifacts instead of sdd-status (c38f182)
- specs: accept CRLF headings in spec-compose (47babd0)
- specs: refuse a requirement repeated within one delta section (b1f23f1)
- atomicfile: write through a symlink to its target (bb29e87)
- plugin: never rewrite an existing openspec config in /nerv:init (45f69f0)
- specs: ignore fenced headings when parsing a delta (7f217c4)

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
