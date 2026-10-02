# Nerv by Gentle-AI

<p align="center">
  <a href="https://github.com/Gentleman-Programming/gentle-ai"><img alt="gentle-ai 3.x" src="https://img.shields.io/badge/gentle--ai-3.x-6f42c1?style=for-the-badge"></a>
  <a href="https://code.claude.com/docs"><img alt="Claude Code plugin" src="https://img.shields.io/badge/Claude_Code-plugin-d97757?style=for-the-badge&logo=anthropic&logoColor=white"></a>
  <a href="https://go.dev/"><img alt="Go CLI" src="https://img.shields.io/badge/Go-CLI-00ADD8?style=for-the-badge&logo=go&logoColor=white"></a>
  <a href="https://www.gnu.org/software/bash/"><img alt="Bash hooks" src="https://img.shields.io/badge/Bash-hooks-4EAA25?style=for-the-badge&logo=gnubash&logoColor=white"></a>
  <a href="https://www.markdownguide.org/"><img alt="Markdown agents and skills" src="https://img.shields.io/badge/Markdown-agents_%26_skills-000000?style=for-the-badge&logo=markdown&logoColor=white"></a>
  <a href="https://mermaid.js.org/"><img alt="Mermaid diagrams" src="https://img.shields.io/badge/Mermaid-diagrams-FF3670?style=for-the-badge&logo=mermaid&logoColor=white"></a>
  <a href="https://yaml.org/"><img alt="YAML config" src="https://img.shields.io/badge/YAML-nerv.yaml-CB171E?style=for-the-badge&logo=yaml&logoColor=white"></a>
  <a href="https://github.com/Gentleman-Programming/gentle-ai"><img alt="OpenSpec SDD" src="https://img.shields.io/badge/OpenSpec-SDD_%2B_RDD-0aa?style=for-the-badge"></a>
  <a href="https://github.com/Gentleman-Programming/engram"><img alt="Engram memory" src="https://img.shields.io/badge/Engram-memory-2d3748?style=for-the-badge"></a>
  <a href="https://www.teamwork.com/"><img alt="Teamwork MCP" src="https://img.shields.io/badge/Teamwork-MCP_adapter-FF22B1?style=for-the-badge&logo=teamwork&logoColor=white"></a>
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
engine and contracts unchanged — the SDD artifact pipeline, RDD
(receipt-driven review), the skill registry and resolver, strict TDD,
delivery budgeting with chained PRs, and the lossless blocking-prompt
contract — and expresses Nerv by Gentle-AI's own governance on top. Nerv by
Gentle-AI never modifies any gentle-ai file: nothing is ever written
under `~/.claude/agents` or `~/.claude/skills`, so `gentle-ai sync` cannot
see or touch it.

See [docs/integration.md](docs/integration.md) for the full layering,
activation, and pipeline diagrams.

## Requirements

- **gentle-ai 3.x** — major version 3 is required; minor and patch are free.
  Tested against 3.7.0. 4.x is untested and not supported until Nerv by
  Gentle-AI's contracts are re-verified against it.
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

1. Runs a gentle-ai preflight check (warns if missing or not 3.x; refuses
   with `--require-gentle-ai`).
2. Materializes the embedded plugin under `~/.nerv/marketplace`.
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
exit 1 when gentle-ai is missing or not 3.x).

### Updating

Re-run the install method you used (the `scripts/install.sh` one-liner, or
`go install ...@latest` on Windows), then
run `nerv install` again to refresh the plugin cache and re-apply
configuration.

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
`.nerv/nerv.yaml` with `enabled: true` — run
`nerv configure --init-repo <path>` (with `--repo-base`, `--repo-provider`,
and, for Teamwork, `--repo-project-id`/`--repo-tasklist-id`), or `/nerv:init`
from inside Claude Code for the same thing through guided questions.
Running `--init-repo` on a repo that already has `.nerv/nerv.yaml` also
removes the task providers that no longer exist (`github-projects`, `jira`)
from it, with a backup; any other content is left as written.
`/nerv:status` reports the resolved config and, while a run is in progress,
the orchestrator lock state.

The wizard walks through six sections:

