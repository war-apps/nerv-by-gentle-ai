# Release extension: `nerv version next` and `nerv release`

The release extension (`release@nerv`) versions and packages your own repositories
(any language, single repository or monorepo) from your machine. Its
version engine is native: it gives the version, the tag, the release notes
and the CHANGELOG section that semantic-release with the
`conventionalcommits` preset gives, without semantic-release, node or npm.
Use it where the CI build cannot or should not run; it computes and builds
exactly what CI would.

| Command | Slash command | What it does | Changes anything? |
|---|---|---|---|
| `nerv version next` | `/release:version` | The next semantic version of the repository, or of each project of a monorepo, with the outputs of `cycjimmy/semantic-release-action`. | No. |
| `nerv release` | `/release:build` | Builds, tests and packages what is on origin, in a throwaway clone, with each project's build provider (`docker-compose` or `bash`), per project in dependency order, and leaves a local artifact (`.nerv/artifacts/<project>/<version>/`) for [`nerv deploy`](deploy.md). | Only `.nerv/artifacts/` in your repository; never git (no commit, tag or push) and never a registry. |
| `nerv release setup --import [--force] [--dry-run]` | `/release:setup` | Imports an existing semantic-release config into the version engine's keys (`release.providers.conventional-commits`), once. | Only `.ai-workflow/config.yaml` (nothing with `--dry-run`). |

Both commands read the `release:` section of the project-scope
`<repo>/.ai-workflow/config.yaml`. Without it a repository is one project with the
default branches; `nerv release` needs the section, with a build provider
for each project.

