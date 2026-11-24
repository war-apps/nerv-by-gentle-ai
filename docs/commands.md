# Commands

Nerv by Gentle-AI has two command surfaces: Claude Code slash commands
(`/nerv:*`, run from inside a Claude Code session) and the `nerv` CLI binary
(run from a shell, including from inside the slash commands themselves).

## Claude Code slash commands

| Command | What it does |
|---|---|
| `/nerv:configure` | Configure NERV through guided questions (git, skills, models, repo) |
| `/nerv:init` | Bootstrap NERV for the current repository (writes `.ai-workflow/config.yaml`), then offer the configuration flows |
| `/nerv:status` | Show NERV activation, config resolution, and change status (read-only) |

### Task extension commands (`/task:*`)

The `/task:*` commands belong to the optional task extension (`task`),
not to core. Claude Code serves them from the extension's installed plugin, so nothing
is copied to `~/.claude/commands/task/`. Each one runs the task extension's
contract against the configured providers.

| Command | What it does |
|---|---|
| `/task:all` | List all pending tasks from all work sources (assigned to me AND unassigned), with running timers and tracked time |
| `/task:analysis` | Move a task to the analysis stage and start a local timer on it |
| `/task:blocked` | Mark a task as blocked - asks the mandatory cause, saves it as a task comment, moves it to blocked, stops this session's timer and logs the time |
| `/task:canceled` | Cancel a task - asks the mandatory cause, saves it as a task comment, moves it to canceled, stops this session's timer and logs the time |
| `/task:clear` | Discard a task's local timer WITHOUT logging time (asks for confirmation first) |
| `/task:close` | Close a task - stop this session's timer, consolidate the timelog, move it to implemented and complete it |
| `/task:done` | Mark a task as implemented - moves it to implemented, stops this session's timer and consolidates the timelog (does NOT complete the task; use /task:close for that) |
| `/task:me` | List my pending tasks from all work sources (only assigned to me), with running timers and tracked time |
| `/task:pending` | Move a task to the pending stage |
| `/task:priority` | Change a task's priority (High, Medium or Low) |
| `/task:replay` | Re-dispatch the task events a NERV run could not deliver (blocked or failed with replay: true) |
| `/task:setup` | Set up the tasks section of this repository's .ai-workflow/config.yaml interactively — the task provider, its repository keys and the connection check — without nerv configure's wizard |
| `/task:start` | Start working on a task - local timer, assigns it to me, moves it to the inDev stage, offers a worktree (creates the task if needed) |
| `/task:stop` | Stop working on a task and log its time (coupled blocks merge into one timelog; a gap starts a fresh timelog dated at the new block) |
| `/task:stopAll` | Stop tracking on ALL started tasks - stops every timer and logs time as /task:stop would for each |
| `/task:take` | Take a task - assign it to me (creating it if needed) WITHOUT starting timers or moving stages |
| `/task:testing` | Move a task to the testing stage |
| `/task:timers` | Show the local timers of a task (or all running timers), with their session |

### Release extension commands (`/release:*`)

The optional release extension (`release`) versions, builds and packages; its
agents are `release:versioner` and `release:builder`. Reference:
[release-commands.md](release-commands.md).

| Command | What it does |
|---|---|
| `/release:version` | Show the next release version of the repository (or of each project), computed by the native engine |
| `/release:setup` | Write the `release:` section of `.ai-workflow/config.yaml`: import an existing semantic-release config (`nerv release setup --import`), ask each project's build provider (`docker-compose` or `bash`) and its stage commands or compose file |
| `/release:build` | Build, test and package origin's code into a local artifact: dry run first, the real run only after your explicit yes; never writes to git |

### Deploy commands of the release extension (`/release:deploy*`, `/release:ship`)

The release extension also releases and deploys an artifact; its agent is
`release:deployer`, and its git hosts and targets are providers that come
with it (`release-github`, `release-ssh-compose`, `release-script`).
Reference: [deploy.md](deploy.md).

| Command | What it does |
|---|---|
| `/release:deploy` | Release and deploy the latest local artifact, or the one you name: dry run first, then the release commit, tag, push, image publish, git host release and targets only after your explicit yes |
| `/release:ship` | The next version, the build with the lint and test stages the build provider configures, then `/release:deploy` with the artifact it just packaged; the deploy is skipped when no deploy provider is configured (no `deploy.host`, no `deploy.targets`). Every dry run first, and each real run only after its own explicit yes |
| `/release:deploy-setup` | Write the `deploy:` section (host, targets, git) and suggest the providers to install |

