# Migrating from nerv 4.2.0 or 5.0.0-alpha.1

`nerv migrate` moves a repository, and your user configuration, from nerv
4.2.0 or 5.0.0-alpha.1 to this version. You run it once per repository, plus once for the
user configuration. It is written in Go and works the same way on Linux,
macOS and Windows.

```sh
nerv migrate --dry-run          # list the steps; nothing is written
nerv migrate                    # migrate this repository
nerv migrate --user             # also migrate your user configuration
nerv migrate --install          # then install the extensions the repository used
nerv migrate --force            # also move a change whose lock is still held
```

Until it runs, nerv reads neither old file: every command refuses while a
5.0.0-alpha.1 `.ai-workflow.yml` (or user `config.yml`) is still there,
with `run "nerv migrate" to move it` (`nerv migrate --user` for the user
file), and the SessionStart hook does not activate NERV. A 4.2.0 file is
reported the same way, and the repository is treated as unconfigured.

## What it changes

| 4.2.0 | Now |
|---|---|
| `<repo>/.nerv/nerv.yaml` | `<repo>/.ai-workflow/config.yaml`. The old file is kept as `.nerv/nerv.yaml.migrated`. |
| `<repo>/.ai-workflow.yml` (5.0.0-alpha.1) | `<repo>/.ai-workflow/config.yaml`, with the same content (its header comment names the new file). The old file is removed after its backup under `.nerv/backups/`. |
| `~/.claude/nerv/nerv.yaml` (with `--user`) | the user `config.yaml`: `~/.config/ai-workflow/config.yaml` on Linux, `%AppData%\ai-workflow\config.yaml` on Windows. The old file is kept as `nerv.yaml.migrated`. |
| the user `config.yml` (5.0.0-alpha.1, with `--user`) | the user `config.yaml` in the same folder. The old file is kept as `config.yml.migrated`. |
| `models:` and the provider keys of user scope in the project file, such as `tasks.providers.teamwork.assignee_id` | Moved to the user config when `--user` runs in the same repository. Otherwise they stay where they are and are reported. |
| `tasks.rounding_minutes` | `tasks.providers.teamwork.rounding_minutes` |
| `tasks.provider: jira` or `github-projects`, `tasks.providers.jira` / `.github-projects` | Dropped and reported: those providers were removed. |
| `tasks.providers.teamwork.task_ref_prefix` | Dropped: the provider's manifest fixes the prefix. |
| `ship:` (5.0.0-alpha.1) | `release:`, today's section (see [From 5.0.0-alpha.1](#from-500-alpha1)). |
| `release:` (4.2.0) | `release:`, today's section (see [Release](#release)). A 4.2.0 `release:` already in `.ai-workflow.yml` or `.ai-workflow/config.yaml` is told apart by its 4.2.0 keys (`semantic_release:`, the presets) and converted in place. |
| the semantic-release configuration (`package.json` `release`, `.releaserc*`) | Imported into `release:` by the same code as `nerv release setup --import`. Its warnings are printed. |
| `pipeline:` | `deploy:` (see [Deploy](#deploy)) |
| `openspec/changes/<change>/`, in flight, FULL | `odd/tasks/<change>.md`, with its run files in `odd/<change>/nerv/` and its lock in `.nerv/locks/<change>.lock` (see [ODD](#odd)) |
| `openspec/changes/<change>/`, in flight, DIRECT or LIGHT | `.nerv/runs/<change>/`, with no ODD document, and its lock in `.nerv/locks/<change>.lock` (see [ODD](#odd)) |
| `openspec/changes/archive/`, `openspec/specs/` | They stay where they are, read-only. |
| `openspec/config.yaml` `strict_tdd`, `testing.runner` | `odd.tdd: strict` or `standard`, and `odd.test_runner`, in `.ai-workflow/config.yaml`. |
| — | `.nerv/.gitignore` (`*`): `.nerv/` now holds runtime files only. |

Comments and key order are kept where possible. Untouched lines are copied
byte for byte. Rewritten sections (`release:`, `deploy:`) are written again by
yaml.v3, so a flow mapping loses its inner spaces: `{ a: 1 }` becomes
`{a: 1}`.

### Release

Each 4.2.0 project becomes a project of today's `release:` section with `build: bash`.
Its presets turn into the stage commands they ran in 4.2.0:

- **dotnet**:
  - `dotnet restore`, `dotnet format --verify-no-changes`, `dotnet build -c <configuration> --no-restore -p:Version="$NERV_RELEASE_VERSION"` and `dotnet test -c <configuration> --no-build`;
  - each with `--disable-build-servers` and the build-server-safe environment;
  - `project_file` becomes the project's `version_files`;
  - `coverage_threshold` becomes the project's `coverage.line`, with the test collecting coverage into `$NERV_RELEASE_ARTIFACTS_DIR/coverage`.
- **node**: the frozen-lockfile install (`npm ci`, `pnpm install --frozen-lockfile`, `yarn install --immutable` or `--frozen-lockfile`), then the `lint`, `build` and `test` scripts that `package.json` defines.
- **docker**:
  - one `docker build` per image, with the same tags (the version; the channel off the default channel; `latest` on it), build arguments and labels;
  - then a `docker save` of those tags into the artifact.
- **nuget**: `dotnet pack` into the artifact.
- **npm**: `npm pack` into the artifact.

A stage command the project wrote itself still replaces the presets', as it
did in 4.2.0. The docker preset's builds go into `stages.package`, and every
image 4.2.0 pushed becomes an `images` entry of the project `{ name, tags }` with its
4.2.0 tag kinds (`version` always, the channel off the default channel,
`latest` on it): `nerv release` tags and saves every reference into the
artifact after the package commands and records them, and the
`deploy-ssh-compose` publish pushes them all, as 4.2.0 did. An image with
`push: false` is saved by the package commands themselves, with no
reference to push. `semantic_release:` is dropped. The `publish` stage and the
`publish`, `success`, `before_publish` and `after_publish` hooks are dropped
and reported: publishing belongs to `nerv deploy` now.

Where 4.2.0's behaviour has no exact equivalent, the stage gets a
`# TODO(nerv migrate):` comment and the run prints a warning:

- **NuGet and npm publishing.** No deploy provider publishes them yet.
- **`--disable-build-servers`.** 4.2.0 dropped the flag for a .NET SDK older than 7.0.100. Script mode keeps it.

### Deploy

Each environment of `pipeline.deploy` (or 4.2.0's default four:
`production-green`, `production-blue`, `staging`, `development`) becomes one
`deploy-ssh-compose` target. It carries the `pipeline.deploy` settings that
were written: `source`, `infisical.*`, `update_timeout`, `ssh.*`, `verify.*`.

The user-scope settings (`infisical_domain`, `ssh_auth`, `ssh_key_file`,
`ssh_known_hosts`, `ssh_connect_timeout`) go to the user config under
`deploy.providers.ssh-compose` with `--user`. Without it, they go into each
target, and a warning says so.

When origin is on GitHub, `deploy.host: github` is written. That is also
the case for a repository with only `release:` whose semantic-release
configuration published GitHub releases: the tag, the release commit and
the GitHub release are `nerv deploy`'s job now.

Reported, with no equivalent:

- `pipeline.sync`: there is no back-merge after a release;
- `pipeline.deploy.selection: branches`: a target deploys its environment when the environment's `DOCKER_IMAGE_VERSION` points at the release; choose targets with `nerv deploy --target`;
- `pipeline.registry`'s credential variables: run `docker login` before `nerv deploy`;
- `pipeline.images`: the project's `images`, saved into the artifact by nerv release and pushed by the `deploy-ssh-compose` publish;
- `pipeline.dotnet`: the release stages already cover validate and test.

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

## Extensions

The run ends with the exact install line for the extensions and providers
the old configuration used (each installs as its `<name>@nerv` Claude Code
plugin):

- the `task` extension and its `task-teamwork` provider when `tasks.provider` was `teamwork` (also the 4.2.0 default when nothing set it);
- the `release` extension when the 4.2.0 `release:` section or the 5.0.0-alpha.1 `ship:` section existed;
- the `deploy` extension and its `deploy-github` provider when a git host was written;
- the `deploy-ssh-compose` provider when `pipeline.deploy` existed.

```
Next, install the extensions it uses: nerv install --with task,task-teamwork,release,deploy,deploy-github
```

`--install` runs that line through `nerv install` (without the closing
wizard). The install step reports old `/task:*` copies in
`~/.claude/commands/task`, and removes them when you confirm on a terminal.
`nerv migrate` never removes them itself.

## From 5.0.0-alpha.1

In 5.0.0-alpha.1 the release extension was called ship: its section was
`ship:`, its command `nerv ship`, and its configuration lived in
`<repo>/.ai-workflow.yml`. The rename is a clean break: no command reads
the old names. Until `nerv migrate` runs, `nerv version next`, `nerv
release` and `nerv release setup` refuse a `ship:` section (exit 1, the
file untouched) with `ship: the ship: section of nerv 5.0.0-alpha.1 is no
longer read ... run nerv migrate to convert it into the release: section`.

`nerv migrate` does it all in one transaction (dry run, backups, rollback,
a second run that changes nothing):

1. `.ai-workflow.yml` moves to `.ai-workflow/config.yaml` (a `ship:`
   section already moved there by hand is converted in place).
2. `ship:` is renamed `release:`, where it was, with its comments.
3. The keys move to the release extension's provider slots:

| 5.0.0-alpha.1 | Now |
|---|---|
| `ship.branches`, `release_rules`, `note_keywords`, `tag_format`, `repository_url`, `changelog` | `release.providers.conventional-commits.<key>` (the version engine's keys) |
| `ship.projects[].tag_format`, `changelog` | Unchanged: still project keys. |
| `build: { mode: compose, ... }` | `build: docker-compose` |
| `build: { mode: script, ... }` | `build: bash` |
| `build.file`, `build.test_service` | `release.projects[].providers.docker-compose.file`, `.test_service` |
| `build.shared_host` | `release.projects[].providers.bash.shared_host` |
| `build.images`, `build.version_files`, `build.coverage` | `release.projects[].images`, `.version_files`, `.coverage` |
| everything else (`stages`, `hooks`, `depends_on`, `scopes`, ...) | Unchanged. |

For example:

```yaml
# 5.0.0-alpha.1, .ai-workflow.yml
ship:
  tag_format: "v${version}"
  projects:
    - { name: api, path: ., build: { mode: compose, file: docker-compose.yml, images: ["ghcr.io/acme/app:${version}"] } }

# now, .ai-workflow/config.yaml
release:
  providers:
    conventional-commits:
      tag_format: "v${version}"
  projects:
    - name: api
      path: .
      build: docker-compose
      images: ["ghcr.io/acme/app:${version}"]
      providers:
        docker-compose:
          file: docker-compose.yml
```

A `release:` section with the engine keys at its top or a `build:`
mapping (the same shape under the new name) is moved the same way. A file
with both `ship:` and `release:` is refused: merge them by hand.

The user configuration moves with `nerv migrate --user`: the user
`config.yml` becomes `config.yaml` in the same folder. It holds no `ship:`
keys (the section was project scope only).

Artifacts written by `nerv ship` (schema `nerv.ship-artifact/v1`) are not
converted: `nerv deploy` refuses them with `this artifact was made by nerv
5.0.0-alpha.1 (nerv ship); rebuild it with nerv release`. Run `nerv
release` again for the version you want to deploy.

## Safety

- **It lists its steps, with or without `--dry-run`.** One numbered step per file it creates, updates, moves or deletes, in the order it takes them, each with what the file is or why it changes; then "How the configuration changes", "Check by hand" and the install line. It prints no diffs. `--dry-run` stops there and writes nothing:

  ```
  nerv migrate --dry-run: these are the steps nerv migrate would take; nothing is written:
    1. Create .ai-workflow/config.yaml: the new project configuration, from .nerv/nerv.yaml
    2. Create .nerv/.gitignore: keeps nerv's local files out of git
    3. Move .nerv/nerv.yaml to .nerv/nerv.yaml.migrated: a copy of the old configuration, so an older nerv does not read it

  How the configuration changes:
    - release: converted from the 4.2.0 section
    ...

  Check by hand:
    - deploy.host: the repository has no origin, so no git host is written: set deploy.host when it has one

  Next, install the extensions it uses: nerv install --with task,task-teamwork,release,deploy

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
  - the file has both a `ship:` section (5.0.0-alpha.1) and a `release:` section;
  - a `ship:` key has no place: an unknown `build.mode` or `build:` key, or a key its new place already holds;
  - the migrated `.ai-workflow/config.yaml` does not load with today's `release:` and `deploy:` readers.

Exit codes: 0 for migrated, nothing to migrate, or a dry run; 1 for a
refusal (including running outside a git repository without `--user`); 2
for a usage or environment error.

## Known limits

- The deploy side is checked at the configuration level: the migrated targets load with `deployconfig` and each key and value fits the `deploy-ssh-compose` manifest. A full `nerv deploy --dry-run` needs an artifact, so the tests do not run one.
- `manifest.json` of an artifact is not hashed by its own integrity check. It is a local file, a known limit of the artifact format, not of the migration.
