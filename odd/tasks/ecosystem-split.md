# Split task and release out of nerv into independent ecosystem tools

Objective: task and release stop being nerv extensions and become independent NERV-ecosystem tools, each with its own repository, install script, CLI, Claude Code plugin and configuration. nerv only offers to install or upgrade them, and ends with no extensions.
Scope: four public repositories. This document is the master plan; each repository keeps its own ODD document for the work done there.
Delivery: nerv stays on 6.x (minor releases, no major); the tools start at their own 1.0.0. TDD: strict (session configuration); runner `go test ./...` in every Go repository.

## Repositories

| Name | Repository | Go module | Binary |
|---|---|---|---|
| nerv | `war-apps/nerv-by-gentle-ai` | `github.com/war-apps/nerv-by-gentle-ai` | `nerv` |
| nerv-kit | `war-apps/nerv-kit-by-gentle-ai` | `github.com/war-apps/nerv-kit-by-gentle-ai` | none (library) |
| nerv-task | `war-apps/nerv-task-by-gentle-ai` | `github.com/war-apps/nerv-task-by-gentle-ai` | `nerv-task` |
| nerv-release | `war-apps/nerv-release-by-gentle-ai` | `github.com/war-apps/nerv-release-by-gentle-ai` | `nerv-release` |

## Specs

- **S1** (user, 2026-11-12) "separar las extensiones de task y release de nerv, estas deben estar en repositorios separados ... cuando instales nerv ofrecer instalar estas ... como lista seleccionable. en su instalacion permitir configurarla globalmente con valores por default y que cada una permita elegir sus proveedores. si inicializas un repositorio permitir configurar cada uno de las extensiones instaladas y permitir seleccionar ninguno. nerv debe quedar limpio de extensiones".
- **S2** (user, 2026-11-13) Install model: "que se instalen por un script como nerv o gentle-ai, la unica diferencia es que cuando se instale o actualice nerv te ofrezca instalarlo o actualizarlo. las extensiones deben ser totalmente independientes con sus scripts, comandos, agentes, configuraciones, etc. ya no se llamarian como extensiones, pero es parte del ecosistema de nerv". gentle-ai cannot install third-party packages (research, 2026-11-12), so it plays no installer role.
- **S3** Each tool: its own repository and releases (goreleaser for Linux, macOS and Windows; `install.sh` and `install.ps1`); a CLI with `install`, `upgrade`, `configure` (global, with defaults and a choice of providers) and `init` (repository); its own Claude Code plugin (commands, agents, skills) with its providers inside the same repository. It installs and works on a machine without nerv, proven in its CI.
- **S4** (user, 2026-11-13) Configuration: one file per tool, global `~/.config/ai-workflow/<tool>.yaml` (`%AppData%\ai-workflow\` on Windows), repository `.ai-workflow/<tool>.yaml`. nerv keeps `config.yaml`. The `tasks:` section moves to `task.yaml`; `release:` and `deploy:` move to `release.yaml`.
- **S5** nerv's role: `nerv install` and `nerv upgrade` show the tools as a selectable list with their state (installed, version, outdated) and run each chosen tool's installer or `<tool> upgrade`. `/nerv:init` offers each installed tool's `init`, with a "none" option. The orchestrator only publishes its events (`run.started`, `run.closed`, ...); a tool may consume them, and works without them.
- **S6** nerv ends clean: no `plugins/`, no `ExtensionsFS`, no extension-only Go code, tests or migrations, no `tasks:`/`release:`/`deploy:` handling. It migrates the user's machines: installed `task@nerv`/`release@nerv` plugins are replaced by the tools and their config sections moved to the new files.
- **S7** (user, 2026-11-13) Versioning: no new major for nerv ("nadie lo esta descargando"). No commit of the split carries `!` or `BREAKING CHANGE:`; `nerv maintainer-release apply --version 6.X.0` forces it if needed.
- **S8** (user, 2026-11-13) Option A: nerv-kit in its own repository, the shared Go library of the three CLIs: Claude Code plugin registration, self-upgrade (including the Windows running-`.exe` swap), and reading and writing configuration under `ai-workflow/`. It starts at `v0.x` until the three consumers use it, then `v1.0.0`.
- **S9** (user, 2026-11-13) All four repositories are public. Before publishing a tool, scan its docs and examples for internal hostnames or company data; user servers and tokens stay in the user configuration.

## Phases

| ID | Phase | Repositories | Route | Status | Evidence |
|---|---|---|---|---|---|
| P1 | nerv-kit: extract the shared library from nerv; nerv consumes it with no behavior change | nerv-kit, nerv | delegated (explorer, then writers per repository) | in progress | |
| P2 | nerv-task (pilot): CLI, plugin, agent, `teamwork` provider, install scripts, goreleaser, CI that proves it works without nerv | nerv-task | delegated | pending | |
| P3 | nerv offers tools: selectable list in `install`/`upgrade`, `/nerv:init` offers each tool's `init`, migration of `task@nerv` and `tasks:` | nerv | delegated | pending | |
| P4 | nerv-release: Go engine (~23k lines), 6 providers, `/release:*` commands, its own CLI, migration of `release@nerv`, `release:` and `deploy:` | nerv-release, nerv | delegated | pending | |
| P5 | nerv clean: remove `plugins/`, `ExtensionsFS`, extension-only Go, tests and migrations | nerv | delegated | pending | |

Route evidence: every phase touches 4+ files across packages and repositories: mapping and writer triggers.

## Log

- **L1** (2026-11-12) Research: task is nearly decoupled (17 of 18 commands never call nerv; `task-teamwork` is MCP-only); release is hard-coupled (commands call `nerv release`/`deploy`/`version next`; 5 of 6 providers wrap the hidden `{nerv} provider ...`; ~22.9k lines of extension-only Go). Core hardcodes `workflowconfig.Sections`, `TaskPlugin` special cases, extension migrations, lockstep plugin versions and ~15 prose tests over `ExtensionsFS`. gentle-ai 4.3.1 has no third-party install mechanism.
- **L2** (2026-11-13) Decisions S2, S4, S7, S8, S9; repositories created by the user (public, `LICENSE` + initial commit on `main`, no `develop`). ODD chosen over SDD: product decisions are settled, what remains is execution.
- **L3** (2026-11-13) No Teamwork task, as with the previous nerv features.