### /nerv:configure

Walks through the sections you name — `git`, `skills`, `models`, `repo`,
or `all` — reading the current state first and writing
only the changes you confirm. Use it any time you want to change
`config.yaml` and `.ai-workflow/config.yaml` without hand-editing YAML.

```
/nerv:configure git
```

```
/nerv:configure models
```

```
/nerv:configure all
```

### /nerv:init

Sets up NERV in a repository that doesn't have it yet: asks only for the
keys that aren't already resolved (the base branch), writes `.ai-workflow/config.yaml`
and `.nerv/.gitignore`, and refreshes the skill registry with `gentle-ai
skill-registry refresh`. It creates no `odd/` file: the orchestrator
creates each feature's ODD document and run folder when a run starts, and
resolves the TDD mode and test runner per feature. It then offers, in one
multi-select question, the configuration flows available on the machine:
`/nerv:configure` (always); the tasks repository setup, `nerv-task init`
when nerv-task is installed or `/task:setup` when the task extension is
installed without nerv-task (never both); the release repository setup,
`nerv-release init` when nerv-release is installed or `/release:setup` and
`/release:deploy-setup` when the release extension is installed without
nerv-release (never both); and "None" to skip this step. It runs each selected flow in that order. Use
it once per repository.

```
/nerv:init
```

### /nerv:status

Read-only report of NERV's state for the current repo: activation,
resolved config (project vs. user vs. default), the installed `gentle-ai`
version, active changes and orchestrator lock state, and a per-role
model/effort table with drift detection. Use it to check what NERV will
actually do before trusting the session, or to debug a configuration that
isn't taking effect.

```
/nerv:status
```

## nerv CLI

| Command | What it does |
|---|---|
| `nerv version` | Print the binary and plugin versions |
| `nerv migrate` | Move a repository (and, with `--user`, the user configuration) from nerv 4.2.0 or 5.0.0-alpha.1 |
| `nerv doctor` | Check that NERV is in order on this machine and in the repository, and list the steps to take |
| `nerv upgrade` | Replace the nerv binary with the newest release (`--alpha`, `--rc`), upgrade gentle-ai within its 4.x major version, then run nerv's install — gentle-ai still gets upgraded and install still runs, with the running executable, when nerv itself cannot be replaced |
| `nerv configure` | Configure NERV non-interactively (or run the wizard) |
| `nerv install` | Install NERV (plugin, cache, skills, Engram KB) |
| `nerv uninstall` | Uninstall NERV |
| `nerv apply-models` | Apply `models:` overrides to the cached agents |
| `nerv skills` | Install or verify the skills NERV's defaults reference |

### nerv configure

With no mode flag, runs the interactive setup wizard (when stdin is a
terminal, or when `--answers` is given). Otherwise exactly one mode flag
is required.

```
Usage: nerv configure [flags]

With no mode flag, runs the interactive setup wizard (when stdin is a
terminal, or when --answers is given):
  --answers <file>          Drive the wizard from a file, one answer per
                             line (blank line = keep the current value)
                             instead of prompting
  --skip-skills             Skip the required-skills install offer
  --skip-models             Skip the per-role models editor
  --skip-repos              Skip the repository init loop
  --no-refresh              Skip the closing apply-models/cache-refresh step

Otherwise, exactly one mode flag is required:
  --print                   Print the current configuration as JSON
  --set key=value           Set a managed config key (repeatable)
  --set-model role=spec     Set a role's model/effort override (repeatable);
                             spec is model[/effort], from:<phase>, or default
  --init-repo <path>        Initialize a repository for NERV

With --init-repo:
  --repo-base <branch>      Base branch override
  --repo-provider-config <provider>.<key>=<value>
                            Write tasks.providers.<provider>.<key> into the
                            repository's .ai-workflow/config.yaml (repeatable; repo-scope
                            keys of the provider's provider.yaml only, shipped
                            or the project's in .ai-workflow/providers/)

If `.ai-workflow/config.yaml` already exists it is not recreated, and an existing
`tasks:` block in it is left untouched.

Other flags:
  --json                    Print the result as JSON (ignored by --print, which always does)
  --config <path>           Override the user-scope config.yaml path
```

```
nerv configure --print --json
```

```
nerv configure --set git.base_branch=develop --set git.worktree=ask
```

```
nerv configure --set-model aoba=opus/high
```

```
nerv configure --init-repo . --repo-base develop --repo-provider-config teamwork.project_id=1271726 --repo-provider-config teamwork.tasklist_id=3951970
```

