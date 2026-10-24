# Commands

Nerv by Gentle-AI has two command surfaces: Claude Code slash commands
(`/nerv:*`, run from inside a Claude Code session) and the `nerv` CLI binary
(run from a shell, including from inside the slash commands themselves).

## Claude Code slash commands

| Command | What it does |
|---|---|
| `/nerv:configure` | Configure NERV through guided questions (git, skills, models, repo) |
| `/nerv:init` | Bootstrap NERV for the current repository (writes `.nerv/nerv.yaml`) |
| `/nerv:status` | Show NERV activation, config resolution, and change status (read-only) |
| `/nerv:version` | Show the next release version of the repository (or of each project) |
| `/nerv:ship` | Release the repository from origin: dry run first, the real release only after your explicit yes |
| `/nerv:release-setup` | Write the `release:` section of `.nerv/nerv.yaml` from what the repository has, asking only what it cannot detect |
| `/nerv:pipeline` | Run the Docker pipeline (validate, test, release, docker, sync, deploy): dry run first, the real run only after your explicit yes |

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

### /nerv:configure

Walks through the sections you name — `git`, `skills`, `models`, `repo`,
or `all` — reading the current state first and writing
only the changes you confirm. Use it any time you want to change
`nerv.yaml` without hand-editing YAML.

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
keys that aren't already resolved (the base branch), writes `.nerv/nerv.yaml`, creates `openspec/config.yaml`
(`strict_tdd` and the test runner) and the `openspec/specs/` and
`openspec/changes/archive/` folders when they are missing, and refreshes
the skill registry with `gentle-ai skill-registry refresh`. An existing
`openspec/config.yaml` is never overwritten. Use it once per repository.

```
/nerv:init
```

### /nerv:version

Runs `nerv version next --json` with the arguments you give (`--branch`,
`--project`, `--offline`, ...) and shows, per project, whether there is a
new release, the next version, its channel and tag, and the last release.
By default it reads origin's release tags and notes with your git
credentials; add `--offline` to use the local ones only.

```
/nerv:version --branch develop
```

### /nerv:ship

Always rehearses first with `nerv ship --dry-run --json` (the pipeline runs
in a throwaway clone; nothing is tagged or published) and shows, per
project, the version, channel, tag and last version, and the plan's
stages. Then it asks you; only on your explicit yes does it run `nerv ship
--yes --json` and report the result, with the recovery advice of a failed
project. `--branch` and `--project` pass through. It releases what is on
origin: push or pull first when your branch differs.

```
/nerv:ship --branch develop --project api
```

### /nerv:release-setup

