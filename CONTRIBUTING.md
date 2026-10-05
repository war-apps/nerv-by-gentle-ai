# Contributing to Nerv by Gentle-AI

## Welcome and scope

Thank you for your interest in contributing to Nerv by Gentle-AI — a
governance overlay plugin for Claude Code, distributed as a single Go
binary (`nerv`) plus the plugin tree it embeds.

**Claude Code only, for now.** This project targets Claude Code exclusively;
OpenCode, Codex, and Pi support is out of scope until it lands deliberately.

## Prerequisites

- Go 1.26+
- Claude Code CLI
- gentle-ai 4.x, installed and configured (`gentle-ai install`)
- `actionlint` and `shellcheck` — optional, for linting the workflows and
  bash hooks/scripts locally before pushing

## Getting started

```bash
git clone https://github.com/war-apps/nerv-by-gentle-ai.git
cd nerv-by-gentle-ai
go build ./cmd/nerv
go test ./...
```

Also run the bash suites that exercise the hooks and the installer script:

```bash
bash tests/hook-session-start.test.sh
bash tests/hook-engram-project.test.sh
timeout 120 bash tests/install-sh.test.sh < /dev/null
```

The installer suite is bounded with `timeout` and reads from `/dev/null`
on purpose — it never attaches to a real terminal, so it never triggers
the interactive wizard on your machine.

## Branching

This project follows Gitflow: `feature/*` and `bugfix/*` branches off
`develop`, `release/*` and `hotfix/*` per the model. Never commit directly
to `main` or `develop`.

## Commits

Use Conventional Commits (`feat:`, `fix:`, `chore:`, `docs:`, `refactor:`,
`test:`, …). Keep one work unit per commit — its code, its tests, and its
docs land together, so the commit is reviewable and revertible on its own.

## Test-driven development

TDD here is strict: write the failing test first, then the code that makes
it pass, then refactor with the suite green. Every package that touches
the filesystem or runs a command is built around a small interface, with a
fake filesystem and a fake command runner injected in tests — never run
the real binary's `install` or `configure` paths against your own machine
while testing; exercise them through the fakes in `go test ./...` instead.

## Pull requests

- Target `develop` (or `main` for a hotfix).
- Keep a PR at or below the usual ~400 changed-line review budget, or
  split it into a chained PR set; when a single PR must exceed the
  budget, say so explicitly in the description with the `size:exception`
  rationale.
- CI must be green: `gofmt -l`, `go vet ./...`, `go test ./...`, and the
  hook/installer suites.
- When a PR is part of a chain, describe where it sits in that chain and
  link the neighboring PRs.

## Design principles

- **KISS** — the simplest thing that satisfies the CLI contract and its
  tests; no speculative flags or config knobs.
- **DRY** — one write path for `nerv.yaml` (the format-preserving editor
  in `internal/config`); every command that mutates it goes through that
  path, never a second YAML writer.
- **YAGNI** — no flag without a consumer; a script's old behavior is
  ported only when something in the CLI contract or its tests still needs
  it.
- **SOLID** — small interfaces per package (filesystem root, command
  runner, clock), package boundaries drawn by responsibility
  (`internal/config`, `internal/claude`, `internal/plugin`,
  `internal/wizard`, `internal/release`, …), not by "everything the
  installer does."

## Releases

See [docs/releases.md](docs/releases.md) for versioning, channels, and cutting
a release.

## Code of conduct

Be respectful and constructive. Critique code, not people; assume good
faith in reviews; welcome newcomers and their questions. We're building
this together.
