# Release commands: `nerv version next` and `nerv ship`

Two commands for versioning and packaging your own repositories (any
language, single repository or monorepo) from your machine, with the
repository's own semantic-release configuration. Use them where the CI
build cannot or should not run; they compute and build exactly what CI
would.

| Command | What it does | Changes anything? |
|---|---|---|
| `nerv version next` (`/nerv:version`) | The next semantic version of the repository, or of each project of a monorepo, from a semantic-release dry run, with the outputs of `cycjimmy/semantic-release-action`. | No. |
| `nerv ship` (`/nerv:ship`) | Builds, tests and packages what is on origin, in a throwaway clone, in each project's build mode (`compose` or `script`), per project in dependency order, and leaves a local artifact (`.nerv/artifacts/<project>/<version>/`) for `nerv deploy`. | Only `.nerv/artifacts/` in your repository; never git (no commit, tag or push) and never a registry. |

Both read the optional `release:` section of the project-scope
`<repo>/.nerv/nerv.yaml`. Without it a repository is one project and is
computed exactly like plain semantic-release; `nerv ship` needs the
section, with a `build.mode` for each project.

## Contents

- [Requirements](#requirements)
- [Quick start](#quick-start)
- [Configuration (`release:`)](#configuration-release)
- [`nerv version next`](#nerv-version-next)
- [`nerv ship`](#nerv-ship)
- [Build modes](#build-modes)
- [The artifact](#the-artifact)
- [Stages, hooks and commands](#stages-hooks-and-commands)
- [Security notes](#security-notes)
- [Troubleshooting and recovery](#troubleshooting-and-recovery)
- [Known limitations](#known-limitations)

## Requirements

| Tool | Needed for |
|---|---|
| git | Always. 2.31 or newer when a project's `path` is not `.` (path filters); an older git stops before computing anything, naming the git found and the minimum. |
| Node.js 20.8.1 or newer, and npm | Computing versions: semantic-release and its plugins are installed in a NERV cache, never in the repository ([Installed plugins](#installed-plugins)). `nerv ship --version <semver>` does not compute. |
| bash, pwsh (PowerShell 7) or node | `nerv ship`: the stage commands and hooks that use them. Inline pwsh commands need PowerShell 7.3 or newer. On Windows, bash is Git Bash. |
| docker with the compose plugin | `nerv ship` for a project in `compose` mode. |
| Your build tools (.NET SDK, npm, ...) | `nerv ship` for a project in `script` mode: whatever its stage commands run. |

Read access to origin is needed for both commands (`nerv version next
--offline` skips it). `nerv ship` needs no push access and no registry
credentials: it never pushes or publishes.

## Quick start

### Interactive setup

In Claude Code, `/nerv:release-setup` writes the `release:` section for
you:

1. It detects what the repository already has: its semantic-release
   configuration (branches, plugins that need pinning), compose files,
   `.csproj` / `.sln` / `.slnx` files, Dockerfiles, `package.json`
   scripts, CI build steps, release scripts, and a monorepo layout with its
   commit scopes.
2. It proposes a complete section with a `build.mode` for each project
   (`compose` when a compose file builds it, otherwise `script` with the
   stage commands it found), explains each non-obvious choice, and asks
   only what it cannot detect, one question at a time (the build mode when
   both fit, the compose test service, a coverage floor, `depends_on`).
3. It writes the section into `.nerv/nerv.yaml` (the rest of the file is
   kept as it is) and validates it with `nerv version next --offline`,
   showing each project's next version.

It never commits, pushes or releases. `nerv version next` and `nerv ship`
read your local `.nerv/nerv.yaml` as it is, committed or not (commit it for
your team's sake; it is not required). A semantic-release configuration it
creates must be committed and pushed: the versions are computed from
origin's. Then run `/nerv:ship`. `/nerv:release-setup --project <name>`
adds or edits one project of an existing section. The examples below show
the shapes it produces.

### Single repository: dotnet API in script mode

A repository shaped like `dim.comprobantes.api`: semantic-release
configured in its `package.json` `release` (branches main, `release/rc`
with channel `rc`, develop with channel `alpha`), the API's `.csproj`
receives the version, the tests collect coverage with a line floor, and
the package stage leaves the NuGet package and the image archive in the
outputs. Add to `.nerv/nerv.yaml`:

```yaml
release:
  projects:
    - name: api
      path: .
      build:
        mode: script
        version_files: [src/Comprobantes.Api/Comprobantes.Api.csproj]
        coverage: { line: 80 }
      stages:
        restore: { run: dotnet restore Comprobantes.sln }
        lint: { run: dotnet format Comprobantes.sln --verify-no-changes }
        build: { run: dotnet build Comprobantes.sln -c Release --no-restore }
        test: { run: 'dotnet test Comprobantes.sln -c Release --no-build --collect "XPlat Code Coverage" --results-directory "$NERV_RELEASE_COVERAGE_DIR"' }
        package:
          run: |
            dotnet pack src/Comprobantes.Api/Comprobantes.Api.csproj -c Release --no-build -o "$NERV_RELEASE_OUTPUTS_DIR"
            docker build -t ghcr.io/org/comprobantes-api:$NERV_RELEASE_VERSION .
            docker save -o "$NERV_RELEASE_OUTPUTS_DIR/comprobantes-api.tar" ghcr.io/org/comprobantes-api:$NERV_RELEASE_VERSION
      hooks:
        after_test:
          - run: scripts/report-coverage.ps1
            shell: pwsh
```

With one project the tag is `v1.4.0` and the CHANGELOG is `CHANGELOG.md`.
Then:

```sh
nerv version next --branch develop    # what would be released (read-only)
nerv ship --dry-run                   # restore, lint, build, test, coverage, package in a clone
nerv ship                             # the same, then the artifact, after a [y/N] confirmation
```

In Claude Code, `/nerv:version` and `/nerv:ship` do the same; `/nerv:ship`
always rehearses with `--dry-run` first and packages only after your
explicit yes.

### Single repository: compose mode

When a compose file already builds the images and runs the tests:

```yaml
release:
  projects:
    - name: api
      path: .
      build:
        mode: compose
        file: docker-compose.yml
        test_service: tests
        images: ["ghcr.io/org/api:${version}"]
```

`nerv ship` runs `docker compose -f docker-compose.yml build`, then
`docker compose -f docker-compose.yml run --rm tests` (its exit code is the
test result), then `docker save` of each image into the artifact.

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
      build:
        mode: script
        version_files: [src/core/Core.csproj]
      stages:
        build: { run: dotnet build src/core -c Release }
        package: { run: 'dotnet pack src/core -c Release --no-build -o "$NERV_RELEASE_OUTPUTS_DIR"' }
    - name: api
      path: src/api
      shared_paths: [Directory.Build.props]
      depends_on:
        - { project: core, bump: patch }
      build:
        mode: compose
        file: src/api/docker-compose.yml
        test_service: api-tests
        images: ["ghcr.io/org/api:${version}"]
    - name: web
      path: web
      scopes: [web, ui]
      build: { mode: script }
      stages:
        restore: { run: npm ci, workdir: web }
        test: { run: npm test, workdir: web }
```

`nerv version next` prints one block per project; `nerv ship --project api`
packages only `api` (its dependencies are computed for `depends_on`, not
packaged).

## Configuration (`release:`)

The section is project scope only: it is never read from the user-scope
`~/.claude/nerv/nerv.yaml` and never merged with it. When it is absent, the
repository simply has no release configuration.

The versioning rules themselves (branches and channels, `releaseRules`,
presets, plugins) stay in the repository's own semantic-release
configuration (`.releaserc*`, `release.config.*` or `package.json`
`release`). `release:` adds what semantic-release does not know: which
projects a monorepo holds, how they depend on each other, and how each one
is built, tested and packaged.

Both commands read the `release:` section from your local
`.nerv/nerv.yaml` (the working-tree file, committed or not), never origin's
copy. For `nerv ship` that is the only thing taken from your machine: the
code, the semantic-release configuration and the branch content are
origin's, in the release clone, whose own `.nerv/nerv.yaml` is ignored. The
paths the section names are resolved inside the release clone; a path that
is not on origin's branch (a project folder that exists only locally) is
refused before any stage.

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

### Built-in semantic-release configuration

A repository with a semantic-release configuration of its own
(`package.json` `release`, `.releaserc`, `.releaserc.{json,yaml,yml,js,cjs,mjs,ts}`,
`.config/releaserc*`, `release.config.{js,cjs,mjs,ts}`, found in the
repository root exactly as semantic-release finds it) keeps it, unchanged.
A repository with **none** uses NERV's built-in configuration,
dim.comprobantes.api's:

- branches: `main`; `develop` (prerelease `alpha`, channel `alpha`);
  `release/*` (prerelease `rc`, channel `beta`, widening dim's single
  `release/rc`); `hotfix/*` (prerelease `beta`, channel `beta`). A
  `pipeline:` section's `branches` replace these
  ([pipeline.md](pipeline.md#pipelinebranches));
- `@semantic-release/commit-analyzer` and
  `@semantic-release/release-notes-generator` with the `conventionalcommits`
  preset and note keywords `BREAKING CHANGE`, `BREAKING CHANGES`,
  `BREAKING`;
- `@semantic-release/changelog` (`CHANGELOG.md` at the root, or the
  project's own changelog, see [The project's changelog](#the-projects-changelog));
- `@semantic-release/git` committing the changelog (its default release
  commit message, as dim);
- `@semantic-release/github` (the GitHub release), only when origin is on
  GitHub (`github.com`, or the GitHub Enterprise host in `GH_HOST`):
  elsewhere it would fail its checks (`EINVALIDGITHUBURL`). These plugins
  only matter to `nerv pipeline release`'s real run: `nerv ship` only
  computes with them (a dry run needs no token).

dim's `@semantic-release/npm` (`npmPublish: false`) and
`@semantic-release/exec` (`bump-csproj.sh`, `set-release-outputs.sh`) are
left out: `nerv ship` writes `build.version_files` and NERV prints the
release outputs itself. Every plugin it needs is bundled or pinned, so no
`extra_plugins` are required.

`nerv version next` (text: `semantic-release config: repository (<file>)`
or `built-in default (dim)`; JSON: `semantic_release_config`
`repository` / `default` and `semantic_release_config_file`) and `nerv ship`
(stderr line; `--json` document field `semantic_release_config`) say which
one was used.

A prerelease glob with a fixed id (`release/*` with `rc`) serves one
matching branch at a time: semantic-release 24 rejects two branches with
the same prerelease id (`EPRERELEASEBRANCHES`, verified with two
`release/x` branches on origin). With the built-in configuration NERV
refuses that case first, naming the branches: keep one release branch at a
time (merge or delete the others), or give each its own prerelease id in
the `pipeline:` section's `branches`. The check reads origin's branches as
`nerv ship` last fetched them.

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
| `changelog` | `<path>/CHANGELOG.md` | The project's CHANGELOG file (repository-relative); see [The project's changelog](#the-projects-changelog). |
| `depends_on` | `[]` | Projects whose new release bumps this one; see below. |
| `build` | required by `nerv ship` | How the project is built, tested and packaged: its `mode` (`compose` or `script`, no default) and settings; see [`build`](#build). `nerv version next` ignores it. |
| `stages` | `{}` | The stage commands; see [Stages, hooks and commands](#stages-hooks-and-commands). |
| `hooks` | `{}` | Extra scripts around stages and at the ship lifecycle steps; same section. |
| `submodules` | `false` | `true` runs `git submodule update --init --recursive` at the very start of `restore` (the stage is created if the project has no restore command). |

### The project's changelog

For `nerv version next` and the version `nerv ship` computes, `changelog`
applies when the section has **more than one project** (each project
writes its own CHANGELOG, `<path>/CHANGELOG.md` by default) or when it is
**set explicitly** for the project. A single project without it keeps the
repository's own semantic-release configuration unchanged (for example a
`changelogFile: src/Api/CHANGELOG.md` stays as it is).

When it applies, each computation of the project uses the repository's
plugin list with:

- every `@semantic-release/changelog` entry writing `changelogFile: <changelog>`;
- every `@semantic-release/git` entry committing `<changelog>` instead of
  the file the changelog entries pointed at (only that exact path is
  dropped; every other asset is kept; when `assets` is not set, the git
  plugin's defaults `CHANGELOG.md`, `package.json`, `package-lock.json`,
  `npm-shrinkwrap.json` are the starting list; `assets: false` is left
  alone);
- every other plugin untouched.

`nerv ship` itself writes the release's CHANGELOG section at the top of
`changelog` (under its `# ` title when it has one) in the release clone
only, and stores the new file in the artifact for `nerv deploy`'s release
commit.

### `depends_on[]`

| Field | Default | Meaning |
|---|---|---|
| `project` | required | Another project of the section (not itself). |
| `bump` | `patch` | What a new release of that project does to this one: `patch`, `minor` or `none` (no bump, but still released after it). |

Dependencies must not form a cycle; the error names it
(`dependency cycle a -> b -> a`). Projects are computed and packaged in
dependency order, otherwise in declaration order.

### `build`

`mode` is required by `nerv ship` and has no default. The other keys depend
on it:

| Field | Modes | Default | Meaning |
|---|---|---|---|
| `mode` | both | required | `compose` or `script` ([Build modes](#build-modes)). |
| `file` | compose | required | The compose file, repository-relative: `docker compose -f <file>`. |
| `test_service` | compose | none (no test stage) | The compose service `docker compose -f <file> run --rm <test_service>` runs; its exit code is the test result. |
| `images` | compose | `[]` | Image references `docker save` packages into the artifact, one archive each; they may use `${version}`, `${channel}` (empty on the default channel) and `${tag}`. They must be the images the compose file builds. |
| `shared_host` | script | `false` | `true` gives the `restore`, `lint`, `build`, `test` and `package` commands the build-server-safe dotnet environment (`MSBUILDDISABLENODEREUSE=1`, `UseSharedCompilation=false`, `DOTNET_CLI_USE_MSBUILD_SERVER=0`) under their own `env`, for shared hosts where reused build servers crash dotnet. A command's own `env` wins key by key; its `run:` is never rewritten. |
| `version_files` | both | `[]` | MSBuild project files (`.csproj`, `.fsproj`, `.vbproj`, `.props`, `.targets`) that receive `<Version>` and `<PackageReleaseNotes>` in the clone before the pipeline ([Version files](#version-files)). |
| `coverage` | both | none | Coverage floors checked after the `coverage` stage over every Cobertura report (`*.cobertura.xml`) under `coverage.dir` ([Coverage](#coverage)). |
| `coverage.dir` | both | `$NERV_RELEASE_ARTIFACTS_DIR/coverage` | Directory searched for the reports, repository-relative (a compose test service writes them through a bind mount). |
| `coverage.line`, `coverage.branch`, `coverage.method` | both | `0` (not checked) | Minimum percentages, 0-100; at least one must be set. |

#### Migrating from the presets

The `dotnet`, `node`, `docker`, `nuget` and `npm` presets were removed.
A section that still has `presets:` or one of their settings blocks is
refused, each key with what to write instead:

| Old key | Write instead |
|---|---|
| `presets` | `build: { mode: script }` with each stage's command under `stages`, or `build: { mode: compose, file: ..., test_service: ..., images: [...] }`. |
| `dotnet` | Its commands as `stages` (`dotnet restore`, `dotnet format --verify-no-changes`, `dotnet build -c Release --no-restore`, `dotnet test -c Release --no-build`); `project_file` to `build.version_files`, `coverage_threshold` to `build.coverage`, `shared_host` to `build.shared_host`; for `sdk`, a `before_restore` hook writing `global.json`. |
| `node` | Its commands as `stages` (`npm ci`, `npm run lint`, `npm run build`, `npm test`). |
| `docker` | `build.mode: compose`, or a script `package` stage running `docker build` and `docker save -o "$NERV_RELEASE_OUTPUTS_DIR/<name>.tar"`. Pushing moved to `nerv deploy`. |
| `nuget`, `npm` | A `package` stage writing the package to `$NERV_RELEASE_OUTPUTS_DIR` (`dotnet pack -o`, `npm pack --pack-destination`). Publishing moved to `nerv deploy`. |
| `stages.publish`, `hooks.publish`, `hooks.success` | Removed from `nerv ship`: publishing, and what runs after it, belong to `nerv deploy`. |

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
such as `scopes: api`) stops at the decoding problems. Both commands
refuse an invalid section (exit 1) before reading origin; `nerv ship` also
refuses a project without `build.mode`.

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
      build: { mode: script, version_files: [api/Api.csproj] }
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
          [--version <semver>] [--dry-run] [--force] [--yes]
          [--fetch-timeout <dur>] [--json] [--expect <project>=<version> ...]
```

Builds, tests and packages **what is on origin**, in a throwaway clone, as
the CI build would, and leaves a local artifact per project for `nerv
deploy`. It never writes to git: no release commit, no tag, no push (and no
registry push or publish); an artifact that is never deployed leaves no
trace in the repository. In Claude Code, `/nerv:ship [--branch <name>]
[--project <name>] [--version <semver>]` always runs `nerv ship --dry-run
--json` first, shows the plan, and runs `nerv ship --yes --json --expect
<project>=<version> ...` (the dry run's versions) only after your explicit
yes, so the versions you approved are the only ones that can be packaged.

| Flag | Default | Meaning |
|---|---|---|
| `--cwd <dir>` | `.` | Any directory inside the repository. |
| `--branch <name>` | current branch | Branch to build; your local branch must equal origin's. |
| `--project <name>` | all projects | Only this project of the `release:` section. |
| `--version <semver>` | computed | Use this version instead of computing it (no semantic-release run). With several projects `--project` is required. It must be a semantic version newer than every tag of the project on origin (its `tag_format`); the tag follows `tag_format`, the channel is the prerelease's first identifier (`rc` for `2.0.0-rc.1`, empty for a stable version), and the CHANGELOG section is a `## <version> (<date>)` heading. A malformed version is a usage error (exit 2). |
| `--dry-run` | off | Run the pipeline and the prepare hooks in the clone; write no artifact. |
| `--force` | off | Replace an existing artifact of the same version (otherwise refused). |
| `--yes` | off | Package without asking (required without a terminal). |
| `--fetch-timeout <dur>` | `60s` | Bound for each read of origin; must be positive. |
| `--json` | off | Final summary as JSON. |
| `--expect <project>=<version>` | none | Repeatable. Package only when the releases are exactly these: every listed project releases that version and no other project releases. Otherwise the run is refused (exit 1, `--json` status `refused`, expected vs computed per project) before any stage runs. Use the names and versions a `--dry-run --json` printed. A malformed value or a project given twice is a usage error (exit 2). It works with `--version` too. |

### Flow

1. Resolve the repository and the branch (default: the current one), load
   the `release:` section from your local `.nerv/nerv.yaml` (committed or
   not) and refuse, before anything else, a repository without the
   section, an invalid section, a project without `build.mode`, an unknown
   `--project`, or `--version` with several projects and no `--project`.
   This section is used for the whole run; the clone's copy is ignored.
2. Create the **release clone**: a fresh `git clone --shared` of your
   repository in a temporary directory (never a worktree of it), whose
   `origin` is your origin's address; origin's branches, tags and
   semantic-release notes are fetched into it and the branch is checked out
   at origin's commit. Local-only branches and tags never reach it. Your
   repository is never modified.
3. Refuse (exit 1) when your local branch is not exactly origin's (ahead:
   push first; behind: pull first): `nerv ship` builds what origin has and
   never pushes your commits. A branch that is not on origin is refused
   too. Refuse also, before any version is computed, when a path the
   section names (project `path`, a `shared_paths` entry without glob
   characters, `build.file`, a `build.version_files` entry, a stage or hook
   `workdir`) does not exist on origin's branch.
4. Get every project's version: computed in the clone (as `nerv version
   next`, from the refs just read), or `--version`. Projects without a new
   release are reported and skipped. With `--expect`, refuse here unless
   the releasing projects and versions are exactly the expected ones. A
   real run refuses an artifact that already exists for a releasing
   project's version, unless `--force`.
5. Print each releasing project's version, channel, tag, last release, its
   plan ([Build modes](#build-modes)) and its artifact directory, then ask
   `Package <n> project(s)? [y/N]` on an interactive terminal. `--yes`
   skips the question; without a terminal `--yes` is required (exit 1
   otherwise); `--dry-run` never asks.
6. For each releasing project, in dependency order (`depends_on`):
   1. write, in the clone only, `<Version>` and `<PackageReleaseNotes>`
      into `build.version_files` ([Version files](#version-files)) and the
      CHANGELOG section at the top of the project's `changelog`, before the
      pipeline, so the packages carry them;
   2. run the pipeline (`restore` to `package`) and the `prepare` hooks;
   3. write the artifact ([The artifact](#the-artifact)) from what the
      package stage left in `$NERV_RELEASE_OUTPUTS_DIR`, and print its
      path.
7. Remove the clone and print the summary on stdout. The first failure
   runs the project's `fail` hooks (not after Ctrl-C) and stops the run;
   later projects are reported as not started.

`--dry-run` runs 6.1 and 6.2 for real in the clone (build, test, package,
prepare hooks) and writes no artifact. `NERV_RELEASE_DRY_RUN` is `true`.

With `--project`, only that project is packaged (its dependencies are
computed for `depends_on`, as in `nerv version next`, but not packaged).

In stages and hooks, `NERV_RELEASE_REPO_ROOT` is the release clone, and
`NERV_RELEASE_GIT_HEAD` is the origin commit being built.

### Summary

stdout carries the summary: one line per project (`packaged in <artifact
directory>`; `no new release`; `not started`; `dry run`; `failed` with the
error and what to do). With `--json`, every run that gets past the flags
prints exactly one JSON document, early refusals and failures included:

```json
{"status":"packaged","semantic_release_config":"repository","projects":[
  {"project":"core","branch":"main","status":"packaged","version":"1.1.0","tag":"core-v1.1.0",
   "last_version":"1.0.0","artifact":"/repo/.nerv/artifacts/core/1.1.0"}]}
```

- `status`: `packaged`, `dry-run`, `nothing-to-release`, `refused` (exit
  1), `declined` (answered no; exit 1), `failed` (exit 2) or `interrupted`
  (128 + signal).
- `error`: the reason, when the run did not succeed.
- `semantic_release_config`: where the computing configuration came from
  (`repository` or `default`); absent with `--version`.
- `projects`: one object per project (an empty array when the run stopped
  before getting the versions), with `project`, `branch`, `status`
  (`packaged`, `dry-run`, `no-release`, `failed`, `not-started`),
  `version`, `channel`, `tag`, `last_version`, `artifact` (the artifact
  directory, when packaged), `error` and `action` (what to do after a
  failure; see [Troubleshooting and recovery](#troubleshooting-and-recovery)).

### Exit codes

| Code | When |
|---|---|
| `0` | Packaged, nothing to release, or a dry run that passed. |
| `1` | Refused: no or an invalid `release:` section, a project without `build.mode`, unknown `--project`, `--version` not newer than the last tag or without `--project` in a monorepo, not a git repository or not on a branch, no origin remote, local branch not equal to origin's, an existing artifact without `--force`, no confirmation (or not a terminal without `--yes`), a plan that cannot be built. |
| `2` | Usage error, an environment error (origin unreachable, node or npm missing), or a failed step. |
| `128 + signal` | Interrupted (`130` Ctrl-C, `143` SIGTERM, `129` SIGHUP); a second Ctrl-C ends `nerv` at once. |

## Build modes

`nerv ship` plans each project's pipeline from its `build.mode`, `stages`
and `hooks` (`internal/shipplan`), for the release being made (version,
channel, tag):

| Stage | `script` | `compose` |
|---|---|---|
| `restore` | `stages.restore` | `stages.restore` |
| `lint` | `stages.lint` | `stages.lint` |
| `build` | `stages.build` | `docker compose -f <file> build` |
| `test` | `stages.test` | `docker compose -f <file> run --rm <test_service>` (none without `test_service`) |
| `coverage` | `stages.coverage`, then the floor check | `stages.coverage`, then the floor check |
| `package` | `stages.package` | one `docker save -o $NERV_RELEASE_OUTPUTS_DIR/<image>.tar <image>` per `build.images` entry |

1. A stage with no command is skipped together with its `before_<stage>`
   and `after_<stage>` hooks; otherwise its `before_<stage>` hooks run
   first and its `after_<stage>` hooks last. `submodules: true` adds
   `git submodule update --init --recursive` at the start of `restore`; a
   `build.coverage` floor adds the `coverage:check` step at the end of
   `coverage`.
2. In compose mode the project may write only `stages.restore`,
   `stages.lint` and `stages.coverage`: a `build`, `test` or `package`
   command is refused (compose runs them).
3. The first failing step stops the project, naming the stage and step.
4. The plan is printed on stderr before the confirmation, every step per
   stage:

   ```text
   api 1.4.0 (default channel, tag v1.4.0, build mode compose)
     build
       compose:build  docker compose -f docker-compose.yml build
     test
       compose:test  docker compose -f docker-compose.yml run --rm tests
     package
       compose:save ghcr.io/org/api:1.4.0  docker save -o /tmp/.../outputs/ghcr.io_org_api_1.4.0.tar ghcr.io/org/api:1.4.0
   ```

Compose mode's commands run **without a shell**: each is an argument list
given to `docker` (or `git` for the submodules), found on `PATH`, so paths
with spaces need no quoting on any OS; a missing program fails the step
with `<program> not found on PATH`. An image reference, once its
placeholders are expanded, must hold only `A-Z a-z 0-9 . _ / : @ -`.
Stage commands and hooks run through their `shell` as described in
[How a command runs](#how-a-command-runs).

Every step also receives `NERV_RELEASE_ARTIFACTS_DIR`, a per-project
directory outside the repository for build outputs, and
`NERV_RELEASE_OUTPUTS_DIR` (`$NERV_RELEASE_ARTIFACTS_DIR/outputs`), where
the package stage writes what goes into the artifact; with `build.coverage`
also `NERV_RELEASE_COVERAGE_DIR`, the directory the floor check searches.
The outputs directory, and the default coverage directory, are emptied
before the first stage of every run, so a report or package left by an
earlier run is never counted or packaged.

### Coverage

With `build.coverage`, the `coverage:check` step reads every Cobertura
report (a file named `*.cobertura.xml`, such as coverlet's
`coverage.cobertura.xml`) under the coverage directory, merges them by
summing their counts (several test projects give several reports; their
rates are not averaged) and fails the stage when a floor is missed:

- `line`: lines covered over lines valid;
- `branch`: branches covered over branches valid (a report without branch
  counts is an error when a branch floor is set);
- `method`: the reports' `<method>` elements, a method being covered when
  one of its lines has a hit (coverlet's method coverage).

It also fails when there is no report, a report cannot be read, or a line
floor finds no coverable line. As in coverlet, a branch or method metric
with nothing to cover counts as 100%, and a line covered in two test
projects' reports counts in both. The summary line reads e.g. `line
coverage 82.31% (823 of 1000 lines, 2 reports), threshold 80%`, truncated,
never rounded up. The test stage must write its reports to the coverage
directory (`$NERV_RELEASE_COVERAGE_DIR`), e.g. `dotnet test --collect "XPlat
Code Coverage" --results-directory "$NERV_RELEASE_COVERAGE_DIR"` (the test
projects must reference `coverlet.collector`).

### Version files

Before the pipeline, `nerv ship` writes the release into each
`build.version_files` entry in the release clone, so the packages carry it,
and stores the new file in the artifact for `nerv deploy`'s release commit
(what `dim.comprobantes.api`'s `scripts/bump-csproj.sh` does with sed).
`<PackageReleaseNotes>` is the link to the release,
`https://<host>/<owner>/<repo>/releases/tag/<tag>`, from origin's address
(ssh addresses become https); without a public origin (a local path) only
`<Version>` is written, with a warning.

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
  (a symlinked file updates its target and keeps the link).
- The notes are XML-escaped (`&`, `<`, `>`) and MSBuild-escaped (`%` as
  `%25`, `$` as `%24`, `@` as `%40`), so `$(Foo)` in a commit message is
  not expanded; `dotnet pack` restores them in the package's
  `releaseNotes`.

## The artifact

A real run writes, for each packaged project,
`.nerv/artifacts/<project>/<version>/` in your repository's root (not the
clone), which outlives the clone. `.nerv/artifacts/` gets a `.gitignore` of
`*` the first time, so your own `.gitignore` is never changed and the
artifacts never show in `git status`. A directory that resolves outside the
repository through a symlink is refused. The artifact is assembled in a
temporary sibling directory and renamed into place, so a failed run leaves
no partial artifact.

| Entry | Content |
|---|---|
| `manifest.json` | Schema `nerv.ship-artifact/v1`: `project`, `version`, `channel`, `tag`, `base_commit` (the origin commit built), `branch`, `build_mode`, `created_at`, `outputs` (`path`, `kind` `image-tar`, `file` or `dir`, `sha256`), `images` (the saved image references) and `release_files` (`path` in the repository, `stored` in the artifact, `sha256`). |
| `notes.md` | The release notes. |
| `CHANGELOG.section.md` | The CHANGELOG section of the release. |
| `outputs/` | What the package stage left in `$NERV_RELEASE_OUTPUTS_DIR`: each saved image's archive (`image-tar`), and every other entry as a `file` or a `dir`. A directory's `sha256` is the sha256 of its `sha256sum`-style listing (`<sha256>  <path>` per file, sorted by path). |
| `release-files/` | The new contents of the files the release changes (the version files and the CHANGELOG), at their repository paths, so `nerv deploy` can make the release commit without rebuilding. |

The same version again is refused unless `--force`, which replaces the
whole artifact. `internal/artifact` reads an artifact and verifies every
`sha256` for `nerv deploy`.

## Stages, hooks and commands

### `stages`

Keys are pipeline stages, run in this order: `restore`, `lint`, `build`,
`test`, `coverage`, `package`. In `script` mode each key is the stage's
command, and a stage without one is skipped; in `compose` mode only
`restore`, `lint` and `coverage` may be written (compose runs `build`,
`test` and `package`). Any failing stage stops the project before an
artifact is written. The `package` stage writes what goes into the artifact
to `$NERV_RELEASE_OUTPUTS_DIR`. There is no `publish` stage: publishing
belongs to `nerv deploy`.

```yaml
stages:
  test: { run: "dotnet test --no-build", shell: bash }
```

### `hooks`

Keys are `before_<stage>` / `after_<stage>` (for any stage above) and the
lifecycle keys `prepare` (after the pipeline, before the artifact is
written; also in a dry run) and `fail` (when a stage, a hook or the
artifact write fails; not after Ctrl-C). Each key holds a list of commands
run in order. `before_<stage>` / `after_<stage>` run only when the stage
has commands: a stage without any is skipped with its hooks. The `publish`
and `success` hooks were removed: they belong to `nerv deploy`.

### Command fields

Stage commands and hooks are commands:

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
| `NERV_RELEASE_GIT_HEAD` | The origin commit being built. |
| `NERV_RELEASE_LAST_VERSION` | Last release on the channel; empty when none. |
| `NERV_RELEASE_NOTES_FILE` | File holding the release notes (Markdown). |
| `NERV_RELEASE_BRANCH` | Branch being built. |
| `NERV_RELEASE_DRY_RUN` | `true` under `nerv ship --dry-run`, else `false`. |
| `NERV_RELEASE_ARTIFACTS_DIR` | Per-project directory for build outputs, outside the repository (see [Build modes](#build-modes)). |
| `NERV_RELEASE_OUTPUTS_DIR` | `$NERV_RELEASE_ARTIFACTS_DIR/outputs`, emptied before the run: what the package stage writes there becomes the artifact's `outputs/`. |
| `NERV_RELEASE_COVERAGE_DIR` | With `build.coverage`, the directory the coverage check searches for `*.cobertura.xml` reports (`build.coverage.dir` in the clone, or `$NERV_RELEASE_ARTIFACTS_DIR/coverage`); not set otherwise. |

## Security notes

### Credentials

`nerv ship` runs with your own credentials; nothing is stored by NERV. It
only reads origin (git: your ssh keys or a credential helper such as `gh
auth setup-git`) and never pushes, tags or publishes, so it reads no
`GH_TOKEN`, `GITHUB_TOKEN`, registry login or package key. The release
clone's origin is your origin's address without user info; when your
origin URL itself carries credentials (`https://user:token@host/...`), the
real URL is written only into the release clone's own `.git/config` as
`url.<real>.insteadOf` (an owner-only file in an owner-only temporary
directory, removed at the end), never on a command line or in any
environment. Git error output may still echo that URL; prefer a credential
helper. Your stage commands run with your environment: a `docker compose`
build that pulls private images uses your `docker login`.

`nerv version next` needs no `GH_TOKEN` or `NPM_TOKEN` (the credential
checks are skipped), only read access to origin.

### Reading origin

Every read of origin (the fetch of both commands, the read of the branch
for the rerun advice) never prompts and is bounded:

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
- Only origin is built: `nerv ship` never pushes your local commits and
  refuses a local branch that differs from origin's; it plans from your
  local `release:` section, applied to origin's code.

## Troubleshooting and recovery

### Refusals and environment errors

Nothing has been built or written in any of these cases.

| Message (abridged) | Cause | What to do |
|---|---|---|
| `your local <b> (...) is not origin/<b> (...)` | `nerv ship` builds what is on origin. | `git push` or `git pull`, then run again. |
| `the repository has no release: section` / `release.projects[N].build.mode: required by nerv ship` | `nerv ship` needs a `build.mode` per project. | Add the section (`/nerv:release-setup`) or the mode. |
| `release.projects[N].presets: the presets were removed ...` (or `.dotnet`, `.docker`, ...) | The section still uses a removed preset. | Write what the message says ([Migrating from the presets](#migrating-from-the-presets)). |
| `the artifact <dir> already exists: pass --force to replace it` | That version was already packaged. | `--force` to rebuild it, or deploy the existing one. |
| `--version X: project <p> already has Y (tag ...)` | An explicit version must be newer than the project's last tag. | Pass a newer version. |
| `the release: section names paths that are not on origin's <b>: "<path>" (project <p>, <field>)` | The section is your local one; the path exists only locally (or not at all). | Push the code that adds it, or fix the section. |
| `branch <b> is not on origin` / `has no origin remote` | Nothing on origin to build. | Push the branch (or add the remote). |
| `the computed release is not the expected one (<project>: expected X, computed Y; ...)` | `--expect` no longer matches: origin moved since the dry run. | Run the dry run again, review the new plan, then package with its versions. |
| `confirmation needed but stdin is not an interactive terminal` | No terminal for `[y/N]`. | `--yes` (after a `--dry-run`), or run it in a terminal. |
| `invalid release configuration: ...` | The `release:` section (every problem listed). | Fix each line; see [Validation](#validation). |
| `unknown project "x"; the release: section declares: ...` | `--project` typo. | Use one of the listed names. |
| origin unreachable or timed out | Network, credentials or a prompt blocked by the no-prompt rules. | Fix access ([Reading origin](#reading-origin)); `nerv version next --offline` works without origin. |
| `git X.Y.Z is too old: selecting a project's commits by path needs git 2.31.0 or newer` | Path filters need `--diff-merges`. | Upgrade git. |
| `Node.js ... is too old` / node or npm not found | semantic-release needs Node.js 20.8.1+. | Install or upgrade Node.js. |
| a package not pinned in `release.semantic_release.extra_plugins` | The semantic-release configuration names a package NERV does not install. | Add it as `name@version`. |
| `<program> not found on PATH` / missing interpreter | docker (compose mode) or a stage or hook shell is missing. | Install it (Docker with compose, Git for Windows, PowerShell 7, Node.js, ...). |
| an inline pwsh command exits `3` | PowerShell older than 7.3. | Upgrade PowerShell. |

### Failures during a run

A failure stops the project: nothing is ever written to origin or to your
repository's git, and no artifact is left for the failed project (the
artifact is renamed into place only once complete). The summary (and the
`--json` `action`) says what to do: fix the cause and run `nerv ship`
again. Projects packaged before the failure keep their artifacts; a rerun
refuses them unless `--force`, so run the rest with `--project`.

- The advice to run again checks origin's branch at the end of the run:
  when it moved, your local branch is now behind origin, which `nerv ship`
  refuses, so the advice says to `git pull` first. When the branch was
  deleted on origin, the advice says so instead; when origin cannot be read
  (within 10 seconds, also after Ctrl-C), it says to check with `git
  fetch`.
- A failure runs the project's `fail` hooks (not after Ctrl-C).
- The release clone and everything built in it are removed at the end, on
  success or failure; only the artifacts stay.

## Known limitations

- **Windows is compile-checked only**: the whole-tree kill of a cancelled
  command (`taskkill /T`), the Git Bash lookup and pwsh/node script-file
  runs are unit-tested with a fake OS, never on a real Windows machine. On
  Windows, a descendant process that outlives its command keeps running.
- **No Git LFS**: the release clone does not fetch LFS objects, so a build
  that needs them uses LFS pointer files. Submodules are initialised only
  for projects with `submodules: true`.
- An origin whose `HEAD` names a missing branch fails semantic-release's
  own fetch.
- `--project` applies the dependency bumps of dependencies it does not
  release, in both commands.
- Offline (`nerv version next --offline`), a prerelease branch may compute
  another version than CI when a channel note exists only on origin.
- `build.version_files` takes MSBuild project files only; other version
  files (`package.json`, ...) are written by a stage command, in the clone,
  and do not reach the artifact's release files.
- Compose mode leaves the services `docker compose run` started (the test
  service's dependencies) running; add an `after_test` hook running
  `docker compose -f <file> down` when that matters.
- A project named `pipeline` would share `.nerv/artifacts/pipeline/` with
  `nerv pipeline`'s step outputs.
- Commands run in their own session or process group without a
  controlling terminal, so a terminal Ctrl-C reaches them only through
  NERV, which cancels them on the first signal.
