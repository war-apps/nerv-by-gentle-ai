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
base branch), writes `.nerv/nerv.yaml`, creates `openspec/config.yaml`
(`strict_tdd` and the test runner) and the `openspec/specs/` and
`openspec/changes/archive/` folders when they are missing, and refreshes
the skill registry with `gentle-ai skill-registry refresh`. An existing
`openspec/config.yaml` is never overwritten. Use it once per repository.

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
  --repo-provider <name>    Tasks provider override (teamwork | none)
  --repo-project-id <id>    Teamwork project id override
  --repo-tasklist-id <id>   Teamwork tasklist id override

If `.nerv/nerv.yaml` already exists it is not recreated; `--init-repo` only
removes the task providers that no longer exist (`github-projects`, `jira`)
from it, after a `.bak-configure-<yyyyMMdd-HHmmss>` backup, and says so. A file
with nothing to remove is left untouched.

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
  --require-gentle-ai   Fail (exit 1) when gentle-ai is missing or not 4.x
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

`nerv-orchestrator` and `nerv-tasks` are Claude Code **skills**, not
commands you type. The SessionStart activation header has the session
invoke the orchestrator skill with the Skill tool in repos where NERV is
enabled, and it governs how the session
routes work to the named agents (Ikari, Misato, the MAGI, the pilots,
Hyuga, Kaji, and so on); `nerv-tasks` is the provider-agnostic task-tracking
port that only the Hyuga agent calls. You never invoke either by name —
see [integration.md#roles](integration.md#roles) for what each agent does
and when it runs.
