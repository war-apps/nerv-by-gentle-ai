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
  <a href="https://www.teamwork.com/"><img alt="Teamwork task provider" src="https://img.shields.io/badge/Teamwork-task_provider-FF22B1?style=for-the-badge&logo=teamwork&logoColor=white"></a>
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

## Core and optional extensions

The marketplace ships the core and three optional extensions (`task`,
`release`, `deploy`) with their providers. Each one is installed as its own
Claude Code plugin:

| Plugin | Required | What it does |
|---|---|---|
| `nerv@nerv` | yes | Core: adapts gentle-ai's agents to the NERV pipeline. It holds no task, timer or tracker logic. |
| `task` | no | The task extension: the `/task:*` commands, local timers, and the `task:dispatcher` agent that turns pipeline events into tracker operations. |
| `task-teamwork` | no | A task provider for Teamwork, with a `provider.yaml` manifest. |
| `release` | no | The release extension: `/release:version`, `/release:setup`, `/release:build`, the `release:versioner` and `release:builder` agents. Its providers come with it: the native `conventional-commits` version engine (no semantic-release) and the `docker-compose` and `bash` builds, which leave a local artifact; it never writes to git. |
| `release-conventional-commits` | no | The release version provider: the native Conventional Commits engine, the `version` slot's default. Installed with `release`. |
| `release-docker-compose` | no | A release build provider: `docker compose` build, test service and saved images. Installed with `release`. |
| `release-bash` | no | A release build provider: each stage runs the project's own command. Installed with `release`. |
| `deploy` | no | The deploy extension (requires `release`): `/release:deploy`, `/release:ship` (build and deploy in one go), `/release:deploy-setup` and the `release:deployer` agent. It turns an artifact into a release: release commit, tag, push, targets, git host release. |
| `release-github` | no | A deploy git host provider for GitHub releases (`gh`), with a `provider.yaml` manifest. Installed with `deploy`. |
| `release-ssh-compose` | no | A deploy target provider: pushes the artifact's images and updates the containers over SSH with the host's compose script. Installed with `deploy`. |
| `release-script` | no | A deploy target provider: runs the repository's own deploy script from the release commit. Installed with `deploy`. |

