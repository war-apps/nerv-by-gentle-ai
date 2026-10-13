# Release commands: `nerv version` and `nerv ship`

## Objective

Two repo-agnostic commands for any repository NERV works on:

- `nerv version` (`/nerv:version`): the next semantic version of a repo (or of
  each project of a monorepo) from a branch's commits, using the repo's own
  semantic-release configuration, with the outputs of
  `cycjimmy/semantic-release-action`.
- `nerv ship` (`/nerv:ship`): a full local release that replaces the CI
  workflow where it cannot or should not run: pipeline (restore, lint, build,
  test, coverage, package), version files, publish (docker, nuget, npm), tag,
  CHANGELOG and GitHub release, configurable through `.nerv/nerv.yaml` and
  hooks running node, pwsh or bash scripts. It reuses `nerv version`.

## Why

User request (2026-10-13). Some repos cannot (or should not) run their GitHub
Actions release; the reference is `dim.comprobantes.api` (semantic-release in
`package.json` `release`, branches main / `release/rc` (rc) / develop (alpha)
/ `hotfix/*`, `@semantic-release/exec` bumping the `.csproj`, docker images
tagged with version, channel and `latest`).

## Decisions (user, 2026-10-13)

- Engine: semantic-release itself, run through its programmatic API (dry run
  for `nerv version`), so every semantic-release config form is accepted
  (`.releaserc*`, `release.config.*`, `package.json` `release`, presets,
  `releaseRules`, `branches`, `extends`). Requires Node; semantic-release and
  the plugins the config names are installed at pinned versions in a NERV
  cache, never in the repo.
- Tag, CHANGELOG, version commit and GitHub release: semantic-release (real
  run inside `nerv ship`), with the repo's configuration.
- Runs locally only (developer machine or T2), with the developer's own
  credentials (`gh auth token`, `docker login`, NuGet key from env).
- `nerv ship` runs the whole pipeline the CI would: restore, lint, build,
  test, coverage, package, publish; any failing stage aborts before a tag
  exists. It works on a clean worktree of the branch.
- dotnet: write `<Version>` and `<PackageReleaseNotes>` into the project's
  `.csproj` and include it in the release commit.
- Monorepo: projects declared in `nerv.yaml` (path, optional
  `shared_paths`), each with its own version, tag (e.g. `api-v1.4.0`) and
  CHANGELOG. A commit counts for a project when it touches the project's
  path or `shared_paths`, OR its conventional scope matches the project; a
  commit can release several projects.
- Inter-project dependencies: per project `depends_on` entries, each with
  its own bump (`patch` | `minor` | `none`); a dependency's new release bumps
  the dependent accordingly, released in dependency order.

## Technical design (parent's proposal)

- Names `version` / `ship` avoid the existing `nerv release`, NERV's own
  maintainer tool.
- `nerv version [--branch <b>] [--project <p>] [--json|--format env]`:
  semantic-release dry run in a temporary `git worktree` of the branch; a
  NERV-owned node plugin filters commits per project (path OR scope) and
  applies `depends_on` bumps. Output: the 12 action outputs
  (`new_release_published`, `new_release_version`,
  `new_release_major_version`, `new_release_minor_version`,
  `new_release_patch_version`, `new_release_channel`, `new_release_notes`,
  `new_release_git_head`, `new_release_git_tag`, `last_release_version`,
  `last_release_git_head`, `last_release_git_tag`), per project.
- `nerv ship [--project <p>] [--dry-run] [--yes]`: shows `nerv version`'s
  result and asks for confirmation; runs the pipeline stages; then runs
  semantic-release for real with an injected NERV lifecycle plugin, so
  version-file writes, providers and hooks run at semantic-release's
  `prepare` / `publish` / `success` / `fail` steps.
- Config: a `release:` section in `.nerv/nerv.yaml`, read with
  `gopkg.in/yaml.v3` (the existing line editor cannot hold lists or maps).
  Built-in presets per technology (dotnet, node, docker, nuget, npm), each
  stage overridable; hooks per stage with `shell: node | pwsh | bash`,
  receiving `NERV_RELEASE_*` env vars (version, channel, tag, notes file,
  project).
