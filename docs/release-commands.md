# Release commands: `nerv version next` and `nerv ship`

Two commands for releasing your own repositories (any language, single
repository or monorepo) from your machine, with the repository's own
semantic-release configuration. Use them where the CI release workflow
cannot or should not run; they compute and release exactly what CI would.

| Command | What it does | Changes anything? |
|---|---|---|
| `nerv version next` (`/nerv:version`) | The next semantic version of the repository, or of each project of a monorepo, from a semantic-release dry run, with the outputs of `cycjimmy/semantic-release-action`. | No. |
| `nerv ship` (`/nerv:ship`) | A full release of what is on origin, in a throwaway clone: pipeline (restore, lint, build, test, coverage, package), semantic-release for real (release commit, CHANGELOG, tag, GitHub release), publish (docker, NuGet, npm) and hooks, per project in dependency order. | Yes, on origin and your registries; never your local repository. |

Both read the optional `release:` section of the project-scope
`<repo>/.nerv/nerv.yaml`. Without it a repository is one project and is
computed exactly like plain semantic-release.

## Contents

- [Requirements](#requirements)
- [Quick start](#quick-start)
- [Configuration (`release:`)](#configuration-release)
- [`nerv version next`](#nerv-version-next)
- [`nerv ship`](#nerv-ship)
- [Pipeline and presets](#pipeline-and-presets)
- [Stages, hooks and commands](#stages-hooks-and-commands)
- [Security notes](#security-notes)
- [Troubleshooting and recovery](#troubleshooting-and-recovery)
- [Known limitations](#known-limitations)

## Requirements

| Tool | Needed for |
|---|---|
| git | Always. 2.31 or newer when a project's `path` is not `.` (path filters); an older git stops before computing anything, naming the git found and the minimum. |
| Node.js 20.8.1 or newer, and npm | Always: semantic-release and its plugins are installed in a NERV cache, never in the repository ([Installed plugins](#installed-plugins)). |
| bash, pwsh (PowerShell 7) or node | `nerv ship`: the stage overrides and hooks that use them. Inline pwsh commands need PowerShell 7.3 or newer. On Windows, bash is Git Bash. |
| .NET SDK | The `dotnet` and `nuget` presets. |
| docker | The `docker` preset (and your `docker login` to push). |
| npm, pnpm or yarn | The `node` and `npm` presets. |
| `gh` (optional) | `nerv ship`: the GitHub token when `GH_TOKEN` / `GITHUB_TOKEN` are not set. |

Read access to origin is needed for both commands (`nerv version next
--offline` skips it); push access for `nerv ship`.

## Quick start

### Interactive setup

In Claude Code, `/nerv:release-setup` writes the `release:` section for
you:

1. It detects what the repository already has: its semantic-release
   configuration (branches, plugins that need pinning), `.csproj` /
   `.sln` / `.slnx` files, Dockerfiles and CI docker steps, `package.json`
   scripts, release scripts, and a monorepo layout with its commit scopes.
2. It proposes a complete section, explains each non-obvious choice, and
   asks only what it cannot detect, one question at a time (an image
   registry, the variable holding a NuGet key, a coverage threshold,
   `depends_on`).
3. It writes the section into `.nerv/nerv.yaml` (the rest of the file is
   kept as it is) and validates it with `nerv version next --offline`,
   showing each project's next version.

It never commits, pushes or releases: commit and push `.nerv/nerv.yaml`
yourself, then run `/nerv:ship`. `/nerv:release-setup --project <name>`
adds or edits one project of an existing section. The examples below show
the shapes it produces.

### Single repository: dotnet API with a docker image

A repository shaped like `dim.comprobantes.api`: semantic-release
configured in its `package.json` `release` (branches main, `release/rc`
with channel `rc`, develop with channel `alpha`), the API's `.csproj`
receives the version, and one docker image is tagged with the version, the
release channel and `latest` (only on releases of the default channel,
main). Add to `.nerv/nerv.yaml`, commit and push it:

```yaml
release:
  projects:
    - name: api
      path: .
      presets: [dotnet, docker]
      dotnet:
        project_file: src/Comprobantes.Api/Comprobantes.Api.csproj
        solution: Comprobantes.sln
        coverage_threshold: 80
      docker:
        images:
          - name: ghcr.io/org/comprobantes-api
            dockerfile: Dockerfile
            context: .
            build_args: { APP_VERSION: "${version}" }
            tags: [version, channel, latest]
      hooks:
        after_test:
          - run: scripts/report-coverage.ps1
            shell: pwsh
```

With one project the tag is `v1.4.0` and the CHANGELOG is `CHANGELOG.md`.
Then:

```sh
nerv version next --branch develop    # what would be released (read-only)
nerv ship --dry-run                   # build, test, pack and docker build in a clone
nerv ship                             # the real release, after a [y/N] confirmation
```

In Claude Code, `/nerv:version` and `/nerv:ship` do the same; `/nerv:ship`
always rehearses with `--dry-run` first and releases only after your
explicit yes.

### Monorepo

Each project gets its own version, tag (`<name>-v${version}`) and CHANGELOG
under its `path`. A commit counts for a project when it touches its `path`
or `shared_paths`, or when its conventional scope is one of the project's
`scopes`; one commit can release several projects. A new release of `core`
releases `api` with at least a patch:

```yaml
release:
  projects:
    - name: core
      path: src/core
      presets: [dotnet, nuget]
      dotnet: { project_file: src/core/Core.csproj }
    - name: api
      path: src/api
      shared_paths: [Directory.Build.props]
      depends_on:
        - { project: core, bump: patch }
      presets: [dotnet, docker]
      dotnet: { project_file: src/api/Api.csproj }
      docker:
        images:
          - name: ghcr.io/org/api
            dockerfile: src/api/Dockerfile
    - name: web
      path: web
      scopes: [web, ui]
      presets: [node]
```

`nerv version next` prints one block per project; `nerv ship --project api`
releases only `api` (its dependencies are computed for `depends_on`, not
released).

## Configuration (`release:`)

The section is project scope only: it is never read from the user-scope
`~/.claude/nerv/nerv.yaml` and never merged with it. When it is absent, the
repository simply has no release configuration.

The versioning rules themselves (branches and channels, `releaseRules`,
presets, plugins) stay in the repository's own semantic-release
configuration (`.releaserc*`, `release.config.*` or `package.json`
`release`). `release:` adds what semantic-release does not know: which
projects a monorepo holds, how they depend on each other, and how each one
is built and published.

`nerv ship` plans the release from the `release:` section committed on
origin (the release clone's `.nerv/nerv.yaml`); your local copy is only
validated first. Commit and push changes to it before releasing.

### Paths

Every path in the section is relative to the repository root, whatever the
project's own `path` is. Forward slashes are canonical; backslashes are
accepted and normalised. Absolute paths (`/x`, `C:\x`) and paths that
escape the repository (`../x`) are rejected.

### `release.semantic_release`

| Field | Default | Meaning |
|---|---|---|
| `version` | `24.2.3` | Exact semantic-release version installed in the NERV cache (never in the repo). Ranges such as `^24` are rejected. |
| `extra_plugins` | `[]` | npm packages the repo's semantic-release configuration needs beyond the bundled plugins, each pinned as `name@version` (e.g. `@semantic-release/exec@6.0.3`). |

#### Installed plugins

NERV installs semantic-release with its plugins in
`<user cache dir>/nerv/semantic-release/<hash>` (on Linux
`~/.cache/nerv/...`, on Windows `%LocalAppData%\nerv\...`), once per set of
versions, with `npm install --ignore-scripts`. Always available:

| Package | Version |
|---|---|
| `semantic-release` | `version` above |
| `@semantic-release/commit-analyzer`, `@semantic-release/release-notes-generator`, `@semantic-release/npm`, `@semantic-release/github`, `conventional-changelog-angular` | bundled with semantic-release |
| `@semantic-release/changelog` | `6.0.3` |
| `@semantic-release/git` | `10.0.1` |
| `@semantic-release/exec` | `7.1.0` |
| `conventional-changelog-conventionalcommits` | `8.0.0` (9.x also works; 10 and later fail with the `conventional-changelog-writer` 8 that semantic-release 24 bundles, see `ConventionalCommitsVersion` in `internal/semrel/cache.go`) |

Any other package the repository's semantic-release configuration names (a
plugin, a shareable config in `extends`, a `preset`'s
`conventional-changelog-<preset>` package, a `config` or `releaseRules`
module) must be pinned in `extra_plugins`; otherwise the command fails and
names it. An `extra_plugins` entry for one of the packages above replaces
its version.

### `release.projects[]`

At least one project is required. A single-repo setup is one project with
`path: .`.

| Field | Default | Meaning |
|---|---|---|
| `name` | required | Unique id, `[a-z0-9][a-z0-9-]*`. Used in tags, `--project` and `depends_on`. |
| `path` | required | The project's directory. A commit touching it counts for the project. |
| `shared_paths` | `[]` | Extra paths or globs whose changes also count for the project (shared code, build files). |
| `scopes` | `[<name>]` | Conventional-commit scopes whose commits count for the project wherever they touch. A commit counts when it touches `path`/`shared_paths` **or** its scope matches; one commit can release several projects. |
| `tag_format` | `v${version}` (one project) / `<name>-v${version}` | semantic-release `tagFormat`. Must contain `${version}`, no other placeholder, and be unique across projects. |
| `changelog` | `<path>/CHANGELOG.md` | The project's CHANGELOG file. |
| `depends_on` | `[]` | Projects whose new release bumps this one; see below. |
| `presets` | `[]` | Built-in stages and publishers: any of `dotnet`, `node`, `docker`, `nuget`, `npm`. A settings block (`dotnet:`, `docker:`, ...) without its preset enabled is an error. |
| `stages` | preset commands | Override or add a pipeline stage command; see [Stages, hooks and commands](#stages-hooks-and-commands). |
| `hooks` | `{}` | Extra scripts around stages and release steps; same section. |

### `depends_on[]`

| Field | Default | Meaning |
|---|---|---|
| `project` | required | Another project of the section (not itself). |
| `bump` | `patch` | What a new release of that project does to this one: `patch`, `minor` or `none` (no bump, but still released after it). |

Dependencies must not form a cycle; the error names it
(`dependency cycle a -> b -> a`). Projects are computed and released in
dependency order, otherwise in declaration order.

### Preset settings

| Block | Field | Default | Meaning |
|---|---|---|---|
| `dotnet` | `project_file` | required | The `.csproj` that receives `<Version>` and `<PackageReleaseNotes>` ([The dotnet project file](#the-dotnet-project-file)). |
| | `solution` | the project file | The `.sln`/`.slnx` restore, build and test run against. |
| | `coverage_threshold` | `0` (off) | Minimum line coverage percentage, 0-100. |
| | `configuration` | `Release` | Build configuration. |
| `node` | `package_manager` | `npm` | `npm`, `pnpm` or `yarn`. |
| `docker` | `images[]` | required (one or more) | Images built, tagged and pushed in order. |
| | `images[].name` | required | Image repository, e.g. `ghcr.io/org/api`. |
| | `images[].dockerfile` | `Dockerfile` | Dockerfile path. |
| | `images[].context` | `.` | Build context. |
| | `images[].build_args` | `{}` | `--build-arg` values; may use `${version}`, `${channel}` and `${tag}`. |
| | `images[].tags` | `[version, channel, latest]` | `version` (e.g. `1.4.0`), `channel` (e.g. `alpha`, `rc`), `latest` (default-channel releases only). |
| | `images[].push` | `true` | Push the tags after building. |
| `nuget` | `source` | `https://api.nuget.org/v3/index.json` | Push source. |
| | `api_key_env` | `NUGET_API_KEY` | Environment variable holding the API key. |
| `npm` | `registry` | npm's own configuration | Publish registry. |
| | `access` | `public` | `public` or `restricted`. |
| | `dist_tag` | the release channel | npm dist-tag. |

### Validation

Unknown keys are errors, and every problem (unknown keys included) is
reported at once with its path in the section, for example:

```text
invalid release configuration:
  release.projects[0].pth: unknown field (line 5)
  release.projects[0].path: required
  release.projects[1].depends_on[0].project: unknown project "lib2"
```

Only a section that cannot be decoded at all (a value of the wrong kind,
such as `presets: docker`) stops at the decoding problems. Both commands
refuse an invalid section (exit 1) before reading origin.

### YAML notes

- Only the `release:` block is parsed: it is cut out of `nerv.yaml` by
  indentation, so a YAML error elsewhere in the file never affects the
  release commands. When the file's layout does not allow the cut
  (`---` between documents, an indented or flow top level), the whole file
  is parsed instead. The key may be quoted (`"release":`, `'release':`) or
  carry an anchor or a tag (`&r release:`, `!!str release:`). If the file
  is not valid YAML and a top-level line may still be the release key
  without being recognised (e.g. an escaped quoted key), that is an error
  naming the line, not a repository without release configuration.
- Anchors and merge keys work inside the section, e.g. to share settings
  between projects:

  ```text
  projects:
    - &base
      name: api
      path: api
      presets: [dotnet]
      dotnet: { project_file: api/Api.csproj }
    - <<: *base
      name: worker
      path: worker
  ```

  Keys reached through a merge are checked like any other.
- An alias inside the section cannot refer to an anchor defined outside
  it; that is reported as an error. An alias whose anchor is defined
  nowhere before it (a typo, or an alias used before its anchor) is
  reported as an undefined anchor, with the line of the alias.
- A file whose top level is not a mapping is an error; an empty file, or
  one without `release:`, means the repository has no release
  configuration.

## `nerv version next`

```
nerv version next [--cwd <dir>] [--branch <name>] [--project <name>]
                  [--offline] [--fetch-timeout <dur>] [--format text|json|env | --json]
```

Computes the next semantic version of the repository, or of each project of
its `release:` section, with the repository's own semantic-release
configuration in dry-run mode. Nothing in the repository changes. In Claude
Code, `/nerv:version [args]` runs it with `--json` and shows a table per
project. Plain `nerv version` (and `nerv version --json`) still prints the
binary and plugin versions.

| Flag | Default | Meaning |
|---|---|---|
| `--cwd <dir>` | `.` | Any directory inside the repository; the git top level is used. |
| `--branch <name>` | current branch | Local branch to compute. |
| `--project <name>` | all projects | Only this project of the `release:` section (its dependencies are still computed, for `depends_on`). An unknown name is refused, listing the declared projects. |
| `--offline` | off | Use the local tags and notes only. By default origin's release tags and semantic-release notes are read first, with your git credentials, into a throwaway copy. Offline, a prerelease branch may compute a different version than CI when a release's channel note exists only on origin. |
| `--fetch-timeout <dur>` | `60s` | Bound for reading origin (Go duration: `30s`, `2m`); must be positive, there is no unbounded mode (use `--offline` to skip origin). |
| `--format text\|json\|env` | `text` | Output format. |
| `--json` | | Same as `--format json`. |

### How the next version is computed

- semantic-release runs in dry-run mode with the repository's own
  configuration; only the commit analysis and the release notes run, exactly
  as configured. The steps that check credentials or have side effects
  (`verifyConditions`, `verifyRelease`, `prepare`, `publish`, `success`,
  `fail`) are skipped, so no `GH_TOKEN`, `NPM_TOKEN` or exec script is
  needed.
- It runs on a disposable mirror of the repository, checked out on the
  requested local branch: the working tree, branches and tags of the
  repository are never changed.
- semantic-release sees origin's branches, as in CI, not your local ones: a
  local-only branch (say `hotfix/x` under a `hotfix/*` prerelease entry)
  does not count, and a branch that exists only on origin does. The branch
  you compute is taken at your local commit, so its unpushed commits count.
  Online, origin's branches are read with its tags and notes; with
  `--offline`, your `refs/remotes/origin/*` (as of your last fetch) are
  used. With no `origin` remote, or offline with no remote-tracking branch,
  the local branches are used as they are.
- Release tags and semantic-release's git notes are read from the `origin`
  remote first (read access is enough). semantic-release records each
  release's channel in a note that a plain `git fetch` never brings; without
  it a prerelease tag such as `v2.9.0-alpha.8` is not recognised as the last
  `alpha` release. Offline, only the local tags are used and the result may
  differ from CI on prerelease branches. How origin is read safely:
  [Reading origin](#reading-origin).
- The release notes link to the repository's public address, as in CI:
  the `repositoryUrl` of its semantic-release configuration, the
  `package.json` `repository`, or the `origin` remote's URL (offline too;
  user info, query and fragment removed, so credentials never reach the
  notes). With none of these, or a local path, the notes have no links
  (`## 1.0.1 (2026-10-13)`, `* fix thing (c1918a5)`).
- In a monorepo, each project is computed in dependency order with its own
  `tag_format`, from the commits that touch its `path` or `shared_paths` or
  carry one of its `scopes` (`type(scope): ...`, several scopes separated by
  commas). A merge commit touches what it brings into the branch (its
  changes against its first parent); the merged commits count on their own.
  This needs git 2.31 or newer. A project whose `path` is `.` sees every
  commit. When a dependency gets a new release, the dependent is released
  with at least the dependency's `bump`, even without commits of its own.
- A single project with the default `tag_format` keeps the repository's own
  `tagFormat`, and a repository without a `release:` section is computed
  exactly like plain semantic-release.

### Outputs

The outputs of `cycjimmy/semantic-release-action`, per project. Unlike the
action, `last_release_*` are also filled when there is no new release.

| Output | Meaning |
|---|---|
| `new_release_published` | `true` when there is a new release, else `false` |
| `new_release_version` | Next version, e.g. `1.2.0-alpha.2` |
| `new_release_major_version` | Its major number |
| `new_release_minor_version` | Its minor number |
| `new_release_patch_version` | Its patch number |
| `new_release_channel` | Distribution channel (`alpha`, `rc`, ...); empty for the default channel |
| `new_release_notes` | Release notes (Markdown, several lines) |
| `new_release_git_head` | Commit the release would be tagged on |
| `new_release_git_tag` | Tag it would get, e.g. `v1.2.0` or `api-v1.2.0` |
| `last_release_version` | Last release on the branch's channel; empty when none |
| `last_release_git_head` | Its commit |
| `last_release_git_tag` | Its tag |

### Formats

stdout carries only the result; progress and errors go to stderr.

`text` (default), one block per project:

```
api (develop)
  next release: no new release
  last release: 1.0.0 (tag api-v1.0.0)

web-app (develop)
  next release: 1.1.0-alpha.1 (channel alpha, tag web-app-v1.1.0-alpha.1)
  last release: 1.0.0 (tag web-app-v1.0.0)
```

The output shape follows the request, not the number of results: when the
`release:` section declares more than one project and `--project` is not
given, the output covers every project (a JSON array, prefixed env keys);
otherwise (no `release:` section, a single project, or `--project`) it is
exactly that one project, even though the engine also computes the
dependencies of `--project` for `depends_on` (they are not printed). If the
engine returns no result for the requested project, the command fails
(exit 2) and prints nothing on stdout.

`json`: one object with `project`, `branch` and the 12 outputs for one
project; an array of those objects, in dependency order, for several.

```json
{"project":"widgets","branch":"main","new_release_published":true,"new_release_version":"1.0.1","new_release_major_version":"1","new_release_minor_version":"0","new_release_patch_version":"1","new_release_channel":"","new_release_notes":"## 1.0.1 ...","new_release_git_head":"c1918a5...","new_release_git_tag":"v1.0.1","last_release_version":"1.0.0","last_release_git_head":"ac22764...","last_release_git_tag":"v1.0.0"}
```

`env`: GitHub Actions output syntax, ready to append to `$GITHUB_OUTPUT`.
Single-line values are `key=value`; a multi-line value (the notes) uses the
heredoc form with a random `ghadelimiter_<hex>` delimiter that does not
occur in the value. For one project the keys are the output names; for
several, each key is prefixed with the project name, `-` written as `_`
(`web-app` gives `web_app_new_release_version`).

```
new_release_published=true
new_release_version=1.0.1
...
new_release_channel=
new_release_notes<<ghadelimiter_b098f27b1474f83aefb3da07e735c9d1
## [1.0.1](...) (2026-10-13)

### Bug Fixes

* **api:** handle empty input (c1918a5)

ghadelimiter_b098f27b1474f83aefb3da07e735c9d1
new_release_git_head=c1918a53581d83a5f8ba787da39446550e1bb166
...
```

### Exit codes

| Code | When |
|---|---|
| `0` | Computed, including when there is no new release (`new_release_published=false`). |
| `1` | Refused: an invalid `release:` section (every problem, path-qualified), an unknown `--project`, `--project` without a `release:` section, a directory outside any git repository. |
| `2` | Usage error, or an environment error: git, node or npm missing or too old, a plugin not pinned, origin unreachable or timed out (the message suggests `--offline`), no result for the requested project, the result could not be written to stdout. |
| `128 + signal` | Interrupted: `130` for Ctrl-C (SIGINT), `143` for SIGTERM, `129` for SIGHUP. |

Ctrl-C, `SIGTERM` and (on Unix) `SIGHUP` cancel the run and stop git, npm
and node, which run in their own session or process group. After the first
signal NERV stops catching them, so a second Ctrl-C ends `nerv` at once.
The same holds for `nerv ship`.

## `nerv ship`

```
nerv ship [--cwd <dir>] [--branch <name>] [--project <name>]
          [--dry-run] [--yes] [--fetch-timeout <dur>] [--json]
          [--expect <project>=<version> ...]
```

A full release from your machine, replacing the CI release workflow where
it cannot or should not run. It releases **what is on origin**, in a
throwaway clone, with the repository's own semantic-release configuration.
In Claude Code, `/nerv:ship [--branch <name>] [--project <name>]` always
runs `nerv ship --dry-run --json` first, shows the plan, and runs `nerv
ship --yes --json --expect <project>=<version> ...` (the dry run's
versions) only after your explicit yes, so the release you approved is the
only one that can run.

| Flag | Default | Meaning |
|---|---|---|
| `--cwd <dir>` | `.` | Any directory inside the repository. |
| `--branch <name>` | current branch | Branch to release; your local branch must equal origin's. |
| `--project <name>` | all projects | Only this project of the `release:` section. |
| `--dry-run` | off | Build and run the prepare hooks in the clone; never release or publish. |
| `--yes` | off | Release without asking (required without a terminal). |
| `--fetch-timeout <dur>` | `60s` | Bound for each read of origin (the fetch, the push check, the tag check); must be positive. |
| `--json` | off | Final summary as JSON. |
| `--expect <project>=<version>` | none | Repeatable. Release only when the computed releases are exactly these: every listed project releases that version and no other project releases. Otherwise the run is refused (exit 1, `--json` status `refused`, expected vs computed per project) before any stage runs. Use the names and versions a `--dry-run --json` printed (for a repository without a `release:` section, the project name it prints). A malformed value or a project given twice is a usage error (exit 2). |

### Flow

1. Resolve the repository and the branch (default: the current one), load
   the `release:` section (a repository without one is a single project)
   and refuse an invalid section or an unknown `--project` before anything
   else.
2. Create the **release clone**: a fresh `git clone --shared` of your
   repository in a temporary directory (never a worktree of it), whose
   `origin` is your origin's address; origin's branches, tags and
   semantic-release notes are fetched into it and the branch is checked out
   at origin's commit. Local-only branches and tags never reach it. Your
   repository is never modified.
3. Refuse (exit 1) when your local branch is not exactly origin's (ahead:
   push first; behind: pull first): CI releases what origin has, and
   `nerv ship` never pushes your commits. A branch that is not on origin is
   refused too.
4. Compute every project's next version in the clone (as `nerv version
   next`, from the refs just read; origin is read once). Projects without a
   new release are reported and skipped. With `--expect`, refuse here
   unless the releasing projects and versions are exactly the expected
   ones.
5. Print each releasing project's version, channel, tag and last release,
   and its plan ([Pipeline and presets](#pipeline-and-presets), secrets
   shown as `$<VARIABLE>`), then ask `Release <n> project(s)? [y/N]` on an
   interactive terminal. `--yes` skips the question; without a terminal
   `--yes` is required (exit 1 otherwise); `--dry-run` never asks.
6. Before the first release, check that git can push the branch to origin
   with your credentials (`git push --dry-run`), and read `GH_TOKEN` (see
   [Credentials](#credentials)).
7. For each releasing project, in dependency order (`depends_on`):
   1. dotnet preset: write `<Version>` and `<PackageReleaseNotes>` into
      `dotnet.project_file` ([The dotnet project file](#the-dotnet-project-file)),
      before the pipeline, so the packages carry them;
   2. run the pipeline (`restore` to `package`) and the `prepare` hooks;
   3. run semantic-release for real in the clone: the repository's own
      plugins (release commit, CHANGELOG, tag, GitHub release), pushing to
      origin. If semantic-release would release another version than the
      one you confirmed, it is stopped before the tag;
   4. run the `publish` stage (docker push, NuGet, npm, publish override),
      then the `publish` hooks, then the `success` hooks.
8. Remove the clone and print the summary on stdout. The first failure
   stops the run; later projects are reported as not started.

`--dry-run` runs 7.1 and 7.2 for real in the clone (build, test, pack,
docker build, prepare hooks) and lists 7.3 and 7.4 without running them:
nothing is pushed, tagged or published. `NERV_RELEASE_DRY_RUN` is `true`.

With `--project`, only that project is released (its dependencies are
computed for `depends_on`, as in `nerv version next`, but not released).

In stages and hooks, `NERV_RELEASE_REPO_ROOT` is the release clone, and
`NERV_RELEASE_GIT_HEAD` is the commit computed in step 4 until the release,
then the commit semantic-release tagged (its release commit when a plugin
such as `@semantic-release/git` made one).

### Summary

stdout carries the summary: one line per project (`released`, with the
publish steps that completed; `no new release`; `not started`; `dry run`).
With `--json`, every run that gets past the flags prints exactly one JSON
document, early refusals and failures included:

```json
{"status":"failed","error":"project core failed (see above)","projects":[
  {"project":"core","branch":"main","status":"failed","version":"1.1.0","tag":"core-v1.1.0",
   "last_version":"1.0.0","tagged":true,"published":["publish"],"error":"...","action":"..."}]}
```

- `status`: `released`, `dry-run`, `nothing-to-release`, `refused` (exit
  1), `declined` (answered no; exit 1), `failed` (exit 2) or `interrupted`
  (128 + signal).
- `error`: the reason, when the run did not succeed.
- `projects`: one object per computed project (an empty array when the run
  stopped before computing them), with `project`, `branch`, `status`
  (`released`, `dry-run`, `no-release`, `failed`, `not-started`),
  `version`, `channel`, `tag`, `last_version`, `tagged`, `published`
  (publish-stage steps that completed), `error` and `action` (what to do
  after a failure; see [Troubleshooting and recovery](#troubleshooting-and-recovery)).

### Exit codes

| Code | When |
|---|---|
| `0` | Released, nothing to release, or a dry run that passed. |
| `1` | Refused: invalid `release:` section, unknown `--project`, not a git repository or not on a branch, no origin remote, local branch not equal to origin's, no confirmation (or not a terminal without `--yes`), a plan that cannot be built. |
| `2` | Usage error, an environment error (origin unreachable, git cannot push, node or npm missing), or a failed step or release. |
| `128 + signal` | Interrupted (`130` Ctrl-C, `143` SIGTERM, `129` SIGHUP); a second Ctrl-C ends `nerv` at once. |

## Pipeline and presets

`nerv ship` plans each project's pipeline from its presets, `stages` and
`hooks` (`internal/shipplan`), for the release being made (version,
channel, tag):

1. For each stage in order (`restore`, `lint`, `build`, `test`,
   `coverage`, `package`, `publish`), the commands are the stage's override
   in `stages:` if there is one, otherwise the commands of each enabled
   preset, in the order of `presets:`.
2. The stage's `before_<stage>` hooks run first and its `after_<stage>`
   hooks last. A stage with no command is skipped, hooks included.
3. Every stage but `publish` runs before semantic-release; the first
   failing step stops the release, naming the stage and step. The
   `publish` stage runs after semantic-release has tagged the release;
   under `--dry-run` it is listed but never run.
4. The plan is printed on stderr before the confirmation, every step per
   stage, secrets shown as `$<VARIABLE>`:

   ```text
   api 2.9.0-alpha.3 (channel alpha, tag v2.9.0-alpha.3)
     restore
       dotnet:restore  dotnet restore Comprobantes.sln
     ...
     publish (not run: dry run)
       docker:push ghcr.io/org/comprobantes-api:2.9.0-alpha.3  docker push ghcr.io/org/comprobantes-api:2.9.0-alpha.3
   ```

Preset commands run **without a shell**: each is an argument list given
to the program found on `PATH` (`dotnet`, `docker`, `npm`, `pnpm`,
`yarn`), so paths with spaces need no quoting on any OS. A missing program
fails the step with `<program> not found on PATH`. On Windows `npm`,
`pnpm` and `yarn` are `.cmd` shims that run through `cmd.exe`, which
interprets `& | < > ^ %` even inside an argument: the node and npm presets
pass only fixed arguments plus `npm.dist_tag`, `npm.registry` and the
release channel, and refuse those values (on every OS) when they hold one
of these characters, a `"` or a line break. Overrides and hooks run
through their `shell` as described in
[How a command runs](#how-a-command-runs).

Every step (preset command, override or hook) also receives
`NERV_RELEASE_ARTIFACTS_DIR`: a per-project directory outside the
repository for build outputs. The dotnet preset writes its coverage
reports to `$NERV_RELEASE_ARTIFACTS_DIR/coverage` and its packages to
`$NERV_RELEASE_ARTIFACTS_DIR/nuget`; an override replacing those commands
writes there too. Both directories are emptied before the first stage of
every run, so a coverage report or package left by an earlier run is never
counted or pushed (only those two, and only inside the artifacts
directory).

### Preset commands

`<target>` is `dotnet.solution`, or `dotnet.project_file` when there is no
solution; `<cfg>` is `dotnet.configuration`; `<artifacts>` is
`$NERV_RELEASE_ARTIFACTS_DIR`. dotnet and docker commands run in the
repository root (their paths are repository-relative); node and npm
commands run in the project's `path`.

| Preset | Stage | Step | Command |
|---|---|---|---|
| `dotnet` | restore | `dotnet:restore` | `dotnet restore <target>` |
| | lint | `dotnet:lint` | `dotnet format <target> --verify-no-changes` |
| | build | `dotnet:build` | `dotnet build <target> -c <cfg> --no-restore -p:Version=<version>` |
| | test | `dotnet:test` | `dotnet test <target> -c <cfg> --no-build`, plus `--collect "XPlat Code Coverage" --results-directory <artifacts>/coverage` when `coverage_threshold` > 0 |
| | coverage | `dotnet:coverage` | only when `coverage_threshold` > 0: the coverage check below (in NERV, no process) |
| | package | `dotnet:pack` | only with the `nuget` preset: `dotnet pack <project_file> -c <cfg> --no-build -p:Version=<version> -o <artifacts>/nuget` |
| `node` | restore | `node:restore` | `npm ci`; `pnpm install --frozen-lockfile`; `yarn install --frozen-lockfile` (Yarn 1) or `yarn install --immutable` (Yarn 2+, detected by `packageManager: "yarn@2…"` or later in `package.json`, or a `.yarnrc.yml` in the project or repository root) |
| | lint, build, test | `node:<script>` | `<package_manager> run <script>`, only for the scripts `lint`, `build`, `test` that `package.json` defines |
| `docker` | package | `docker:build <name>` | per image: `docker build -f <dockerfile> -t <name>:<tag>... --build-arg K=V... --label org.opencontainers.image.version=<version> [--label org.opencontainers.image.revision=<commit>] <context>` (build args sorted by name, placeholders replaced) |
| | publish | `docker:push <name>:<tag>` | per tag, when `push` is true: `docker push <name>:<tag>` |
| `nuget` | publish | `nuget:push` | in NERV, no process: pushes `<artifacts>/nuget/*.nupkg` (and each matching `.snupkg`) to `<source>` over NuGet's push protocol, with the key from `$<api_key_env>`; see [NuGet API key](#nuget-api-key) |
| `npm` | publish | `npm:publish` | `npm publish --access <access> [--tag <dist_tag, else the channel>] [--registry <registry>]` |

Modelled on `dim.comprobantes.api`'s CI (restore, `dotnet format
--verify-no-changes`, one Release build, one test run collecting
coverage). Its CI also builds with `-warnaserror` and tests with
`--settings coverlet.runsettings --logger trx`; such repository-specific
flags go in a stage override, e.g.
`stages: { build: { run: "dotnet build X.slnx -c Release --no-restore -warnaserror -p:Version=$NERV_RELEASE_VERSION" } }`.

- The node preset never writes the package version: semantic-release's
  `@semantic-release/npm` plugin (or the repository's own configuration)
  does. Enable the `npm` preset only when that plugin does not publish
  already (`npmPublish: false`, or the plugin not in the configuration);
  otherwise the package would be published twice.
- The `nuget` preset needs the packages `dotnet pack` writes: it requires
  the `dotnet` preset, or a `package` stage override writing `*.nupkg` to
  `$NERV_RELEASE_ARTIFACTS_DIR/nuget`.

### Docker tags

As `dim.comprobantes.api`'s workflow tags its images, for each entry of
`tags` (in order, duplicates dropped):

| Tag kind | Tag | When |
|---|---|---|
| `version` | the release version, e.g. `1.4.0-alpha.3` | always |
| `channel` | the channel, e.g. `alpha`, `rc` | only when the channel is not the default one (empty or `default`) |
| `latest` | `latest` | only on the default channel |

So main gives `1.4.0` and `latest`, develop `1.4.0-alpha.3` and `alpha`. A
tag that is not a valid docker tag (a version with `+build` metadata, a
channel with `/`) is an error when the plan is made, before anything runs.
An image with no applicable tag is still built (as a check) but not pushed.

### Coverage

With `dotnet.coverage_threshold` above 0, the test stage collects coverage
with coverlet's `XPlat Code Coverage` collector (the xunit template's
`coverlet.collector` package must be referenced by the test projects), and
the `coverage` stage reads every `*.cobertura.xml` under
`$NERV_RELEASE_ARTIFACTS_DIR/coverage`, sums their `lines-covered` and
`lines-valid` (several test projects give several reports; their rates are
not averaged) and fails when the **line** coverage is below the threshold,
e.g. `line coverage 79.90% (799 of 1000 lines, 1 report) is below the
threshold 80%`. No report, a malformed report or no coverable line also
fail. A line covered in two test projects' reports counts in both. When
`test` is overridden, the override must write the reports there (or
override `coverage` too).

### The dotnet project file

Before the pipeline, `nerv ship` writes the release into
`dotnet.project_file` in the release clone, so the packages carry it and
the release commit holds it when the repository's `@semantic-release/git`
`assets` list the file (what `dim.comprobantes.api`'s
`scripts/bump-csproj.sh` does with sed). `<PackageReleaseNotes>` is the
link to the release, `https://<host>/<owner>/<repo>/releases/tag/<tag>`,
from origin's address (ssh addresses become https); without a public
origin (a local path) only `<Version>` is written, with a warning.

- `<Version>` and `<PackageReleaseNotes>` get the new values in every
  existing element outside XML comments (attributes kept; `<Version />` is
  opened). A missing element is added at the top of the first
  `<PropertyGroup>` without a `Condition`, indented like its first child,
  or in a new group after `<Project>`.
- The rest of the file is kept byte for byte: a UTF-8 BOM, CRLF or LF line
  endings (the notes are written with the file's), indentation.
- The file is replaced atomically: the new content goes to a temp file in
  the same directory with the original's permissions, is synced to disk
  and renamed over it, so a failed write leaves the original unchanged
  (a symlinked project file updates its target and keeps the link).
- The notes are XML-escaped (`&`, `<`, `>`) and MSBuild-escaped (`%` as
  `%25`, `$` as `%24`, `@` as `%40`), so `$(Foo)` in a commit message is
  not expanded; `dotnet pack` restores them in the package's
  `releaseNotes`.

## Stages, hooks and commands

### `stages`

Keys are pipeline stages, run in this order: `restore`, `lint`, `build`,
`test`, `coverage`, `package`, `publish`. A key **is** the stage: it
replaces the commands of every enabled preset for that stage (with `dotnet`
and `node` both enabled, `stages.build` replaces both builds), or adds the
stage when no preset provides it. Any failing stage before `publish` aborts
the release before a tag exists.

```yaml
stages:
  test: { run: "dotnet test --no-build", shell: bash }
```

### `hooks`

Keys are `before_<stage>` / `after_<stage>` (for any stage above) and the
lifecycle keys `prepare` (after the pipeline, before semantic-release:
write version files here, in time for the release commit), `publish`
(after the `publish` stage), `success` (last) and `fail` (on any failure
once semantic-release has started; not after Ctrl-C). Each key holds a list
of commands run in order. `before_<stage>` / `after_<stage>` run only when
the stage has commands: a stage without any (no preset command, no
override) is skipped with its hooks.

### Command fields

Stage overrides and hooks are commands:

| Field | Default | Meaning |
|---|---|---|
| `run` | required | Command line or script path. |
| `shell` | `bash` | `node`, `pwsh` or `bash`. |
| `env` | `{}` | Extra environment variables. Names starting with `NERV_RELEASE_` are reserved for the variables NERV passes (version, channel, tag, notes file, project). |
| `workdir` | the repository root | Working directory, relative to the repository root (not to the project's `path`); absolute or escaping paths are rejected like any other path. |
| `timeout` | none | Bound for the command, as a Go duration (`90s`, `10m`, `1h30m`); must be positive. When it expires the command and every process it started are killed and the stage or hook fails. |

### How a command runs

`run` is a **script file** when, trimmed, it is one line naming an existing
file relative to the repository root (backslashes accepted, never outside
the repository, symlinks included) with an extension the shell runs as a
file: any for `bash`, `.ps1` for `pwsh`, `.js`, `.mjs` or `.cjs` for
`node`. The file is passed relative to the working directory. A script path
whose real location, once symlinks are resolved, is outside the repository
fails the stage or hook (it is neither run as a file nor as an inline
command). Anything else, including a script path followed by arguments, is
an **inline** command:

| Shell | Script file | Inline command |
|---|---|---|
| `bash` | `bash --noprofile --norc <file>` | `bash --noprofile --norc -e -o pipefail -c <run>` |
| `pwsh` | `pwsh -NoLogo -NoProfile -NonInteractive -File <file>` | `pwsh -NoLogo -NoProfile -NonInteractive -Command <run>`, preceded by `$ErrorActionPreference = 'Stop'; $PSNativeCommandUseErrorActionPreference = $true` and followed by `exit 0` |
| `node` | `node <file>` | `node -e <run>` |

- Inline commands stop at the first failure, like a CI step: bash with
  `-e` and `pipefail` (`false | true` fails), pwsh with errors made
  terminating, including a native command's non-zero exit. Inline pwsh
  commands need PowerShell 7.3 or newer: an older pwsh exits `3` with a
  message before running the command, because without
  `$PSNativeCommandUseErrorActionPreference` a failing native command could
  look like a success. An inline pwsh command's exit code says how it
  ended: `0` when it ran to its end (a native failure it handled with
  `try`/`catch`, or checked through `$LASTEXITCODE` and moved on, does not
  count), `1` on an uncaught error (an unhandled native failure included),
  `N` on an explicit `exit N`. A script file runs as written: its own
  `set -e`, `$ErrorActionPreference` and `exit` codes decide.
- Interpreters are looked up on `PATH`: `bash`, `pwsh` (PowerShell 7;
  Windows PowerShell 5.1 `powershell.exe` is not used) and `node`. A
  missing interpreter fails the stage or hook with a message saying what
  to install. On Windows, `bash` is Git Bash: `bash.exe` on `PATH` unless
  it is WSL's launcher (`System32\bash.exe` or its `WindowsApps` alias),
  then `bin\bash.exe` of the Git for Windows installation holding
  `git.exe`, then `%ProgramFiles%\Git\bin\bash.exe` and
  `%LocalAppData%\Programs\Git\bin\bash.exe`.
- The working directory is the repository root, or `workdir`; a
  `workdir` that resolves outside the repository through a symlink fails
  the stage or hook.
- Standard input is empty (the null device) and the command has no
  terminal: a tool that prompts fails instead of waiting.
- stdout and stderr are streamed live, as one stream in the order they were
  written, each line prefixed with `[<project>:<stage or hook>] `, e.g.
  `[api:before_test] restoring tools`.
- A failure names the project, the stage or hook, the shell (and the
  script file), and the exit code, the signal, `timed out after <timeout>`
  or `interrupted`, followed by the last 20 lines of output (terminal
  colour codes removed). Environment values are never printed.
- Ctrl-C, or the `timeout`, kills the command and every process it
  started.

### Environment variables

A command inherits NERV's environment, plus its `env` (which overrides an
inherited variable of the same name), plus these variables, always set
(empty when the value does not apply). Inherited `NERV_RELEASE_*` variables
are removed first.

| Variable | Value |
|---|---|
| `NERV_RELEASE_PROJECT` | Project name. |
| `NERV_RELEASE_PROJECT_PATH` | Project `path`, repository-relative with forward slashes (`.` for the root). |
| `NERV_RELEASE_REPO_ROOT` | Absolute repository root (under `nerv ship`, the release clone). |
| `NERV_RELEASE_STAGE` | The stage or hook key: `test`, `before_test`, `prepare`, ... |
| `NERV_RELEASE_VERSION` | Next version, e.g. `1.2.0-alpha.2`. |
| `NERV_RELEASE_CHANNEL` | Release channel (`alpha`, `rc`, ...); empty for the default channel. |
| `NERV_RELEASE_TAG` | Next tag, e.g. `api-v1.2.0`. |
| `NERV_RELEASE_GIT_HEAD` | Commit being released. |
| `NERV_RELEASE_LAST_VERSION` | Last release on the channel; empty when none. |
| `NERV_RELEASE_NOTES_FILE` | File holding the release notes (Markdown). |
| `NERV_RELEASE_BRANCH` | Branch being released. |
| `NERV_RELEASE_DRY_RUN` | `true` under `nerv ship --dry-run`, else `false`. |
| `NERV_RELEASE_ARTIFACTS_DIR` | Per-project directory for build outputs, outside the repository (see [Pipeline and presets](#pipeline-and-presets)). |

## Security notes

### Credentials

`nerv ship` runs with your own credentials; nothing is stored by NERV.

- git: your own credentials (ssh keys, a credential helper such as
  `gh auth setup-git`, recommended). The release clone's origin is your
  origin's address without user info; when your origin URL itself carries
  credentials (`https://user:token@host/...`), the real URL is written only
  into the release clone's own `.git/config` as `url.<real>.insteadOf`
  (an owner-only file in an owner-only temporary directory, removed at the
  end), never on a command line or in any environment. Git error output
  may still echo that URL; prefer a credential helper. Checking the push
  first also keeps semantic-release from falling back to a token embedded
  in the push URL.
- GitHub: `GH_TOKEN` or `GITHUB_TOKEN` from the environment; when neither
  is set, `gh auth token` is read and passed as `GH_TOKEN` to
  semantic-release's environment only (never printed, never given to the
  pipeline). Without a token a GitHub plugin fails its checks, before any
  tag.
- docker: your `docker login`. NuGet: the variable named by
  `nuget.api_key_env` ([NuGet API key](#nuget-api-key)). npm: `NPM_TOKEN`
  or your npm configuration.

`nerv version next` needs no `GH_TOKEN` or `NPM_TOKEN` (the credential
checks are skipped), only read access to origin.

### Reading origin

Every read of origin (the fetch of both commands, the push check, the tag
check) never prompts and is bounded:

- It runs with `GIT_TERMINAL_PROMPT=0`, `GCM_INTERACTIVE=never`, no
  standard input and ssh in batch mode (`-o BatchMode=yes` is appended to
  `GIT_SSH_COMMAND`, or to `core.sshCommand`, when that command is `ssh`; a
  non-ssh command such as `plink`, or a `GIT_SSH` program, is used as it is
  and must not prompt by itself). Credentials must therefore come from a
  credential helper, an ssh agent or an unencrypted key.
- It fails after 60 seconds (`--fetch-timeout`); a cancelled or timed-out
  command is killed with every process it started.
- The origin URL never appears on a command line, and credentials embedded
  in it (`https://user:token@host/...`, `?access_token=...`) are hidden in
  every error. A relative local origin path is resolved against the
  repository.
- Only origin is released: `nerv ship` never pushes your local commits,
  refuses a local branch that differs from origin's, and plans from the
  `release:` section committed on origin.

### NuGet API key

The NuGet API key is read from the variable named by `nuget.api_key_env`
(default `NUGET_API_KEY`) only when the push runs, and it never appears on
any command line, in step names, in the `--dry-run` listing (shown as
`$NUGET_API_KEY`), in the output or in errors (a feed that echoes it gets
it replaced by `***`). If the variable is not set or empty, the release
stops before the first stage, naming the step and the variable; a dry run
does not need it.

To keep the key off every command line, NERV pushes the packages itself
instead of running `dotnet nuget push --api-key`, which puts the key on
its argv (readable by other users of a shared machine through the process
list). The alternatives fail: a clear-text `<apikeys>` entry in a
temporary `NuGet.Config` is refused on Linux ("encryption is not supported
on non-Windows platforms": NuGet always decrypts it with Windows DPAPI), a
response file given to `dotnet nuget push` is expanded and forwarded on the
argv of the child `dotnet exec NuGet.CommandLine.XPlat.dll` process,
`NuGet.CommandLine.XPlat.dll` reads no response files, and NuGet reads no
API key from the environment (observed with the .NET SDK 10.0.112, NuGet
7.0.3).

The push follows [NuGet's push protocol](https://learn.microsoft.com/nuget/api/package-publish-resource),
as the NuGet client sends it:

- `source` ending in `index.json` is a v3 service index: NERV reads it
  (without the key) and pushes to its `PackagePublish` resource, and each
  `<package>.snupkg` next to a package to its `SymbolPackagePublish`
  resource when the index has one. Any other `source` is a push URL (a v2
  feed); symbol packages are then not pushed.
- Each package is a `PUT` to the push URL (with a trailing `/`) of a
  `multipart/form-data` body whose one part is the package, with the key in
  the `X-NuGet-ApiKey` header. A `409 Conflict` means that version is
  already on the feed and is skipped, like `--skip-duplicate`.
- Each request (the service index read, each package's upload) is bounded
  by 10 minutes; a feed that hangs fails the push.
- Redirects are not followed (the key would go with them), and a plain
  `http://` source is refused unless its host is loopback.

## Troubleshooting and recovery

### Refusals and environment errors

Nothing has been tagged or published in any of these cases.

| Message (abridged) | Cause | What to do |
|---|---|---|
| `your local <b> (...) is not origin/<b> (...)` | `nerv ship` releases what is on origin. | `git push` or `git pull`, then run again. |
| `branch <b> is not on origin` / `has no origin remote` | Nothing on origin to release. | Push the branch (or add the remote). |
| `the computed release is not the expected one (<project>: expected X, computed Y; ...)` | `--expect` no longer matches: origin moved since the dry run. | Run the dry run again, review the new plan, then release with its versions. |
| `confirmation needed but stdin is not an interactive terminal` | No terminal for `[y/N]`. | `--yes` (after a `--dry-run`), or run it in a terminal. |
| `invalid release configuration: ...` | The `release:` section (every problem listed). | Fix each line; see [Validation](#validation). |
| `unknown project "x"; the release: section declares: ...` | `--project` typo. | Use one of the listed names. |
| origin unreachable or timed out | Network, credentials or a prompt blocked by the no-prompt rules. | Fix access ([Reading origin](#reading-origin)); `nerv version next --offline` works without origin. |
| `git X.Y.Z is too old: selecting a project's commits by path needs git 2.31.0 or newer` | Path filters need `--diff-merges`. | Upgrade git. |
| `Node.js ... is too old` / node or npm not found | semantic-release needs Node.js 20.8.1+. | Install or upgrade Node.js. |
| a package not pinned in `release.semantic_release.extra_plugins` | The semantic-release configuration names a package NERV does not install. | Add it as `name@version`. |
| `<program> not found on PATH` / missing interpreter | A preset tool or a hook shell is missing. | Install it (Git for Windows, PowerShell 7, Node.js, ...). |
| an inline pwsh command exits `3` | PowerShell older than 7.3. | Upgrade PowerShell. |
| the NuGet key variable is not set or empty | `nuget.api_key_env`. | Export it; a `--dry-run` does not need it. |

### Failures during a release

On a failure the summary (and the `--json` `action`) says what happened and
what to do:

| Failure | State | What to do |
|---|---|---|
| Project file, pipeline or `prepare` hook | Nothing tagged or published for this project. | Fix it and run `nerv ship` again (after `git pull` when origin moved, see below). |
| semantic-release, no tag on origin | Nothing tagged (a prepare plugin such as `@semantic-release/git` may have pushed a release commit). | Fix it, pull if origin moved, run again. |
| Another version than the confirmed one, stopped by the guard | Stopped before the tag. | Run again (pull if origin moved) to review the new plan. |
| semantic-release released another version anyway | That version's tag is on origin; NERV's publish stage did not run (it was built for the confirmed version). | Publish by hand from that tag rebuilt with its version, or delete the tag and release if wrong; pull; do not rerun. |
| semantic-release after the tag (e.g. the GitHub release) | Tag on origin; NERV's publish stage not run. | Finish semantic-release's remaining steps and publish by hand from the tag. |
| Publish stage or `publish` hook | Tag and release on origin; the summary lists what was published and what was not. | Publish the rest by hand from the tag. |
| `success` hook | Released and published. | Run what the hook does by hand if needed. |

- The advice to run again checks origin's branch at the end of the run:
  when it moved (an earlier project's release commit, or a commit a prepare
  plugin pushed) your local branch is now behind origin, which `nerv ship`
  refuses, so the advice says to `git pull` first. When the branch was
  deleted on origin, the advice says so instead; when origin cannot be read
  (within 10 seconds, also after Ctrl-C), it says to check with `git
  fetch`.
- Once semantic-release has started, a failure runs the project's `fail`
  hooks (not after Ctrl-C). Do not run `nerv ship` again for a release
  whose tag is on origin: it would compute the next version.
- A NuGet push stops at the first package the feed refuses, and a push
  cannot be undone, so the error names what the feed already holds, e.g.
  `pushed: Sdk.1.4.0.nupkg, Sdk.1.4.0.snupkg; already on the feed:
  Cli.1.4.0.nupkg; failed on Web.1.4.0.nupkg: HTTP 500 ...; not attempted:
  Zip.1.4.0.nupkg` (`nothing pushed; failed on ...` when the first one
  fails). Run the release's push again after fixing the cause: packages
  already there are skipped.
- The release clone and everything built in it are removed at the end, on
  success or failure; to publish by hand, check the tag out (`git checkout
  <tag>`) and rebuild.

## Known limitations

- **Windows is compile-checked only**: the whole-tree kill of a cancelled
  command (`taskkill /T`), the Git Bash lookup and pwsh/node script-file
  runs are unit-tested with a fake OS, never on a real Windows machine. On
  Windows, a descendant process that outlives its command keeps running.
- **No submodules or Git LFS**: the release clone initialises neither, so
  a build that needs them fails (or uses LFS pointer files).
- **NuGet feeds whose service index needs credentials** (GitHub Packages,
  Azure Artifacts) are not supported by the `nuget` preset: use a `publish`
  stage override for them.
- An origin whose `HEAD` names a missing branch fails semantic-release's
  own fetch.
- `--project` applies the dependency bumps of dependencies it does not
  release, in both commands.
- semantic-release's `addChannel` step (moving earlier releases to a
  channel) runs before the version guard.
- Offline (`nerv version next --offline`), a prerelease branch may compute
  another version than CI when a channel note exists only on origin.
- Coverage is checked on line coverage only (no branch threshold).
- Commands run in their own session or process group without a
  controlling terminal, so a terminal Ctrl-C reaches them only through
  NERV, which cancels them on the first signal.
