# Commands

Nerv by Gentle-AI has two command surfaces: Claude Code slash commands
(`/nerv:*`, run from inside a Claude Code session) and the `nerv` CLI binary
(run from a shell, including from inside the slash commands themselves).

## Claude Code slash commands

| Command | What it does |
|---|---|
| `/nerv:configure` | Configure NERV through guided questions (git, skills, models, repo) |
| `/nerv:init` | Bootstrap NERV for the current repository (writes `.ai-workflow/config.yaml`) |
| `/nerv:status` | Show NERV activation, config resolution, and change status (read-only) |

### Task plugin commands (`/task:*`)

The `/task:*` commands belong to the optional task plugin (`task@nerv`),
not to core. Claude Code serves them from the installed plugin, so nothing
is copied to `~/.claude/commands/task/`. Each one runs the task plugin's
contract against the configured provider plugins.

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
| `/task:start` | Start working on a task - local timer, assigns it to me, moves it to the inDev stage, offers a worktree (creates the task if needed) |
| `/task:stop` | Stop working on a task and log its time (coupled blocks merge into one timelog; a gap starts a fresh timelog dated at the new block) |
| `/task:stopAll` | Stop tracking on ALL started tasks - stops every timer and logs time as /task:stop would for each |
| `/task:take` | Take a task - assign it to me (creating it if needed) WITHOUT starting timers or moving stages |
| `/task:testing` | Move a task to the testing stage |
| `/task:timers` | Show the local timers of a task (or all running timers), with their session |

### Release plugin commands (`/release:*`)

The optional release plugin (`release@nerv`) versions, builds and packages; its
agents are `release:versioner` and `release:builder`. Reference:
[release-commands.md](release-commands.md).

| Command | What it does |
|---|---|
| `/release:version` | Show the next release version of the repository (or of each project), computed by the native engine |
| `/release:setup` | Write the `release:` section of `.ai-workflow/config.yaml`: import an existing semantic-release config (`nerv release setup --import`), ask each project's build provider (`docker-compose` or `bash`) and its stage commands or compose file |
| `/release:build` | Build, test and package origin's code into a local artifact: dry run first, the real run only after your explicit yes; never writes to git |

### Deploy plugin commands (`/deploy:*`)

The optional deploy plugin (`deploy@nerv`, which requires `release`) releases
an artifact; its agent is `deploy:deployer`, and its git hosts and targets
are provider plugins (`deploy-github`, `deploy-ssh-compose`). Reference:
[deploy.md](deploy.md).

| Command | What it does |
|---|---|
| `/deploy:run` | Release and deploy the latest local artifact, or the one you name: dry run first, then the release commit, tag, push, targets and git host release only after your explicit yes |
| `/deploy:setup` | Write the `deploy:` section (host, targets, git) and suggest the provider plugins to install |

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
resolves the TDD mode and test runner per feature. Use it once per
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
| `nerv version next` | Compute the next release version with the native engine (release plugin) |
| `nerv release` | Build, test and package origin's code into a local artifact; no git writes (release plugin) |
| `nerv release setup --import` | Import an existing semantic-release config into the `release:` section |
| `nerv deploy` | Release and deploy a release artifact: release commit, tag, push, targets, git host release (deploy plugin) |
| `nerv migrate` | Move a repository (and, with `--user`, the user configuration) from nerv 4.2.0 to this version |
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
                            keys of the provider's provider.yaml only)

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

When `task@nerv` is enabled, the wizard's "Task providers" section prompts
each enabled provider's scalar config keys, read from its `provider.yaml`,
plus `tasks.provider` when more than one provider is enabled. It replaces
the former tasks, Teamwork and commands sections, and the flags
`--repo-provider`, `--repo-project-id`, `--repo-tasklist-id`,
`--install-commands` and `--skip-commands` are gone.

### nerv version next