The release extension has two provider slots
([extensions.md](extensions.md#the-release-slots)): the **version engine**
(`release.version`, by default `conventional-commits`, today's native
engine) and the **build** (`release.build` for every project, overridden
by `release.projects[].build`: `docker-compose` or `bash`). Each provider's
keys live under `release.providers.<provider>`, and a project overrides a
build provider's keys under `release.projects[].providers.<provider>`.
nerv calls them through the common provider contract, and installs all
three with the release extension.

The extension ships two agents, `release:versioner` (computes and explains the
next version) and `release:builder` (runs the dry run, and the real build only
with your approved versions), and the `release` skill, its contract. Install
it with:

```sh
nerv install --with release
```

## Contents

- [Requirements](#requirements)
- [Quick start](#quick-start)
- [Configuration (`release:`)](#configuration-release)
- [Migrating from the 4.2.0 `release:` section and semantic-release](#migrating-from-the-420-release-section-and-semantic-release)
- [`nerv version next`](#nerv-version-next)
- [`nerv release`](#nerv-release)
- [Build providers](#build-providers)
- [The artifact](#the-artifact)
- [Stages, hooks and commands](#stages-hooks-and-commands)
- [Security notes](#security-notes)
- [Troubleshooting and recovery](#troubleshooting-and-recovery)
- [Known limitations](#known-limitations)

## Requirements

| Tool | Needed for |
|---|---|
| git | Always. 2.31 or newer when a project's `path` is not `.` (path filters); an older git stops before computing anything, naming the git found and the minimum. |
| bash, pwsh (PowerShell 7) or node | `nerv release`: the stage commands and hooks that use them. Inline pwsh commands need PowerShell 7.3 or newer. On Windows, bash is Git Bash. |
| docker with the compose plugin | `nerv release` for a project in `compose` mode. |
| Your build tools (.NET SDK, npm, ...) | `nerv release` for a project in `script` mode: whatever its stage commands run. |

Computing a version needs no node or npm. Read access to origin is needed
for both commands (`nerv version next --offline` skips it). `nerv release`
needs no push access and no registry credentials: it never pushes or
publishes.

## Quick start

### Interactive setup

In Claude Code, `/release:setup` writes the `release:` section for you:

1. When the repository has a JSON or YAML semantic-release config
   (`package.json` `release`, `.releaserc.json`, `.releaserc.yaml`/`.yml`),
   it runs `nerv release setup --import --dry-run`, shows the translated
   branches, release rules, note keywords, tag format and changelog, and
   every part that cannot be translated, then imports it on your yes. A
   `.js` config is reported for you to migrate by hand.
2. It detects compose files, `.csproj` / `.sln` / `.slnx` files,
   Dockerfiles, `package.json` scripts, CI build steps, release scripts,
   and a monorepo layout with its commit scopes.
3. It asks, for each project, its build provider (`docker-compose` or
   `bash`), and then the compose file, test service and images, or the
   command of each stage. There are no presets. It asks one question at a time, only what it
   cannot detect.
4. It writes the section into `.ai-workflow/config.yaml` (the rest of the file is
   kept as it is) and validates it with `nerv version next --offline`,
   showing each project's next version.

It never commits, pushes, ships or deploys. `nerv version next` and `nerv
release` read your local `.ai-workflow/config.yaml` as it is, committed or not (commit
it for your team's sake; it is not required). Then run `/release:build`.
`/release:setup --project <name>` adds or edits one project of an existing
section. The examples below show the shapes it produces.

### Single repository: dotnet API built with bash

A repository shaped like `dim.comprobantes.api`: the API's `.csproj`
receives the version, the tests collect coverage with a line floor, and
the package stage leaves the NuGet package and the image archive in the
outputs. The default branches (main, develop as `alpha`, `release/*` as
`rc` on `beta`, `hotfix/*` as `beta`) apply. Add to `.ai-workflow/config.yaml`:

```yaml
release:
  projects:
    - name: api
      path: .
      build: bash
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
nerv release --dry-run                   # restore, lint, build, test, coverage, package in a clone
nerv release                             # the same, then the artifact, after a [y/N] confirmation
```

In Claude Code, `/release:version` and `/release:build` do the same; `/release:build`
always rehearses with `--dry-run` first and packages only after your
explicit yes.

### Single repository: docker compose

When a compose file already builds the images and runs the tests:

```yaml
release:
  build: docker-compose
  providers:
    docker-compose: { file: docker-compose.yml, test_service: tests }
  projects:
    - name: api
      path: .
      images: ["ghcr.io/org/api:${version}"]
```

`nerv release` runs `docker compose -f docker-compose.yml build`, then
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
      build: bash
      version_files: [src/core/Core.csproj]
      stages:
        build: { run: dotnet build src/core -c Release }
        package: { run: 'dotnet pack src/core -c Release --no-build -o "$NERV_RELEASE_OUTPUTS_DIR"' }
    - name: api
      path: src/api
      shared_paths: [Directory.Build.props]
      depends_on:
        - { project: core, bump: patch }
      build: docker-compose
      providers:
        docker-compose: { file: src/api/docker-compose.yml, test_service: api-tests }
      images: ["ghcr.io/org/api:${version}"]
    - name: web
      path: web
      scopes: [web, ui]
      build: bash
      stages:
        restore: { run: npm ci, workdir: web }
        test: { run: npm test, workdir: web }
```

`nerv version next` prints one block per project; `nerv release --project api`
packages only `api` (its dependencies are computed for `depends_on`, not
packaged).

## Configuration (`release:`)

The section is project scope only: it is never read from the user-scope
configuration and never merged with it. The user file's `defaults.release`
(same shape) is only a seed: `/release:setup` copies the keys the project
lacks when it writes the section, and shows them; see
[configuration.md](configuration.md#extension-sections-tasks-release-deploy).
It holds everything: the
versioning rules (branches and channels, release rules, note keywords, tag
format, changelog), which projects a monorepo holds, how they depend on each
other, and how each one is built, tested and packaged. No semantic-release
configuration is read.

Both commands read the `release:` section from your local `.ai-workflow/config.yaml`
(the working-tree file, committed or not), never origin's copy. For `nerv
release` that is the only thing taken from your machine: the code and the
branch content are origin's, in the release clone, whose own
`.ai-workflow/config.yaml` is ignored. The paths the section names are resolved
inside the release clone; a path that is not on origin's branch (a project
folder that exists only locally) is refused before any stage.

### Paths

Every path in the section is relative to the repository root, whatever the
project's own `path` is. Forward slashes are canonical; backslashes are
accepted and normalised. Absolute paths (`/x`, `C:\x`) and paths that
escape the repository (`../x`) are rejected.

### Slots and providers

| Key | Default | Meaning |
|---|---|---|
| `version` | `conventional-commits` | The version engine (the `version` slot's provider). |
| `build` | none | The build provider of every project that names none: `docker-compose` or `bash`. `nerv release` needs one per project. |
| `providers.<provider>` | `{}` | The provider's keys, as its `provider.yaml` declares them: [Version engine keys](#version-engine-keys) for `conventional-commits`, [`build`](#build) for the build providers. A key the provider does not declare, a value of another type, or a provider nerv does not ship is refused. |

The keys moved from earlier alpha builds are refused with their new place:
an engine key directly under `release:` (`release.branches: moved to
release.providers.conventional-commits.branches`), and a project's `build:`
mapping (`build.mode` is the provider's name now, `build.file` and
`build.test_service` go to `providers.docker-compose`, `build.shared_host`
to `providers.bash`, `build.images`, `build.version_files` and
`build.coverage` to the project itself).

### Version engine keys

The `conventional-commits` engine's keys, under
`release.providers.conventional-commits`:

```text
release:
  providers:
    conventional-commits:
      branches:
        - main
        - { name: develop, prerelease: alpha, channel: alpha }
        - { name: "release/*", prerelease: rc, channel: beta }
        - { name: "hotfix/*", prerelease: beta, channel: beta }
      release_rules:
        - { type: docs, scope: readme, release: patch }
        - { type: chore, release: none }
      note_keywords: [BREAKING CHANGE, BREAKING CHANGES, BREAKING]
      tag_format: "v${version}"
      changelog: { file: CHANGELOG.md, title: "# Changelog" }
  projects: [...]
```

| Field | Default | Meaning |
|---|---|---|
| `branches` | `main`; `develop` (prerelease `alpha`, channel `alpha`); `release/*` (prerelease `rc`, channel `beta`); `hotfix/*` (prerelease `beta`, channel `beta`) | The release branches, in semantic-release's order: a branch takes the first entry whose `name` (a branch or a glob) matches it. An entry is a bare name or `{name, prerelease, channel}`. `prerelease` is an identifier, or `true` for the branch name (refused when the branch name is not a valid identifier, such as `hotfix/x`). `channel` defaults to semantic-release's: the default channel for the first release branch, the branch name otherwise. |
| `release_rules` | none | commit-analyzer's `releaseRules`: `type` and `scope` (globs), `breaking`, `revert`, and `release` (`major`, `minor`, `patch` or `none`; `false` is read as `none`). A commit no rule matches falls back to the Conventional Commits defaults (a breaking-change note is major, `feat` minor, `fix` and `perf` patch). |
| `note_keywords` | `BREAKING CHANGE`, `BREAKING CHANGES`, `BREAKING` | The keywords that start a breaking-change note in a commit's body or footer (`parserOpts.noteKeywords`). |
| `tag_format` | `v${version}` | The tag; with several projects each one defaults to `<name>-` followed by it. It holds `${version}` and no other placeholder. |
| `repository_url` | origin's URL, without credentials | What the release notes link to. A local origin, or none, gives notes without links. |
| `changelog.file` | `CHANGELOG.md` | The CHANGELOG file name under each project's `path`. |
| `changelog.title` | none | Written at the top of the file (`changelogTitle`). |

The engine was verified field by field (version, channel, tag, notes)
against semantic-release 24.2.9 with the `conventionalcommits` preset on
real repositories. It reads the channel notes semantic-release (and `nerv
deploy`) write (`refs/notes/semantic-release-<tag>`), so prerelease
numbering continues where CI left it. Not supported: maintenance branches
(`1.x`, ranges), `addChannel`, `extends` and presets other than
`conventionalcommits`.

### Importing a semantic-release config

```sh
nerv release setup --import --dry-run   # show the translation and the warnings
nerv release setup --import             # write it into release.providers.conventional-commits
```

The import is the version engine's own (its optional `import`
operation): `conventional-commits` reads the config semantic-release would (`package.json` `release`,
`.releaserc`, `.releaserc.json`, `.releaserc.yaml`, `.releaserc.yml`) and
translates `branches`, the commit-analyzer's `releaseRules` and
`parserOpts.noteKeywords`, `tagFormat`, `repositoryUrl` and the changelog
plugin's `changelogFile` and `changelogTitle`. Every part it cannot
translate is reported, with what to do:

- `@semantic-release/npm` and `@semantic-release/exec` (`prepareCmd`,
  `publishCmd`, ...): write them as stages, hooks or `version_files`,
  and publishing as a [deploy](deploy.md) target;
- `@semantic-release/git` and `@semantic-release/github`: they belong to the
  `deploy:` section (`git.commit_message`, `host: github`);
- `presetConfig.types` (custom section titles), `extends`, other presets and
  maintenance branches;
- a `prerelease: true` branch whose name is not a valid identifier.

The import writes only the engine keys the config sets
(`branches`, `release_rules`, `note_keywords`, `tag_format`,
`repository_url`, `changelog`), under `release.providers.<engine>`, and
keeps every other key of the section with its comments. A key that block
already sets to another value is never
replaced silently: the import refuses (exit 1), writes nothing and lists
each such key with its current and imported values; run it again with
`--force` to replace them, and it prints each replaced value. A 4.2.0
`release:` section is not converted: the import refuses it and names
`nerv migrate`. Without a semantic-release config there is nothing to
import, and the import refuses (exit 1).

A JavaScript config (`.releaserc.js`, `.cjs`, `.mjs`, `release.config.*`)
is never imported: it is reported, and you migrate it by hand. After the
import, the semantic-release config is no longer read; delete it once
`/release:version` gives the versions you expect.

### `release.projects[]`

At least one project is required. A single-repo setup is one project with
`path: .`.

| Field | Default | Meaning |
|---|---|---|
| `name` | required | Unique id, `[a-z0-9][a-z0-9-]*`. Used in tags, `--project` and `depends_on`. |
| `path` | required | The project's directory. A commit touching it counts for the project. |
| `shared_paths` | `[]` | Extra paths or globs whose changes also count for the project (shared code, build files). |
| `scopes` | `[<name>]` | Conventional-commit scopes whose commits count for the project wherever they touch. A commit counts when it touches `path`/`shared_paths` **or** its scope matches; one commit can release several projects. |
| `tag_format` | the engine's: `v${version}` (one project) / `<name>-v${version}` | The project's tag. Must contain `${version}`, no other placeholder, and be unique across projects. |
| `changelog` | the engine's: `<path>/CHANGELOG.md` | The project's CHANGELOG file (repository-relative); see [The project's changelog](#the-projects-changelog). |
| `depends_on` | `[]` | Projects whose new release bumps this one; see below. |
| `build` | `release.build` | The project's build provider (`docker-compose` or `bash`); `nerv release` needs one; see [`build`](#build). `nerv version next` ignores it. |
| `providers.<provider>` | `{}` | The project's own keys of a build provider, over `release.providers.<provider>`. |
| `images` | `[]` | The images saved into the artifact; see [`build`](#build). |
| `version_files` | `[]` | MSBuild files that receive the version; see [`build`](#build). |
| `coverage` | none | Coverage floors; see [`build`](#build). |
| `stages` | `{}` | The stage commands; see [Stages, hooks and commands](#stages-hooks-and-commands). |
| `hooks` | `{}` | Extra scripts around stages and at the release lifecycle steps; same section. |
| `submodules` | `false` | `true` runs `git submodule update --init --recursive` at the very start of `restore` (the stage is created if the project has no restore command). |

### The project's changelog

Each project writes its own CHANGELOG: `changelog`, or `<path>/` followed
by the engine's `changelog.file` (`CHANGELOG.md`). `nerv release` writes the release's
CHANGELOG section at the top of it (under its `# ` title when it has one)
in the release clone only, and stores the new file in the artifact for
`nerv deploy`'s release commit.

### `depends_on[]`

| Field | Default | Meaning |
|---|---|---|
| `project` | required | Another project of the section (not itself). |
| `bump` | `patch` | The lowest release this project gets when that project releases: `patch`, `minor`, `major` or `none` (no bump, but still released after it). |

Dependencies must not form a cycle; the error names it
(`dependency cycle a -> b -> a`). Projects are computed and packaged in
dependency order, otherwise in declaration order.

### `build`

The build provider (`release.projects[].build`, else `release.build`) is
required by `nerv release` and has no default. Its keys are
`release.providers.<provider>.<key>`, overridden per project by
`release.projects[].providers.<provider>.<key>`; `images`, `version_files`
and `coverage` are the project's own, whatever its provider:

| Key | Where | Default | Meaning |
|---|---|---|---|
| `build` | project or section | required | `docker-compose` or `bash` ([Build providers](#build-providers)). |
| `file` | `providers.docker-compose` | required | The compose file, repository-relative: `docker compose -f <file>`. |
| `test_service` | `providers.docker-compose` | none (no test stage) | The compose service `docker compose -f <file> run --rm <test_service>` runs; its exit code is the test result. |
| `images` | project | `[]` | The images `docker save` packages into the artifact after the `package` stage, one archive per entry, every reference recorded in the manifest's `images` so the `deploy-ssh-compose` publish pushes them all. An entry is either a plain image reference, which may use `${version}`, `${channel}` (empty on the default channel) and `${tag}`, or a mapping `{name: <repository>, tags: [version, channel, latest]}` ([Image tags](#image-tags)). With `docker-compose` they must be the images the compose file builds; with `bash` the project's own commands (usually `stages.package`) must build them. |
| `shared_host` | `providers.bash` | `false` | `true` gives the `restore`, `lint`, `build`, `test` and `package` commands the build-server-safe dotnet environment (`MSBUILDDISABLENODEREUSE=1`, `UseSharedCompilation=false`, `DOTNET_CLI_USE_MSBUILD_SERVER=0`) under their own `env`, for shared hosts where reused build servers crash dotnet. A command's own `env` wins key by key; its `run:` is never rewritten. |
| `version_files` | project | `[]` | MSBuild project files (`.csproj`, `.fsproj`, `.vbproj`, `.props`, `.targets`) that receive `<Version>` and `<PackageReleaseNotes>` in the clone before the pipeline ([Version files](#version-files)). |
| `coverage` | project | none | Coverage floors checked after the `coverage` stage over every Cobertura report (`*.cobertura.xml`) under `coverage.dir` ([Coverage](#coverage)). |
| `coverage.dir` | project | `$NERV_RELEASE_ARTIFACTS_DIR/coverage` | Directory searched for the reports, repository-relative (a compose test service writes them through a bind mount). |
| `coverage.line`, `coverage.branch`, `coverage.method` | project | `0` (not checked) | Minimum percentages, 0-100; at least one must be set. |

#### Image tags

A mapping entry tags its repository as 4.2.0's docker preset did. `tags`
lists tag kinds, `[version, channel, latest]` by default:

| Kind | Tag | When |
|---|---|---|
| `version` | the release version, e.g. `1.4.0-alpha.2` | always |
| `channel` | the release channel, e.g. `alpha` | only off the default channel |
| `latest` | `latest` | only on the default channel |

So a stable release gives `<name>:<version>` and `<name>:latest`, and an
`alpha` release `<name>:<version>` and `<name>:alpha`. Duplicate tags are
dropped, a tag that is not a valid docker tag is refused, and so is an
unknown kind. `name` is a repository without a tag or digest (a registry
`host:port/` prefix is fine).

The build must produce `<name>:<version>` (the compose file's `image:`, or
the package commands' `docker build -t`). After the `package` stage,
`nerv release` runs `docker tag <name>:<version> <name>:<tag>` for each other
tag, then one `docker save` of every tag into the image's archive, and
records each reference in the manifest:

```yaml
build: docker-compose
providers:
  docker-compose: { file: docker-compose.yml }
images:
  - { name: ghcr.io/org/api }                   # version, channel, latest
  - { name: ghcr.io/org/worker, tags: [version] }
  - "ghcr.io/org/tools:${version}"              # a plain reference
```

#### Migrating from the presets

The `dotnet`, `node`, `docker`, `nuget` and `npm` presets were removed.
A section that still has `presets:` or one of their settings blocks is
refused, each key with what to write instead:

| Old key | Write instead |
|---|---|
| `presets` | `build: bash` with each stage's command under `stages`, or `build: docker-compose` with `providers: { docker-compose: { file: ..., test_service: ... } }` and `images: [...]`. |
| `dotnet` | Its commands as `stages` (`dotnet restore`, `dotnet format --verify-no-changes`, `dotnet build -c Release --no-restore`, `dotnet test -c Release --no-build`); `project_file` to `version_files`, `coverage_threshold` to `coverage`, `shared_host` to `providers.bash.shared_host`; for `sdk`, a `before_restore` hook writing `global.json`. |
| `node` | Its commands as `stages` (`npm ci`, `npm run lint`, `npm run build`, `npm test`). |
| `docker` | `build: docker-compose`, or a bash `package` stage running `docker build -t <name>:$NERV_RELEASE_VERSION`, with each image as `images: [{name: <name>, tags: [...]}]` so release tags and saves it as the preset did ([Image tags](#image-tags)). Pushing moved to `nerv deploy`. |
| `nuget`, `npm` | A `package` stage writing the package to `$NERV_RELEASE_OUTPUTS_DIR` (`dotnet pack -o`, `npm pack --pack-destination`). Publishing moved to `nerv deploy`. |
| `stages.publish`, `hooks.publish`, `hooks.success` | Removed from `nerv release`: publishing, and what runs after it, belong to `nerv deploy`. |

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
refuse an invalid section (exit 1) before reading origin, including what
the version engine's `validate` refuses (its keys, the projects' tag
formats); `nerv release` also refuses a project without a build provider,
an unknown provider, a provider key it does not declare, and what the
build provider's `validate` refuses (`docker-compose`: a missing or
escaping `file`, a bad `test_service`, a `build`, `test` or `package`
stage command).

### YAML notes

- Only the `release:` block is parsed: it is cut out of `.ai-workflow/config.yaml` by
  indentation, so a YAML error elsewhere in the file never affects the
  release commands. When the file's layout does not allow the cut
  (`---` between documents, an indented or flow top level), the whole file
  is parsed instead. The key may be quoted (`"release":`, `'release':`) or
  carry an anchor or a tag (`&s release:`, `!!str release:`). If the file
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
      build: bash
      version_files: [api/Api.csproj]
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

## Migrating from the 4.2.0 `release:` section and semantic-release

The 4.2.0 section had the same key, `release:`. It is told apart by its
4.2.0 keys, `semantic_release:` and the project presets (`presets`,
`dotnet`, `node`, `docker`, `nuget`, `npm`): both commands refuse a section
with one, naming each key and `nerv migrate`. `nerv migrate --dry-run` shows the
whole migration of a repository (the configuration files, the `release:`
and `pipeline:` sections, the semantic-release import and the extensions to
install), and `nerv migrate` applies it with backups. By hand:

1. Drop `semantic_release:` (its `version`, `24.2.3` by default, and
   `extra_plugins`): no node runner is left.
2. Run `nerv release setup --import` to bring the branches and rules over from
   the semantic-release config, or write them
   ([Version engine keys](#version-engine-keys)).
3. Give every project a build provider (`build: bash` runs the presets'
   commands as stages), and replace the presets
   ([Migrating from the presets](#migrating-from-the-presets)).
4. Move publishing (`@semantic-release/git`, `@semantic-release/github`,
   docker push, NuGet, npm) to the `deploy:` section ([deploy.md](deploy.md)).

The `pipeline:` section and `nerv pipeline` were removed too: see
[deploy.md](deploy.md#migrating-from-nerv-pipeline).

## `nerv version next`

```
nerv version next [--cwd <dir>] [--branch <name>] [--project <name>]
                  [--offline] [--fetch-timeout <dur>] [--format text|json|env | --json]
```

Computes the next semantic version of the repository, or of each project of
its `release:` section, with the native engine. Nothing in the repository
changes. Its flags and outputs are the ones it had with semantic-release. In
Claude Code, `/release:version [args]` runs it with `--json` and shows a table
per project; the `release:versioner` agent also explains which commits decided
each bump. Plain `nerv version` (and `nerv version --json`) still prints the
binary and plugin versions.

| Flag | Default | Meaning |
|---|---|---|
| `--cwd <dir>` | `.` | Any directory inside the repository; the git top level is used. |
| `--branch <name>` | current branch | Local branch to compute. |
| `--project <name>` | all projects | Only this project of the `release:` section (its dependencies are still computed, for `depends_on`). An unknown name is refused, listing the declared projects. |
| `--offline` | off | Use the local tags and notes only. By default origin's release tags and channel notes are read first, with your git credentials, into a throwaway copy. Offline, a prerelease branch may compute a different version than CI when a release's channel note exists only on origin. |
| `--fetch-timeout <dur>` | `60s` | Bound for reading origin (Go duration: `30s`, `2m`); must be positive, there is no unbounded mode (use `--offline` to skip origin). |
| `--format text\|json\|env` | `text` | Output format. |
| `--json` | | Same as `--format json`. |

### How the next version is computed

- It reads a throwaway copy of the repository: the working tree, branches
  and tags of the repository are never changed.
- The branch is matched against `branches` as semantic-release matches it.
  The branch you compute is taken at your local commit, so its unpushed
  commits count.
- Release tags (by `tag_format`) and the channel notes
  (`refs/notes/semantic-release-<tag>`) are read from origin first (read
  access is enough). A plain `git fetch` never brings those notes; without
  them a prerelease tag such as `v2.9.0-alpha.8` is not recognised as the
  last `alpha` release. Offline, only the local tags and notes are used.
  How origin is read safely: [Reading origin](#reading-origin).
- The commits since the last release on the branch's channel are parsed as
  Conventional Commits; the highest bump of `release_rules` and the
  defaults wins. A prerelease branch numbers its releases per prerelease id
  (`1.2.0-alpha.1`, `-alpha.2`, ...).
- The release notes and the CHANGELOG section are the
  `conventionalcommits` preset's, linking to `repository_url`.
- In a monorepo, each project is computed in dependency order with its own
  `tag_format`, from the commits that touch its `path` or `shared_paths` or
  carry one of its `scopes` (`type(scope): ...`, several scopes separated by
  commas). A merge commit touches what it brings into the branch (its
  changes against its first parent); the merged commits count on their own.
  This needs git 2.31 or newer. A project whose `path` is `.` sees every
  commit. When a dependency gets a new release, the dependent is released
  with at least the dependency's `bump`, even without commits of its own.

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
| `1` | Refused: an invalid `release:` section (every problem, path-qualified), a 4.2.0 `release:` section, an unknown `--project`, `--project` without a `release:` section, a directory outside any git repository. |
| `2` | Usage error, or an environment error: git missing or too old, origin unreachable or timed out (the message suggests `--offline`), no result for the requested project, the result could not be written to stdout. |
| `128 + signal` | Interrupted: `130` for Ctrl-C (SIGINT), `143` for SIGTERM, `129` for SIGHUP. |

Ctrl-C, `SIGTERM` and (on Unix) `SIGHUP` cancel the run and stop git and
every command, which run in their own session or process group. After the first
signal NERV stops catching them, so a second Ctrl-C ends `nerv` at once.
The same holds for `nerv release`.

## `nerv release`

```
nerv release [--cwd <dir>] [--branch <name>] [--project <name>]
          [--version <semver>] [--dry-run] [--force] [--yes]
          [--fetch-timeout <dur>] [--json] [--expect <project>=<version> ...]
```

Builds, tests and packages **what is on origin**, in a throwaway clone, as
the CI build would, and leaves a local artifact per project for `nerv
deploy`. It never writes to git: no release commit, no tag, no push (and no
registry push or publish); an artifact that is never deployed leaves no
trace in the repository. In Claude Code, `/release:build [--branch <name>]
[--project <name>] [--version <semver>]` always runs `nerv release --dry-run
--json` first, shows the plan, and runs `nerv release --yes --json --expect
<project>=<version> ...` (the dry run's versions) only after your explicit
yes, so the versions you approved are the only ones that can be packaged.

| Flag | Default | Meaning |
|---|---|---|
| `--cwd <dir>` | `.` | Any directory inside the repository. |
| `--branch <name>` | current branch | Branch to build; your local branch must equal origin's. |
| `--project <name>` | all projects | Only this project of the `release:` section. |
| `--version <semver>` | computed | Use this version instead of computing it. With several projects `--project` is required. It must be a semantic version that fits the branch (the branch's prerelease identifier on a prerelease branch, a stable version on a release branch) and is newer than the project's last release on the branch, picked as the version engine picks it: only the tags reachable from the branch, of its channel (a stable tag on `main` that `develop` never merged does not count). Its tag must not exist yet. The tag follows `tag_format`, the channel is the one the `branches` entry of the branch gives (`beta` for `2.0.0-rc.1` on `release/*` with the defaults; empty for a stable version), and the engine writes the release notes and the CHANGELOG section for the version from the commits since that last release. A malformed version is a usage error (exit 2). |
| `--dry-run` | off | Run the pipeline and the prepare hooks in the clone; write no artifact. |
| `--force` | off | Replace an existing artifact of the same version (otherwise refused). |
| `--yes` | off | Package without asking (required without a terminal). |
| `--fetch-timeout <dur>` | `60s` | Bound for each read of origin; must be positive. |
| `--json` | off | Final summary as JSON. |
| `--expect <project>=<version>` | none | Repeatable. Package only when the releases are exactly these: every listed project releases that version and no other project releases. Otherwise the run is refused (exit 1, `--json` status `refused`, expected vs computed per project) before any stage runs. Use the names and versions a `--dry-run --json` printed. A malformed value or a project given twice is a usage error (exit 2). It works with `--version` too. |

### Flow

1. Resolve the repository and the branch (default: the current one), load
   the `release:` section from your local `.ai-workflow/config.yaml` (committed or
   not) and refuse, before anything else, a repository without the
   section, an invalid section, a project without a build provider, an
   unknown provider or provider key, what the version engine's or a build
   provider's `validate` refuses, an unknown `--project`, or `--version`
   with several projects and no `--project`.
   This section is used for the whole run; the clone's copy is ignored.
2. Create the **release clone**: a fresh `git clone --shared` of your
   repository in a temporary directory (never a worktree of it), whose
   `origin` is your origin's address; origin's branches, tags and
   channel notes are fetched into it and the branch is checked out
   at origin's commit. Local-only branches and tags never reach it. Your
   repository is never modified.
3. Refuse (exit 1) when your local branch is not exactly origin's (ahead:
   push first; behind: pull first): `nerv release` builds what origin has and
   never pushes your commits. A branch that is not on origin is refused
   too. Refuse also, before any version is computed, when a path the
   section names (project `path`, a `shared_paths` entry without glob
   characters, a `version_files` entry, a stage or hook `workdir`) does not
   exist on origin's branch; a file the build provider plans with
   (`docker-compose`'s `file`) is checked the same way when the project is
   planned, before any stage runs.
4. Get every project's version: computed in the clone (as `nerv version
   next`, from the refs just read), or `--version`. Projects without a new
   release are reported and skipped. With `--expect`, refuse here unless
   the releasing projects and versions are exactly the expected ones. A
   real run refuses an artifact that already exists for a releasing
   project's version, unless `--force`.
5. Print each releasing project's version, channel, tag, last release, its
   plan ([Build providers](#build-providers)) and its artifact directory, then ask
   `Package <n> project(s)? [y/N]` on an interactive terminal. `--yes`
   skips the question; without a terminal `--yes` is required (exit 1
   otherwise); `--dry-run` never asks.
6. For each releasing project, in dependency order (`depends_on`):
   1. write, in the clone only, `<Version>` and `<PackageReleaseNotes>`
      into `version_files` ([Version files](#version-files)) and the
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
{"status":"packaged","projects":[
  {"project":"core","branch":"main","status":"packaged","version":"1.1.0","tag":"core-v1.1.0",
   "last_version":"1.0.0","artifact":"/repo/.nerv/artifacts/core/1.1.0"}]}
```

- `status`: `packaged`, `dry-run`, `nothing-to-release`, `refused` (exit
  1), `declined` (answered no; exit 1), `failed` (exit 2) or `interrupted`
  (128 + signal).
- `error`: the reason, when the run did not succeed.
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
| `1` | Refused: no or an invalid `release:` section (or provider keys), a 4.2.0 `release:` section, a project without a build provider, unknown `--project`, `--version` not newer than the branch's last release, with an existing tag, or without `--project` in a monorepo, not a git repository or not on a branch, no origin remote, local branch not equal to origin's, an existing artifact without `--force`, no confirmation (or not a terminal without `--yes`), a plan that cannot be built. |
| `2` | Usage error, an environment error (origin unreachable, git or docker missing), or a failed step. |
| `128 + signal` | Interrupted (`130` Ctrl-C, `143` SIGTERM, `129` SIGHUP); a second Ctrl-C ends `nerv` at once. |

## Build providers

`nerv release` plans each project's pipeline from the steps its build
provider answers (the provider's `plan` operation), its `stages` and its
`hooks` (`internal/releaseplan`), for the release being made (version,
channel, tag):

| Stage | `bash` | `docker-compose` |
|---|---|---|
| `restore` | `stages.restore` | `stages.restore` |
| `lint` | `stages.lint` | `stages.lint` |
| `build` | `stages.build` | `docker compose -f <file> build` |
| `test` | `stages.test` | `docker compose -f <file> run --rm <test_service>` (none without `test_service`) |
| `coverage` | `stages.coverage`, then the floor check | `stages.coverage`, then the floor check |
| `package` | `stages.package`, then, per `images` entry, `docker tag` of a mapping's other tags and one `docker save -o $NERV_RELEASE_OUTPUTS_DIR/<image>.tar <references>` | per `images` entry, `docker tag` of a mapping's other tags and one `docker save -o $NERV_RELEASE_OUTPUTS_DIR/<image>.tar <references>` |

1. A stage with no command is skipped together with its `before_<stage>`
   and `after_<stage>` hooks; otherwise its `before_<stage>` hooks run
   first and its `after_<stage>` hooks last. `submodules: true` adds
   `git submodule update --init --recursive` at the start of `restore`; a
   `coverage` floor adds the `coverage:check` step at the end of
   `coverage`.
2. With `docker-compose` the project may write only `stages.restore`,
   `stages.lint` and `stages.coverage`: a `build`, `test` or `package`
   command is refused (compose runs them).
3. The first failing step stops the project, naming the stage and step.
4. The plan is printed on stderr before the confirmation, every step per
   stage:

   ```text
   api 1.4.0 (default channel, tag v1.4.0, build docker-compose)
     build
       compose:build  docker compose -f docker-compose.yml build
     test
       compose:test  docker compose -f docker-compose.yml run --rm tests
     package
       compose:save ghcr.io/org/api:1.4.0  docker save -o /tmp/.../outputs/ghcr.io_org_api_1.4.0.tar ghcr.io/org/api:1.4.0
   ```

The build provider's own commands run **without a shell**: each is an argument list
given to `docker` (or `git` for the submodules), found on `PATH`, so paths
with spaces need no quoting on any OS; a missing program fails the step
with `<program> not found on PATH`. An image reference, once its
placeholders are expanded, must hold only `A-Z a-z 0-9 . _ / : @ -`.
Stage commands and hooks run through their `shell` as described in
[How a command runs](#how-a-command-runs).

Every step also receives `NERV_RELEASE_ARTIFACTS_DIR`, a per-project
directory outside the repository for build outputs, and
`NERV_RELEASE_OUTPUTS_DIR` (`$NERV_RELEASE_ARTIFACTS_DIR/outputs`), where
the package stage writes what goes into the artifact; with `coverage`
also `NERV_RELEASE_COVERAGE_DIR`, the directory the floor check searches.
The outputs directory, and the default coverage directory, are emptied
before the first stage of every run, so a report or package left by an
earlier run is never counted or packaged.

### Coverage

With `coverage`, the `coverage:check` step reads every Cobertura
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

Before the pipeline, `nerv release` writes the release into each
`version_files` entry in the release clone, so the packages carry it,
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
| `manifest.json` | Schema `nerv.release-artifact/v1`: `project`, `version`, `channel`, `tag`, `base_commit` (the origin commit built), `branch`, `build_provider` (the build provider that built it), `created_at`, `outputs` (`path`, `kind` `image-tar`, `file` or `dir`, `sha256`), `images` (every saved image reference, each tag of a mapping entry included) and `release_files` (`path` in the repository, `stored` in the artifact, `sha256`). |
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
| `NERV_RELEASE_REPO_ROOT` | Absolute repository root (under `nerv release`, the release clone). |
| `NERV_RELEASE_STAGE` | The stage or hook key: `test`, `before_test`, `prepare`, ... |
| `NERV_RELEASE_VERSION` | Next version, e.g. `1.2.0-alpha.2`. |
| `NERV_RELEASE_CHANNEL` | Release channel (`alpha`, `rc`, ...); empty for the default channel. |
| `NERV_RELEASE_TAG` | Next tag, e.g. `api-v1.2.0`. |
| `NERV_RELEASE_GIT_HEAD` | The origin commit being built. |
| `NERV_RELEASE_LAST_VERSION` | Last release on the channel; empty when none. |
| `NERV_RELEASE_NOTES_FILE` | File holding the release notes (Markdown). |
| `NERV_RELEASE_BRANCH` | Branch being built. |
| `NERV_RELEASE_DRY_RUN` | `true` under `nerv release --dry-run`, else `false`. |
| `NERV_RELEASE_ARTIFACTS_DIR` | Per-project directory for build outputs, outside the repository (see [Build providers](#build-providers)). |
| `NERV_RELEASE_OUTPUTS_DIR` | `$NERV_RELEASE_ARTIFACTS_DIR/outputs`, emptied before the run: what the package stage writes there becomes the artifact's `outputs/`. |
| `NERV_RELEASE_COVERAGE_DIR` | With `coverage`, the directory the coverage check searches for `*.cobertura.xml` reports (`coverage.dir` in the clone, or `$NERV_RELEASE_ARTIFACTS_DIR/coverage`); not set otherwise. |

## Security notes

### Credentials

`nerv release` runs with your own credentials; nothing is stored by NERV. It
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

`nerv version next` needs no `GH_TOKEN` or `NPM_TOKEN`, only read access
to origin.

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
- Only origin is built: `nerv release` never pushes your local commits and
  refuses a local branch that differs from origin's; it plans from your
  local `release:` section, applied to origin's code.

## Troubleshooting and recovery

### Refusals and environment errors

Nothing has been built or written in any of these cases.

| Message (abridged) | Cause | What to do |
|---|---|---|
| `your local <b> (...) is not origin/<b> (...)` | `nerv release` builds what is on origin. | `git push` or `git pull`, then run again. |
| `the repository has no release: section` / `release.projects[N].build: required by nerv release` | `nerv release` needs a build provider per project. | Add the section (`/release:setup`), `release.build`, or the project's `build`. |
| `release.<key>: moved to release.providers.conventional-commits.<key>` / `release.projects[N].build.<key>: moved to ...` | The section has the keys of an earlier alpha. | Move each key where the message says (`nerv migrate`). |
| a 4.2.0 `release:` section | The section has a 4.2.0 key (`semantic_release:`, a preset): today's section has neither. | `nerv migrate`, or by hand ([Migrating](#migrating-from-the-420-release-section-and-semantic-release)). |
| `release.projects[N].presets: the presets were removed ...` (or `.dotnet`, `.docker`, ...) | The section still uses a removed preset. | Write what the message says ([Migrating from the presets](#migrating-from-the-presets)). |
| `the artifact <dir> already exists: pass --force to replace it` | That version was already packaged. | `--force` to rebuild it, or deploy the existing one. |
| `--version X: project <p> already has Y (tag ...)` | An explicit version must be newer than the project's last release on the branch. | Pass a newer version. |
| `the release: section names paths that are not on origin's <b>: "<path>" (project <p>, <field>)` | The section is your local one; the path exists only locally (or not at all). | Push the code that adds it, or fix the section. |
| `branch <b> is not on origin` / `has no origin remote` | Nothing on origin to build. | Push the branch (or add the remote). |
| `the computed release is not the expected one (<project>: expected X, computed Y; ...)` | `--expect` no longer matches: origin moved since the dry run. | Run the dry run again, review the new plan, then package with its versions. |
| `confirmation needed but stdin is not an interactive terminal` | No terminal for `[y/N]`. | `--yes` (after a `--dry-run`), or run it in a terminal. |
| `invalid release configuration: ...` | The `release:` section (every problem listed). | Fix each line; see [Validation](#validation). |
| `unknown project "x"; the release: section declares: ...` | `--project` typo. | Use one of the listed names. |
| origin unreachable or timed out | Network, credentials or a prompt blocked by the no-prompt rules. | Fix access ([Reading origin](#reading-origin)); `nerv version next --offline` works without origin. |
| `git X.Y.Z is too old: selecting a project's commits by path needs git 2.31.0 or newer` | Path filters need `--diff-merges`. | Upgrade git. |
| `<program> not found on PATH` / missing interpreter | docker (the docker-compose build) or a stage or hook shell is missing. | Install it (Docker with compose, Git for Windows, PowerShell 7, Node.js, ...). |
| an inline pwsh command exits `3` | PowerShell older than 7.3. | Upgrade PowerShell. |

### Failures during a run

A failure stops the project: nothing is ever written to origin or to your
repository's git, and no artifact is left for the failed project (the
artifact is renamed into place only once complete). The summary (and the
`--json` `action`) says what to do: fix the cause and run `nerv release`
again. Projects packaged before the failure keep their artifacts; a rerun
refuses them unless `--force`, so run the rest with `--project`.

- The advice to run again checks origin's branch at the end of the run:
  when it moved, your local branch is now behind origin, which `nerv release`
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
- `--project` applies the dependency bumps of dependencies it does not
  release, in both commands.
- Offline (`nerv version next --offline`), a prerelease branch may compute
  another version than CI when a channel note exists only on origin.
- `version_files` takes MSBuild project files only; other version
  files (`package.json`, ...) are written by a stage command, in the clone,
  and do not reach the artifact's release files.
- Compose mode leaves the services `docker compose run` started (the test
  service's dependencies) running; add an `after_test` hook running
  `docker compose -f <file> down` when that matters.
- Commands run in their own session or process group without a
  controlling terminal, so a terminal Ctrl-C reaches them only through
  NERV, which cancels them on the first signal.