When the `task` extension is enabled, the wizard's "Task providers" section prompts
each enabled provider's scalar config keys, read from its `provider.yaml`,
plus `tasks.provider` when more than one provider is enabled. It replaces
the former tasks, Teamwork and commands sections, and the flags
`--repo-provider`, `--repo-project-id`, `--repo-tasklist-id`,
`--install-commands` and `--skip-commands` are gone.

### Moved to nerv-release

`nerv version next`, `nerv release`, `nerv release setup --import` and
`nerv deploy` moved to the nerv-release tool (`nerv install --tools
nerv-release`): they are now `nerv-release version next`, `nerv-release
build`, `nerv-release setup --import` and `nerv-release deploy`. Run with
nerv, each prints a line naming its replacement and exits 2. The hidden `nerv provider`
command is gone. `nerv pipeline` was removed earlier; its steps are
nerv-release's build and deploy.

### nerv migrate

Moves a repository from nerv 4.2.0 or 5.0.0-alpha.1 to this version, once:

- `.nerv/nerv.yaml` (4.2.0) or `.ai-workflow.yml` (5.0.0-alpha.1) becomes `.ai-workflow/config.yaml`;
- the 4.2.0 `release:` section becomes today's `release:` section, with the semantic-release config imported;
- `pipeline:` becomes `deploy:`;
- the in-flight `openspec/changes` become ODD feature documents;
- a project file without `tasks.provider` gets the user's
  (`defaults.tasks.provider`, or the older user `tasks.provider`), since
  `tasks:` is read from the project only;
- when nerv-task is installed (on `PATH` or in its default install
  directory), `tasks:` moves to nerv-task's own `task.yaml` files instead
  of staying in nerv's own configuration: the project file's `tasks:` to
  `.ai-workflow/task.yaml` (its repository keys), and, with `--user`, the
  user config's personal `tasks:` keys and its `defaults.tasks:` seed to
  the global `task.yaml` (`~/.config/ai-workflow/task.yaml`,
  `%AppData%\ai-workflow\task.yaml` on Windows), both keyed without the
  `tasks.`/`defaults.tasks.` prefix;
- when nerv-release is installed, `release:` and `deploy:` move to
  nerv-release's own `release.yaml` files, every key at the same path
  (nerv-release reads `release:`, `deploy:`, `defaults.release:` and
  `defaults.deploy:` under the same names): the project file's `release:`
  and `deploy:` to `.ai-workflow/release.yaml`, and, with `--user`, the
  user config's personal deploy keys
  (`deploy.providers.ssh-compose.{infisical_domain, ssh_auth, ssh_key_file, ssh_known_hosts, ssh_connect_timeout}`)
  and its `defaults.release:`/`defaults.deploy:` seeds to the global
  `release.yaml` (next to the user `config.yaml`);
- `.nerv/.gitignore` is created.