1. **Prerequisites** — informational checks for `gentle-ai` (major version
   3 required), `engram` (optional), and `claude` (required for the cache
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
   pattern, branch and commit-ref patterns), `tasks` (provider,
   ask-when-missing, subtasks-per-wave, timer store, rounding minutes, and — when the
   provider is Teamwork — the ref prefix, assignee id, default
   project/tasklist ids, and the seven workflow-stage names), `skills`
   (one comma-separated stack per consuming role), `critical_paths`, and
   `artifacts.commit`, then writes `~/.claude/nerv/nerv.yaml`, backing up
   any existing file first. A field left unchanged (Enter keeps the shown
   value) is never rewritten at all; a whole block (`git:`, `tasks:`,
   `skills:`, `artifacts:`, or the `providers.teamwork` sub-block) is only
   generated fresh when it is missing from the file entirely. Anything the
   wizard has no field for — extra keys, `known_projects:`, `sources:`,
   `sources_howto:`, or anything else you added by hand — is never dropped
   or regenerated.
4. **Models** (`--skip-models`) — optionally edits per-role model/effort
   overrides (see "Configuring models and effort" below).
5. **Repos** (`--skip-repos`) — optionally writes `.nerv/nerv.yaml` in one
   or more local git repositories, leaving an already-initialized repo
   untouched.
6. **Slash commands** (`--skip-commands`) — optionally copies the Teamwork
   procedures under `plugin/skills/nerv-tasks/providers/teamwork/procedures/`
   into `~/.claude/commands/task/` as the 16 `/task:*` commands, never
   overwriting a file that already exists there.

The closing step (`--no-refresh` to skip) applies the resolved `models:`
block to the plugin cache and refreshes it.

Every prompt shows its current or default value in brackets; Enter keeps
it. `--answers <path>` drives the whole wizard from a text file (one
answer per line) instead of prompting, for scripted setup.

After the wizard, open Claude Code in each configured repository and run
`/nerv:init` once — this bootstraps gentle-ai's SDD registry when it is
missing; the wizard itself only writes `.nerv/nerv.yaml`.

Outside the wizard, the plugin stays self-contained apart from these
requirements: **gentle-ai 3.x** installed and configured (`gentle-ai
install`), **Claude Code** with the **Teamwork MCP** configured when the
Teamwork task-tracker adapter is used, and **Engram** (optional — used for
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
nerv configure --init-repo <path> --repo-base <branch> --repo-provider <provider> [--repo-project-id <id> --repo-tasklist-id <id>]
nerv configure --install-commands
```

`--set` and `--set-model` are repeatable flags — pass every changed key of
one batch as its own occurrence on a single invocation. Nothing is written
when nothing changed, and the command exits 1 on a rejected key or value,
or on any unknown key in a `--set`/`--set-model` batch (writing nothing for
that batch). `--config <path>` overrides the user-scope `nerv.yaml` path,
for scripted or tested runs.

Skills and models directly:

```bash
nerv skills --dry-run --json
nerv apply-models
```

Where files live: user scope `~/.claude/nerv/nerv.yaml` (personal
defaults, never committed), project scope `<repo>/.nerv/nerv.yaml`
(committed), and the plugin cache under
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
disposable, backed-up `~/.claude/nerv/nerv.yaml` — never by pointing a real
`nerv install`/`nerv configure` run at a throwaway home directory.

Behavioral tests live under `bench/`: see [`bench/README.md`](bench/README.md)
for the manual journey suite and the `claude plugin eval` corpus.

## Configuration schema

Every Nerv by Gentle-AI setting lives in `nerv.yaml`, in two scope copies
(user `~/.claude/nerv/nerv.yaml` and project `<repo>/.nerv/nerv.yaml`,
project overriding user key by key) with sections for skills, per-role
model/effort overrides, critical paths, artifacts, git, and tasks.

See [docs/configuration.md](docs/configuration.md) for the full schema,
example YAML, and how to configure per-role models and effort (wizard,
`nerv configure --set-model`, or hand-editing).

## Documentation

- [docs/commands.md](docs/commands.md) — every command you can run: the
  Claude Code slash commands and the `nerv` CLI, with usage and examples.
- [docs/integration.md](docs/integration.md) — how Nerv by Gentle-AI
  integrates with gentle-ai, the agent roles table, and operations
  (activation, artifacts, task tracker, Engram project detection).
- [docs/configuration.md](docs/configuration.md) — the `nerv.yaml`
  configuration schema and how to configure per-role models and effort.
- [docs/releases.md](docs/releases.md) — versioning, release channels,
  and how to cut a release.
- [docs/troubleshooting.md](docs/troubleshooting.md) — known limitations
  and troubleshooting.

## Releases

The plugin follows [SemVer](https://semver.org/), with versions computed
from Conventional Commits and published automatically per Gitflow branch
(`develop` → alpha, `release/*` → rc, `main` → stable), plus the guard
checks and CI that gate a release.

See [docs/releases.md](docs/releases.md) for versioning, channels, cutting
a release, pre-releases, the release guard, CI, and marketplace notes.
