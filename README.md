# NERV Gentle-AI

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

NERV Gentle-AI is a Claude Code plugin implementing an Evangelion-named
multi-agent governance workflow. Ikari orchestrates a cast of named
agents through it; see [docs/integration.md](docs/integration.md#roles) for
what each one does and when it runs.

NERV Gentle-AI brings governance that a plain implementation loop lacks: a
per-task MAGI vote gated by criticality, a governance veto on new
skills/scripts/commands, a mandatory quality gate before any audit, a
multi-pass audit compiler with a ranked user issue gate, an append-only
deliberation log, and a run summary reporting tokens, time, and model per
agent.

**Claude Code only, for now.** NERV Gentle-AI targets Claude Code; OpenCode,
Codex and Pi are not supported yet.

## Relation to gentle-ai

NERV Gentle-AI is an **overlay**, not a fork. It reuses gentle-ai's native
engine and contracts unchanged — the SDD artifact pipeline, RDD
(receipt-driven review), the skill registry and resolver, strict TDD,
delivery budgeting with chained PRs, and the lossless blocking-prompt
contract — and expresses NERV Gentle-AI's own governance on top. NERV
Gentle-AI never modifies any gentle-ai file: nothing is ever written
under `~/.claude/agents` or `~/.claude/skills`, so `gentle-ai sync` cannot
see or touch it.

See [docs/integration.md](docs/integration.md) for the full layering,
activation, and pipeline diagrams.

## Requirements

- **gentle-ai 3.x** — major version 3 is required; minor and patch are free.
  Tested against 3.7.0. 4.x is untested and not supported until NERV
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

**macOS:**

```bash
brew install war-apps/tap/nerv
```

**Linux and macOS:**

```bash
curl -fsSL https://raw.githubusercontent.com/war-apps/nerv-gentle-ai/main/scripts/install.sh | bash
```

or with `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/war-apps/nerv-gentle-ai/main/scripts/install.sh | bash
```

Environment options (export before running, or prefix the command):
`NERV_CHANNEL` (`stable` default, `rc`, or `alpha`), `NERV_INSTALL_DIR`
(default `$HOME/.local/bin`), `NERV_NO_INSTALL` (non-empty: download and
install the binary only, skip running `nerv install` afterwards), and
`NERV_VERSION` (an explicit release tag, e.g. `v1.2.3`, overriding channel
resolution entirely).

**Windows:**

```bash
go install github.com/war-apps/nerv-gentle-ai/cmd/nerv@latest
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
7. Installs the skills the plugin defaults reference.
8. Runs the interactive configuration wizard when stdin is a terminal, or
   prints the `nerv configure` hint otherwise.

Flags: `--no-configure` (skip the closing wizard/hint entirely),
`--no-skills` (skip installing skills), `--require-gentle-ai` (fail with
exit 1 when gentle-ai is missing or not 3.x).

### Updating

Re-run the install method you used (`brew upgrade war-apps/tap/nerv`, the
`scripts/install.sh` one-liner, or `go install ...@latest` on Windows), then
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
git-clone path), configure NERV Gentle-AI with the guided wizard:

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
`/nerv:status` reports the resolved config and, while a run is in progress,
the orchestrator lock state.

The wizard walks through six sections:

1. **Prerequisites** — informational checks for `gentle-ai` (major version
   3 required), `engram` (optional), and `claude` (required for the cache
   refresh step).
2. **Required skills** (`--skip-skills`) — installs the missing external
   skills the plugin defaults reference, via
   `npx skills add <repo> --skill <id> -g -a claude-code -y`; an
   already-present skill is never touched. `nerv skills --dry-run --json`
   previews the same check without the wizard.
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

1. `git clone https://github.com/war-apps/nerv-gentle-ai.git` (the folder
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

Every NERV Gentle-AI setting lives in `nerv.yaml`. There are exactly two
copies, same schema: user scope `~/.claude/nerv/nerv.yaml` (personal
defaults, never committed) and project scope `<repo>/.nerv/nerv.yaml`
(committed). Project overrides user key by key; a missing key falls back
to the user file, then to the built-in default.

```yaml
# ~/.claude/nerv/nerv.yaml (user) and <repo>/.nerv/nerv.yaml (project): same schema
enabled: true                       # project scope only: activates NERV in this repo
skills:                             # stacks per consuming role; names must exist in .atl/skill-registry.md
  testing: [tdd, playwright-best-practices]                        # ritsuko, kaworu, maya
  code: [dotnet-best-practices, typescript-best-practices]         # pilots
  best-practices: [best-practices, solid-principles, clean-code-guard]  # balthasar
  architecture: [hexagonal-architecture, c4-architecture]          # melchor
  audit: [security-review, clean-code-guard]                       # kaji passes
models:                             # per-role model and effort; project overrides user, key by key
  misato: { model: fable, effort: high }
  melchor: { from: jd-judge-b }     # inherit gentle-ai's assignment for that phase (state.json)
  aoba: { model: sonnet, effort: low }
critical_paths: [auth/, payments/, migrations/, infra/]            # Hyuga auto-critical
artifacts:
  commit: at-close                  # with-change | at-close | never (default: at-close)
git:
  base_branch: develop              # default base for the worktree offer
  worktree: ask                     # ask | always | never
  worktree_pattern: ".claude/worktrees/{slug}"   # where task worktrees are created; {slug} {branch} {prefix} {id} {repo}
                                                  # `nerv configure`'s wizard offers default / herdr / custom; herdr
                                                  # reuses its own [worktrees] directory as <directory>/{repo}/{slug}
  branch_pattern: "feature/{prefix}-{id}-{slug}"   # prefix comes from the provider (tw, gh, jira)
  commit_ref_pattern: "({PREFIX}-{id})"
tasks:
  provider: teamwork                # teamwork | github-projects | jira | none ; "ask" when absent
  ask_when_missing: true            # preflight asks task + worktree + branch if no active task
  subtasks_per_wave: false
  providers:                        # one block per provider, only the enabled one is required
    teamwork:
      task_ref_prefix: tw           # {prefix} in branch_pattern / commit_ref_pattern
      assignee_id: 686035           # user scope
      project_id: 1271726           # project scope
      tasklist_id: 3951970          # project scope
      stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }
    github-projects: { task_ref_prefix: gh, owner: "", project_number: 0 }    # later
    jira: { task_ref_prefix: jira, site: "", project_key: "" }               # later
  sources:                          # extra work sources for listings (replaces ~/.claude/work/sources.md)
    - { name: erp-proveedores, type: google-sheets, ... }
```

`models:` assigns a per-role `model` and `effort` override, with project
overriding user key by key like every other section — each entry is an
inline map, one role per line, with `model`, `effort`, or `from` keys;
`from: <gentle-ai-phase>` inherits that phase's `model`/`effort` from
`~/.gentle-ai/state.json`'s `claude_phase_assignments`, and an explicit
`model`/`effort` on the same line wins over the inherited one. A role
absent from both `models:` files keeps the plugin default (see
[docs/integration.md](docs/integration.md#roles)). `model` and `effort` are
applied differently, because Claude Code itself treats them differently:
`model` is a per-call launch parameter, so Ikari reads the resolved
`models:` map (both scopes merged) and passes it to every agent launch —
it takes effect immediately, no reinstall needed. `effort` is honored only
from the launched agent file's own frontmatter, so it can only be applied
by rewriting the cached agent files themselves: `nerv apply-models` reads
`models:` from the **user-scope** file only (`~/.claude/nerv/nerv.yaml`)
and rewrites `model:`/`effort:` in each affected `<role>.md` under the
plugin cache. Project-scope `models:` therefore applies to `model` only —
`effort` is not applicable at project scope, since project config cannot
reach into a user's local plugin cache. `nerv install` runs this same apply
step automatically at the end of its own cache refresh.
`/nerv:status` reports the resolved `models:` table (both scopes merged)
and warns when the cached agent frontmatter has drifted from it, so a
pending `nerv apply-models` run is visible without inspecting the cache by
hand.

### Configuring models and effort

The wizard's **Models** section (see "Setup" above) prints the resolved
table (role, model, effort, source — `override`, `gentle-ai:<phase>`, or
`default`), then lets you edit it role by role, or by group (`magi`,
`pilots`, `kaji-passes`, `all`), until you type `done`. For each role it
asks for a model (`sonnet`/`opus`/`haiku`/`fable`/`inherit`, a custom
`claude-...` id, or `from:` a gentle-ai phase listed from
`~/.gentle-ai/state.json`) and an effort (`low`/`medium`/`high`/`xhigh`/
`max`), with Enter keeping the current value; `reset <role|group>` clears
an override back to the plugin default. Confirming writes the block to the
user-scope `nerv.yaml`, after backing up the file to
`<path>.bak-models-<yyyyMMdd-HHmmss>`, then offers to run
`nerv apply-models` immediately.

Non-interactively, `nerv configure --set-model role=model[/effort]` (or
`role=from:<gentle-ai-phase>`, or `role=default` to clear an override)
applies one role's override at a time to the same user-scope file; pass it
repeatedly for a batch. This always edits the user-scope file — project
scope has no dedicated command, since project-scope `models:` only ever
affects `model` (see above). Edit the project `.nerv/nerv.yaml` `models:`
block by hand for that case, using the same inline-map syntax and `from:`
behavior described above.

You can also skip both the wizard and `--set-model`, and edit the
`models:` block by hand in either `nerv.yaml`, then run
`nerv apply-models` yourself. Either way, restart Claude Code afterwards
for the change to take effect.

## Documentation

- [docs/integration.md](docs/integration.md) — how NERV Gentle-AI
  integrates with gentle-ai, the agent roles table, and operations
  (activation, artifacts, task tracker, Engram project detection).
- [docs/troubleshooting.md](docs/troubleshooting.md) — known limitations
  and troubleshooting.

## Releases

**Versioning.** The plugin follows [SemVer](https://semver.org/). The
`version` in `plugin/.claude-plugin/plugin.json`, the git tag `vX.Y.Z`,
the matching `## [X.Y.Z]` section in `CHANGELOG.md`, and the `nerv version`
binary version (injected at build time from the tag by goreleaser) all
agree. This is not cosmetic: Claude Code detects plugin updates by
comparing `plugin.json`'s `version` string, so a release that does not
bump it is invisible to every installed user; `nerv release guard` asserts
the binary and plugin versions agree before a release is allowed to ship.

**How the version is computed.** `nerv release preview`/`apply` derives the
next version from the [Conventional Commits](https://www.conventionalcommits.org/)
reachable since the last `vX.Y.Z` tag: `feat` bumps minor, `fix`/`perf`
bump patch, a `!` before the colon or a `BREAKING CHANGE:` footer bumps
major, and every other type (`docs`, `chore`, `test`, `refactor`, `ci`,
`build`, `style`) contributes no bump on its own.

**Channels.** Every push to a Gitflow branch publishes automatically —
nothing to run by hand:

| Branch push | Publishes |
|---|---|
| `develop` | `vX.Y.Z-alpha.N` pre-release — version computed from Conventional Commits since the last stable tag; nothing is published when there is nothing releasable. |
| `release/*` | `vX.Y.Z-rc.N` pre-release — version taken from `plugin.json` on that branch. |
| `main` (merge) | Stable `vX.Y.Z` release — the matching `CHANGELOG.md` section becomes the release notes, and goreleaser publishes the linux/darwin amd64/arm64 binaries, their checksums, and the Homebrew formula (when the `war-apps/homebrew-tap` repository and `HOMEBREW_TAP_TOKEN` secret exist; otherwise the formula publish step is skipped). |

**Cutting a release (Gitflow).**

1. `git checkout -b release/X.Y.Z develop`
2. `go run ./cmd/nerv release preview` — review the computed version and
   changelog section.
3. `go run ./cmd/nerv release apply` (or `--version X.Y.Z` to override the
   computed bump) — writes `plugin.json` and inserts the section into
   `CHANGELOG.md`.
4. Review `CHANGELOG.md`, then commit `chore(release): X.Y.Z`.
5. Open the PR to `main`. On merge, the Release workflow runs the full
   suite plus `nerv release guard`, creates the tag and the GitHub Release
   with that changelog section as its notes, runs goreleaser to publish
   the binaries/checksums/formula, and opens the back-merge PR to
   `develop` — merge that PR to close the loop.

Hotfixes follow the same steps from `hotfix/X.Y.Z` branched off `main`
instead of `develop`.

**Pre-releases.** Pre-releases are automatic (see "Channels" above): the
`release.yml` workflow itself computes the version, creates the tag, and
publishes the GitHub pre-release when it sees the push — a `release/*`
push runs the `rc` job and a `develop` push runs the `alpha` job, neither
going through `main` nor through goreleaser (goreleaser only runs on the
stable release, after the tag is created). There is nothing to run by hand
and no tag to push yourself; `nerv release preview --pre-release
alpha|rc` is what the workflow calls internally.

**What the guard refuses, and how to recover.** `nerv release guard` fails
the Release workflow before it tags or publishes anything when the version
in `plugin.json` is already tagged, `CHANGELOG.md` has no matching
`## [X.Y.Z]` section, or the binary version and the embedded plugin
version disagree. If the Release job fails *after* the tag was already
pushed (e.g. the GitHub Release step itself failed), create the release by
hand with `gh release create vX.Y.Z --notes-file <section>` — re-running
the workflow will refuse, since the guard sees the tag already exists.

**CI.** `.github/workflows/ci.yml` runs `gofmt -l`, `go vet ./...`, and
`go test ./...`, plus the two hook suites
(`tests/hook-session-start.test.sh`, `tests/hook-engram-project.test.sh`)
and `tests/install-sh.test.sh`, on every pull request targeting `develop`
or `main`.

**Marketplace consumers.** A marketplace added from GitHub serves the
repository's default branch. For users to receive released versions
rather than in-progress `develop` content, the default branch must be
`main`, or the marketplace entry must pin an explicit `ref`.
