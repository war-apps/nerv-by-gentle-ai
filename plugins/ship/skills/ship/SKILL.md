---
name: ship
description: NERV ship plugin contract. Covers the ship section of .ai-workflow.yml (projects, branches, release rules, tag_format, changelog, build modes, stages, hooks, coverage), the native version engine behind nerv version next, the local artifact nerv ship leaves, the dry-run-then-yes rule and the migration from release:, pipeline: and semantic-release. Loaded by the /ship:* commands and by the ship:versioner and ship:builder agents.
---

# NERV Ship Plugin — Contract

The ship plugin computes the next version of a repository (or of each project of a monorepo), and builds, tests and packages what is on origin into a **local artifact**. It never writes to git and never publishes: the release commit, the tag, the push, the images and the git host release belong to the `deploy` plugin (`deploy:deploy` skill).

The plugin drives two `nerv` subcommands and nothing else:

| Command | Does |
|---|---|
| `nerv version next` | Computes the next version with the native engine. Read-only. |
| `nerv ship` | Builds, tests and packages in a throwaway clone of origin and writes `.nerv/artifacts/<project>/<version>/`. |
| `nerv ship setup --import [--dry-run]` | Translates an existing semantic-release config into the ship section, once. |

If `nerv` is not on PATH, tell the user to install it — `curl -fsSL https://raw.githubusercontent.com/war-apps/nerv-by-gentle-ai/main/scripts/install.sh | bash`, or `go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest` — and stop.

## 1. Rules

- **Never run semantic-release**, node or npm to get a version: the engine is native Go. The only contact with semantic-release is reading an existing config through `nerv ship setup --import`.
- **Never write to git**: no commit, no tag, no push, from this plugin. Never run `git tag`, `git push`, `docker push`, `dotnet nuget push` or `npm publish`.
- **Dry run first, always.** A real `nerv ship` runs only after the user's explicit yes to the exact versions the dry run printed, bound with `--expect` (§5).
- Agents never ask the user anything. A question goes back to the session that launched them.

## 2. Configuration

The configuration is the `ship:` section of `<repo>/.ai-workflow.yml`. Unknown keys are refused, with a path-qualified message per problem.

```yaml
ship:
  branches:                       # semantic-release's order: first match wins
    - main
    - { name: develop, prerelease: alpha, channel: alpha }
    - { name: "release/*", prerelease: rc, channel: beta }
    - { name: "hotfix/*", prerelease: beta, channel: beta }
  tag_format: "v${version}"
  changelog:
    file: CHANGELOG.md
    title: "# Changelog"
  projects:
    - name: api
      path: src/api
      shared_paths: [Directory.Build.props]
      scopes: [api]
      depends_on:
        - { project: core, bump: patch }
      build:
        mode: compose
        file: docker-compose.yml
        test_service: tests
        images: ["ghcr.io/org/api:${version}"]
        version_files: [src/api/Api.csproj]
        coverage: { line: 80 }
      stages:
        restore: { run: dotnet restore }
      hooks:
        prepare:
          - { run: ./scripts/stamp.sh, shell: bash }
```

### 2.1 Version engine keys

| Key | Default | Meaning |
|---|---|---|
| `branches` | `main`; `develop` (alpha); `release/*` (rc on `beta`); `hotfix/*` (beta) | Release branches. An entry is a name or a glob, or `{name, prerelease, channel}`. `prerelease` is an identifier, or `true` for the branch name (which must then be a valid identifier). |
| `release_rules` | none | Extra commit-analyzer rules (`type`, `scope`, `breaking`, `revert`, `release`: `major`, `minor`, `patch` or `none`). A commit no rule matches falls back to the Conventional Commits defaults. |
| `note_keywords` | `BREAKING CHANGE`, `BREAKING CHANGES`, `BREAKING` | Keywords that start a breaking-change note. |
| `tag_format` | `v${version}` (one project); `<name>-v${version}` (several) | The tag. It holds `${version}` and no other placeholder, and is unique across projects. |
| `repository_url` | origin's URL, without credentials | What the release notes link to. |
| `changelog.file`, `changelog.title` | `CHANGELOG.md`, none | The CHANGELOG file under each project's path, and its title. |

The engine follows semantic-release with the `conventionalcommits` preset for these behaviours: the bump, the prerelease numbering per channel (it reads the `refs/notes/semantic-release-<tag>` channel notes), the tag, the release notes and the CHANGELOG section. It does not support maintenance branches (`1.x`, ranges), `addChannel`, `extends` or other presets; the importer warns about each.

### 2.2 Project keys

| Key | Default | Meaning |
|---|---|---|
| `projects[].name` | required | `[a-z0-9][a-z0-9-]*`, unique. |
| `projects[].path` | required | The project's folder; `.` is the whole repository. A commit touching it counts for the project. |
| `projects[].shared_paths` | none | Extra paths or globs whose changes count too. |
| `projects[].scopes` | `[<name>]` | Conventional-commit scopes that count wherever they touch. |
| `projects[].tag_format`, `projects[].changelog` | from §2.1 | Per-project overrides. |
| `projects[].depends_on` | none | `{project, bump}`: a release of `project` gives this one at least `bump` (`patch`, `minor`, `major` or `none`; default `patch`). |
| `projects[].submodules` | `false` | Initialize submodules in the clone before `restore`. |

No `ship:` section means one project, the whole repository, for `nerv version next`; `nerv ship` needs the section, because every project needs `build.mode`.

### 2.3 Build

`build.mode` is required for every project and has no default. There are no presets: in `script` mode the project writes every stage command.