Computes the next semantic version of the repository, or of each project of
its `release:` section, with nerv's native engine (no semantic-release, node or
npm), and prints the 12 outputs of `cycjimmy/semantic-release-action` as
text, JSON or GitHub Actions output lines. Its flags and outputs are the
ones it had with semantic-release. Flags, formats, monorepo key naming and
exit codes: [release-commands.md](release-commands.md#nerv-version-next).

```
nerv version next --branch develop --json
```

```
nerv version next --offline --format env >> "$GITHUB_OUTPUT"
```

### nerv release

Builds, tests and packages what is on origin, in a throwaway clone, in each
project's build provider (`docker-compose` or `bash`), per project in dependency
order, and leaves the artifact `.nerv/artifacts/<project>/<version>/`. It
never writes to git. It asks for confirmation (`--yes` to skip; required
without a terminal); `--dry-run` runs every stage and writes no artifact;
`--expect <project>=<version>` packages only the approved versions;
`--version <semver>` skips the version computation (it must fit the branch
and be newer than the branch's last release; the notes and CHANGELOG
section are still written from the commits since then) and takes the
channel from the branch's `branches` entry. Flow, failure handling and exit codes:
[release-commands.md](release-commands.md#nerv-release).

```
nerv release --dry-run
nerv release --branch develop --project api
```

### nerv release setup --import

Translates an existing JSON or YAML semantic-release config (`package.json`
`release`, `.releaserc.json`, `.releaserc.yaml`/`.yml`) into the `release:`
section, once, and lists every part it cannot translate (the `npm` and
`exec` plugins, `presetConfig.types`, `extends`, maintenance branches). A
`.js` config is reported, never imported. An engine key the `release:` section
already sets to another value is never replaced silently: the import
refuses (exit 1) and lists each key with its current and imported values;
`--force` replaces them and prints each replaced value. A 4.2.0 `release:`
section (one with `semantic_release:` or the presets) is refused: `nerv
migrate` converts it. `--dry-run` prints the result and writes nothing. See
[release-commands.md](release-commands.md#importing-a-semantic-release-config).

```
nerv release setup --import --dry-run
nerv release setup --import
nerv release setup --import --force
```

### nerv deploy

Releases and deploys an artifact `nerv release` left: it preflights every
target's tools and data before any push, then makes the release commit, the
lightweight tag and the channel note, pushes them atomically, runs each
target's publish, deploy and verify, and creates the git host release.
`--dry-run` prepares the release in a clone and shows the steps; `--target`
selects targets; `--rebase` re-applies the release on origin's moved branch.
See [deploy.md](deploy.md).

```
nerv deploy --artifact .nerv/artifacts/api/1.4.0 --dry-run
nerv deploy --artifact .nerv/artifacts/api/1.4.0 --yes
```

`nerv pipeline` was removed: its steps moved to release and deploy
([pipeline.md](pipeline.md)).

### nerv migrate

Moves a repository from nerv 4.2.0 or 5.0.0-alpha.1 to this version, once:

- `.nerv/nerv.yaml` (4.2.0) or `.ai-workflow.yml` (5.0.0-alpha.1) becomes `.ai-workflow/config.yaml`;
- the 4.2.0 `release:` section becomes today's `release:` section, with the semantic-release config imported;
- `pipeline:` becomes `deploy:`;
- the in-flight `openspec/changes` become ODD feature documents;
- `.nerv/.gitignore` is created.

`--user` also moves `~/.claude/nerv/nerv.yaml`, or the user `config.yml`,
to the user `config.yaml`,
with the personal keys of the project file. `--dry-run` prints every change
as a unified diff and writes nothing. `--install` installs the plugins the
repository used. Every changed or removed file is backed up first, and a
second run changes nothing. See [migration.md](migration.md).

```
nerv migrate --dry-run
nerv migrate --user --install
```

### nerv install

Materializes the embedded plugin, registers it in Claude Code's
`settings.json`, refreshes the plugin cache, applies model/effort
assignments, ensures the Engram "nerv" knowledge base, and installs
skills. It also installs the optional extension plugins named by `--with`,
keeps the ones already enabled up to date, and, when stdin is a terminal,
offers the others, in this order:

1. the task plugin (`task@nerv`, the `/task:*` commands);
2. its providers (`task-<name>@nerv`, such as `task-teamwork@nerv`), only
   once task is installed;
3. the release plugin (`release@nerv`: `nerv release`), which brings its
   providers with it, unasked (`release-conventional-commits@nerv`, the
   version engine, `release-docker-compose@nerv` and `release-bash@nerv`);
4. the deploy plugin (`deploy@nerv`: `nerv deploy`), only once release is
   installed, which brings its providers with it, unasked
   (`deploy-github@nerv`, `deploy-ssh-compose@nerv`).

An extension whose `extension.yaml` says `providers: bundled` (release,
deploy) installs every provider it ships with it, each listed as `Adding
<provider> (bundled with <extension>)`; one that says `providers:
suggested` (task) has each provider offered after it.

Requirements: `deploy` requires `release`, a task provider requires `task`,
and a deploy provider requires `deploy`; each is offered only once what it
requires is installed or chosen. A plugin named in `--with` brings what it
requires with it, and a requirement this nerv does not ship is refused. Copies of the `/task:*` commands that an older release put in
`~/.claude/commands/task/` are reported, and removed only after you
confirm. See the README's "Install" section for the shell one-liners and
environment variables (`NERV_CHANNEL`, `NERV_INSTALL_DIR`,
`NERV_NO_INSTALL`, `NERV_VERSION`); the install script itself is
[`../scripts/install.sh`](../scripts/install.sh).

```
Usage: nerv install [flags]

Flags:
  --require-gentle-ai   Fail (exit 1) when gentle-ai is missing or not 4.x
  --no-skills           Skip installing skills
  --no-configure        Skip the closing wizard/hint entirely
  --with <names>        Comma-separated extension plugins to install, such as
                        task,task-<provider>,release,deploy,deploy-<provider>
                        (each brings what it requires)
```

```
nerv install
```

```
nerv install --with task-teamwork
```

```
nerv install --with release,deploy,deploy-github
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
Hyuga, Kaji, and so on). The task plugin's `task` skill and its
`task:dispatcher` agent work the same way: the orchestrator launches the
dispatcher only when the task plugin is installed. The release plugin's `release` skill
with its `release:versioner` and `release:builder` agents, and the deploy
plugin's `deploy` skill with its `deploy:deployer` agent, are loaded and
launched by their own `/release:*` and `/deploy:*` commands. You never invoke them by name —
see [integration.md#roles](integration.md#roles) for what each agent does
and when it runs.
