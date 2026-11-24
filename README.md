# Nerv by Gentle-AI

<p align="center">
  <a href="https://github.com/Gentleman-Programming/gentle-ai"><img alt="gentle-ai 4.x" src="https://img.shields.io/badge/gentle--ai-4.x-6f42c1?style=for-the-badge"></a>
  <a href="https://code.claude.com/docs"><img alt="Claude Code plugin" src="https://img.shields.io/badge/Claude_Code-plugin-d97757?style=for-the-badge&logo=anthropic&logoColor=white"></a>
  <a href="https://go.dev/"><img alt="Go CLI" src="https://img.shields.io/badge/Go-CLI-00ADD8?style=for-the-badge&logo=go&logoColor=white"></a>
  <a href="https://www.gnu.org/software/bash/"><img alt="Bash hooks" src="https://img.shields.io/badge/Bash-hooks-4EAA25?style=for-the-badge&logo=gnubash&logoColor=white"></a>
  <a href="https://www.markdownguide.org/"><img alt="Markdown agents and skills" src="https://img.shields.io/badge/Markdown-agents_%26_skills-000000?style=for-the-badge&logo=markdown&logoColor=white"></a>
  <a href="https://mermaid.js.org/"><img alt="Mermaid diagrams" src="https://img.shields.io/badge/Mermaid-diagrams-FF3670?style=for-the-badge&logo=mermaid&logoColor=white"></a>
  <a href="https://yaml.org/"><img alt="YAML config" src="https://img.shields.io/badge/YAML-.ai--workflow.yml-CB171E?style=for-the-badge&logo=yaml&logoColor=white"></a>
  <a href="https://github.com/Gentleman-Programming/gentle-ai"><img alt="ODD and RDD" src="https://img.shields.io/badge/ODD-RDD-0aa?style=for-the-badge"></a>
  <a href="https://github.com/Gentleman-Programming/engram"><img alt="Engram memory" src="https://img.shields.io/badge/Engram-memory-2d3748?style=for-the-badge"></a>
  <a href="https://skills.sh/"><img alt="skills.sh" src="https://img.shields.io/badge/skills.sh-manifest-333?style=for-the-badge"></a>
  <a href="https://git-scm.com/"><img alt="git" src="https://img.shields.io/badge/git-gitflow-F05032?style=for-the-badge&logo=git&logoColor=white"></a>
</p>

<p align="center">
  <a href="https://github.com/Gentleman-Programming/gentle-ai"><img alt="Built with Gentle-AI" src="https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/docs/assets/brand/built-with-gentle-ai.png"></a>
</p>

Nerv by Gentle-AI is a Claude Code plugin implementing an Evangelion-named
multi-agent governance workflow. Ikari orchestrates a cast of named
agents through it; see [docs/integration.md](docs/integration.md#roles) for
what each one does and when it runs.

Nerv by Gentle-AI brings governance that a plain implementation loop lacks: a
per-task MAGI vote gated by criticality, a governance veto on new
skills/scripts/commands, a mandatory quality gate before any audit, a
multi-pass audit compiler with a ranked user issue gate, an append-only
deliberation log, and a run summary reporting tokens, time, and model per
agent.

**Claude Code only, for now.** Nerv by Gentle-AI targets Claude Code; OpenCode,
Codex and Pi are not supported yet.

## Relation to gentle-ai

Nerv by Gentle-AI is an **overlay**, not a fork. It reuses gentle-ai's native
engine and contracts unchanged — ODD (Organic Driven Development) and its
feature document, RDD (receipt-driven review), the skill registry and
resolver, the `jd` judgment agents, delivery budgeting with chained PRs,
and the lossless blocking-prompt contract — and expresses Nerv by
Gentle-AI's own governance on top. Each FULL run is tracked on an ODD
feature document, `odd/tasks/{feature}.md`, which the NERV agents write:
Misato authors its `## Specs` and `## Tasks`, the MAGI vote is recorded per
task row, Aoba records each commit, and Ritsuko's test plan goes into its
`## Log`. NERV's governance files (state, deliberation log, audit rounds,
run summary) sit next to it in `odd/{feature}/nerv/` and are committed
with the feature in the close commit. DIRECT and LIGHT runs, like
gentle-ai's ODD for small work, create no document and no close commit:
their run files stay in the runtime folder `.nerv/runs/{run-id}/`, and
their only commit is the work commit. The orchestrator lock is runtime
only, in `.nerv/locks/`. Nerv by
Gentle-AI never modifies any gentle-ai file: nothing is ever written
under `~/.claude/agents` or `~/.claude/skills`, so `gentle-ai sync` cannot
see or touch it.