| Key | Mode | Meaning |
|---|---|---|
| `build.mode` | both | `compose` or `script`. |
| `build.file` | compose | The compose file. |
| `build.test_service` | compose | The service `docker compose run --rm` runs for the tests. |
| `build.images` | compose | The `image:` names it builds, with `${version}`; they are saved into the artifact. |
| `build.shared_host` | script | Disables the reused dotnet build servers on a shared host. |
| `build.version_files` | both | `.csproj`/`.props` files whose `<Version>` and `<PackageReleaseNotes>` are written in the clone. |
| `build.coverage` | both | Floors `{line, branch, method}` over the Cobertura reports in `$NERV_RELEASE_COVERAGE_DIR`, or `dir` where a compose test service writes them. |

**Stages** run in this order: `restore`, `lint`, `build`, `test`, `coverage`, `package`. In `script` mode each one the project uses is a command `{run, shell, env, workdir, timeout}` (`shell`: `bash`, `pwsh` or `node`). In `compose` mode only `restore`, `lint` and `coverage` may be written: compose builds, tests (`test_service`) and packages (`docker save` of `images`) itself. The `package` stage writes what the artifact keeps into `$NERV_RELEASE_OUTPUTS_DIR`. There is no `publish` stage.

**Hooks**: `before_<stage>`, `after_<stage>`, `prepare` (after the pipeline, before the artifact is written) and `fail`, each a list of commands. Publishing hooks belong to deploy.

## 3. `nerv version next`

```
nerv version next [--branch <name>] [--project <name>] [--offline] [--fetch-timeout <dur>] [--format text|json|env | --json]
```

It reads origin's tags and channel notes into a throwaway copy (never the repository), then computes. `--offline` uses the local tags and notes only, and may differ from CI on prerelease branches. With several projects and no `--project`, `json` is an array in dependency order; otherwise one object. Each entry has `project`, `branch` and `new_release_published`, `new_release_version`, `new_release_channel`, `new_release_git_tag`, `new_release_notes`, `last_release_version`, `last_release_git_tag` and the other outputs, with the same names and meaning they had. Exit codes: 1 a refusal (invalid section, unknown project, not a git repository, an old `release:` section); 2 a usage or environment problem (git missing, origin unreachable: suggest `--offline`); 130, 143 or 129 interrupted.

## 4. The artifact

`nerv ship` leaves one artifact per packaged project, outside the clone, in the user's repository:

```
.nerv/artifacts/<project>/<version>/
  manifest.json          schema nerv.ship-artifact/v1: project, version, channel, tag,
                         base_commit, branch, build_mode, created_at, outputs, images,
                         release_files, and the sha256 of every file
  notes.md               the release notes
  CHANGELOG.section.md   the CHANGELOG section
  outputs/               the packaged outputs (image tars, packages, directories)
  release-files/         the new contents of the files the release changes
```

`.nerv/artifacts/` ignores itself, so the user's `.gitignore` is never changed. An artifact that is never deployed leaves no trace in the repository. `nerv deploy --artifact <dir>` consumes it.

## 5. `nerv ship`

```
nerv ship [--branch <name>] [--project <name>] [--version <semver>] [--dry-run] [--yes] [--expect <project>=<version> ...] [--force] [--json]
```

- It builds **what is on origin**: the local branch must equal origin's. The `ship:` section is the local `.ai-workflow.yml`, committed or not.
- `--version <semver>` uses that version instead of computing it. It must be newer than the project's last tag, needs `--project` in a monorepo, and takes its channel from the `branches` entry of the branch.
- The dry run is real work in the clone (every stage and the `prepare` hooks) and writes no artifact. The real run needs `--yes` (its own `[y/N]` prompt cannot be answered from an agent's shell) and one `--expect <project>=<version>` per project the dry run would package: when origin moved and the versions differ, it refuses before any stage.
- An existing artifact of the same version is refused with `pass --force to replace it`. Never add `--force` on your own.
- stdout is one JSON document `{status, error, projects}`; stderr carries progress and each project's plan. `status`: `dry-run`, `packaged`, `nothing-to-release`, `declined`, `refused`, `failed`, `interrupted`. Each project: `project`, `branch`, `status` (`dry-run`, `packaged`, `no-release`, `failed`, `not-started`), `version`, `channel`, `tag`, `last_version`, `artifact`, `error`, `action`.
- Exit codes: 0 packaged, nothing to release, or a dry run; 1 refused (invalid section, a project without `build.mode` or with a preset key, local branch not origin's, an unknown project, an existing artifact); 2 a step or the environment failed; 128 + the signal interrupted.

## 6. Migration

- **`release:` section.** It is no longer read: commands refuse with a message pointing to `/ship:setup`. `nerv migrate` rewrites it into `ship:`; by hand, rename the key, drop `semantic_release:` (no node runner is left), add `build.mode` to every project, and move the branches and rules from the semantic-release config into the section (`nerv ship setup --import`).
- **Presets** (`presets:`, `dotnet:`, `node:`, `docker:`, `nuget:`, `npm:`, a `publish` stage, `publish` or `success` hooks) are removed. Write the stage commands in `script` mode or use `compose`; publishing goes to the `deploy:` section.
- **`pipeline:` section and `nerv pipeline`.** Removed. `validate` and `test` become ship stages; `release` is `nerv ship` then `nerv deploy`; `docker` push and `deploy` are the `deploy-ssh-compose` target; the `sync` back-merge is not ported.
- **semantic-release config** (`package.json` `release`, `.releaserc.json`, `.releaserc.yaml`/`.yml`). `nerv ship setup --import` translates it once and lists what it cannot translate: the `npm` and `exec` plugins, `presetConfig.types`, `extends`, maintenance branches. `@semantic-release/git` and `@semantic-release/github` go to the `deploy:` section. A `.js` config is never imported: it is reported, and the user migrates it by hand. After the import the semantic-release config is no longer used and can be removed.