`--user` also moves `~/.claude/nerv/nerv.yaml`, or the user `config.yml`,
to the user `config.yaml`,
with the personal keys of the project file, and moves the user file's
run-time `tasks:`, `release:` and `deploy:` keys under
`defaults.<section>`, keeping the personal ones in place (see
[configuration.md](configuration.md#extension-sections-tasks-release-deploy)). `--dry-run` lists the steps
it would take and writes nothing. `--install` installs the extensions and
providers the repository used. Every changed or removed file is backed up first, and a
second run changes nothing. See [migration.md](migration.md).

**The task.yaml step.** A key already set in nerv-task's `task.yaml` is
never overwritten: it is left where it is (the project file, or the user
config) and reported as a conflict instead, the same way every other
already-set key is. The step is idempotent (a second run moves nothing
already moved) and one-directional — only `--user` ever writes the global
`task.yaml`, from the user config; a repository's `tasks:` never becomes a
global value. `--dry-run` lists this step like every other, and every
`task.yaml` it writes is backed up first, next to the rest of the run's
backups.

**The release.yaml step** gives the same guarantees: a key already set in
`release.yaml` is kept where it is and reported, the tool's file is
written before `config.yaml` loses the moved keys, and only `--user`
writes the global `release.yaml`. A personal ssh-compose key found in a
project file is never moved into any `release.yaml`: it stays there and
is reported, for you to set in the global `release.yaml`.

```
nerv migrate --dry-run
nerv migrate --user --install
```

### nerv doctor

Checks that everything NERV needs is in order and changes nothing:

- on this machine: gentle-ai 4.x, Claude Code and engram on the PATH, the
  NERV plugin installed, enabled and at this binary's version, its skills,
  and the user configuration (a warning names its `tasks:`, `release:` and
  `deploy:` keys that no longer apply, with `nerv migrate --user` as the
  step; a `tool config` warning names a pending move of its settings to
  the global `task.yaml` or `release.yaml` of nerv-task or nerv-release,
  when that tool is installed);
- in the repository: `.ai-workflow/config.yaml` with `enabled: true`, a
  pending `nerv migrate` (naming `.ai-workflow/task.yaml` or
  `.ai-workflow/release.yaml` when it moves settings there, for an
  installed nerv-task or nerv-release), the project extensions and providers, the
  `release:`, `deploy:` and `tasks:` sections (read from the project file
  only) with the extensions they need, the origin remote and the base branch.

Each check prints `[ok]`, `[!!]` (should be changed), `[xx]` (NERV cannot
work until it is fixed) or `[--]` (optional, not set up). Then come the
steps to take, failures first, and the optional ones:

```
This repository (/home/me/widgets):
  [!!]  config       .nerv/nerv.yaml is the nerv 4.2.0 configuration
  [!!]  migration    nerv migrate has 3 step(s) to take

Summary: 6 ok, 2 warning(s), 0 failed: needs attention

Next steps:
  1. Run nerv migrate --dry-run to see the steps, then nerv migrate
```

`--json` prints the same results. The exit code is 1 when a check failed,
0 otherwise.

### nerv upgrade

```bash
nerv upgrade          # newest stable release
nerv upgrade --rc     # newest of stable and vX.Y.Z-rc.N
nerv upgrade --alpha  # newest of stable and vX.Y.Z-alpha.N
```

Replaces the running `nerv` binary (symlinks resolved) with the newest
release of the channel, the way `scripts/install.sh` installs one: it
downloads `nerv_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows) and
`checksums.txt` from the GitHub release, verifies the archive's SHA-256,
and installs the new binary over the old one — renamed directly over it
on Linux and macOS, or, on Windows, renamed aside to `nerv.exe.old` first
since Windows refuses to replace a running executable in place; a stale
`nerv.exe.old` left over from an earlier upgrade is removed, best-effort,
at the start of the next `nerv upgrade`. A prerelease channel also
considers stable releases, so it never stays on an older prerelease when a
newer stable one exists. Then it runs the new binary's `nerv install`
(`nerv install --no-configure` without an interactive terminal) and exits
with its exit code.

Before that install, it upgrades gentle-ai when it is on PATH at 4.x and
its newest GitHub release is newer and still 4.x, the major version NERV
supports: it runs `gentle-ai upgrade gentle-ai`, then `gentle-ai sync`.
`gentle-ai upgrade` always installs the newest release, so when that
release is a new major version (5.x) nerv upgrade says so and leaves
gentle-ai as it is; a gentle-ai that is missing or not 4.x is not touched
either. After a gentle-ai upgrade, `nerv install` runs even when nerv was
already up to date, to lay NERV over the files `gentle-ai sync` rewrote. A
failed gentle-ai upgrade is reported, the rest of the upgrade still runs,
and the exit code is 1.

`nerv install` also upgrades gentle-ai on its own (see "nerv install"
below), so the install `nerv upgrade` runs here is told
`--skip-gentle-ai-upgrade`: gentle-ai already got its one upgrade attempt
above and is never upgraded twice.

Already on the newest release, or on a newer one (a `dev` build counts as
older than any release), it says so and changes nothing. A failed
download or checksum — or an unsupported platform (only Linux, macOS and
Windows on amd64 or arm64 have prebuilt binaries; elsewhere use
`go install ...@latest`) — leaves the current binary untouched, but
gentle-ai still gets its upgrade attempt and `nerv install` still runs
with the running executable, so NERV's own files stay laid over
gentle-ai's; nerv upgrade still exits 1 for the failed replace.
`--alpha` with `--rc` is a usage error (exit 2).

After that install (whether or not it actually ran anything), it offers
the NERV-ecosystem tools the same way `nerv install` does — every
catalogue tool's state shown and confirmed one by one on a terminal,
`--tools <names>` / `--no-tools` controlling the step the same way — but
running `<tool> upgrade` for each confirmed or named tool instead of its
install script. A failed individual tool upgrade is only a warning and
never stops the others; naming an unknown tool in `--tools` refuses and
sets `nerv upgrade`'s own exit code to 1, when nothing else already set
it to a non-zero value.

Then it offers `nerv migrate --user` the same way `nerv install` does
(see "nerv install" below); the install it ran above leaves that offer to
it, so it is asked once.

### nerv install

Before any of that, when gentle-ai is on PATH and already 4.x, it
upgrades gentle-ai to its newest 4.x release the same way `nerv upgrade`
does (`gentle-ai upgrade gentle-ai`, then `gentle-ai sync`), so NERV's own
files below get laid over the newest gentle-ai; a newer major version is
reported and left alone, and a failed release lookup or upgrade is only a
warning — install continues with the gentle-ai it found, and the exit
code is unaffected.

Next, it offers the NERV-ecosystem tools (`nerv-task`, then
`nerv-release`; see the README's "NERV ecosystem" section): on a terminal with neither `--tools`
nor `--no-tools`, every catalogue tool's state (not installed, installed
with its version, or outdated against its latest release) is shown and
confirmed one by one, before anything about NERV's own extensions is
asked; a confirmed tool's official install script then runs (`install.sh`
on Linux/macOS, `install.ps1` on Windows). `--tools <names>`
(comma-separated) installs those tools outright, skipping the prompt;
`--no-tools` skips the step entirely; without a terminal and neither
flag, a hint naming both flags is printed instead. A tool confirmed or
named here, or already installed on this machine, also stops the steps
below from ever offering, selecting or re-enabling the extension it
replaces (`task`, `task-teamwork` for `nerv-task`; `release` and its six
providers `release-bash`, `release-conventional-commits`,
`release-docker-compose`, `release-github`, `release-script`,
`release-ssh-compose` for `nerv-release`) — an extension already
enabled is left exactly as found, since disabling it is the replacing
tool's own install, never nerv's.

After the extensions (and before the closing wizard), when a tool is
installed — before or in this run — and its settings are still in your
user `config.yaml`, it offers `nerv migrate --user`: on a terminal it
shows the steps its dry run lists, under `=== Move settings to the
NERV-ecosystem tools ===`, and asks `Run nerv migrate --user now? [y/N]`;
only a `y` writes anything (with the usual backups). Without a terminal it
prints a hint naming `nerv migrate --user --dry-run` (to see the steps),
then `nerv migrate --user`. It covers the user configuration
only: a repository's own move is `nerv migrate`'s, which `nerv doctor`
reports.

Materializes the embedded core plugin, registers it in Claude Code's
`settings.json`, refreshes the plugin cache, applies model/effort
assignments, ensures the Engram "nerv" knowledge base, and installs
skills. It also installs the optional extensions and providers named by `--with`,
keeps the ones already enabled up to date, and, when stdin is a terminal,
offers the others, in this order:

1. the task extension (`task`, the `/task:*` commands);
2. its providers (`task-<name>`, such as `task-teamwork`), only
   once task is installed;
3. the release extension (`release`: `nerv release`, `nerv deploy`), which
   brings its providers with it, unasked (`release-conventional-commits`,
   the version engine, `release-docker-compose` and `release-bash`, the
   builds, and `release-github`, `release-ssh-compose` and
   `release-script`, the deploy).

An extension whose `extension.yaml` says `providers: bundled` (release)
installs every provider it ships with it, each listed as `Adding
<provider> (bundled with <extension>)`; one that says `providers:
suggested` (task) has each provider offered after it.

Requirements: a task provider requires `task` and a release provider
`release`; each is offered only once its extension is installed or chosen.
The `deploy` plugins of 5.2.0 and earlier merged into `release`: install
removes them from `settings.json`, and `--with deploy` is refused with where
it went. An extension or provider named in `--with` brings what it
requires with it, and a requirement this nerv does not ship is refused. Copies of the `/task:*` commands that an older release put in
`~/.claude/commands/task/` are reported, and removed only after you
confirm. See the README's "Install" section for the shell one-liners and
environment variables (`NERV_CHANNEL`, `NERV_INSTALL_DIR`,
`NERV_NO_INSTALL`, `NERV_VERSION`); the install scripts are
[`../scripts/install.sh`](../scripts/install.sh) (Linux, macOS) and
[`../scripts/install.ps1`](../scripts/install.ps1) (Windows, same
environment variables, `%LOCALAPPDATA%\nerv\bin` as the default
`NERV_INSTALL_DIR`).

```
Usage: nerv install [flags]

Flags:
  --require-gentle-ai   Fail (exit 1) when gentle-ai is missing or not 4.x
  --no-skills           Skip installing skills
  --no-configure        Skip the closing wizard/hint entirely
  --with <names>        Comma-separated extensions and providers to install, such as
                        task,task-<provider>,release,deploy,release-<provider>
                        (each brings what it requires)
  --tools <names>       Comma-separated NERV-ecosystem tools to install (such as
                        nerv-task,nerv-release), skipping the confirm prompt
  --no-tools            Skip the NERV-ecosystem tools step entirely
```

```
nerv install
```

```
nerv install --with task-teamwork
```

```
nerv install --with release
```

```
nerv install --no-configure --no-skills
```

### nerv uninstall

Removes NERV's `settings.json` registration, uninstalls the cached plugin,
and removes the materialized marketplace directory.

```
Usage: nerv uninstall [flags]
```

```
nerv uninstall
```

### nerv apply-models

Applies the `models:` overrides from the user-scope `config.yaml` (merged
over the plugin's own committed defaults) to the cached agent frontmatter.
Run this after hand-editing `models:`, or after `nerv configure --set-model`
if you skipped the automatic apply step.

```
Usage: nerv apply-models [flags]
```

```
nerv apply-models
```

### nerv skills

Installs or verifies the Claude Code user-scope skills the NERV plugin
defaults reference: external ones via `npx skills add ... -g`, gentle-ai
ones via one `gentle-ai sync --agents claude-code --skills ...` for all the
missing ones. In a terminal it asks first (default No), naming the missing
skills and warning that `gentle-ai sync` also rewrites gentle-ai's managed
files (e.g. `~/.claude/CLAUDE.md`); a declined or failed sync, or a run
without a terminal (which never syncs), leaves a remedy line per skill still
missing. `nerv install` and the wizard's skills step follow the same rule.

```
Usage: nerv skills [flags]

Flags:
  --dry-run          Print the exact npx and gentle-ai commands for the
                      missing skills instead of running them
  --json             Print the computed status as a JSON array
  --only name,...    Restrict processing to these skill names
```

```
nerv skills --dry-run --json
```

```
nerv skills --only tdd,solid-principles
```

## Maintainer: nerv maintainer-release

`maintainer-release` is maintainer-only tooling, listed under "Maintainer commands"
in `nerv --help`. It computes the next semantic version from
Conventional Commits since the last release tag (`preview`/`apply`), or
checks release readiness before tagging (`guard`). In normal use this is
driven by CI, not run by hand — see [docs/releases.md](releases.md) for
the full Gitflow release procedure and the automatic pre-release channels.

```
Usage: nerv maintainer-release <preview|apply|guard> [flags]

preview/apply flags:
  --repo <dir>                      Repository root (default ".")
  --version X.Y.Z                   Explicit version override
  --pre-release <label>             Pre-release label (e.g. alpha, beta, rc)
  --pre-release-base next|current   What --pre-release bumps from (default "next")
  --json                            Print the result as JSON

guard flags:
  --repo <dir>     Repository root (default ".")
  --notes <path>   Write the changelog section body here on success
  --json           Print the result as JSON
```

```
go run ./cmd/nerv maintainer-release preview
```

```
go run ./cmd/nerv maintainer-release guard --notes CHANGELOG_SECTION.md
```

## Automatic (hooks)

Two hooks run on every `SessionStart` (`startup`, `resume`, `clear`, or
`compact`), with no command to type: `nerv-session-start.sh` resolves
whether NERV is enabled and prints a short activation header — Claude
Code caps hook stdout at 10,000 characters, so the hook cannot inject the
full protocol itself; the header has the session load the orchestrator
skill instead — and
`nerv-engram-project.sh` ensures the Engram project context for the
current repository is set up. See
[`plugin/hooks/hooks.json`](../plugin/hooks/hooks.json).

## Not commands: skills and agents

`nerv-orchestrator` is a Claude Code **skill**, not a command you
type. The SessionStart activation header has the session
invoke the orchestrator skill with the Skill tool in repos where NERV is
enabled, and it governs how the session
routes work to the named agents (Ikari, Misato, the MAGI, the pilots,
Hyuga, Kaji, and so on). The task extension's `task` skill and its
`task:dispatcher` agent work the same way: the orchestrator launches the
dispatcher only when the task extension is installed. The release extension's `release` skill
with its `release:versioner` and `release:builder` agents, and the deploy
extension's `deploy` skill with its `release:deployer` agent, are loaded and
launched by their own `/release:*` and `/release:deploy*` commands. You never invoke them by name —
see [integration.md#roles](integration.md#roles) for what each agent does
and when it runs.
