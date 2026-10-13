# Release configuration (`release:`)

The `release:` section of the project-scope `<repo>/.nerv/nerv.yaml`
configures how a repository is versioned and released. It is read by the
release commands `nerv version next` (the next version of each project,
from a semantic-release dry run; see [below](#nerv-version-next)) and
`nerv ship` (a full local release: pipeline, publish, tag, CHANGELOG and
GitHub release; **coming in a later task**).

The section is project scope only: it is never read from the user-scope
`~/.claude/nerv/nerv.yaml` and never merged with it. When the section is
absent, the repository simply has no release configuration.

The versioning rules themselves (branches and channels, `releaseRules`,
presets, plugins) stay in the repository's own semantic-release
configuration (`.releaserc*`, `release.config.*` or `package.json`
`release`). `release:` adds what semantic-release does not know: which
projects a monorepo holds, how they depend on each other, and how each one
is built and published.

## `nerv version next`

```
nerv version next [--cwd <dir>] [--branch <name>] [--project <name>]
                  [--offline] [--fetch-timeout <dur>] [--format text|json|env | --json]
```

Computes the next semantic version of the repository, or of each project of
its `release:` section, with the repository's own semantic-release
configuration in dry-run mode (see
[How the next version is computed](#how-the-next-version-is-computed)).
Nothing in the repository changes. In Claude Code, `/nerv:version [args]`
runs it with `--json` and shows a table per project.

Plain `nerv version` (and `nerv version --json`) still prints the binary and
plugin versions.

| Flag | Default | Meaning |
|---|---|---|
| `--cwd <dir>` | `.` | Any directory inside the repository; the git top level is used. |
| `--branch <name>` | current branch | Local branch to compute. |
| `--project <name>` | all projects | Only this project of the `release:` section (its dependencies are still computed, for `depends_on`). An unknown name is refused, listing the declared projects. |
| `--offline` | off | Use the local tags and notes only. By default origin's release tags and semantic-release notes are read first, with your git credentials, into a throwaway copy. Offline, a prerelease branch may compute a different version than CI when a release's channel note exists only on origin. |
| `--fetch-timeout <dur>` | `60s` | Bound for reading origin (Go duration: `30s`, `2m`); must be positive, there is no unbounded mode (use `--offline` to skip origin). |
| `--format text\|json\|env` | `text` | Output format. |
| `--json` | | Same as `--format json`. |

Requirements: git (2.31 or newer when a project's `path` is not `.`),
Node.js 20.8.1 or newer and npm (semantic-release and its plugins are
installed in the NERV cache, see [Installed plugins](#installed-plugins)).

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

### Exit codes and errors

| Code | When |
|---|---|
| `0` | Computed, including when there is no new release (`new_release_published=false`). |
| `1` | Refused: an invalid `release:` section (every problem, path-qualified), an unknown `--project`, `--project` without a `release:` section, a directory outside any git repository. |
| `2` | Usage error, or an environment error: git, node or npm missing or too old, a plugin not pinned, origin unreachable or timed out (the message suggests `--offline`), no result for the requested project, the result could not be written to stdout. |
| `128 + signal` | Interrupted: `130` for Ctrl-C (SIGINT), `143` for SIGTERM, `129` for SIGHUP. |

Ctrl-C, `SIGTERM` and (on Unix) `SIGHUP` cancel the run and stop git, npm
and node, which run in their own session or process group. After the first
signal NERV stops catching them, so a second Ctrl-C ends `nerv` at once.

## Paths

Every path in the section is relative to the repository root, whatever the
project's own `path` is. Forward slashes are canonical; backslashes are
accepted and normalised. Absolute paths (`/x`, `C:\x`) and paths that
escape the repository (`../x`) are rejected.

## Example: dotnet API with a docker image

A single-project repository shaped like `dim.comprobantes.api`: the API's
`.csproj` receives the version, and one docker image is tagged with the
version, the release channel (`alpha` on develop, `rc` on `release/rc`) and
`latest` (only on releases of the default channel, main).

```yaml
release:
  semantic_release:
    version: "24.2.3"
    extra_plugins: ["@semantic-release/exec@6.0.3"]
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
Split the same repository into `api` and `worker` projects and the tags
become `api-v1.4.0` and `worker-v2.0.1`, each with its own CHANGELOG under
its `path`.

## Field reference

### `release.semantic_release`

| Field | Default | Meaning |
|---|---|---|
| `version` | `24.2.3` | Exact semantic-release version installed in the NERV cache (never in the repo). Ranges such as `^24` are rejected. |
| `extra_plugins` | `[]` | npm packages the repo's semantic-release configuration needs beyond the bundled plugins, each pinned as `name@version` (e.g. `@semantic-release/exec@6.0.3`). |

#### Installed plugins

NERV installs semantic-release with its plugins in
`<user cache dir>/nerv/semantic-release/<hash>` (on Linux
`~/.cache/nerv/...`, on Windows `%LocalAppData%\nerv\...`), once per set of
versions, with `npm install --ignore-scripts`. Node.js 20.8.1 or newer and
npm are required. Always available:

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

#### How the next version is computed

- semantic-release runs in dry-run mode with the repository's own
  configuration; only the commit analysis and the release notes run, exactly
  as configured. The steps that check credentials or have side effects
  (`verifyConditions`, `verifyRelease`, `prepare`, `publish`, `success`,
  `fail`) are skipped, so no `GH_TOKEN`, `NPM_TOKEN` or exec script is
  needed.
- It runs on a disposable mirror of the repository (all local branches and
  tags), checked out on the requested local branch: the working tree,
  branches and tags of the repository are never changed.
- Release tags and semantic-release's git notes are read from the `origin`
  remote first (read access is enough). semantic-release records each
  release's channel in a note that a plain `git fetch` never brings; without
  it a prerelease tag such as `v2.9.0-alpha.8` is not recognised as the last
  `alpha` release. Offline, only the local tags are used and the result may
  differ from CI on prerelease branches.
- That read never prompts and is bounded: it runs with
  `GIT_TERMINAL_PROMPT=0`, `GCM_INTERACTIVE=never`, no standard input and
  ssh in batch mode (`-o BatchMode=yes` is appended to `GIT_SSH_COMMAND`,
  or to `core.sshCommand`, when that command is `ssh`; a non-ssh command
  such as `plink`, or a `GIT_SSH` program, is used as it is and must not
  prompt by itself), and it fails after 60 seconds (`--fetch-timeout`). Credentials must
  therefore come from a credential helper, an ssh agent or an unencrypted
  key. The origin URL never appears on a command line, and credentials
  embedded in it (`https://user:token@host/...`, `?access_token=...`) are
  hidden in every error. A relative local origin path is resolved against
  the repository.
- In a monorepo, each project is computed in dependency order with its own
  `tag_format`, from the commits that touch its `path` or `shared_paths` or
  carry one of its `scopes` (`type(scope): ...`, several scopes separated by
  commas). A merge commit touches what it brings into the branch (its
  changes against its first parent); the merged commits count on their own.
  This needs git 2.31 or newer: with an older git, a release with a
  project `path` other than `.` stops before computing anything, with an
  error naming the git found and the minimum. A project whose `path` is `.` sees every
  commit. When a dependency gets a new release, the dependent is released
  with at least the dependency's `bump`, even without commits of its own. A single project with the default `tag_format` keeps
  the repository's own `tagFormat`, and a repository without a `release:`
  section is computed exactly like plain semantic-release.
- The result per project is the outputs of
  `cycjimmy/semantic-release-action` (`new_release_published`,
  `new_release_version`, `new_release_major_version`,
  `new_release_minor_version`, `new_release_patch_version`,
  `new_release_channel`, `new_release_notes`, `new_release_git_head`,
  `new_release_git_tag`, `last_release_version`, `last_release_git_head`,
  `last_release_git_tag`); the `last_release_*` values are also given when
  there is no new release.

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
| `stages` | preset commands | Override or add a pipeline stage command; see below. |
| `hooks` | `{}` | Extra scripts around stages and release steps; see below. |

### `depends_on[]`

| Field | Default | Meaning |
|---|---|---|
| `project` | required | Another project of the section (not itself). |
| `bump` | `patch` | What a new release of that project does to this one: `patch`, `minor` or `none` (no bump, but still released after it). |

Dependencies must not form a cycle; the error names it
(`dependency cycle a -> b -> a`). Projects are released in dependency
order, otherwise in declaration order.

### Presets

| Block | Field | Default | Meaning |
|---|---|---|---|
| `dotnet` | `project_file` | required | The `.csproj` that receives `<Version>` and `<PackageReleaseNotes>` in the release commit. |
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

### `stages`

Keys are pipeline stages, run in this order: `restore`, `lint`, `build`,
`test`, `coverage`, `package`, `publish`. A key replaces the preset's
command for that stage, or adds one when no preset provides it. Any
failing stage aborts the release before a tag exists.

```yaml
stages:
  test: { run: "dotnet test --no-build", shell: bash }
```

### `hooks`

Keys are `before_<stage>` / `after_<stage>` (for any stage above) and the
semantic-release steps `prepare` (before the release commit; write version
files here), `publish`, `success` and `fail`. Each key holds a list of
commands run in order.

### Commands (stage overrides and hooks)

| Field | Default | Meaning |
|---|---|---|
| `run` | required | Command line or script path. |
| `shell` | `bash` | `node`, `pwsh` or `bash`. |
| `env` | `{}` | Extra environment variables. Names starting with `NERV_RELEASE_` are reserved for the variables NERV passes (version, channel, tag, notes file, project). |

## Validation

Unknown keys are errors, and every problem (unknown keys included) is
reported at once with its path in the section, for example:

```text
invalid release configuration:
  release.projects[0].pth: unknown field (line 5)
  release.projects[0].path: required
  release.projects[1].depends_on[0].project: unknown project "lib2"
```

Only a section that cannot be decoded at all (a value of the wrong kind,
such as `presets: docker`) stops at the decoding problems.

## YAML notes

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
