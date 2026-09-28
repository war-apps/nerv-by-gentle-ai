# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