- New process runner that streams output and takes env and working
  directory (the current `env.Runner` only captures output). Windows
  support is required (the team's main environment).

## Constraints

- Agnostic to the repo's language and layout; nothing NERV-specific leaks
  into the target repo (no files written except the release's own changes).
- Generated artifacts in English; conventional commits, no AI attribution.
- Do not change the existing `nerv release` maintainer command.

## Tasks

- [x] T1 — `release:` config schema in `.nerv/nerv.yaml`: projects, paths,
  `shared_paths`, scope, `depends_on` (+ bump), tag format, semantic-release
  pin and extra plugins, stages, presets, providers, hooks; parsing with
  yaml.v3, validation with clear errors, docs.
- [ ] T2 — node engine: embedded runner using semantic-release's API, pinned
  dependency cache, branch worktree, NERV commit-filter plugin (path OR
  scope), dependency bumps; output normalised to the 12 action outputs.
- [ ] T3 — `nerv version` CLI (single repo and monorepo, `--json` and
  `--format env`) plus `/nerv:version`.
- [ ] T4 — streaming script runner with env and working directory; hooks
  for node, pwsh and bash on Linux and Windows.
- [ ] T5 — pipeline stages and built-in presets (dotnet: restore, format
  check, build, test, coverage, pack; node; docker build/tag/push; nuget
  push; npm publish) and the dotnet `.csproj` version / release-notes writer.
- [ ] T6 — `nerv ship`: clean worktree, confirmation, pipeline, real
  semantic-release run with the injected lifecycle plugin, monorepo order by
  `depends_on`, `--dry-run`.
- [ ] T7 — `/nerv:ship`, user docs (`docs/release-commands.md`), CHANGELOG.

## Acceptance criteria

- On a copy of `dim.comprobantes.api`, `nerv version --branch develop` returns
  the same next version and channel as `npx semantic-release --dry-run` on
  that branch.
- A monorepo fixture releases two projects from one commit (path + scope),
  and a `depends_on` bump releases the dependent in order.
- `nerv ship --dry-run` runs every stage and writes nothing; a failing stage
  aborts before any tag.
- `go test ./...`, `go vet ./...` pass; `gofmt -l .` clean.

## Delivery

Branch `feat/release-commands` from da18e6f. Forecast well above the ~400
authored-line budget (roughly 3000+ lines across seven tasks). Strategy:
`ask-on-risk`; chain strategy (user, 2026-10-13): `stacked-to-main`
(each task's PR branches from the previous one and merges into develop in
order). Slice boundaries: one PR per task, recorded below as they open.

## Progress

Branch created from da18e6f. Engram mirror `odd/release-commands/tasks`:
pending (save failed: host session registration not confirmed).

### T1

- Route: delegated (writer trigger: 2+ non-trivial files — new package,
  docs, module files).
- Package `internal/shipconfig` (`Load(repoRoot)`, `ErrNotConfigured`,
  `ValidationError`, `Release.TopoOrder()`, `Release.Project()`), parsed with
  `gopkg.in/yaml.v3` v3.0.1 reading only the `release:` key. Unknown keys are
  rejected by a reflection walk over the yaml tags (yaml.v3 `KnownFields`
  only works on a whole-document decoder, and the rest of `nerv.yaml` must
  stay free-form). Paths are validated as written, then normalised to clean
  forward-slash form. Docs: `docs/release-commands.md`, linked from
  `docs/configuration.md`; a test keeps its first example loadable.
- Schema matches the T1 sketch with no renames; added `DockerImage.ShouldPush`
  and the reserved `NERV_RELEASE_` env prefix for hooks. Settings blocks
  without their preset enabled are rejected; `dotnet.project_file` is
  required by the dotnet preset (no auto-detect).
- RED: `go test ./internal/shipconfig/` -> `no non-test Go files ... [build
  failed]`. GREEN: same command -> `ok` (after one fix: path errors now
  quote the path as written instead of its normalised form).
- Checks: `go test ./...` ok (all packages), `go vet ./...` clean,
  `gofmt -l .` empty.
- Size: `git diff --cached --shortstat` (code, tests, docs, go.mod/go.sum)
  `9 files changed, 1445 insertions(+), 1 deletion(-)`, plus this document.
  Above the ~400-line heuristic: the schema is wide (six setting blocks,
  stages, hooks) and roughly a third is the table-driven test suite and the
  field reference; splitting it would ship a schema without its validation.
- Commit: this commit (`feat(ship): add the release configuration schema`).

## Next step

T2 (node engine) on top of T1.