When the task extension is installed, the orchestrator emits pipeline events
(`run.started`, `verification.started`, `audit.ranked`, `issue.deferred`,
`wave.started`, `run.closed`, `run.halted`) to `task:dispatcher`, which
answers one envelope per provider. When it is not installed, core emits
nothing, asks no task question and logs nothing. See
[docs/integration.md](docs/integration.md#task-extension) for the event
table, the envelope, the per-event switches and `/task:replay`.

The release and deploy extensions are independent of the orchestrator: install
them with `nerv install --with release` (add
`release-ssh-compose` for SSH and compose hosts), run `/release:setup` and
`/release:deploy-setup` once, then `/release:build` leaves a local artifact in
`.nerv/artifacts/<project>/<version>/` and `/release:deploy` releases it (`/release:ship` chains the two). Both
always rehearse with a dry run and act only after your explicit yes. A
repository set up with nerv 4.2.0 or 5.0.0-alpha.1 (`.ai-workflow.yml`)
migrates with `nerv migrate` (see [docs/migration.md](docs/migration.md));
until then nerv refuses to run on the old file.

## Requirements

- **gentle-ai 4.x** — major version 4 is required; minor and
  patch are free. Tested against 4.2.0. On 3.x, upgrade with
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
- Go 1.26+ — only when installing on Windows with `go install` (see
  "Install" below).
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

```bash
go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest
nerv install
```

Requires Go 1.26+ (see "Requirements" above).

### Channels

- `stable` (default) — the latest GitHub Release, published from `main`.
- `rc` — the newest `vX.Y.Z-rc.N` pre-release, published from a
  `release/*` branch.
- `alpha` — the newest `vX.Y.Z-alpha.N` pre-release, published from
  `develop`.

### What `nerv install` does

1. Runs a gentle-ai preflight check (warns if missing or not 4.x; refuses
   with `--require-gentle-ai`).
2. Materializes the embedded core plugin, and the extensions and providers
   you install, under `~/.nerv/marketplace`.
3. Registers that directory marketplace in Claude Code's `settings.json`.
4. Refreshes the plugin cache (`claude plugin uninstall`/`install`).
5. Applies model/effort assignments to the cached agents.
6. Ensures the Engram `nerv` knowledge-base project exists (skipped when
   `engram` is not on PATH).
7. Installs the skills the plugin defaults reference. Missing external
   skills go through `npx skills add`. Missing gentle-ai skills need
   `gentle-ai sync --agents claude-code --skills ...`; in a terminal
   `nerv install` asks first (default No), because that sync also rewrites
   gentle-ai's own managed files (e.g. `~/.claude/CLAUDE.md`). Without a
   terminal it never runs and a remedy line per missing skill is printed.
8. Runs the interactive configuration wizard when stdin is a terminal, or
   prints the `nerv configure` hint otherwise.

Flags: `--no-configure` (skip the closing wizard/hint entirely),
`--no-skills` (skip installing skills), `--require-gentle-ai` (fail with
exit 1 when gentle-ai is missing or not 4.x), `--with <names>` (install
extensions and providers such as `task,task-teamwork` or
`release,deploy,release-github`; in a terminal, install offers the task extension,
its providers, the release extension, then the deploy extension; release and deploy
bring every provider they ship with them, unasked; `deploy` requires
`release`).

### Updating

On Linux and macOS, run `nerv upgrade` (`--alpha` or `--rc` to follow a
pre-release channel): it downloads the newest release, verifies its
checksum, replaces the binary, upgrades gentle-ai if a newer 4.x release
exists (never to a new major version), then runs the new binary's `nerv install`.
See [docs/commands.md](docs/commands.md#nerv-upgrade).

On Windows, re-run `go install ...@latest`, then run `nerv install` again
to refresh the plugin cache and re-apply configuration.

**Upgrading from a release with built-in tasks.** An existing `tasks:`
block in `.ai-workflow/config.yaml` keeps working once the task extension is
installed. The task settings that are not personal are read from the project
only: `nerv migrate --user` moves the ones in `~/.config/ai-workflow/config.yaml`
under `defaults.tasks`, which seeds new projects. Without the extension, core ignores the block and never
deletes it. Copies of the `/task:*` commands that an older release put in
`~/.claude/commands/task/` are reported by `nerv install` and removed only
after you confirm. See [docs/configuration.md](docs/configuration.md#tasks-task-extension).

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
`nerv configure --init-repo <path>` (with `--repo-base`, and a repeatable
`--repo-provider-config <provider>.<key>=<value>` for task-provider
settings), or `/nerv:init` from inside Claude Code for the same thing
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
6. **Task providers** — shown only when the `task` extension is enabled. It prompts
   each enabled provider's scalar config keys, read from its
   `provider.yaml`, plus the default task provider for new repositories
   (`defaults.tasks.provider`) when more than one provider is enabled. The `/task:*` commands are served by the `task` extension itself,
   so nothing is copied to `~/.claude/commands/task/`.

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
install`), **Claude Code** (plus, for the optional `task-teamwork`
provider, the Teamwork MCP server it declares), and **Engram** (optional — used for
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
nerv configure --init-repo <path> --repo-base <branch> [--repo-provider-config <provider>.<key>=<value> ...]
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
(committed, next to the repository's own extensions and providers in
`.ai-workflow/extensions/` and `.ai-workflow/providers/`; see
[docs/extensions.md](docs/extensions.md#project-extensions-and-providers)),
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
model/effort overrides, critical paths, artifacts and git. The `tasks:`
section belongs to the optional task extension; core ignores it.

See [docs/configuration.md](docs/configuration.md) for the full schema,
example YAML, and how to configure per-role models and effort (wizard,
`nerv configure --set-model`, or hand-editing).

## Documentation

- [docs/commands.md](docs/commands.md) — every command you can run: the
  Claude Code slash commands and the `nerv` CLI, with usage and examples.
- [docs/integration.md](docs/integration.md) — how Nerv by Gentle-AI
  integrates with gentle-ai, the agent roles table, and operations
  (activation, artifacts, the task extension and its events, Engram project
  detection).
- [docs/configuration.md](docs/configuration.md) — the configuration
  configuration schema and how to configure per-role models and effort.
- [docs/releases.md](docs/releases.md) — versioning, release channels,
  and how to cut a release.
- [docs/release-commands.md](docs/release-commands.md) — the release extension
  for your own repositories: `nerv version next` / `/release:version` (the next
  semantic version of a repo or of each monorepo project, from the native
  engine), `nerv release` / `/release:build` (build, test and package what is on
  origin into a local artifact) and the `release:` section, which
  `/release:setup` writes interactively (importing an existing semantic-release
  config with `nerv release setup --import`).
- [docs/deploy.md](docs/deploy.md) — the deploy extension: `nerv deploy` /
  `/release:deploy` (release commit, tag, push, targets, git host release from
  an artifact), the `deploy:` section (`/release:deploy-setup`) and the provider
  contract (`release-github`, `release-ssh-compose`).
- [docs/extensions.md](docs/extensions.md) — the two contracts of the
  extensions and their providers: `extension.yaml` (an extension and the core) and
  `provider.yaml` (a provider and its extension), with the common
  request/response envelope.
- [docs/migration.md](docs/migration.md) — `nerv migrate`: moving a
  repository and the user configuration from nerv 4.2.0 or 5.0.0-alpha.1 (configuration
  files, `release:` and `deploy:`, the semantic-release import, openspec to
  ODD, the extensions to install).
- [docs/troubleshooting.md](docs/troubleshooting.md) — known limitations
  and troubleshooting.

## Releases

The plugin follows [SemVer](https://semver.org/), with versions computed
from Conventional Commits and published automatically per Gitflow branch
(`develop` → alpha, `release/*` → rc, `main` → stable), plus the guard
checks and CI that gate a release.

See [docs/releases.md](docs/releases.md) for versioning, channels, cutting
a release, pre-releases, the release guard, CI, and marketplace notes.
