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

### Commands of the NERV-ecosystem tools (`/task:*`, `/release:*`)

nerv ships only the `/nerv:*` commands. The `/task:*` commands (and the
`task:dispatcher` agent the orchestrator emits its task events to) come
from the [nerv-task](https://github.com/war-apps/nerv-task-by-gentle-ai)
tool, and the `/release:*` commands from the
[nerv-release](https://github.com/war-apps/nerv-release-by-gentle-ai)
tool. Install them with `nerv install --tools nerv-task,nerv-release` (see
[nerv install](#nerv-install)); each tool documents its own commands. Up
to 6.6 they were extensions nerv shipped itself; see
[migration.md](migration.md#from-66-extensions-moved-to-tools).

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
`/nerv:configure` (always); `nerv-task init` when nerv-task is installed;
`nerv-release init` when nerv-release is installed; and "None" to skip
this step. It runs each selected flow in that order. Use it once per
repository.

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

Other flags:
  --json                    Print the result as JSON (ignored by --print, which always does)
  --config <path>           Override the user-scope config.yaml path
```

With `--init-repo`, an existing `.ai-workflow/config.yaml` is not
recreated, and an existing `tasks:` block in it is left untouched; a new
one gets no `tasks:` section (task settings belong to nerv-task).

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
nerv configure --init-repo . --repo-base develop
```

The wizard asks nothing about tasks: task settings belong to nerv-task
(`nerv-task configure`, `nerv-task init`). Its former "Task providers"
section and `--repo-provider-config` were removed in 7.0, after the
earlier `--repo-provider`, `--repo-project-id`, `--repo-tasklist-id`,
`--install-commands` and `--skip-commands`.

### Moved to nerv-release

`nerv version next`, `nerv release`, `nerv release setup --import` and
`nerv deploy` moved to the nerv-release tool (`nerv install --tools
nerv-release`): they are now `nerv-release version next`, `nerv-release
build`, `nerv-release setup --import` and `nerv-release deploy`. Run with
nerv, each prints a line naming its replacement and exits 2. The hidden `nerv provider`
command is gone (an unknown command now). `nerv pipeline` was removed earlier; its steps are
nerv-release's build and deploy. The complete list of what left nerv with
the extensions, and its replacements, is in
[migration.md](migration.md#from-66-extensions-moved-to-tools).

### nerv migrate

Moves a repository from nerv 4.2.0 or 5.0.0-alpha.1 to this version, once:

- `.nerv/nerv.yaml` (4.2.0) or `.ai-workflow.yml` (5.0.0-alpha.1) becomes `.ai-workflow/config.yaml`;
- an older nerv's release or deploy configuration (a 4.2.0 `release:` or
  `pipeline:` section, the 5.0.0-alpha.1 `ship:` section, a 5.x
  `provider: bash` deploy target or `deploy-<name>` project provider) is
  converted by nerv-release: when it is installed, `nerv migrate` moves
  those sections, as they are, to `release.yaml` and then runs
  `nerv-release migrate --cwd <repository>` (`--user` for the user
  config's), which converts them (not on `--dry-run`; its refusal is exit
  1, any other failure exit 2; a later run runs it again while
  `release.yaml` still holds an older section); without it, they are refused, naming
  `nerv install --tools nerv-release` (see
  [migration.md](migration.md#older-release-and-deploy-sections));
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
[configuration.md](configuration.md#task-release-and-deploy-settings)). `--dry-run` lists the steps
it would take and writes nothing. `--install` then runs `nerv install
--tools` with the tools the repository uses. Every changed or removed file is backed up first, and a
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
  installed nerv-task or nerv-release), the extension plugins nerv no
  longer ships that the repository's `.claude/settings.json` or
  `.claude/settings.local.json` still enable (an `old plugins` warning:
  `nerv install` and `nerv upgrade` clean only your user settings), the
  origin remote and the base branch. Task, release and deploy settings are
  the tools' own to check.

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
running `<tool> upgrade` for each confirmed or named tool that is
installed instead of its install script (a tool that is not installed is
asked as "Install" and gets its install script). Then it disables the extension plugins nerv no longer
ships, the way `nerv install` does (see "nerv install" below), after its
tools step, so it knows which tools are installed. A failed individual tool upgrade is only a warning and
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
confirmed one by one; a confirmed tool's official install script then
runs (`install.sh` on Linux/macOS, `install.ps1` on Windows). `--tools <names>`
(comma-separated) installs those tools outright, skipping the prompt;
`--no-tools` skips the step entirely; without a terminal and neither
flag, a hint naming both flags is printed instead.

Materializes the embedded core plugin, registers it in Claude Code's
`settings.json`, refreshes the plugin cache, applies model/effort
assignments, ensures the Engram "nerv" knowledge base, and installs
skills. nerv installs no other plugin: tasks and releases come from the
tools above.

**Extension plugins left behind.** In the same backed-up `settings.json`
write, it disables the plugins of the nerv marketplace that nerv no
longer ships, found there (enabled or not) or in Claude Code's plugin
cache: `task@nerv` and `task-teamwork@nerv` (replaced by nerv-task);
`release@nerv`, its six providers and the 5.2.0 `deploy@nerv` plugins
(replaced by nerv-release). Each is removed from `settings.json` and
uninstalled with `claude plugin uninstall` (a failure is only a warning).
When the replacing tool is installed, or was chosen in this run and
installed, each gets a note:

```
Removed task@nerv from settings.json (replaced by nerv-task)
```

Otherwise (declined, failed, or not asked without a terminal or with
`--no-tools`) one warning per tool says what is lost until it is
installed:

```
Warning: disabled task@nerv, task-teamwork@nerv: nerv no longer ships them, so /task:* is unavailable until you install nerv-task: nerv install --tools nerv-task
```

Then, when a tool is installed — before or in this run — and its
settings are still in your user `config.yaml`, it offers
`nerv migrate --user`: on a terminal it shows the steps its dry run
lists, under `=== Move settings to the NERV-ecosystem tools ===`, and asks
`Run nerv migrate --user now? [y/N]`; only a `y` writes anything (with
the usual backups). Without a terminal it prints a hint naming
`nerv migrate --user --dry-run` (to see the steps), then
`nerv migrate --user`. It covers the user configuration only: a
repository's own move is `nerv migrate`'s, which `nerv doctor` reports.

Copies of the `/task:*` commands that an older release put in
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
  --tools <names>       Comma-separated NERV-ecosystem tools to install (such as
                        nerv-task,nerv-release), skipping the confirm prompt
  --no-tools            Skip the NERV-ecosystem tools step entirely
```

```
nerv install
```

```
nerv install --tools nerv-task,nerv-release
```

```
nerv install --no-configure --no-skills
```

### nerv uninstall

Removes NERV's `settings.json` registration, uninstalls the cached plugin
and every other enabled plugin of the nerv marketplace, and removes the
materialized marketplace directory. The binary, the user config and each
repository's `.ai-workflow/` stay.

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
Hyuga, Kaji, and so on). nerv-task's `task` skill and its
`task:dispatcher` agent work the same way: the orchestrator launches the
dispatcher only when the nerv-task tool is installed (the agent is
available). nerv-release's skills and agents are loaded and launched by
its own `/release:*` commands. You never invoke them by name —
see [integration.md#roles](integration.md#roles) for what each agent does
and when it runs.
