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
  is parsed instead.
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
  it; that is reported as an error.
- A file whose top level is not a mapping is an error; an empty file, or
  one without `release:`, means the repository has no release
  configuration.
