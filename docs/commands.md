# Commands

Nerv by Gentle-AI has two command surfaces: Claude Code slash commands
(`/nerv:*`, run from inside a Claude Code session) and the `nerv` CLI binary
(run from a shell, including from inside the slash commands themselves).

## Claude Code slash commands

| Command | What it does |
|---|---|
| `/nerv:configure` | Configure NERV through guided questions (git, tasks, skills, models, repo, commands) |
| `/nerv:init` | Bootstrap NERV for the current repository (writes `.nerv/nerv.yaml`) |
| `/nerv:status` | Show NERV activation, config resolution, and change status (read-only) |

### /nerv:configure

Walks through the sections you name — `git`, `tasks`, `skills`, `models`,
`repo`, `commands`, or `all` — reading the current state first and writing
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
keys that aren't already resolved (task provider, Teamwork project/tasklist,
base branch), writes `.nerv/nerv.yaml`, and delegates gentle-ai's own
bootstrap (`sdd-init`) if it hasn't run yet. Use it once per repository.

```
/nerv:init
```

### /nerv:status

Read-only report of NERV's state for the current repo: activation,
resolved config (project vs. user vs. default), the installed `gentle-ai`
version, active SDD changes and orchestrator lock state, and a per-role
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
  --skip-commands           Skip the Teamwork /task:* commands install
  --no-refresh              Skip the closing apply-models/cache-refresh step

Otherwise, exactly one mode flag is required:
  --print                   Print the current configuration as JSON
  --set key=value           Set a managed config key (repeatable)
  --set-model role=spec     Set a role's model/effort override (repeatable);
                             spec is model[/effort], from:<phase>, or default
  --init-repo <path>        Initialize a repository for NERV
  --install-commands        Install the /task:* Teamwork slash commands

With --init-repo:
  --repo-base <branch>      Base branch override
  --repo-provider <name>    Tasks provider override (teamwork | github-projects | jira | none)
  --repo-project-id <id>    Teamwork project id override
  --repo-tasklist-id <id>   Teamwork tasklist id override

Other flags:
  --json                    Print the result as JSON (ignored by --print, which always does)
  --config <path>           Override the user-scope nerv.yaml path
```

```
nerv configure --print --json
```

```
nerv configure --set git.base_branch=develop --set tasks.provider=teamwork
```

```
nerv configure --set-model aoba=opus/high
```

### nerv install

Materializes the embedded plugin, registers it in Claude Code's
`settings.json`, refreshes the plugin cache, applies model/effort
assignments, ensures the Engram "nerv" knowledge base, and installs
skills. See the README's "Install" section for the shell one-liners and
environment variables (`NERV_CHANNEL`, `NERV_INSTALL_DIR`,
`NERV_NO_INSTALL`, `NERV_VERSION`); the install script itself is
[`../scripts/install.sh`](../scripts/install.sh).

```
Usage: nerv install [flags]

Flags:
  --require-gentle-ai   Fail (exit 1) when gentle-ai is missing or not 3.x
  --no-skills           Skip installing skills
  --no-configure        Skip the closing wizard/hint entirely
```

```
nerv install
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
ones verified only.

```
Usage: nerv skills [flags]

Flags:
  --dry-run          Print the exact npx command for each missing skill
                      instead of running it
  --json             Print the computed status as a JSON array
  --only name,...    Restrict processing to these skill names
```

```
nerv skills --dry-run --json
```

```
nerv skills --only tdd,solid-principles
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

`nerv-orchestrator` and `nerv-tasks` are Claude Code **skills**, not
commands you type. The SessionStart activation header has the session
invoke the orchestrator skill with the Skill tool in repos where NERV is
enabled, and it governs how the session
routes work to the named agents (Ikari, Misato, the MAGI, the pilots,
Hyuga, Kaji, and so on); `nerv-tasks` is the provider-agnostic task-tracking
port that only the Hyuga agent calls. You never invoke either by name —
see [integration.md#roles](integration.md#roles) for what each agent does
and when it runs.