Sets up the `release:` section used by `/nerv:version` and `/nerv:ship`.
It detects the repository's semantic-release configuration, .NET projects
and solutions, Dockerfiles and CI docker steps, `package.json` scripts,
release scripts and monorepo layout; proposes a complete section; asks
only what it cannot detect, one question at a time; writes the section
into `.nerv/nerv.yaml` without touching the rest of the file; and
validates it with `nerv version next --offline`. An existing section is
shown first and never overwritten without asking. `--project <name>` adds
or edits one project. It never commits, pushes or releases. See
[release-commands.md](release-commands.md#interactive-setup).

```
/nerv:release-setup
```

### /nerv:pipeline

Runs the Docker pipeline of the `pipeline:` section with `nerv pipeline
run`: a dry run first (validate and test in the working tree, the release
computed on origin, the images built, the merge-backs previewed, the
deploys listed without connecting), then a summary of the version, images,
merge-backs and environments, and the real run only after your explicit
yes, bound to the approved version (`--expect`). It never releases,
pushes or deploys without that yes; approving all but the deploy runs the
steps up to sync one at a time. `--from <step>` or `--only <step>` narrows
the run. See [pipeline.md](pipeline.md#getting-started).

```
/nerv:pipeline
/nerv:pipeline --from docker
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
| `nerv version next` | Compute the next release version with semantic-release |
| `nerv ship` | Release the repository from origin: pipeline, semantic-release, publish |
| `nerv pipeline <step>` | Run a step of the Docker pipeline (`validate`, `test`, `release`, `docker`) |
| `nerv configure` | Configure NERV non-interactively (or run the wizard) |
| `nerv install` | Install NERV (plugin, cache, skills, Engram KB) |
| `nerv uninstall` | Uninstall NERV |
| `nerv apply-models` | Apply `models:` overrides to the cached agents |
| `nerv skills` | Install or verify the skills NERV's defaults reference |
| `nerv spec-compose` | Merge a change's delta spec into its canonical spec (used by Aoba's archive step) |

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
                            repository's .nerv/nerv.yaml (repeatable; repo-scope
                            keys of the provider's provider.yaml only)

If `.nerv/nerv.yaml` already exists it is not recreated, and an existing
`tasks:` block in it is left untouched.

Other flags:
  --json                    Print the result as JSON (ignored by --print, which always does)
  --config <path>           Override the user-scope nerv.yaml path
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
its `release:` section, with the repository's own semantic-release
configuration in dry-run mode, and prints the 12 outputs of
`cycjimmy/semantic-release-action` as text, JSON or GitHub Actions output
lines. Flags, formats, monorepo key naming and exit codes:
[release-commands.md](release-commands.md#nerv-version-next).

```
nerv version next --branch develop --json
```

```
nerv version next --offline --format env >> "$GITHUB_OUTPUT"
```

### nerv pipeline

Runs one step of the Docker pipeline configured in the `pipeline:` section
(a standalone copy of dim.comprobantes.api's CI/CD): `validate` (`dotnet
restore`, `dotnet format --verify-no-changes`) and `test` (restore, build
with `-warnaserror`, `dotnet test` with coverage and a trx logger, results
in `.nerv/artifacts/pipeline/test`, and the runsettings coverage thresholds
enforced by nerv, since coverlet's collector ignores them; a passing run
is recorded in `.nerv/artifacts/pipeline/test.json`); both run in your
working tree. `release` releases what is on origin as `nerv ship` does,
without building or publishing (version files, release commit, tag;
outputs in `.nerv/artifacts/pipeline/release.json`); `docker` builds, tags
and pushes every image from the released commit (results in
`.nerv/artifacts/pipeline/docker.json`); `sync` merges the released branch
back as dim's sync jobs (main or `release/*` into develop, main into every
`release/*`; conflicts aborted and reported, committed as you; only after
passing test and docker records of the release; results in
`.nerv/artifacts/pipeline/sync.json`); `deploy` updates the release's
environments over SSH as dim's `/update-deploy` (selected by their
`DOCKER_IMAGE_VERSION` or by branch, data from environment variables or
Infisical; health and version verified; results in
`.nerv/artifacts/pipeline/deploy.json`); `run` runs every step in that
order, stopping at the first that does not pass (summary in
`.nerv/artifacts/pipeline/run.json`). Details:
[pipeline.md](pipeline.md#running-the-steps).

```
nerv pipeline validate
nerv pipeline test --json
nerv pipeline release --dry-run
nerv pipeline docker --dry-run
nerv pipeline sync --dry-run
nerv pipeline deploy --dry-run
nerv pipeline run --dry-run
```

### nerv ship

A full local release of what is on origin, in a throwaway clone: dotnet
project file, pipeline (restore to package), semantic-release for real
(release commit, CHANGELOG, tag, GitHub release), then publish (docker,
NuGet, npm) and the lifecycle hooks, per project in dependency order. It
asks for confirmation (`--yes` to skip; required without a terminal);
`--dry-run` builds without releasing or publishing. Flow, credentials,
failure handling and exit codes:
[release-commands.md](release-commands.md#nerv-ship).

```
nerv ship --dry-run
nerv ship --branch develop --project api
```

### nerv install

Materializes the embedded plugin, registers it in Claude Code's
`settings.json`, refreshes the plugin cache, applies model/effort
assignments, ensures the Engram "nerv" knowledge base, and installs
skills. It also installs the optional extension plugins named by `--with`,
keeps the ones already enabled up to date, and, when stdin is a terminal,
offers the others: first the task plugin (`task@nerv`, the `/task:*`
commands), then, only if it is installed, its providers
(`task-teamwork@nerv`). A provider named in `--with` brings the task plugin
with it. Copies of the `/task:*` commands that an older release put in
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
                        task,task-teamwork (a provider brings the task plugin)
```

```
nerv install
```

```
nerv install --with task-teamwork
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

Applies the `models:` overrides from the user-scope `nerv.yaml` (merged
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

### nerv spec-compose

Merges a change's delta spec into the canonical spec it amends. It applies
`RENAMED`, `MODIFIED`, `REMOVED`, then `ADDED` requirements in that order
and leaves every unrelated byte of the canonical spec untouched. Names must
match exactly; a `RENAMED` or `REMOVED` entry needs a `(Reason: ...)` line.
An unmatched name, a name repeated within one delta section, an `ADDED`
name that already exists, an empty delta, or a canonical spec with no
requirements is an error: nothing is written, stderr names the section and
requirement, and the exit code is 1 (2 for a usage or I/O error).

`--output` defaults to stdout. A file is replaced atomically (a temp file in
the same directory, then a rename) and keeps its existing mode (a symlink is followed: its target is updated
and the link stays), so
`--output` may name the canonical spec itself. It is a port of the merge gentle-ai
3.x shipped, which gentle-ai 4 removed.

```
Usage: nerv spec-compose --canonical <path> --delta <path> [--output <path|->]

Flags:
  --canonical <path>   The canonical openspec/specs/<domain>/spec.md
  --delta <path>       The change's openspec/changes/<change>/specs/<domain>/spec.md
  --output <path|->    Where to write the composed spec (default "-", stdout)
```

```
nerv spec-compose --canonical openspec/specs/widgets/spec.md \
  --delta openspec/changes/add-tags/specs/widgets/spec.md \
  --output openspec/specs/widgets/spec.md
```

## Maintainer: nerv release

`release` is maintainer-only tooling, listed under "Maintainer commands"
in `nerv --help`. It computes the next semantic version from
Conventional Commits since the last release tag (`preview`/`apply`), or
checks release readiness before tagging (`guard`). In normal use this is
driven by CI, not run by hand — see [docs/releases.md](releases.md) for
the full Gitflow release procedure and the automatic pre-release channels.

```
Usage: nerv release <preview|apply|guard> [flags]

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
go run ./cmd/nerv release preview
```

```
go run ./cmd/nerv release guard --notes CHANGELOG_SECTION.md
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
dispatcher only when the task plugin is installed. You never invoke them by name —
see [integration.md#roles](integration.md#roles) for what each agent does
and when it runs.
