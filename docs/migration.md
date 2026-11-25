# Migrating from nerv 4.2.0, 5.0.0-alpha.1 or 5.2.0

`nerv migrate` moves a repository, and your user configuration, from nerv
4.2.0, 5.0.0-alpha.1 or 5.2.0 (the separate deploy extension) to this
version. You run it once per repository, plus once for the
user configuration. It is written in Go and works the same way on Linux,
macOS and Windows.

```sh
nerv migrate --dry-run          # list the steps; nothing is written
nerv migrate                    # migrate this repository
nerv migrate --user             # also migrate your user configuration
nerv migrate --install          # then install the tools the repository uses
nerv migrate --force            # also move a change whose lock is still held
```

Until it runs, nerv reads neither old file: every command refuses while a
5.0.0-alpha.1 `.ai-workflow.yml` (or user `config.yml`) is still there,
with `run "nerv migrate" to move it` (`nerv migrate --user` for the user
file), and the SessionStart hook does not activate NERV. A 4.2.0 file is
reported the same way, and the repository is treated as unconfigured.

### Older release and deploy sections

The release and deploy engines left nerv for
[nerv-release](https://github.com/war-apps/nerv-release-by-gentle-ai), and so
did the conversion of an older nerv's release and deploy configuration:

- a 4.2.0 `pipeline:` or `release:` section in `.nerv/nerv.yaml`, and a
  4.2.0 `release:` already moved into `.ai-workflow.yml` or
  `.ai-workflow/config.yaml` (told apart by its `semantic_release:` key or
  its projects' presets);
- a 5.0.0-alpha.1 `ship:` section;
- a 5.x deploy target with `provider: bash` (rename it `provider: script`);
- a 5.x project deploy provider, `.ai-workflow/providers/deploy-<name>/` with
  `extension: deploy` (move it to `release-<name>/` with `extension: release`).

**With nerv-release installed**, `nerv migrate` hands them to
`nerv-release migrate`, in one run:

1. it moves each section, as it is, to nerv-release's own file: the
   repository's to `.ai-workflow/release.yaml` (`pipeline:` and `ship:`
   keep their names), the user config's `deploy:` and `defaults.deploy:`
   (with `--user`) to the global `release.yaml`, with the same guarantees
   as the rest of the migration (a key already set there is kept and
   reported, backups, a second run moves nothing). A personal ssh-compose
   key in the project file still stays there and is reported. A
   `deploy-<name>` project provider stays where it is. A note says which
   sections were left for `nerv-release migrate`;
2. once its steps are written, it runs `nerv-release migrate --cwd
   <repository>` (and `nerv-release migrate --cwd <repository> --user` for
   the user config's), printing its output. nerv-release converts the
   sections inside `release.yaml` and renames the provider to
   `release-<name>`.

`--dry-run` does not run `nerv-release migrate`; it says it would. When
`nerv-release migrate` refuses (exit 1), `nerv migrate` exits 1 too: the
sections are already in `release.yaml`, as they were; fix what it reports
and run `nerv-release migrate` again. Any other failure of it is exit 2;
both messages name the command to run again. A later `nerv migrate` also
looks in the repository's and (with `--user`) the global `release.yaml`:
while they still hold an older section, it says so and runs
`nerv-release migrate` again, even with nothing of its own to write, and
`nerv doctor` reports it as a pending step.

**Without nerv-release**, `nerv migrate` refuses (exit 1, nothing written),
naming the file, the section and the command that installs nerv-release:
`nerv install --tools nerv-release`, then `nerv migrate` again. You can
also rewrite the section by hand as nerv-release's `release:` and `deploy:`
sections.

## What it changes

| 4.2.0 | Now |
|---|---|
| `<repo>/.nerv/nerv.yaml` | `<repo>/.ai-workflow/config.yaml`. The old file is kept as `.nerv/nerv.yaml.migrated`. |
| `<repo>/.ai-workflow.yml` (5.0.0-alpha.1) | `<repo>/.ai-workflow/config.yaml`, with the same content (its header comment names the new file). The old file is removed after its backup under `.nerv/backups/`. |
| `~/.claude/nerv/nerv.yaml` (with `--user`) | the user `config.yaml`: `~/.config/ai-workflow/config.yaml` on Linux, `%AppData%\ai-workflow\config.yaml` on Windows. The old file is kept as `nerv.yaml.migrated`. |
| the user `config.yml` (5.0.0-alpha.1, with `--user`) | the user `config.yaml` in the same folder. The old file is kept as `config.yml.migrated`. |
| `models:` and the provider keys of user scope in the project file, such as `tasks.providers.teamwork.assignee_id` | Moved to the user config when `--user` runs in the same repository. Otherwise they stay where they are and are reported. |
| `tasks.rounding_minutes` | `tasks.providers.teamwork.rounding_minutes` |
| `tasks:`, `release:` and `deploy:` keys in the user `config.yaml` that are not personal (with `--user`) | Moved under `defaults.<section>`: they only seed new projects now, and are never read at run time. Personal keys stay: provider keys of user scope, `tasks.timer_store`, `tasks.stale_after_hours`. |
| a project file without `tasks.provider`, when the user file has one | The project gets `tasks.provider` from `defaults.tasks.provider`, or else from the old user `tasks.provider`. |
| `tasks.provider: jira` or `github-projects`, `tasks.providers.jira` / `.github-projects` | Dropped and reported: those providers were removed. |
| `tasks.providers.teamwork.task_ref_prefix` | Dropped: the provider's manifest fixes the prefix. |
| `ship:` (5.0.0-alpha.1), `release:` (4.2.0), `pipeline:`, `provider: bash`, `deploy-<name>` providers | With nerv-release installed, moved as they are to `release.yaml` and converted there by `nerv-release migrate`, which `nerv migrate` runs; refused without it (see [Older release and deploy sections](#older-release-and-deploy-sections)). |
| `openspec/changes/<change>/`, in flight, FULL | `odd/tasks/<change>.md`, with its run files in `odd/<change>/nerv/` and its lock in `.nerv/locks/<change>.lock` (see [ODD](#odd)) |
| `openspec/changes/<change>/`, in flight, DIRECT or LIGHT | `.nerv/runs/<change>/`, with no ODD document, and its lock in `.nerv/locks/<change>.lock` (see [ODD](#odd)) |
| `openspec/changes/archive/`, `openspec/specs/` | They stay where they are, read-only. |
| `openspec/config.yaml` `strict_tdd`, `testing.runner` | `odd.tdd: strict` or `standard`, and `odd.test_runner`, in `.ai-workflow/config.yaml`. |
| — | `.nerv/.gitignore` (`*`): `.nerv/` now holds runtime files only. |

Comments and key order are kept where possible. Untouched lines are copied
byte for byte.

### ODD

Only FULL runs get an ODD document. A change whose `state.yaml` has
`nerv.path: direct` or `light` moves whole (`state.yaml`, `proposal.md`,
`design.md`, `tasks.md`, `specs/` and every file of `nerv/`) to the runtime
folder `.nerv/runs/<change>/`, where the orchestrator keeps DIRECT and LIGHT
runs, and its lock to `.nerv/locks/<change>.lock`. `.nerv/` is
git-ignored, so those files leave version control. A change with no `path`
is treated as FULL.

Each in-flight FULL change becomes an ODD feature document
(`plugin/skills/nerv-orchestrator/references/odd-document.md`):

- **Header:** the branch, task and tier from `state.yaml`, and the TDD mode and runner from `openspec/config.yaml`, under a `<!-- nerv migrate: openspec/changes/<change> -->` marker.
- **`## Specs`:** the spec deltas and the proposal, verbatim, with their headings lowered two levels.
- **`## Tasks`:** the task table built from `tasks.md`'s `## T<n>` sections (ID, specs, route, pilot, depends on, status), then `tasks.md` itself.
- **`## Log`:** the design, and pointers to the exploration and the test plan.

`state.yaml`, the exploration files and every file of `nerv/` move to
`odd/<change>/nerv/`. The lock moves to `.nerv/locks/<change>.lock`.

A change whose lock is held on this machine is refused: its `host` is this
machine and its `heartbeat_at` is under 15 minutes old (the orchestrator's
stale threshold), or its `waiting_on` is `user` at any age. The refusal
names the session and the heartbeat. Let that run end, or pass `--force` to
move it anyway; that session then loses its lock. A lock of another machine
cannot be checked, so make sure no session there still runs the change.

A FULL change named `tasks` is left where it is, because it would collide
with `odd/tasks/`.

## Tools

The run ends with the exact install line for the NERV-ecosystem tools
that replace what the old configuration used: nerv-task when
`tasks.provider` was `teamwork` (also the 4.2.0 default when nothing set
it).

```
Next, install the tools it uses: nerv install --tools nerv-task
```

`--install` runs that line through `nerv install` (without the closing
wizard). The install step reports old `/task:*` copies in
`~/.claude/commands/task`, and removes them when you confirm on a terminal.
`nerv migrate` never removes them itself.

## To the nerv-task and nerv-release tools

When a tool of the NERV ecosystem is installed (on `PATH` or in its
default install directory), `nerv migrate` also moves its sections out of
nerv's own configuration into the tool's file:

| Now | With the tool installed |
|---|---|
| `tasks:` in `.ai-workflow/config.yaml` (nerv-task) | `.ai-workflow/task.yaml`, keys without the `tasks.` prefix. |
| the user config's personal `tasks:` keys and `defaults.tasks:` (nerv-task, `--user`) | the global `task.yaml`, keys without the `tasks.` prefix; `defaults.tasks` becomes `defaults:`. |
| `release:` and `deploy:` in `.ai-workflow/config.yaml` (nerv-release) | `.ai-workflow/release.yaml`, every key at the same path (`release.projects`, `deploy.host`, ...). |
| the user config's personal deploy keys, `deploy.providers.ssh-compose.{infisical_domain, ssh_auth, ssh_key_file, ssh_known_hosts, ssh_connect_timeout}`, and `defaults.release:` / `defaults.deploy:` (nerv-release, `--user`) | the global `release.yaml` (`~/.config/ai-workflow/release.yaml`, `%AppData%\ai-workflow\release.yaml` on Windows), every key at the same path. |

A key the tool's file already sets is never overwritten: it stays in
`config.yaml` and is reported. A personal ssh-compose key in a project
file never moves into any `release.yaml`: it stays and is reported, so
you set it in the global `release.yaml` yourself. The tool's file is
written before `config.yaml` loses the moved keys, both are backed up,
`--dry-run` lists the steps, and a second run moves nothing.

You do not have to remember it: `nerv install` and `nerv upgrade` offer
`nerv migrate --user` once a tool is installed and its settings are still
in your user `config.yaml` (showing the dry run, then asking; without a
terminal, a hint), and `nerv doctor` reports a pending `task.yaml` or
`release.yaml` move, in the user configuration and in the repository.

## From 6.6: extensions moved to tools

Up to 6.6, nerv shipped optional extensions as Claude Code plugins of its
own marketplace, and the engines behind them in its binary. Since 7.0
they are separate tools of the NERV ecosystem, installed with
`nerv install --tools <tool>` (or offered by `nerv install` and
`nerv upgrade` on a terminal):

- [nerv-task](https://github.com/war-apps/nerv-task-by-gentle-ai) brings
  the `/task:*` commands and the `task:dispatcher` agent the orchestrator
  emits its task events to;
- [nerv-release](https://github.com/war-apps/nerv-release-by-gentle-ai)
  brings the `/release:*` commands, its versioner, builder and deployer
  agents, and the version, build and deploy engines.

What nerv no longer has, and what replaces it:

| Removed from nerv | Replacement |
|---|---|
| `nerv release` | `nerv-release build` |
| `nerv release setup --import` | `nerv-release setup --import` |
| `nerv deploy` | `nerv-release deploy` |
| `nerv version next` | `nerv-release version next` (`nerv version` still prints nerv's own versions) |
| the hidden `nerv provider` command | none: providers belong to the tools (it is now an unknown command) |
| `nerv install --with <extensions>` (extension and provider selection) | `nerv install --tools nerv-task,nerv-release` (`--with` is an unknown flag, exit 2) |
| `nerv configure --repo-provider-config <provider>.<key>=<value>` | `nerv-task init` and `nerv-task configure`, which write nerv-task's `task.yaml` |
| `nerv configure --init-repo` copying `defaults.tasks` into a new project | `nerv-task init` |
| the wizard's "Task providers" section | `nerv-task configure` |
| `nerv doctor`'s release, deploy, tasks and project-extension checks | the tools' own commands; `nerv doctor` still reports a pending move to `task.yaml` or `release.yaml` |
| the `task@nerv` plugin (`/task:*`, `task:dispatcher`) and `task-teamwork@nerv` | nerv-task |
| the `release@nerv` plugin (`/release:*`) and its 6 providers: `release-conventional-commits@nerv`, `release-docker-compose@nerv`, `release-bash@nerv`, `release-github@nerv`, `release-ssh-compose@nerv`, `release-script@nerv` | nerv-release |
| the 5.2.0 `deploy@nerv`, `deploy-github@nerv`, `deploy-ssh-compose@nerv` and `deploy-bash@nerv` | nerv-release |

Run with this nerv, `nerv release`, `nerv release setup`, `nerv deploy`
and `nerv version next` print one line naming their replacement and exit 2.

**Plugins left behind.** `nerv install` and `nerv upgrade` disable the
plugins above when they find them in Claude Code's `settings.json`
(enabled or not) or in its plugin cache: they are removed from
`settings.json` (backed up first) and uninstalled with
`claude plugin uninstall` (a failure is only a warning). When the tool
that replaces them is installed, or chosen in the same run, each one gets
a note:

```
Removed task@nerv from settings.json (replaced by nerv-task)
```

Otherwise one warning per tool names the plugins, the commands lost and
how to get them back:

```
Warning: disabled task@nerv, task-teamwork@nerv: nerv no longer ships them, so /task:* is unavailable until you install nerv-task: nerv install --tools nerv-task
```

**Settings.** nerv reads no `tasks:`, `release:` or `deploy:` section any
more. `nerv migrate` moves them to the tools' files (see
[To the nerv-task and nerv-release tools](#to-the-nerv-task-and-nerv-release-tools)).

The orchestrator is unchanged: it still emits its task events when the
`task:dispatcher` agent is available, which nerv-task now provides.

## Older versions

The conversions from 5.0.0-alpha.1 (`ship:`, `nerv ship`) and 5.2.0 (the
separate `deploy` extension, `provider: bash` deploy targets,
`deploy-<name>` project providers) are nerv-release's: `nerv migrate`
hands them to `nerv-release migrate` when nerv-release is installed, and
refuses them otherwise (see
[Older release and deploy sections](#older-release-and-deploy-sections)). The `deploy@nerv` plugins of 5.2.0 are disabled with the other
extension plugins (see [From 6.6](#from-66-extensions-moved-to-tools)).

## Safety

- **It lists its steps, with or without `--dry-run`.** One numbered step per file it creates, updates, moves or deletes, in the order it takes them, each with what the file is or why it changes; then "How the configuration changes", "Check by hand" and the install line. It prints no diffs. `--dry-run` stops there and writes nothing:

  ```
  nerv migrate --dry-run: these are the steps nerv migrate would take; nothing is written:
    1. Create .ai-workflow/config.yaml: the new project configuration, from .nerv/nerv.yaml
    2. Create .nerv/.gitignore: keeps nerv's local files out of git
    3. Move .nerv/nerv.yaml to .nerv/nerv.yaml.migrated: a copy of the old configuration, so an older nerv does not read it

  How the configuration changes:
    - tasks.provider: teamwork (the 4.2.0 default): the nerv-task tool
    ...

  Next, install the tools it uses: nerv install --tools nerv-task

  nerv migrate: dry run: 3 change(s), nothing was written
  ```

  A real run prints the same steps, then the backups it made and `nerv migrate: <n> change(s) written`.
- **Backups.** Every file the migration changes, moves or removes is copied first. The repository's copies go to `.nerv/backups/migrate-<yyyyMMdd-HHmmss>/`, at the same relative path and with the file's permission bits; that folder is git-ignored. The user config's copies go next to the file, as `<file>.bak-migrate-<yyyyMMdd-HHmmss>`.
- **Atomic writes.** Each file is written to a temporary file and renamed into place.
- **One transaction.** When any step fails, every step already done is undone in reverse order, and the backups and `.nerv/.gitignore` that run created are removed; the error ends with `every change was rolled back`. Fix the cause and run it again.
- **An interrupted run** (killed before it could roll back) is finished by the next one: it keeps the ODD document that carries its own marker and a `.ai-workflow/config.yaml` equal to the one it would write, and moves what is left.
- **A second run** changes nothing and prints `nerv migrate: nothing to migrate`.
- **Refusals.** Nothing is written when:
  - the old file and its replacement both exist with different content (nerv migrate does not merge them);
  - an ODD document of the same name already exists, and `nerv migrate` did not write it;
  - a change's lock is still held on this machine, without `--force`;
  - an older release or deploy section is found (see [Older release and deploy sections](#older-release-and-deploy-sections)).

Exit codes: 0 for migrated, nothing to migrate, or a dry run; 1 for a
refusal (including running outside a git repository without `--user`); 2
for a usage or environment error.
