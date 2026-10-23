---
description: Set up the release section of .nerv/nerv.yaml interactively — detects the repository's semantic-release config, projects, compose files, Dockerfiles and scripts, picks each project's build mode (compose or script), asks only what it cannot detect, writes the section and validates it with nerv version next
argument-hint: [--project <name>]
---

# /nerv:release-setup

Build the `release:` section of `<repo>/.nerv/nerv.yaml` for `nerv version
next` and `nerv ship` (with each project's `build.mode`): detect what the
repository already has, propose a
complete section, ask only what cannot be detected (one question at a
time), write it, and prove it with `nerv version next --offline`. It never
commits, pushes or releases.

The source of truth for every field, default and rule is
`docs/release-commands.md` → Configuration (`release:`). Never invent a
field it does not list: unknown keys are errors.

**Arguments.** `$ARGUMENTS` may be empty or `--project <name>` (a non-empty
value not starting with `-`). With `--project`, only that project of an
existing section is added or edited; every other project stays as it is.
Anything else: say what is not accepted and stop.

Talk to the user in their language; everything written to files (YAML,
comments, JSON) is in English.

## 1. Preconditions

1. Run `nerv version`. If `nerv` is not on PATH, tell the user to install
   it — `curl -fsSL
   https://raw.githubusercontent.com/war-apps/nerv-by-gentle-ai/main/scripts/install.sh
   | bash`, or `go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest`
   — and stop.
2. Run `git rev-parse --show-toplevel`. On failure, say the release
   commands need a git repository and stop. Every path below is relative
   to that top level.
3. Read `.nerv/nerv.yaml` if it exists. If it already has a top-level
   `release:` key, show that block as it is and ask one question: **edit
   it** (keep it as the starting point; the default) or **start over**
   (replace it). Stop and wait. Never overwrite it silently.

The file's other content is never touched: only the `release:` block is
added (at the end of the file, after one blank line) or replaced (from the
`release:` line up to the next line that starts at column 0 with a
non-space, non-`#` character, or the end of the file). Keep every other
byte as it is — the rest of the file is managed by `nerv configure`'s
line-based editor and must not be reformatted. No `.nerv/nerv.yaml`
yet: create it holding only the section, and mention that `/nerv:init`
adds the rest of NERV's configuration.

## 2. Detect (read-only)

Use your file tools and read-only git commands only. Nothing is installed,
built or run.

| Look at | To learn |
|---|---|
| `.releaserc`, `.releaserc.{json,yaml,yml,js,cjs,mjs}`, `release.config.{js,cjs,mjs}`, `package.json` `release` | Whether semantic-release is configured; its `branches` (channels); every plugin, `extends` and `preset` it names; `@semantic-release/changelog` `changelogFile`; `@semantic-release/git` `assets`; `@semantic-release/npm` `npmPublish`; `@semantic-release/exec` commands. |
| `.github/workflows/*.yml` | The semantic-release version and plugin versions CI installs; the build, test, coverage and package commands CI runs (the script stages), with their flags (e.g. `-warnaserror`); docker build steps (image names, registry, Dockerfiles, context, build args). |
| `docker-compose*.yml`, `compose*.yaml` | Whether compose builds the project: its services with a `build:` key and their `image:` names (the `build.images`), and a service that runs the tests (the `build.test_service`). |
| `*.csproj`, `*.sln`, `*.slnx` | dotnet projects: the deployable one (the one CI versions or a bump script edits), packable libraries (`<IsPackable>`, `<PackageId>`, `dotnet pack` in CI), test projects (not release targets). |
| `Dockerfile`, `*.Dockerfile`, `docker/` | Images and their `ARG`s (e.g. `APP_VERSION`). |
| `package.json` (each project) | Scripts `lint` / `build` / `test` (candidates for the script stages); the package manager from `packageManager`, else the lockfile (`pnpm-lock.yaml`, `yarn.lock`, `package-lock.json`), for the restore command (`npm ci`, `pnpm install --frozen-lockfile`, ...). |
| Release scripts (`scripts/*`, exec commands, CI steps) | Steps nerv ship covers (a bump-csproj script: `build.version_files` writes `<Version>` and `<PackageReleaseNotes>`), steps that become stages, and the rest (candidates for hooks). Publishing steps (docker push, nuget push, npm publish) are not ship's: they belong to `nerv deploy`. |
| Layout and `git log --format=%s -200` | Monorepo or not: several deployable units in separate folders, and the conventional-commit scopes used per folder. |

Classify every package the semantic-release configuration names:

- bundled with semantic-release: `@semantic-release/commit-analyzer`,
  `@semantic-release/release-notes-generator`, `@semantic-release/npm`,
  `@semantic-release/github`, `conventional-changelog-angular`;
- pinned by NERV: `@semantic-release/changelog`, `@semantic-release/git`,
  `@semantic-release/exec`, `conventional-changelog-conventionalcommits`
  (versions in `docs/release-commands.md` → Installed plugins);
- anything else (another plugin, an `extends` config, a
  `conventional-changelog-<preset>`, a `releaseRules` module) must go into
  `semantic_release.extra_plugins` as `name@<exact version>`. Take the
  version from CI when it pins one; otherwise it is a question (step 4).

When CI pins semantic-release itself to an exact version other than the
default, propose it as `semantic_release.version` so both compute alike.

## 3. Propose the complete section

Build the whole `release:` section from the detection and show it as one
YAML block, then a short list with one line of reason per non-obvious
choice (why this project file, why a stage override, why a hook). Rules:

- **Projects.** One project with `path: .` for a single deployable repo;
  one per deployable unit in a monorepo, each with its `path`, its
  `scopes` when the log uses scopes other than the project name, and
  `shared_paths` for shared build files (`Directory.Build.props`,
  `Directory.Packages.props`, a shared library folder). Names match
  `[a-z0-9][a-z0-9-]*`. Keep the default `tag_format` unless the
  repository's existing tags use another one (then match them).
- **`changelog`**: in a monorepo each project writes its own changelog
  (default `<path>/CHANGELOG.md`, applied to the changelog and git
  plugins); set it only to use another file. A single project keeps the
  changelog plugin's own `changelogFile`: leave `changelog` unset unless
  it should override it.
- **`build.mode`** for every project; it is required and has no default.
  `compose` when a compose file builds the project's images (with `file`,
  `test_service` when a service runs the tests, and `images` listing the
  `image:` names it builds, with `${version}` where the tag is the
  version); otherwise `script`. Both fit (a compose file and plain build
  commands): that is a question (step 4).