See [docs/integration.md](docs/integration.md) for the full layering,
activation, and pipeline diagrams.

## NERV ecosystem

The marketplace ships one plugin, the core (`nerv@nerv`): it adapts
gentle-ai's agents to the NERV pipeline and holds no task, timer, tracker,
release or deploy logic. Task tracking and releases are standalone
NERV-ecosystem tools: each has its own repository, CLI, Claude Code
plugin and configuration file, installed by its own script
(`install.sh`/`install.ps1`) exactly like `nerv` itself — it installs and
works on a machine without `nerv`.

| Tool | Repository | What it brings |
|---|---|---|
| [`nerv-task`](https://github.com/war-apps/nerv-task-by-gentle-ai) | `war-apps/nerv-task-by-gentle-ai` | the `/task:*` commands, local timers, the task providers (such as Teamwork) and the `task:dispatcher` agent the orchestrator emits its task events to |
| [`nerv-release`](https://github.com/war-apps/nerv-release-by-gentle-ai) | `war-apps/nerv-release-by-gentle-ai` | the `/release:*` commands and the native version, build and deploy engines (`nerv-release version next`, `build`, `deploy`) |

`nerv install` and `nerv upgrade` offer the catalogue: each tool's state
(not installed, installed with its version, or outdated against its
latest release) is shown and confirmed one by one on a terminal;
`--tools <names>` installs or upgrades the named tools outright, and
`--no-tools` skips the step. See
[docs/commands.md](docs/commands.md#nerv-install) for the full flow and
flags.

When nerv-task is installed (the `task:dispatcher` agent is available),
the orchestrator emits pipeline events (`run.started`,
`verification.started`, `audit.ranked`, `issue.deferred`, `wave.started`,
`run.closed`, `run.halted`) to `task:dispatcher`, which answers one
envelope per provider. When it is not installed, core emits nothing, asks
no task question and logs nothing. See
[docs/integration.md](docs/integration.md#task-tracking-nerv-task).
nerv-release is independent of the orchestrator.

With a tool installed, `/nerv:init` offers its repository setup
(`nerv-task init`, `nerv-release init`), and `nerv migrate` moves the
`tasks:`, `release:` and `deploy:` sections out of `.ai-workflow/config.yaml`
into the tool's own `task.yaml` or `release.yaml` (see
[docs/configuration.md](docs/configuration.md#task-release-and-deploy-settings)).
From then on, configure them through the tools' own commands and
documentation, not NERV's.

Up to 6.6 these were optional extensions nerv shipped itself (`task`,
`task-teamwork`, `release` and its six providers) with `nerv release`,
`nerv deploy` and `nerv version next`. `nerv install` and `nerv upgrade`
disable those plugins when they find them, and say how to install the
tool that replaces them; see
[docs/migration.md](docs/migration.md#from-66-extensions-moved-to-tools)
for everything that moved. A repository set up with nerv 4.2.0 or
5.0.0-alpha.1 (`.ai-workflow.yml`) migrates with `nerv migrate` (see
[docs/migration.md](docs/migration.md)); until then nerv refuses to run
on the old file.

## Requirements

- **gentle-ai 4.x** — major version 4 is required; minor and
  patch are free. Tested against 4.6.0. Within 4.x, `nerv install` and
  `nerv upgrade` keep gentle-ai on its newest 4.x release automatically
  (`gentle-ai upgrade gentle-ai`, then `gentle-ai sync`); on 3.x, upgrade
  by hand with
  `go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@latest`
  or `brew upgrade gentle-ai`, then `gentle-ai sync` (`gentle-ai upgrade`
  from 3.x cannot reach 4.x on go installs).
- Claude Code with plugin marketplaces support (2.1+).
- git — only needed to contribute to this repository (cloning, building,
  work-unit commits). Installing and running `nerv` itself does not require
  git.
- bash available to hooks (Git Bash on Windows).
- **PowerShell is no longer required on any platform** — the `nerv` binary
  replaces every PowerShell script.
- Go 1.26+ — only needed to install with `go install` instead of
  downloading a prebuilt binary (optional on every platform; see "Install"
  below).
- Engram (`engram` on PATH) is optional — without it `nerv install` skips
  the "nerv" knowledge-base check (see [docs/integration.md](docs/integration.md#operations)).

## Install

**Linux and macOS:**

```bash
curl -fsSL https://raw.githubusercontent.com/war-apps/nerv-by-gentle-ai/main/scripts/install.sh | bash
```

or with `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/war-apps/nerv-by-gentle-ai/main/scripts/install.sh | bash
```

Environment options (export before running, or prefix the command):
`NERV_CHANNEL` (`stable` default, `rc`, or `alpha`), `NERV_INSTALL_DIR`
(default `$HOME/.local/bin`), `NERV_NO_INSTALL` (non-empty: download and
install the binary only, skip running `nerv install` afterwards), and
`NERV_VERSION` (an explicit release tag, e.g. `v1.2.3`, overriding channel
resolution entirely).

**Windows:**

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://raw.githubusercontent.com/war-apps/nerv-by-gentle-ai/main/scripts/install.ps1 | iex"
```

Environment options (set before running): `NERV_CHANNEL` (`stable` default,
`rc`, or `alpha`), `NERV_INSTALL_DIR` (default `%LOCALAPPDATA%\nerv\bin`),
`NERV_NO_INSTALL` (non-empty: download and install the binary only, skip
running `nerv install` afterwards), and `NERV_VERSION` (an explicit release
tag, overriding channel resolution entirely).

Or download `nerv_<version>_windows_<amd64|arm64>.zip` from the
[latest release](https://github.com/war-apps/nerv-by-gentle-ai/releases/latest),
extract `nerv.exe` onto `PATH`, then run:

```bash
nerv install
```

or install with `go install` instead (requires Go 1.26+):

```bash
go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest
nerv install
```

From then on, `nerv upgrade` keeps it current, the same as on Linux and
macOS (see "Updating" below).

### Channels

- `stable` (default) — the latest GitHub Release, published from `main`.
- `rc` — the newest `vX.Y.Z-rc.N` pre-release, published from a
  `release/*` branch.
- `alpha` — the newest `vX.Y.Z-alpha.N` pre-release, published from
  `develop`.

### What `nerv install` does

1. Runs a gentle-ai preflight check (warns if missing or not 4.x; refuses
   with `--require-gentle-ai`).
2. Upgrades gentle-ai to its newest 4.x release when it is on PATH at
   4.x and that release is newer (`gentle-ai upgrade gentle-ai`, then
   `gentle-ai sync`), before laying NERV's own files over it below. A
   newer major version is reported and left alone; a failed release
   lookup or upgrade is only a warning (install continues with the
   gentle-ai it found).
3. Materializes the embedded core plugin under `~/.nerv/marketplace`, and
   disables the extension plugins an earlier nerv shipped (see
   "NERV ecosystem" above).
4. Registers that directory marketplace in Claude Code's `settings.json`.
5. Refreshes the plugin cache (`claude plugin uninstall`/`install`).
6. Applies model/effort assignments to the cached agents.
7. Ensures the Engram `nerv` knowledge-base project exists (skipped when
   `engram` is not on PATH).
8. Installs the skills the plugin defaults reference. Missing external
   skills go through `npx skills add`. Missing gentle-ai skills need
   `gentle-ai sync --agents claude-code --skills ...`; in a terminal
   `nerv install` asks first (default No), because that sync also rewrites
   gentle-ai's own managed files (e.g. `~/.claude/CLAUDE.md`). Without a
   terminal it never runs and a remedy line per missing skill is printed.
9. Runs the interactive configuration wizard when stdin is a terminal, or
   prints the `nerv configure` hint otherwise.

Flags: `--no-configure` (skip the closing wizard/hint entirely),
`--no-skills` (skip installing skills), `--require-gentle-ai` (fail with
exit 1 when gentle-ai is missing or not 4.x), `--tools <names>` (install
the named NERV-ecosystem tools, such as `nerv-task,nerv-release`, without
asking) and `--no-tools` (skip the tools step).

### Updating

On Linux, macOS and Windows, run `nerv upgrade` (`--alpha` or `--rc` to
follow a pre-release channel): it downloads the newest release, verifies
its checksum, replaces the binary (on Windows, by renaming the running
`nerv.exe` aside to `nerv.exe.old` first, since Windows refuses to replace
it in place — a stale `.old` from an earlier upgrade is cleaned up on the
next run), upgrades gentle-ai if a newer 4.x release exists (never to a
new major version), then runs the new binary's `nerv install`. Even when
the binary cannot be replaced or the download/checksum fails, gentle-ai
still gets its upgrade attempt and `nerv install` still runs with the
running executable, so NERV's files stay laid over gentle-ai's; `nerv
upgrade` still exits 1 for the failed replace. See
[docs/commands.md](docs/commands.md#nerv-upgrade).

**Upgrading from a release with the task or release extension.** Install
the tool that replaces it (`nerv install --tools nerv-task,nerv-release`),
then run `nerv migrate` in each repository and `nerv migrate --user` once:
they move the `tasks:`, `release:` and `deploy:` settings into the tools'
own files. Core ignores those sections until then. Copies of the
`/task:*` commands that an older release put in `~/.claude/commands/task/`
are reported by `nerv install` and removed only after you confirm. See
[docs/migration.md](docs/migration.md#from-66-extensions-moved-to-tools).

### Uninstall

```bash
nerv uninstall
```

Removes NERV's `settings.json` registration, uninstalls the cached plugin,
and removes the materialized marketplace directory.

## Setup

After installing (see "Install" above, or "Manual setup" below for the
git-clone path), configure Nerv by Gentle-AI with the guided wizard:

```
nerv configure
```

or, from inside Claude Code:

```
/nerv:configure
```

`/nerv:configure` asks the same questions as the terminal wizard, through
native blocking prompts, pre-filled from the current config, and applies
only the answers you confirm.

**Activation per repository.** A repo opts in by writing
`.ai-workflow/config.yaml` with `enabled: true` — run
`nerv configure --init-repo <path>` (with `--repo-base`), or `/nerv:init` from inside Claude Code for the same thing
through guided questions. Running `--init-repo` on a repo that already has
`.ai-workflow/config.yaml` leaves its content as written.
`/nerv:status` reports the resolved config and, while a run is in progress,
the orchestrator lock state.

The wizard walks through these sections:

1. **Prerequisites** — informational checks for `gentle-ai` (major version
   4 required), `engram` (optional), and `claude` (required for the cache
   refresh step).
2. **Required skills** (`--skip-skills`) — installs the missing external
   skills the plugin defaults reference, via
   `npx skills add <repo> --skill <id> -g -a claude-code -y`; an
   already-present skill is never touched. Missing gentle-ai skills are
   provided by `gentle-ai sync` only after a confirmation (default No) that
   warns the sync also rewrites gentle-ai's managed files (e.g.
   `~/.claude/CLAUDE.md`). `nerv skills --dry-run --json` previews the same
   check without the wizard.
3. **User config** — asks `git` (base branch, worktree policy, worktree
   pattern, branch and commit-ref patterns), `skills` (one comma-separated
   stack per consuming role), `critical_paths`, and `artifacts.commit`,
   then writes `~/.config/ai-workflow/config.yaml`, backing up any existing file
   first. A field left unchanged (Enter keeps the shown value) is never
   rewritten at all; a whole block (`git:`, `skills:`, `artifacts:`) is
   only generated fresh when it is missing from the file entirely. An
   existing `tasks:` block is never rewritten or removed. Anything the
   wizard has no field for — extra keys, `known_projects:`, `sources:`,
   `sources_howto:`, or anything else you added by hand — is never dropped
   or regenerated.
4. **Models** (`--skip-models`) — optionally edits per-role model/effort
   overrides (see "Configuring models and effort" below).
5. **Repos** (`--skip-repos`) — optionally writes `.ai-workflow/config.yaml` in one
   or more local git repositories, leaving an already-initialized repo
   untouched.

Task settings are not part of the wizard: they belong to nerv-task
(`nerv-task configure`).

The closing step (`--no-refresh` to skip) applies the resolved `models:`
block to the plugin cache and refreshes it.

Every prompt shows its current or default value in brackets; Enter keeps
it. `--answers <path>` drives the whole wizard from a text file (one
answer per line) instead of prompting, for scripted setup.

After the wizard, open Claude Code in each configured repository and run
`/nerv:init` once — this refreshes the skill registry; the wizard itself
only writes `.ai-workflow/config.yaml`. Runs are tracked on gentle-ai's ODD feature
documents (`odd/tasks/{feature}.md`), which the orchestrator creates for
FULL runs.

Outside the wizard, the plugin stays self-contained apart from these
requirements: **gentle-ai 4.x** installed and configured (`gentle-ai
install`), **Claude Code**, and **Engram** (optional — used for
the shared `nerv` knowledge base and per-repo memory detection).

## Manual setup

For contributing to the plugin itself, or as an alternative to the
installers above (see [CONTRIBUTING.md](CONTRIBUTING.md) for the full
contributor workflow):

Non-interactive configuration:

```bash
nerv configure --print --json
nerv configure --set key=value --set key2=value2 --json
nerv configure --set-model role=model[/effort] --set-model role2=from:<gentle-ai-phase> --json
nerv configure --init-repo <path> --repo-base <branch>
```

`--set` and `--set-model` are repeatable flags — pass every changed key of
one batch as its own occurrence on a single invocation. Nothing is written
when nothing changed, and the command exits 1 on a rejected key or value,
or on any unknown key in a `--set`/`--set-model` batch (writing nothing for
that batch). `--config <path>` overrides the user-scope `config.yaml` path,
for scripted or tested runs.

Skills and models directly:

```bash
nerv skills --dry-run --json
nerv apply-models
```

Where files live: user scope `~/.config/ai-workflow/config.yaml` (personal
defaults, never committed), project scope `<repo>/.ai-workflow/config.yaml`
(committed, next to the NERV-ecosystem tools' own `task.yaml` and
`release.yaml`; see [docs/configuration.md](docs/configuration.md)),
and the plugin cache under
`~/.claude/plugins/cache/nerv/nerv/<version>/`.

### Developer workflow

1. `git clone https://github.com/war-apps/nerv-by-gentle-ai.git` (the folder
   path is registered as a local plugin marketplace, so keep the clone
   where it will stay).
2. `go build ./cmd/nerv`
3. `go test ./...`
4. Run the freshly built local binary to register your working copy of the
   plugin — `go:embed` embeds the plugin tree from disk at build time, so a
   rebuild picks up uncommitted edits: `./nerv install --no-configure` (add
   `--no-skills` to skip the skills step while iterating) materializes and
   registers the current plugin tree, then restart Claude Code.

**Never pass a fake `--home`.** The public CLI has no `--home`/`--settings`
flags (tests inject them through options, not through argv): a fake home
does not sandbox `claude` or `npx`, both of which read the real user's
Claude Code and npm configuration regardless of `HOME`/`USERPROFILE`. Test
config edits through the fakes exercised by `go test ./...`, or against a
disposable, backed-up `~/.config/ai-workflow/config.yaml` — never by pointing a real
`nerv install`/`nerv configure` run at a throwaway home directory.

Behavioral tests live under `bench/`: see [`bench/README.md`](bench/README.md)
for the manual journey suite and the `claude plugin eval` corpus.

## Configuration schema

Every Nerv by Gentle-AI setting lives in two scope copies of one schema
(user `~/.config/ai-workflow/config.yaml` and project `<repo>/.ai-workflow/config.yaml`,
project overriding user key by key) with sections for skills, per-role
model/effort overrides, critical paths, artifacts and git. Task, release
and deploy settings belong to nerv-task and nerv-release; core ignores
them.

See [docs/configuration.md](docs/configuration.md) for the full schema,
example YAML, and how to configure per-role models and effort (wizard,
`nerv configure --set-model`, or hand-editing).

## Documentation

- [docs/commands.md](docs/commands.md) — every command you can run: the
  Claude Code slash commands and the `nerv` CLI, with usage and examples.
- [docs/integration.md](docs/integration.md) — how Nerv by Gentle-AI
  integrates with gentle-ai, the agent roles table, and operations
  (activation, artifacts, the nerv-task events, Engram project
  detection).
- [docs/configuration.md](docs/configuration.md) — the configuration
  configuration schema and how to configure per-role models and effort.
- [docs/releases.md](docs/releases.md) — versioning, release channels,
  and how to cut a release.
- [docs/migration.md](docs/migration.md) — `nerv migrate`: moving a
  repository and the user configuration from nerv 4.2.0 or 5.0.0-alpha.1
  (configuration files, openspec to ODD), the move of `tasks:`, `release:`
  and `deploy:` to the NERV-ecosystem tools, and everything that left nerv
  with the extensions in 7.0 and what replaces it.
- [docs/troubleshooting.md](docs/troubleshooting.md) — known limitations
  and troubleshooting.

## Releases

The plugin follows [SemVer](https://semver.org/), with versions computed
from Conventional Commits and published automatically per Gitflow branch
(`develop` → alpha, `release/*` → rc, `main` → stable), plus the guard
checks and CI that gate a release.

See [docs/releases.md](docs/releases.md) for versioning, channels, cutting
a release, pre-releases, the release guard, CI, and marketplace notes.

## License

NERV is dedicated to the public domain under [CC0 1.0 Universal](LICENSE).
