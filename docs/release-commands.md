# Release configuration (`release:`)

The `release:` section of the project-scope `<repo>/.nerv/nerv.yaml`
configures how a repository is versioned and released. It is read by the
release commands `nerv version` (the next version of each project, from a
semantic-release dry run) and `nerv ship` (a full local release: pipeline,
publish, tag, CHANGELOG and GitHub release). **Both commands are coming in
later tasks**; today the section is parsed and validated only.

The section is project scope only: it is never read from the user-scope
`~/.claude/nerv/nerv.yaml` and never merged with it. When the section is
absent, the repository simply has no release configuration.

The versioning rules themselves (branches and channels, `releaseRules`,
presets, plugins) stay in the repository's own semantic-release
configuration (`.releaserc*`, `release.config.*` or `package.json`
`release`). `release:` adds what semantic-release does not know: which
projects a monorepo holds, how they depend on each other, and how each one
is built and published.

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
  prompt by itself), and it fails after 60 seconds. Credentials must
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
  This needs git 2.31 or newer. A project whose `path` is `.` sees every
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