- **Script stages** (`stages:`) copy the commands CI runs, one per stage
  (`restore`, `lint`, `build`, `test`, `coverage`, `package`), with CI's
  flags; leave out a stage CI does not run. The `package` stage writes
  what the artifact must hold to `$NERV_RELEASE_OUTPUTS_DIR` (`dotnet pack
  -o "$NERV_RELEASE_OUTPUTS_DIR"`, `npm pack --pack-destination
  "$NERV_RELEASE_OUTPUTS_DIR"`, `docker save -o
  "$NERV_RELEASE_OUTPUTS_DIR/<name>.tar" <image>` after a `docker build`).
  Never a publish step (push, publish, upload): there is no `publish`
  stage, and publishing belongs to `nerv deploy`. In compose mode only
  `restore`, `lint` and `coverage` may be written.
- **`build.version_files`**: the `.csproj` (or `.props`) files CI or a bump
  script versions.
- **`build.coverage`** when CI enforces coverage: `{ line: 80 }`, or
  `{ line: 80, branch: 70, method: 85 }` for coverlet's floors; the test
  stage must write the Cobertura reports to `$NERV_RELEASE_COVERAGE_DIR`
  (e.g. `--collect "XPlat Code Coverage" --results-directory
  "$NERV_RELEASE_COVERAGE_DIR"`), or set `coverage.dir` to where a compose
  test service writes them through a bind mount.
- **`build.shared_host: true`** (script mode) when dotnet runs on a shared
  host where reused build servers crash it; do not copy the build-server
  `env` block (`MSBUILDDISABLENODEREUSE`, `UseSharedCompilation`,
  `DOTNET_CLI_USE_MSBUILD_SERVER`) into each stage. A pinned .NET SDK
  becomes a `before_restore` hook writing `global.json`.
