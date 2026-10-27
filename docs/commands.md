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

### Deploy extension commands (`/release:deploy*`)

The optional deploy extension (`deploy`, which requires `release`) releases
an artifact; its agent is `release:deployer`, and its git hosts and targets
are providers (`release-github`, `release-ssh-compose`, `release-script`). Reference:
[deploy.md](deploy.md).

| Command | What it does |
|---|---|
| `/release:deploy` | Release and deploy the latest local artifact, or the one you name: dry run first, then the release commit, tag, push, image publish, git host release and targets only after your explicit yes |
| `/release:ship` | `/release:build`, then `/release:deploy` with the artifact it just packaged: every dry run first, and each real run only after its own explicit yes |
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
| `nerv version next` | Compute the next release version with the native engine (release extension) |
| `nerv release` | Build, test and package origin's code into a local artifact; no git writes (release extension) |
| `nerv release setup --import` | Import an existing semantic-release config into the `release:` section |
| `nerv deploy` | Release and deploy a release artifact: release commit, tag, push, targets, git host release (deploy extension) |
| `nerv migrate` | Move a repository (and, with `--user`, the user configuration) from nerv 4.2.0 or 5.0.0-alpha.1 |
| `nerv doctor` | Check that NERV is in order on this machine and in the repository, and list the steps to take |
| `nerv upgrade` | Replace the nerv binary with the newest release (`--alpha`, `--rc`), upgrade gentle-ai within its 4.x major version, then run nerv's install |
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
([deploy.md](deploy.md#migrating-from-nerv-pipeline)).

### nerv migrate

Moves a repository from nerv 4.2.0 or 5.0.0-alpha.1 to this version, once:

- `.nerv/nerv.yaml` (4.2.0) or `.ai-workflow.yml` (5.0.0-alpha.1) becomes `.ai-workflow/config.yaml`;
- the 4.2.0 `release:` section becomes today's `release:` section, with the semantic-release config imported;
- `pipeline:` becomes `deploy:`;
- the in-flight `openspec/changes` become ODD feature documents;
- a project file without `tasks.provider` gets the user's
  (`defaults.tasks.provider`, or the older user `tasks.provider`), since
  `tasks:` is read from the project only;
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
  step);
- in the repository: `.ai-workflow/config.yaml` with `enabled: true`, a
  pending `nerv migrate`, the project extensions and providers, the
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
downloads `nerv_<version>_<os>_<arch>.tar.gz` and `checksums.txt` from the
GitHub release, verifies the archive's SHA-256, and renames the new binary
over the old one. A prerelease channel also considers stable releases, so
it never stays on an older prerelease when a newer stable one exists. Then
it runs the new binary's `nerv install` (`nerv install --no-configure`
without an interactive terminal) and exits with its exit code.

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

Already on the newest release, or on a newer one (a `dev` build counts as
older than any release), it says so and changes nothing. A failed
download or checksum leaves the current binary untouched and exits 1;
only Linux and macOS on amd64 or arm64 have prebuilt binaries (anywhere
else, use `go install ...@latest`). `--alpha` with `--rc` is a usage
error (exit 2).

### nerv install

Materializes the embedded core plugin, registers it in Claude Code's
`settings.json`, refreshes the plugin cache, applies model/effort
assignments, ensures the Engram "nerv" knowledge base, and installs
skills. It also installs the optional extensions and providers named by `--with`,
keeps the ones already enabled up to date, and, when stdin is a terminal,
offers the others, in this order:

1. the task extension (`task`, the `/task:*` commands);
2. its providers (`task-<name>`, such as `task-teamwork`), only
   once task is installed;
3. the release extension (`release`: `nerv release`), which brings its
   providers with it, unasked (`release-conventional-commits`, the
   version engine, `release-docker-compose` and `release-bash`);
4. the deploy extension (`deploy`: `nerv deploy`), only once release is
   installed, which brings its providers with it, unasked
   (`release-script`, `release-github`, `release-ssh-compose`).

An extension whose `extension.yaml` says `providers: bundled` (release,
deploy) installs every provider it ships with it, each listed as `Adding
<provider> (bundled with <extension>)`; one that says `providers:
suggested` (task) has each provider offered after it.

Requirements: `deploy` requires `release`, a task provider requires `task`,
and a deploy provider requires `deploy`; each is offered only once what it
requires is installed or chosen. An extension or provider named in `--with` brings what it
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
  --with <names>        Comma-separated extensions and providers to install, such as
                        task,task-<provider>,release,deploy,release-<provider>
                        (each brings what it requires)
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
