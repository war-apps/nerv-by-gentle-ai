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

### T1 review

- Commit `f2f7801` (recorded by the parent). Native review (high risk, 4
  lenses, user granted): approved and acknowledged. Advisory findings:
  R3-early-return-unknown-fields (WARNING: an unknown key hides the other
  errors), R3-unrelated-yaml-error (a strict-YAML error elsewhere in
  nerv.yaml fails a repo with no release section), R4-root-not-mapping-silent,
  R3-null-release-untested, R3-coverage-nan, R3-merge-key-rejected,
  R1-alias-walk-amplification, R2-unpinned-plugin-hint,
  R2-inline-placeholder-set, R2-scattered-defaults.

### T1 follow-ups

All ten advisory findings fixed (user authorized). Route: delegated (one
writer; 2+ non-trivial files). Test-first: the new tests were added before
the fix and observed failing.

| Finding | Fix | Evidence |
|---|---|---|
| R3-early-return-unknown-fields | `parse` collects unknown keys and semantic problems into one `*ValidationError`; only a decode (type) error stops early, still listing the unknown keys. | RED: `TestLoad_UnknownFieldsReportedWithOtherProblems` failed (only the unknown key). GREEN. |
| R3-unrelated-yaml-error | New `locate.go`: the top-level `release:` block is cut out by indentation (key line + blank, comment, indented or column-0 `- ` lines) and parsed alone, on its original line numbers. Undecidable layouts (`---`/`...`/`%` markers, `?` keys, indented/flow/sequence top level, `release:` twice) and a failed block parse fall back to the whole document. No `release:` + broken file -> `ErrNotConfigured`. An alias to an anchor outside the block is a `*ValidationError` naming the alias and line. | RED: `TestLoad_NotConfigured/tab_indentation_elsewhere` (`found character that cannot start any token`), `TestLoad_ToleratesUnrelatedYAMLErrors`, `TestLoad_AliasToOutsideAnchor` (loaded, want error). GREEN. |
| R4-root-not-mapping-silent | `releaseNode` returns a `*ValidationError` ("the top level of nerv.yaml must be a mapping") for a non-mapping root; empty, null or comment-only stays `ErrNotConfigured`. | RED: `TestLoad_RootNotMapping` (got `ErrNotConfigured`). GREEN. |
| R3-null-release-untested | Test case `bare release key` (`release:` with no value -> "at least one project is required"). | No RED possible: behaviour was already correct, the test was missing. GREEN. |
| R3-coverage-nan | `math.IsNaN` added to the 0-100 check (±Inf already failed the range). | RED: `coverage not a number` loaded. GREEN (also `-.inf` case). |
| R3-merge-key-rejected | `strict.go` follows `<<: *a` and `<<: [*a, *b]` into the merged mappings (struct and map targets), reporting their unknown keys at the merging path. | RED: `TestLoad_MergeKeys` (`<<: unknown field`). GREEN, including `release.projects[1].dockr: unknown field` reached only through a merge. |
| R1-alias-walk-amplification | The walk counts every visited node, alias and merge expansions included, and fails above `maxWalkNodes` (10000) with "expands to more than 10000 nodes through aliases and merge keys". | RED: `TestLoad_AliasExpansionBudget` (10^9-node merge chain; failed on the merge key before the fix). GREEN, returns in milliseconds. |
| R2-unpinned-plugin-hint | Hint is `"<name>@<version>"`. | RED: updated table case failed on `@6.0.3`. GREEN. |
| R2-inline-placeholder-set | `BuildArgPlaceholders` package variable, rendered with `list(placeholders(...))`. | Refactor; existing `docker unknown placeholder` case stays GREEN. |
| R2-scattered-defaults | New `defaults.go`: exported `DefaultTagFormat`, `DefaultChangelog`, `DefaultBump`, `DefaultDotnetConfiguration`, `DefaultPackageManager`, `DefaultDockerfile`, `DefaultDockerContext`, `DefaultNuGetAPIKeyEnv`, `DefaultNpmAccess`, `DefaultShell` (plus the moved `DefaultSemanticReleaseVersion`, `DefaultNuGetSource`) and the `DefaultDockerTags` variable; `applyDefaults` moved there and uses them. `TestDefaults_MatchDocs` ties each value to the docs. | RED: tests referencing the constants failed to compile (`undefined: shipconfig.DefaultTagFormat`). GREEN. |

- Public API: additive only (the new `Default*` constants,
  `DefaultDockerTags`, `BuildArgPlaceholders`); `Load`, `ErrNotConfigured`,
  `ValidationError`, `TopoOrder`, `Project` unchanged. Behaviour change: an
  alias to an anchor outside `release:` no longer loads (documented).
- Docs: `docs/release-commands.md` Validation section (every problem at
  once) and new "YAML notes" (block cut, merge keys, outside anchors,
  non-mapping top level).
- Checks: `go test ./...` ok (all packages), `go vet ./...` clean,
  `gofmt -l .` empty.
- Commits: `276569f` `fix(ship): report every release config problem and
  tolerate unrelated yaml` (6 files changed, 488 insertions(+), 50
  deletions(-)); `6234e1d` `refactor(ship): name the release config
  defaults` (4 files changed, 225 insertions(+), 140 deletions(-)).

## Next step

T2 (node engine) on top of T1.