- **An existing section with `presets:`** (or `dotnet:`, `node:`,
  `docker:`, `nuget:`, `npm:`, a `publish` stage, `publish` or `success`
  hooks): rewrite it as `docs/release-commands.md` → Migrating from the
  presets says, and list what was moved where; publishing settings are
  dropped and noted for `nerv deploy`.
- **`submodules: true`** on the project when the repository has a
  `.gitmodules` file: the release clone has no submodules otherwise. Do
  not add a `git submodule update` hook for it.
- **Hooks** for repository-specific steps no stage covers, with the right
  `shell` (`bash` for `.sh`, `pwsh` for `.ps1`, `node` for `.js`): around a
  stage (`before_<stage>`, `after_<stage>`), `prepare` (after the
  pipeline, before the artifact) or `fail`. A script
  the repository's own semantic-release configuration already runs (an
  exec `prepareCmd`) keeps running there: do not duplicate it as a hook.
- **`depends_on`** only when one project consumes another's package or
  code (a project reference into another project's `path`).

## 4. Ask only what cannot be detected

Ask ONE question at a time, each with a recommended default, then STOP and
wait for the answer before the next one. Never assume an answer. Skip any
question the detection already answered. Typical questions:

- the build mode, when both `compose` and `script` fit;
- the compose test service, when no service clearly runs the tests;
- the image names, when the compose file or CI does not show them;
- what the `package` stage must leave in the artifact, when CI packages
  nothing detectable;
- the coverage floors, when CI enforces none or a non-line one;
- `depends_on` between projects, and its `bump` (`patch`, `minor`, `none`);
- whether a detected script stays a hook, becomes a stage, or is dropped
  because nerv ship covers it;
- the exact version of a plugin that must go into `extra_plugins`.

After the answers, show the final section again in full and ask once
whether to write it. Write only on an explicit yes.

## 5. No semantic-release configuration

When the repository has none, say that without one every commit analysis
uses semantic-release's defaults, and offer (one yes/no question) to
create a minimal one: in `package.json` `release` when there is a
`package.json`, else `.releaserc.json`. Model it on:

```json
{
  "branches": [
    "main",
    { "name": "release/rc", "channel": "rc", "prerelease": "rc" },
    { "name": "develop", "channel": "alpha", "prerelease": "alpha" }
  ],
  "plugins": [
    ["@semantic-release/commit-analyzer", { "preset": "conventionalcommits" }],
    ["@semantic-release/release-notes-generator", { "preset": "conventionalcommits" }],
    "@semantic-release/changelog",
    ["@semantic-release/git", { "assets": ["CHANGELOG.md"] }],
    "@semantic-release/github"
  ]
}
```

Adjust branch names to the repository's (`master`, no `release/rc`) and
add to the git `assets` each project's CHANGELOG. Create it only on an
explicit yes, and say it must be committed and pushed: `nerv ship` computes
the versions with the semantic-release configuration on origin's branch
(only `.nerv/nerv.yaml` is read locally).

## 6. Write and validate

1. Write the section into `.nerv/nerv.yaml` as described in step 1.
2. Run `nerv version next --offline --format json`, and in a monorepo also
   `nerv version next --offline --format json --project <name>` per
   project. These need node and npm (semantic-release runs in a NERV
   cache); when they are missing, say so, keep the file written and stop.
3. **Exit 1 with `invalid release configuration:`**: show the
   path-qualified lines as they are, fix the section, write it again and
   rerun. At most two fix rounds; if it still fails, stop and show the
   remaining problems. **Exit 2** naming a package not pinned in
   `extra_plugins`: ask for its version (step 4) and rerun. Any other
   exit 2: show stderr and stop.
4. On success, show one row per project: project, next version (or "no
   new release"), channel, tag, last release. Say the result used local
   tags only (`--offline`).

## 7. Next steps

Tell the user, without doing any of it:

1. Review `.nerv/nerv.yaml`: `nerv ship` uses this local file as it is,
   committed or not. Committing it is recommended so the team shares it,
   but not required. If step 5 created a semantic-release configuration,
   commit and push that: the versions are computed from origin's.
2. Then run `/nerv:ship` — it always rehearses with a dry run first and
   packages the artifact only after an explicit yes; it never commits,
   tags or pushes.

Never commit, push, run `nerv ship` or edit any file other than
`.nerv/nerv.yaml` and, on an explicit yes, the semantic-release
configuration.
