# Release commands: `nerv version next` and `nerv ship`

## Objective

Two repo-agnostic commands for any repository NERV works on:

- `nerv version next` (`/nerv:version`): the next semantic version of a repo (or of
  each project of a monorepo) from a branch's commits, using the repo's own
  semantic-release configuration, with the outputs of
  `cycjimmy/semantic-release-action`.
- `nerv ship` (`/nerv:ship`): a full local release that replaces the CI
  workflow where it cannot or should not run: pipeline (restore, lint, build,
  test, coverage, package), version files, publish (docker, nuget, npm), tag,
  CHANGELOG and GitHub release, configurable through `.nerv/nerv.yaml` and
  hooks running node, pwsh or bash scripts. It reuses `nerv version next`.

## Why

User request (2026-10-13). Some repos cannot (or should not) run their GitHub
Actions release; the reference is `dim.comprobantes.api` (semantic-release in
`package.json` `release`, branches main / `release/rc` (rc) / develop (alpha)
/ `hotfix/*`, `@semantic-release/exec` bumping the `.csproj`, docker images
tagged with version, channel and `latest`).

## Decisions (user, 2026-10-13)

- Engine: semantic-release itself, run through its programmatic API (dry run
  for `nerv version next`), so every semantic-release config form is accepted
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

- Names `version next` / `ship` avoid the existing `nerv release`, NERV's own
  maintainer tool, and the existing `nerv version` (binary version; user
  decision 2026-10-13: the computation is the `next` subcommand).
- `nerv version next [--branch <b>] [--project <p>] [--json|--format env]`:
  semantic-release dry run in a temporary `git worktree` of the branch; a
  NERV-owned node plugin filters commits per project (path OR scope) and
  applies `depends_on` bumps. Output: the 12 action outputs
  (`new_release_published`, `new_release_version`,
  `new_release_major_version`, `new_release_minor_version`,
  `new_release_patch_version`, `new_release_channel`, `new_release_notes`,
  `new_release_git_head`, `new_release_git_tag`, `last_release_version`,
  `last_release_git_head`, `last_release_git_tag`), per project.
- `nerv ship [--project <p>] [--dry-run] [--yes]`: shows `nerv version next`'s
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
- [x] T2 — node engine: embedded runner using semantic-release's API, pinned
  dependency cache, branch worktree, NERV commit-filter plugin (path OR
  scope), dependency bumps; output normalised to the 12 action outputs.
- [x] T3 — `nerv version next` CLI (single repo and monorepo, `--json` and
  `--format env`) plus `/nerv:version`.
- [x] T4 — streaming script runner with env and working directory; hooks
  for node, pwsh and bash on Linux and Windows.
- [x] T5 — pipeline stages and built-in presets (dotnet: restore, format
  check, build, test, coverage, pack; node; docker build/tag/push; nuget
  push; npm publish) and the dotnet `.csproj` version / release-notes writer.
- [x] T6 — `nerv ship`: clean worktree, confirmation, pipeline, real
  semantic-release run with the injected lifecycle plugin, monorepo order by
  `depends_on`, `--dry-run`.
- [ ] T7 — `/nerv:ship`, user docs (`docs/release-commands.md`), CHANGELOG.

## Acceptance criteria

- On a copy of `dim.comprobantes.api`, `nerv version next --branch develop` returns
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

- PR #91 (T1): `feat/release-commands` → develop, commits f2f7801..294f7b3. Merged 7a23170.
- T2 branch `feat/release-version-engine` from 7a23170.
- PR #92 (T2): `feat/release-version-engine` → develop, commits d543f72..543a8df.
  Merged.
- T3 branch `feat/release-version-cli` from 2dca250 (commits 95d90a7 and
  this document's update).
- PR #93 (T3): `feat/release-version-cli` → develop, commits 668472b..af996f7.
  Merged 2e2cea2.
- T4 branch `feat/release-script-runner` from 2e2cea2.
- PR #94 (T4): `feat/release-script-runner` → develop. Merged f540bdc.
- PR #95 (T5): `feat/release-pipeline-presets` → develop. Merged aad9dff.
- T6 branch `feat/release-ship` from aad9dff.
- T5 branch `feat/release-pipeline-presets` from f540bdc.

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

### T1 follow-up review

- Native review of a65fbbc..c1f89a9 (medium, user granted): approved and
  acknowledged. Advisory: R3-unknown-anchor-misattributed (WARNING: an alias
  to an anchor defined nowhere is reported as "outside the release section"),
  R3-cut-fallback-untested, R3-missed-key-silent-not-configured (a release
  key written with a node property may be missed and read as not configured),
  R3-mutable-default-docker-tags (exported mutable slice).

### T1 second follow-ups

Three of the four advisory findings fixed (user authorized). Route:
delegated (one writer). Test-first: the new tests were added before the fix
and observed failing.

| Finding | Fix | Evidence |
|---|---|---|
| R3-unknown-anchor-misattributed | `unknownAnchorProblem` (was `outsideAnchorProblem`) says "outside the release: section" only when a line of the file outside the cut block defines `&name` (whole-name match); otherwise "refers to an undefined anchor &name (a typo, or an alias used before its anchor is defined)", with the alias line. | RED: `TestLoad_AliasToUndefinedAnchor` (typo, used before definition, only `&cmdx` outside) got the outside message. GREEN; `TestLoad_AliasToOutsideAnchor` stays GREEN. |
| R3-missed-key-silent-not-configured | `releaseKeyRe` accepts leading node properties (`&r`, `!!str`, `!<verbatim>`, combined); the first-line check no longer treats a property-prefixed key as undecided. When the whole file fails to parse and no key was cut, `possibleReleaseKey` (a top-level key containing the word `release`, or an escaped double-quoted key) turns `ErrNotConfigured` into an error naming the line. | RED: `TestLoad_ReleaseKeyForms` anchored/tagged/verbatim cases (`not configured` after another key, a parse error as first key); `TestLoad_UnrecognisedReleaseKeyInBrokenFile` (`"rel\x65ase":` + tab error -> `ErrNotConfigured`). Quoted cases were already GREEN. GREEN. |
| R3-mutable-default-docker-tags | `DefaultDockerTags` is now `func DefaultDockerTags() []string` returning a fresh slice; `applyDefaults` calls it. Tests compare against the literal. | RED: tests failed to compile (`cannot call shipconfig.DefaultDockerTags (variable of type []string)`). GREEN: `TestDefaultDockerTags_ReturnsACopy`, `TestDefaults_MatchDocs`. |

- R3-cut-fallback-untested: accepted and deferred (no work; the
  block-parse fallback to the whole document stays without a dedicated
  test).
- Public API: `DefaultDockerTags` changed from a `[]string` variable to a
  function returning a copy; everything else unchanged.
- Docs: `docs/release-commands.md` YAML notes (quoted/anchored/tagged
  release key, unrecognised key in a broken file, undefined anchor wording).
- Checks: `go test ./...` ok (all packages), `go vet ./...` clean,
  `gofmt -l .` empty.
- Commit: `e431252` `fix(ship): name undefined anchors, find property
  release keys, copy docker tag defaults` (5 files changed, 199
  insertions(+), 19 deletions(-)).

### T2

- Route: delegated (writer trigger: new package, runner, tests, docs).
- Package `internal/semrel`: `Compute(ctx, Options{RepoRoot, Branch,
  Project, Env, CacheDir, Offline}) ([]ProjectResult, error)` (also
  `Engine{Runner}.Compute`); `ProjectResult` carries project, branch and the
  12 semantic-release-action outputs (JSON names = action names,
  `Outputs()` in action order). Process seam: `Runner` interface
  (`Run(ctx, Command) (Result, error)`), `ExecRunner` for real use.
- Node runner `internal/semrel/runner/nerv-semrel.mjs` (`go:embed`), two
  modes: `plan` loads the repo config with semantic-release's own
  cosmiconfig + `extends` merge and lists every package it needs; `run` is
  one `semanticRelease({dryRun: true, ci: false, ...})` call. Sources relied
  on are cited in its header (semantic-release 24.2.3 `index.js`,
  `lib/get-config.js`, `lib/plugins/{index,normalize,utils}.js`,
  `lib/definitions/plugins.js`, `lib/git.js`, `lib/branches/index.js`).
- verifyConditions/credentials decision: semantic-release supports per-step
  overrides (API options are merged over the config, `options =
  {...plugins, ...options}` in `lib/plugins/index.js`; an empty list is a
  valid non-required step). In a dry run verifyConditions and verifyRelease
  still run (`lib/definitions/plugins.js` `dryRun: true`), so the runner
  sets `verifyConditions`, `verifyRelease`, `prepare`, `publish`,
  `addChannel`, `success`, `fail` to `[]`, and wraps `analyzeCommits` /
  `generateNotes` around the repo's own composed steps (built by
  semantic-release's `getConfig`), so analysis and notes are exactly the
  repo's. The push-permission check (`git push --dry-run <repositoryUrl>`)
  and the branch/tag/notes fetches target a disposable mirror passed as
  `repositoryUrl`: no GH_TOKEN/NPM_TOKEN needed (integration test runs with
  them removed). Notes links use the repo's public URL (config,
  package.json `repository` or origin; a local path becomes `file://`).
- Deviation from the planned `git worktree add` on the user's repo:
  semantic-release's dry run fetches `+refs/heads/<b>:refs/heads/<b>` for
  every release branch and fetches notes, which in a worktree of the user's
  repo writes shared refs. And the channel of each release lives in git
  notes (`refs/notes/semantic-release-<tag>`) that a plain fetch never
  brings (dim.comprobantes.api locally: 0 notes refs); without them a local
  run on a disposable clone of it computed develop as `2.9.0-alpha.1` (last
  `v2.8.0`) although `v2.9.0-alpha.8` exists. So the engine uses
  `git clone --mirror --shared` into a temp dir, fetches origin's
  `+refs/tags/*` and `+refs/notes/*` into the mirror only
  (`GIT_TERMINAL_PROMPT=0`; failure is an error that points to offline
  mode), adds the worktree from the mirror, and deletes the temp dir. The
  user's repo is only read. `Options.Offline` skips the origin read.
- CI detection: env-ci vendor variables are stripped from the runner's
  environment (list from env-ci 11 `services/*.js`), and the runner refuses
  to run if env-ci still detects a CI service.
- Pinning: default pins (verified on npm 2026-10-13) changelog `6.0.3`, git
  `10.0.1`, exec `7.1.0`, conventional-changelog-conventionalcommits
  `8.0.0` (10.x fails with semantic-release 24's bundled writer 8, observed:
  "requires conventional-changelog-writer@9"; 9.x works; statement of
  record in `internal/semrel/cache.go`; changelog 7 / git 11 need Node
  ^22.22.2 || >=24.15). Bundled allowed without pin: commit-analyzer,
  release-notes-generator, npm, github, conventional-changelog-angular.
  `extra_plugins` overrides a default pin of the same name. Cache:
  `UserCacheDir/nerv/semantic-release/<16-hex sha256 of sorted specs>`,
  installed with `npm install --no-audit --no-fund --ignore-scripts` into a
  temp dir then renamed (concurrent runs: first rename wins). `plan`
  packages not bundled/pinned/extra -> error naming them and
  `release.semantic_release.extra_plugins`; an `extends` that cannot load is
  also an error.
- Projects: `TopoOrder()`; `--project` keeps the project plus its
  transitive dependencies (only the requested one is returned). Single
  project with the default tag format keeps the repo's tagFormat; a project
  with `path: .` gets no commit filter; no `release:` section = one project
  named after the repo directory, plain semantic-release. Commit filter
  (JS): `git diff-tree --name-only -r --root -m --first-parent` per commit
  matched against path / shared_paths (prefix or micromatch glob) OR
  header scope in `scopes` (comma-separated scopes split). depends_on (Go,
  `minReleaseType`): highest bump of dependencies released in this run,
  `none` ignored; passed to the runner, which raises the analyzer's type
  to at least it.
- RED: `go test ./internal/semrel/` -> `undefined: Command` ... [build
  failed]. GREEN: `ok`. The mirror redesign came from the integration
  run (RED there: dim clone develop `2.9.0-alpha.1`; then the new
  prerelease scenario failed against a fake origin before the local bare
  origin). Its unit tests (`TestCompute_DisposableCheckoutRemovedOnError`,
  `TestCompute_OfflineOrNoOriginSkipsTheFetch`,
  `TestCompute_OriginFetchFailureIsAnError`) were written together with
  the change, so no unit RED was observed for them.
- Integration (real node 24.21.0, npm 11.19.0, registry):
  `go test -tags semrel_integration -run Integration -v -count=1
  ./internal/semrel/` -> PASS (13.6s with a warm per-run cache; first
  install ~7s): dim-style config on develop with the first alpha's note
  only on origin -> `1.2.0-alpha.2`, channel `alpha`, last
  `v1.2.0-alpha.1` (offline: last `v1.1.0`, `1.2.0-alpha.1`, the dim
  symptom); main -> no release, last 1.1.0; user's refs, worktrees and
  checkout unchanged. Monorepo: one `feat(api)` commit touching `web/` ->
  api 1.1.0 (scope) and web 1.1.0 (path), cli untouched. depends_on: `fix`
  in core -> core 1.0.1, app 1.0.1 (patch), sdk 1.1.0 (minor), tool not
  released (none), order core,app,tool,sdk. Unpinned
  `semantic-release-slack-bot` -> error naming it.
- Not run: `nerv version next` vs `npx semantic-release --dry-run` on a real
  copy of dim.comprobantes.api with origin on GitHub (reading origin is a
  remote operation with the user's credentials; it belongs to T3's
  acceptance run).
- Checks: `go test ./...` ok, `go vet ./...` (and with
  `-tags semrel_integration`) clean, `gofmt -l .` empty.
- Size: `d543f72` 9 files changed, 2175 insertions(+). Above the ~400-line
  heuristic: about half is tests (634 unit, 339 integration); the runner
  (316, mostly the cited-source header and the plan mode) and engine are one
  behaviour that cannot ship without each other.
- Commit: `d543f72` `feat(semrel): compute next versions with
  semantic-release`.

### T2 review

- Native review of a97b002..306b5dc (high risk, 4 lenses, user granted):
  approved and acknowledged. Advisory findings: R1-001 (WARNING: a failed
  origin fetch puts the remote URL, with any embedded credentials, in the
  error), R4-ssh-fetch-can-block (WARNING: ssh can prompt on /dev/tty),
  R4-per-commit-git-spawn (WARNING: one `git diff-tree` per commit, per
  pass, per project, unmemoised), R3-relative-origin-url (WARNING: a
  relative local origin resolves against the temp mirror),
  R3-runner-exit-truncation (WARNING: `process.exit` right after a large
  stdout write can cut the result on async pipes), R2-001 (WARNING: the task
  log and the code disagree on which conventionalcommits versions fail),
  R4-install-temp-leak, R2-002 (implicit `indexOf(null)` raise),
  R3-runner-logic-untested-by-default.

### T2 follow-ups

All nine advisory findings of the T2 review fixed (user authorized). Route:
delegated (one writer). Test-first where a test expresses the defect.

| Finding | Fix | Evidence |
|---|---|---|
| R1-001 | The origin URL is no longer on argv: the mirror fetches the `nerv-origin` remote whose URL comes from `GIT_CONFIG_COUNT`/`GIT_CONFIG_KEY_n`/`GIT_CONFIG_VALUE_n` (extending an existing count). `redact` hides URL userinfo and secret-named query values (`*token*`, `*password*`, `*secret*`, `*key*`, `*sig*`) in every command string (`Engine.run`), stderr tail and runner error. | RED: `TestCompute_OriginFetchFailureHidesCredentials` (error leaked `s3cr3t-pass`, `tok-123`; URL on the fetch argv). GREEN. `TestRedact` written with the fix (no RED). |
| R4-ssh-fetch-can-block | Fetch env: `GIT_TERMINAL_PROMPT=0`, `GCM_INTERACTIVE=never`, `GIT_SSH_COMMAND` with `-o BatchMode=yes` appended following git's precedence (env `GIT_SSH_COMMAND`, then the repo's `core.sshCommand`, then `GIT_SSH`, then `ssh`); a non-ssh command (plink, wrapper) or a `GIT_SSH` program is left alone (documented). No stdin. Bounded by `Options.FetchTimeout` (default `DefaultFetchTimeout` 60s), with a "timed out after" error. | RED: `TestCompute_FetchNeverPrompts` (no BatchMode in 4 cases), `TestCompute_FetchIsBounded` (no deadline). GREEN. `TestCompute_FetchTimeoutIsAnError` written with the fix. |
| R4-per-commit-git-spawn | `commitFileLister`: one `git diff-tree --stdin -z -r --root --name-only --no-renames --diff-merges=first-parent` per pass for the commits not listed yet, cached in a per-run `commit-files.json` (next to the mirror) shared by every project. Merge rule: a merge's files are its diff against its first parent; the old `-m --first-parent` actually listed the union of the diffs against every parent (diff-tree ignores `--first-parent`; observed). Needs git >= 2.31 (documented). | node tests: one git call per pass, cache reuse, real-repo merge case; `TestCompute_ProjectsShareOneCommitFilesCache`. |
| R3-relative-origin-url | `resolveRemoteURL`: a local-path origin is joined to the repo top level; scheme URLs, scp-like and absolute paths unchanged. The integration repos now use a relative origin. | RED: `TestCompute_RelativeOriginResolvedAgainstTheRepository` (`../origin.git` passed as is). GREEN; integration prerelease scenario passes with a relative origin. |
| R3-runner-exit-truncation | `finish(stream, result, code)` exits in the write callback. | Observed: `process.exit` right after an 8 MiB write delivered 65536 bytes; in the callback, all 8388613. node test "finish writes a large result completely" fails against the old write-then-exit and passes now. |
| R2-001 | Verified 2026-10-13 with semantic-release 24.2.3 (release-notes-generator 14.1.1, writer 8.4.0): conventionalcommits 8.0.0, 9.0.0, 9.3.1 render notes; 10.0.0+ fails ("Missing helper: ... requires conventional-changelog-writer@9 or newer"). The review's premise (9.x needs writer 9) is false: the old code comment and docs were wrong, this log was right. Statement of record: `internal/semrel/cache.go`; docs and this log refer to it. | Direct `generateNotes` runs per version. |
| R4-install-temp-leak | `sweepStaleInstalls`: `.install-*` staging dirs in the cache base older than 1h are removed before installing. | RED: `TestEnsureCache_SweepsStaleInstalls`. GREEN. |
| R2-002 | `raiseReleaseType(analyzed, minimum, order)` with the null-analyzer rule explicit and commented. | node test. |
| R3-runner-logic-untested-by-default | Pure logic in `runner/nerv-semrel-lib.mjs` (embedded with the runner; `writeRunner` writes both) tested by `runner/nerv-semrel-lib.test.mjs`, run by `TestRunnerLib_NodeUnitTests` (also `node --check` on the runner); skips without node. | RED: the module did not exist (`ERR_MODULE_NOT_FOUND`). GREEN: 9 node tests (scopes incl. commas and `!`, path/glob matching, diff-tree parsing, batching, merges, filter, raise, plan packages, flush). Skip observed with node off PATH. |

- Public API: added `Options.FetchTimeout` and `DefaultFetchTimeout`;
  nothing removed or changed.
- Behaviour change: merge commits match a project by their first-parent
  changes only (before: the union of every parent's diff).
- Checks: `go test ./...` ok (24 packages), `go test -tags
  semrel_integration -run Integration -v -count=1 ./internal/semrel/` PASS
  (4 tests, 13.6s), `node --test` 9/9, `go vet ./...` (and with the tag)
  clean, `gofmt -l .` empty.
- Commits: `6a21c3f` `fix(semrel): read origin without prompts, argv
  credentials or relative paths` (8 files changed, 480 insertions(+), 18
  deletions(-)); `d4db9b3` `fix(semrel): sweep stale cache install staging
  directories` (2 files changed, 68 insertions(+), 1 deletion(-));
  `ecb142c` `fix(semrel): list commit files once per run and flush the
  runner result` (7 files changed, 604 insertions(+), 127 deletions(-));
  `e6bad1a` `fix(semrel): state which conventionalcommits majors fail with
  writer 8` (2 files changed, 13 insertions(+), 4 deletions(-)).

### T2 follow-up review

- Native review of 06f6590..a82f973 (medium, user granted): approved and
  acknowledged. Advisory: R3-fetch-timeout-unproved-on-real-process
  (WARNING: the 60s bound is a context deadline proven only with a fake
  runner; a real fetch's ssh/https helper children may outlive the kill and
  hold the pipes open), R3-diff-merges-git-version-unchecked (git < 2.31
  fails with a raw unknown-option error).

### T2 second follow-ups

Both advisory findings of the T2 follow-up review fixed (user authorized).
Route: delegated (one writer). Test-first.

| Finding | Fix | Evidence |
|---|---|---|
| R3-fetch-timeout-unproved-on-real-process | `ExecRunner` (the only real runner, so git fetch/clone, npm install and node all get it) starts each command in its own process group (`Setpgid` on Unix; `CREATE_NEW_PROCESS_GROUP` on Windows) and its `cmd.Cancel` kills the whole group (`kill(-pgid, SIGKILL)`; Windows `taskkill /T /F /PID`, falling back to killing the child). `cmd.WaitDelay = processWaitDelay` (5s) bounds Wait after a cancel or exit even if a pipe stays open. Build-tagged `process_unix.go` / `process_windows.go`. | Real-process test `TestExecRunner_CancelKillsTheWholeProcessGroup` (Unix only): `sh -c 'sleep 60 & ...; sleep 60'` under a 200ms context. RED (old runner): "Run still blocked 8.0s after the context deadline: a grandchild holds the output pipe". GREEN: returns in 0.21s and the grandchild pid is gone (ESRCH). Its cleanup kills the grandchild even on failure. |
| R3-diff-merges-git-version-unchecked | `gitversion.go`: when any project has a path filter (a `path` other than `.`), Compute runs `git --version` once before node, the cache or the mirror, and stops with "git X.Y.Z is too old: selecting a project's commits by path needs git 2.31.0 or newer". `parseGitVersion` reads `2.43.0`, `2.39.3 (Apple Git-146)`, `2.45.1.windows.1`, `2.30` (patch 0). | RED (stubbed parser): `TestParseGitVersion` 6 cases failed, `TestCompute_PathFilterNeedsGit231` "want an error for git 2.30.2", `TestCompute_GitVersionCheckedOnlyForPathFilters` "called 0 times, want once". GREEN. |

- Public API: unchanged (new identifiers are unexported).
- Behaviour change: a monorepo with path filters on git < 2.31 now fails
  early with that error (documented in `docs/release-commands.md`);
  cancelled commands no longer leave helper processes behind. Commands now
  run outside the terminal's foreground process group, so a terminal
  Ctrl-C reaches them only through the context.
- Checks: `go test ./...` ok (24 packages), `go test -tags
  semrel_integration -run Integration -count=1 ./internal/semrel/` ok
  (26.1s), `go vet ./...`, `GOOS=windows go vet ./internal/semrel/` and
  `go vet -tags semrel_integration ./internal/semrel/` clean, `gofmt -l .`
  empty.
- Commits: `477aa8a` `fix(semrel): kill the whole process tree of a
  cancelled command` (4 files changed, 138 insertions(+), 1 deletion(-));
  `120d447` `fix(semrel): require git 2.31 before filtering project commits
  by path` (6 files changed, 188 insertions(+), 2 deletions(-)).

### T2 second follow-up review

- Native review of 3a7e2ab..80d7e40 (high risk, 4 lenses, user granted):
  approved and acknowledged. Advisory: R4-setpgid-background-tty and
  R3-setpgid-tty-prompt-stops-child (WARNING: a background process group
  gets SIGTTIN/SIGTTOU when a helper opens the terminal, so it stops until
  the timeout), R4-windows-new-group-ctrlc (WARNING: CREATE_NEW_PROCESS_GROUP
  drops console Ctrl-C for children; taskkill is unbounded),
  R3-waitdelay-turns-success-into-error (a lingering descendant turns a
  successful exit into ErrWaitDelay), R3-windows-tree-kill-unproved,
  R2-unasserted-ran-counter, R2-ctrlc-consequence-undocumented-in-code.
- T3 must cancel the context on SIGINT/SIGTERM (signal.NotifyContext) so a
  terminal Ctrl-C still stops git, npm and node.

### T2 final pass

Final bounded pass on the `internal/semrel` process runner for the T2
second follow-up review's advisory findings (user authorized). Route:
delegated (one writer). Test-first where a test can express the defect.

| Finding | Fix | Evidence |
|---|---|---|
| R4-setpgid-background-tty, R3-setpgid-tty-prompt-stops-child | `process_unix.go`: `Setsid: true` instead of `Setpgid`, so the command has no controlling terminal and a helper that opens /dev/tty fails at once (ENXIO) instead of being stopped by SIGTTIN/SIGTTOU. The session leader's pid is its process-group id, so `kill(-pid, SIGKILL)` still kills the tree. | `TestExecRunner_ChildHasNoControllingTerminal` (`sh -c 'read line < /dev/tty'`, 4s timeout) skips when the test process has no controlling terminal, which is the case in this agent shell; RED/GREEN observed by running it under a pty with `script -qec "go test ..." /dev/null`. RED (Setpgid): "Run hit the timeout after 4.0s: the child was stopped waiting for the terminal (err signal: killed)". GREEN: fails at once (0.00s) with the shell's exit error. `TestExecRunner_CancelKillsTheWholeProcessGroup` stays green (0.21s). |
| R4-windows-new-group-ctrlc | `process_windows.go`: `taskkill /T /F /PID` runs under a 10s `exec.CommandContext` timeout, falling back to `Process.Kill`. The code documents that `CREATE_NEW_PROCESS_GROUP` stops console Ctrl-C from reaching children, so the CLI (T3) must cancel the context on SIGINT/SIGTERM. | No Windows runner: `GOOS=windows go vet ./internal/semrel/` clean (compile check only). |
| R3-waitdelay-turns-success-into-error | `process.go`: when `Wait` returns `exec.ErrWaitDelay`, the leftovers of the process group are killed (`killLeftovers`: group SIGKILL on Unix, no-op on Windows) and, if the context was not cancelled and the process exited 0, Run returns success with the output collected so far. `processWaitDelay` became an unexported variable so tests can shorten it. | `TestExecRunner_LingeringDescendantKeepsASuccessfulExit` (`sh -c 'sleep 30 & ...; echo ok'`, wait delay 300ms). RED: "Run(exit 0 with a lingering grandchild) = exec: WaitDelay expired before I/O complete, want success". GREEN: success with "ok" in 0.31s and the grandchild pid is gone (ESRCH). Cleanup kills the grandchild even on failure. |
| R2-unasserted-ran-counter | `gitversion_test.go`: `TestCompute_GitVersionCheckedOnlyForPathFilters` asserts that semantic-release ran in both phases. | Test change only; passes. |
| R2-ctrlc-consequence-undocumented-in-code | `ExecRunner` doc: commands run in their own session and process group without a controlling terminal, so a terminal Ctrl-C reaches them only through context cancellation (signal.NotifyContext). | Doc only. |

- Documented limitation, R3-windows-tree-kill-unproved: the Windows tree
  kill (taskkill /T) and the Windows behaviour on a lingering descendant
  (left running; taskkill cannot find it once the child exited) are not
  proved on a real Windows process; there is no Windows runner here.
- Public API: unchanged.
- Checks: `go test ./...` ok (24 packages), `go test -tags
  semrel_integration -run Integration -count=1 ./internal/semrel/` ok
  (13.6s), `go vet ./...`, `GOOS=windows go vet ./internal/semrel/` and
  `go vet -tags semrel_integration ./internal/semrel/` clean, `gofmt -l .`
  empty.
- Commit: `12e81f5` `fix(semrel): detach commands from the terminal and
  keep successful exits` (5 files changed, 169 insertions(+), 20
  deletions(-)).

### T2 final pass review

- Native review of 1c72fee..49ed4fb (high risk, 4 lenses, user granted):
  approved and acknowledged. Accepted as known limitations (no further
  rounds, by plan): R3-killleftovers-pid-reuse-window (WARNING: a descendant
  that left the group could, in a narrow window, let the group id be
  reused), R3-setsid-proof-skips-without-tty (WARNING: the Setsid guard test
  only runs under a terminal; RED/GREEN was shown under `script`),
  R4-001 (Windows leftovers keep running), R2-tty-test-silently-skipped,
  R2-mutable-waitdelay-global (tests that change it must not be parallel).
- Carried to T3: R3-setsid-drops-sighup-propagation — the CLI must cancel
  the context on SIGINT, SIGTERM and SIGHUP.

### T3

- Route: delegated (writer trigger: CLI, signals, docs, slash command).
- Name conflict found before writing: `nerv version [--json]` already printed
  the binary and plugin versions. User decision (option 3): the computation
  is `nerv version next [flags]`; bare `nerv version` / `--json` unchanged
  (their tests untouched and green); any other argument after `version` is a
  usage error. `/nerv:version` runs `nerv version next --json [args]`.
- CLI (`cmd/nerv/version_next.go`): flags `--cwd` (git top level through the
  `env.Runner` seam), `--branch`, `--project`, `--offline`, `--fetch-timeout`,
  `--format text|json|env`, `--json` (conflicts with another explicit
  `--format`). Before the engine runs, the `release:` section is loaded:
  `*shipconfig.ValidationError` -> exit 1 with its path-qualified lines;
  unknown `--project` -> exit 1 listing the projects; `--project` with no
  section or a directory outside git -> exit 1. Engine errors -> exit 2
  (validation from the engine also 1); `semrel.ErrOriginFetch` adds an
  "--offline" hint; a cancelled run says "interrupted" (exit 2). No new
  release -> exit 0. Progress and errors on stderr; stdout only the format.
  json: object for one result, array for several. env: `key=value`,
  multi-line values `key<<ghadelimiter_<32 hex>` ... value, newline,
  delimiter (as @actions/core), the delimiter regenerated if it occurs in
  the value; several results -> keys prefixed `<project>_`, `-` as `_`.
- Signals: `signal.NotifyContext` on `cancelSignals` (build-tagged:
  `signal_unix.go` SIGINT, SIGTERM, SIGHUP; `signal_other.go` SIGINT,
  SIGTERM). Seams in `options`: `ComputeVersions` (default
  `semrel.Compute`), `SignalContext` (default `signalContext`).
- `internal/semrel` (small additive change): exported `ErrOriginFetch`,
  wrapped by both origin-fetch errors (failure and timeout), so the CLI
  can suggest `--offline` without matching message text.
- RED: `go test ./cmd/nerv/ ./internal/semrel/` -> build failed
  (`undefined: signalContext`, `opts.ComputeVersions undefined`,
  `undefined: ErrOriginFetch`). GREEN: both `ok`; 39 targeted cmd/nerv
  tests pass. `TestRun_VersionUnknownSubcommandIsAUsageError` had no
  behavioural RED (bare `version` already rejected unknown arguments);
  it pins that the usage now names `version next`.
- Real-signal proof: `TestSignalContext_CancelsOnInterruptTerminateAndHangup`
  (Unix) sends SIGINT, SIGTERM and SIGHUP to the test process and sees the
  context cancelled each time.
- Local end-to-end run (no remote; npm cache warm), fixture repo with
  `package.json` `release.branches: [main, {develop, prerelease alpha}]`,
  tag `v1.0.0`, `fix(api)` on main, `feat(web)` on develop:
  `go run ./cmd/nerv version next --offline --cwd <fixture>` -> `widgets
  (main)` next `1.0.1` (default channel, tag `v1.0.1`), last `1.0.0`;
  `--branch develop` -> `1.1.0-alpha.1` (channel alpha). With a `release:`
  section (api: `api`, web-app: `web`) on develop: api no new release (last
  `api-v1.0.0`), web-app `1.1.0-alpha.1` (`web-app-v1.1.0-alpha.1`); env
  keys `api_*` / `web_app_*`; `--project mobile` -> exit 1 "unknown project
  "mobile"; the release: section declares: api, web-app". All exit 0
  otherwise.
- Observed (T2 behaviour, not changed): with no origin remote and no
  `repository` in package.json, the notes links read
  `https:/<absolute repo path>/compare/...`.
- Docs: `docs/release-commands.md` `nerv version next` section (flags,
  requirements node >= 20.8.1, npm, git >= 2.31 for path filters; outputs
  table; formats with examples; exit codes; signals), `docs/commands.md`
  (both forms and `/nerv:version`), `docs/configuration.md`, README docs
  index, CHANGELOG `[Unreleased]` `### Added`, `plugin/commands/version.md`.
  No prose test pins the command list (checked: `assets_test.go` requires
  only `commands/configure.md`).
- Checks: `go test ./...` ok (24 packages), `go test -tags
  semrel_integration -run Integration -count=1 ./internal/semrel/` ok
  (13.8s), `go vet ./...`, `go vet -tags semrel_integration
  ./internal/semrel/`, `GOOS=windows go vet ./cmd/nerv/ ./internal/semrel/`
  clean, `gofmt -l .` empty.
- Size: `95d90a7` 15 files changed, 1200 insertions(+), 17 deletions(-).
  Above the ~400-line heuristic: about 560 lines are tests and 290 docs;
  the CLI, its formats and the slash command are one user-facing unit.
- Commit: `95d90a7` `feat(cli): add nerv version next and /nerv:version`.

## Decisions after T2 (user, 2026-10-13)

- `nerv version next` reads origin's tags and semantic-release notes by default
  (on the throwaway mirror, with the user's git credentials); `--offline`
  uses local refs only.
- Kept: `last_release_*` filled even without a new release; no `release:`
  section means one project named after the repo directory.

### T3 review

- Native review of 668472b..6b20c5c (high risk, 4 lenses, user granted):
  approved and acknowledged. Advisory: R4-second-signal-swallowed (WARNING:
  later Ctrl-C presses are swallowed until return), R3-env-key-prefix-
  collision (WARNING; refuted by the parent: project names are validated as
  `[a-z0-9-]`, so `web-app` and `web_app` cannot coexist),
  R4-json-text-write-errors-ignored, R2-fetch-timeout-implicit-default,
  R3-shape-depends-on-result-count, R3-empty-results-json-null.

### T3 follow-ups

All T3 review advisories but the refuted R3-env-key-prefix-collision fixed,
plus the T2 notes-link defect found in the T3 run (user authorized). Route:
delegated (one writer). Test-first.

| Item | Fix | Evidence |
|---|---|---|
| R4-second-signal-swallowed | `signalContext` (own `signal.Notify` loop instead of `signal.NotifyContext`) calls `signal.Stop` as soon as the context is done (first signal, parent or stop), before the cancellation is visible; a second Ctrl-C gets the default handler. `signalNotify`/`signalStop` package seams. | RED: `signal_test.go` failed to build (`undefined: signalStop`, `signalContext returns 2 values`); no behavioural RED for the release itself (the seam did not exist). GREEN: `TestSignalContext_FirstSignalReleasesTheHandler` (handler released once, before the cancellation), `..._ParentCancellationReleasesTheHandler`, `..._StopWithoutASignalReleasesTheHandler`; `go test -race -count=3` clean. |
| Interrupted exit codes | `received()` reports the signal; exit `128 + signal` (SIGINT 130, SIGTERM 143, SIGHUP 129), `2` when cancelled without one; stderr `interrupted (<signal>)`. The `SignalContext` seam returns `(ctx, stop, received)`. | RED: build failure as above. GREEN: `TestVersionNext_InterruptedExitsWith128PlusTheSignal` (4 cases); the real-signal Unix test also checks `received()`. |
| R4-json-text-write-errors-ignored | `writeVersionResult` renders every format into memory, writes once and calls `Flush()` when the writer has one; an error is `writing the result: ...`, exit 2. | RED: `TestVersionNext_WriteErrorsAreReported` text/json write+flush and env flush failed (exit 0). GREEN. Manual: `> /dev/full` -> "no space left on device", exit 2. |
| R2-fetch-timeout-implicit-default | `--fetch-timeout` defaults to `semrel.DefaultFetchTimeout`; `<= 0` is a usage error ("must be positive ... use --offline"). | RED: `zero fetch timeout` cases (exit 0) and `TestVersionNext_DefaultsReadOriginOnTheCurrentBranch` (timeout 0 sent). GREEN; `TestVersionNext_UsageShowsTheDefaultFetchTimeout` pins "default 60s". |
| R3-shape-depends-on-result-count, R3-empty-results-json-null | `requestShape`: several (array, prefixed env) only for a `release:` section with more than one project and no `--project`; otherwise one object / unprefixed keys for the requested, only, or repository-named project, dependency results dropped. No result for it -> exit 2 "the engine returned no result ...", nothing on stdout. | RED: `TestVersionNext_ProjectFlagPrintsOnlyThatProject`, `..._MonorepoIsAnArrayEvenWithOneResult`, `..._MissingRequestedResultIsAnError` (15 cases). GREEN; `..._SingleProjectSectionIsAnObject`. |
| Notes links with no remote (T2) | Worse than reported: the mirror's `remote.origin.url` is the user's repository path, so every repository without `repositoryUrl` / package.json `repository` got `https:/<local path>/...` links, GitHub origin included. Go: the mirror's origin is set to `publicURL(origin)` (scheme URLs without user info, query, fragment; scp-like kept; local paths and `file:` -> none), read offline too, or removed. Runner: no public URL (or a local one) -> notes generated against the reserved `https://nerv-no-remote.invalid/...` placeholder, whose Markdown links are then replaced by their text. | RED: `TestPublicURL`, `TestCompute_MirrorOriginIsThePublicURL` (stub); node: missing exports; integration in a temp worktree of the old HEAD: `MonorepoPathOrScope` and the new `NotesLinkThePublicOrigin` showed `https:/tmp/.../compare/...` for a local and a GitHub origin. GREEN: all; node 11/11; integration no-origin notes `## 1.0.1 (2026-10-13)` / `* repair widgets (d3783ab)`, GitHub origin links `https://github.com/example/widgets/compare/v1.0.0...v1.0.1` without the credentials. |

- Docs: `docs/release-commands.md` (output shape by request, positive
  `--fetch-timeout`, exit codes incl. `128 + signal`, second Ctrl-C, notes
  links), `plugin/commands/version.md` (shape, interrupted codes).
- Local run (no remote, offline): `nerv version next --offline --cwd
  <fixture> --branch develop --project web-app --format env` -> notes
  `# 1.1.0-alpha.1 (2026-10-13)` ... `* **web:** add dark mode (ba07a36)`,
  no links; `--json` one object for web-app; `--fetch-timeout 0` usage
  error.
- Checks: `go test ./...` ok (24 packages), `go test -tags
  semrel_integration -run Integration -count=1 ./internal/semrel/` ok (5
  tests), `go vet ./...`, `GOOS=windows go vet ./cmd/nerv/
  ./internal/semrel/` clean, `gofmt -l .` empty.
- Commits: `e9d2c84` `fix(cli): release signals early and shape version
  output by request` (7 files changed, 498 insertions(+), 69 deletions(-));
  `d2d10f1` `fix(semrel): link release notes to the public origin, or not
  at all` (8 files changed, 256 insertions(+), 16 deletions(-)).

### T3 follow-up review

- Native review of a1b20f0..ddc6c48 (medium, user granted): approved and
  acknowledged. One WARNING, R3-no-section-project-name-coupling-untested:
  without a release: section the CLI required the result's project name to
  equal the git top-level's base name, so a name the engine derives
  differently (symlink, Windows case) became "no result". Fixed inline by
  the parent in `ddb79a1` (route: inline, one mechanical file + test):
  a sectionless repository prints the engine's single result whatever its
  name. RED: `TestVersionNext_NoSectionPrintsTheSingleResultWhateverItsName`
  failed; GREEN after the fix; `go test ./...`, `go vet ./...`,
  `GOOS=windows go vet ./cmd/nerv/`, `gofmt -l .` clean.

### T3 acceptance fix

- Bug (parent's acceptance run on dim.comprobantes.api): `nerv version next
  --branch develop` (and `main`) exited 2 with `semantic-release:
  EPRERELEASEBRANCH A pre-release branch configuration is invalid in the
  branches configuration`.
- Cause: the mirror (`git clone --mirror --shared` of the user's repo)
  carried every local head, including a local-only
  `hotfix/tw-49276610-error-config-destino`; dim's `{"name": "hotfix/*",
  "prerelease": true}` made it a release branch whose name is an invalid
  prerelease id. CI only has origin's branches. Same root: a branch only on
  origin was invisible (no local `main` -> `ERELEASEBRANCHES`).
- Fix (route: delegated, one writer; `internal/semrel/heads.go`): the
  origin fetch also brings `+refs/heads/*:refs/nerv-origin-heads/*` (same
  safe path: no prompts, redacted, bounded, `ErrOriginFetch`); `planHeads`
  builds one `git update-ref --stdin` transaction on the mirror: heads =
  origin's (online), or the user's `refs/remotes/origin/*` minus `HEAD`
  (offline), plus the requested branch at the user's local commit (unpushed
  commits count); scratch refs deleted. The user's repo is only read.
- Deviation from the request (for the parent to confirm): with no origin
  remote, or offline with no remote-tracking branch, the local branches are
  kept instead of only the requested branch. Single-branch semantics would
  make every prerelease branch of an origin-less repository fail with
  `ERELEASEBRANCHES` (no `main` to follow), a regression of the earlier
  local runs; it is one condition in `planHeads` if the parent prefers it.
- RED: new `TestIntegration_BranchesAreOrigins` (dim-shaped `hotfix/*`
  config, local-only `hotfix/tw-1-error-config`) -> `EPRERELEASEBRANCH`;
  with the hotfix line removed, the origin-only-main half against the old
  engine (temp worktree of 0f1df74) -> `ERELEASEBRANCHES`. Unit:
  `heads_test.go` failed to build (`undefined: planHeads`,
  `originHeadsRef`). GREEN: all; online and offline develop ->
  `1.1.0-alpha.1` from the unpushed `feat`, head = local develop; main no
  release, last `1.0.0`; without a local main develop still computes.
- Docs: `docs/release-commands.md` (origin's branch set, local commit of
  the requested branch, offline and no-origin rules).
- Checks: `go test ./...` ok (24 packages), integration suite ok (6 tests,
  20.7s), `go vet ./...`, `GOOS=windows go vet ./cmd/nerv/
  ./internal/semrel/` clean, `gofmt -l .` empty.
- Commit: `70bf856` `fix(semrel): compute against origin's branches, not
  local ones` (7 files changed, 346 insertions(+), 13 deletions(-)).

### T3 acceptance (user authorized GitHub read, 2026-10-13)

- Reference: semantic-release 24.2.9 with dim.comprobantes.api's CI plugin
  pins, `--dry-run --no-ci`, on a throwaway clone whose branches equal
  origin's (the user's local `develop` is behind origin), GH_TOKEN from
  `gh auth token`. Result: develop no release; main 2.9.0; release/rc no
  release (last v2.9.0-rc.1).
- `nerv version next` (after `70bf856`) on the same clone: develop no release
  (last 2.9.0-alpha.9); main 2.9.0 (last 2.8.1); release/rc no release (last
  2.9.0-rc.1). Match on all three branches.
- On the user's own repo, `develop` (local commit, behind origin) gives no
  release with last 2.9.0-alpha.8, as designed (the requested branch is
  taken at the local commit). The user's repo was only read.
- Accepted deviation: with no origin (or offline with no remote-tracking
  branches) the local branches are kept, so prerelease branches still find
  their release branches.

### T3 acceptance fix review

- Native review of 0f1df74..8dd5ebf (medium, user granted): approved and
  acknowledged. One SUGGESTION, R3-001: an empty `rev-parse` result with no
  error rendered `%!w(<nil>)`. Fixed inline by the parent in `fe01cbb`
  (RED: `TestCompute_EmptyBranchCommitIsNotALocalBranch` showed the
  artifact; GREEN after the fix; `go test ./...`, `go vet`, `gofmt -l .`
  clean).

### T4

- Route: delegated (writer trigger: new packages `internal/proc` and
  `internal/shiprun`, shipconfig fields, docs).
- `internal/proc` (new): the semrel process layer moved unchanged
  (`Command`, `Result`, `Runner`, `ExecRunner`, Setsid / new process group,
  whole-tree kill on cancel, bounded taskkill, `WaitDelay`, success kept
  when a descendant lingers, stdin on the null device). `internal/semrel`
  keeps its names as type aliases (`type Command = proc.Command`, ...), so
  its code and tests are untouched; the three real-process tests moved
  with the code. Added streaming: `Command.Stdout`/`Stderr` writers get the
  output live; a streamed output keeps only its last `TailLimit` bytes
  (`DefaultTailLimit` 64 KiB) in the `Result`; the same writer for both
  streams gives the process one pipe (os/exec), so stdout/stderr order is
  the process's own and the combined tail is `Result.Stdout`.
- `internal/shipconfig`: `Command.Workdir` (repository-relative, checked and
  normalised like every path) and `Command.Timeout` (Go duration, > 0,
  read through `TimeoutDuration()`), with path-qualified errors.
- `internal/shiprun` (new): `Runner{RepoRoot, Output, Proc, LookPath,
  Environ}.Run(ctx, Step{Name, Command}, Vars) error`; failures are
  `*StepError` (project, step, shell, script, exit code or -1, reason,
  last 20 output lines with ANSI codes stripped; wraps `ErrTimeout`,
  `ErrInterpreterNotFound` or the context error). Output lines are
  prefixed `[<project>:<step>] ` (partial lines held until newline, flush
  or 64 KiB; output write errors never block the script and are reported
  after a successful exit).
- Shell rules decided: `run` is a script file when, trimmed, it is one line
  naming an existing regular file under the repository root with a
  shell-appropriate extension (bash any, pwsh `.ps1`, node
  `.js/.mjs/.cjs`), passed relative to the workdir (`./x`, `../x`, forward
  slashes for bash). bash file `bash --noprofile --norc <file>`; bash
  inline `bash --noprofile --norc -e -o pipefail -c <run>` (as GitHub
  Actions); pwsh file `-NoLogo -NoProfile -NonInteractive -File`; pwsh
  inline `-Command` with prelude `$ErrorActionPreference = 'Stop';
  $PSNativeCommandUseErrorActionPreference = $true` and epilogue `if
  ((Test-Path -LiteralPath variable:\LASTEXITCODE)) { exit $LASTEXITCODE
  }` (verified on pwsh 7.6.6: a thrown error, a failing cmdlet and a
  failing native command each exit 1 before the next statement; `exit 5`
  keeps 5); node file `node <file>`, inline `node -e`. A script file runs
  as written (no -e). Windows bash = Git Bash: PATH `bash` unless it is
  WSL's launcher (System32 or WindowsApps), then `bin\bash.exe` above
  `git.exe` (cmd, bin or mingw64\bin), then `%ProgramFiles%\Git\bin` and
  `%LocalAppData%\Programs\Git\bin`. Missing interpreter: message naming
  what to install (Git for Windows, PowerShell 7, Node.js).
- Environment: inherited env minus any `NERV_RELEASE_*`, then the
  command's `env` (sorted), then `NERV_RELEASE_PROJECT`, `_PROJECT_PATH`,
  `_REPO_ROOT`, `_STAGE`, `_VERSION`, `_CHANNEL`, `_TAG`, `_GIT_HEAD`,
  `_LAST_VERSION`, `_NOTES_FILE`, `_BRANCH`, `_DRY_RUN` (always set;
  `shiprun.EnvVars`, typed `shiprun.Vars`).
- RED: `go test ./internal/proc/` -> build failed (`unknown field Stdout /
  Stderr / TailLimit in struct literal of type Command`);
  `go test ./internal/shipconfig/` -> build failed (`Command has no field
  or method TimeoutDuration / Workdir`); `go test ./internal/shiprun/` ->
  build failed (`undefined: Vars`, `undefined: Runner`). GREEN: all `ok`.
  One test correction after the first GREEN run: the pwsh failure test
  looked for "after" anywhere, but pwsh's error view quotes the source
  line; it now looks for the output line, and the error tail now strips
  terminal colour codes (pwsh colours its error view even on a pipe).
- Real-process tests (all ran here, none skipped: bash 5, pwsh 7.6.6, node
  24.21.0): bash inline success and failures (`false | true`, `false;
  echo`, `exit 4` -> code 4), bash file from a workdir and failing (code
  7), pwsh inline failures (throw, cmdlet, native git, `exit 5`), pwsh
  `.ps1` (code 3, env visible), node `-e` / `.mjs` / `.cjs` (code 9),
  every NERV_RELEASE_* and hook env visible, workdir honoured, streaming
  order of 200 interleaved stdout/stderr lines with prefixes, timeout
  stops `sleep 60` within the bound, timeout kills the script's background
  child (Unix pid check), missing interpreter with an empty PATH.
- Checks: `go test ./...` ok (26 packages), `go test -tags
  semrel_integration -run Integration -count=1 ./internal/semrel/` ok
  (28.9s, after the extraction and at the end), `go vet ./...` and
  `GOOS=windows go vet ./...` clean, `gofmt -l .` empty, `go test -race`
  on proc and shiprun clean.
- Known limitation: the Windows paths (taskkill tree kill, Git Bash
  lookup, pwsh/node file runs) are compile-checked and unit-tested with a
  fake GOOS only; there is no Windows runner here.
- Commits: `26d0544` `refactor(proc): share the hardened process runner`
  (5 files changed, 119 insertions(+), 84 deletions(-)); `55b86c0`
  `feat(proc): stream command output with a bounded tail` (4 files
  changed, 289 insertions(+), 4 deletions(-)); `e769735`
  `feat(shipconfig): add workdir and timeout to commands` (4 files
  changed, 77 insertions(+)); `c34f47b` `feat(shiprun): run stage and hook
  scripts` (8 files changed, 1387 insertions(+): 739 tests, 584 code, 64
  docs).
- Size: `git diff --shortstat d3f0108 c34f47b` 19 files changed, 1868
  insertions(+), 84 deletions(-). Above the ~400-line heuristic: about
  half is tests (real-process tests per shell), and the extraction, the
  streaming layer, the config fields and the runner are separate work-unit
  commits that can be reviewed one by one.

### T4 review

- Native review of d3f0108..54063a7 (high risk, 4 lenses, user granted):
  approved and acknowledged. Advisory: R4-pwsh-stale-lastexitcode (WARNING:
  the inline epilogue exits with a stale `$LASTEXITCODE`, failing a script
  that handled a native failure), R2-env-parallel-arrays (WARNING:
  `Vars.values` and `EnvVars` are zipped by index), R3-symlink-script-escape
  (WARNING: a repo symlink pointing outside the repository is run as a
  script file), R1-001 (Run does not re-check an escaping workdir),
  R2-dangling-doc-ref, R2-duplicated-path-normalisation,
  R2-magic-programs-sentinel, R2-dead-total-counter,
  R3-output-write-error-untested.
- Accepted by the parent: the writer's additions `NERV_RELEASE_REPO_ROOT`,
  dropping inherited `NERV_RELEASE_*` variables, and skipping WSL's bash
  launcher on Windows.

### T4 follow-ups

All nine advisory findings of the T4 review fixed (user authorized). Route:
delegated (one writer). Test-first where a test expresses the defect.

| Finding | Fix | Evidence |
|---|---|---|
| R4-pwsh-stale-lastexitcode | The inline pwsh epilogue is `exit 0`: a command that ran to its end exits 0; an uncaught error (an unhandled native failure included, through the prelude) exits 1; `exit N` exits N. Verified first on pwsh 7.6.6: `try { git rev-parse --verify refs/heads/nope } catch {}` left `$LASTEXITCODE` 128. | RED: `TestReal_PwshExitCodeFollowsHowTheScriptEnded` "handled with try/catch" and "checked and continued" got 128 (want 0); "later native success", "unhandled native failure" (1), "explicit exit" (5), "throw" (1) already passed. GREEN. |
| R2-env-parallel-arrays | One ordered `envTable` of `{name, value(Vars, repoRoot, stage)}`; `EnvVars` derived from it; `environment` iterates the table. | `TestEnvTable_NamesUniqueValuedAndDocumented` (unique names, a value from a filled `Vars`, each documented in `docs/release-commands.md`); refactor, no behavioural RED (failed to build until the table existed). |
| R3-symlink-script-escape | `scriptFile` resolves the root and the file with `filepath.EvalSymlinks`; a script file whose real path is outside the real root is an error wrapping the new `ErrOutsideRepository` (never an inline fallback). A symlink staying inside runs normally. | RED: `TestRun_SymlinkedScriptOutsideTheRepository` (file symlink and symlinked directory) ran `./scripts/link.sh` / `./vendor/tool.sh`. GREEN. |
| R1-001 | `Run` checks the resolved workdir with the same `insideRepository` (EvalSymlinks + `Rel` + `IsLocal`), for a hand-built Command and symlinked directories. | RED: `TestRun_WorkdirOutsideTheRepository` (`../sibling`, symlink `out`, `src/../../sibling`) ran the command. GREEN. |
| R2-dangling-doc-ref | Package doc points at the shell rules above `scriptFile`/`shellArgs` in shell.go. | Doc only. |
| R2-duplicated-path-normalisation | `scriptFile` returns the clean forward-slash path once; `shellArgs` joins it; `StepError.Script` is that path. | RED: `TestRun_ScriptPathIsCheckedAndRunAsOne` got `StepError.Script` `scripts\\.\\build.sh`. GREEN (runs `./scripts/build.sh`). |
| R2-magic-programs-sentinel | `ProgramFiles` and `LocalAppData` are checked for emptiness before joining; the `"Programs"` sentinel is gone. | `TestWindowsBash` stays GREEN. |
| R2-dead-total-counter | Removed from `TestTailBuffer_KeepsTheLastBytes`. | Test change only. |
| R3-output-write-error-untested | Test only: `TestReal_OutputWriteErrorIsReportedAfterTheScript` (writer fails after one write; a 2000-line bash script still writes its end marker, only line1 reached the writer, Run returns a `StepError` "writing its output: output closed"). | Passed at once (behaviour already correct). |

- Public API: added `shiprun.ErrOutsideRepository`; `EnvVars` unchanged.
- Docs: `docs/release-commands.md` (pwsh exit-code rule, epilogue
  `exit 0`, symlinked scripts and workdir refused).
- Checks: `go test ./...` ok (26 packages), `go test -race -count=1
  ./internal/proc/ ./internal/shiprun/` ok, `go test -tags
  semrel_integration -run Integration -count=1 ./internal/semrel/` ok
  (29.9s), `go vet ./...` and `GOOS=windows go vet ./...` clean, `gofmt
  -l .` empty.
- Commits: `c4df6cf` `test(proc): drop an unused byte counter from the
  tail buffer test` (1 file changed, 2 deletions(-)); `edc46c8`
  `fix(shiprun): exit pwsh by how it ended and keep scripts inside the
  repo` (6 files changed, 339 insertions(+), 64 deletions(-)).

### T4 follow-up review

- Native review of 085ff1e..255f97c (medium, user granted): approved and
  acknowledged. One WARNING, R3-pwsh-exit0-pre73: on PowerShell 7.0–7.2
  `$PSNativeCommandUseErrorActionPreference` does not exist, so a failing
  native command as the last statement would reach `exit 0`. Fixed inline
  by the parent in `1023582`: the prelude refuses PowerShell older
  than 7.3 with exit 3 and a message (RED: build failure on the new test;
  GREEN: `TestReal_PwshInlineRefusesAnOlderPowerShell` with the minimum
  raised to 99.0, plus every pwsh test on 7.6.6); docs updated.

### T5

- Route: delegated (writer trigger: new package `internal/shipplan`, a
  shiprun addition, docs).
- `internal/shiprun` (minimal additive change): `Step.Exec []string` runs an
  argument list without a shell (program from PATH; missing -> wraps
  `ErrInterpreterNotFound`, "<program> not found on PATH"); `Command.Env`,
  `Workdir`, `Timeout` still apply; `StepError.Shell` is `exec` and
  `Script` the program, never the arguments. `Step.Stage` sets
  `NERV_RELEASE_STAGE` (empty -> `Name`, so existing callers are
  unchanged). `internal/proc` and `internal/shipconfig` untouched.
- `internal/shipplan`: `Build(repoRoot, shipconfig.Project, Context) (*Plan,
  error)`; `Context{Version (required), Channel, Tag, GitHead, NotesFile,
  DryRun, ArtifactsDir (required, absolute)}`; `Plan{Project, Context,
  Stages []Stage, Hooks map[string][]Step}` with `Pipeline()` (all but
  publish) and `Publish()`; `Step{Name, Stage, Command, Exec []Arg, Check,
  Describe}`, `Arg{Value, SecretEnv}`; `Plan.Render()` for `--dry-run`;
  `Executor{Steps StepRunner, Output, LookupEnv}` with `Run` (pipeline,
  first failure -> `*StepFailure{Stage, Step, Err}`) and `RunPublish`
  (no-op in a dry run); `WriteCsproj(path, version, notes)`;
  `CheckCoverage(dir, threshold) (Coverage, error)`; `ArtifactsEnv`.
- Decisions:
  - A `stages:` override is the stage: it replaces every enabled preset's
    commands for it. Preset commands follow the order of `presets:`. A
    stage with no command is skipped with its hooks.
  - One test run, as dim's CI: with `coverage_threshold` > 0 the
    `dotnet:test` step collects coverage into
    `<artifacts>/<project>/coverage` and the `coverage` stage is an
    in-process Go check (instead of the suggested second `dotnet test`
    with `--collect`, which would run the suite twice). Every step gets
    `NERV_RELEASE_ARTIFACTS_DIR` (via `Command.Env`) so a `test` or
    `package` override can write reports / packages where the checks
    read them.
  - Cobertura merge: sum of `lines-covered` / `lines-valid` over every
    `*.cobertura.xml`; no report, malformed XML, missing counts or zero
    coverable lines fail. Line rate only (no branch threshold).
  - `dotnet pack` only with the nuget preset; nuget without the dotnet
    preset is a plan error unless `stages.package` is overridden.
  - Docker tags as dim's metadata-action: version always; channel when not
    the default (empty or `default`); latest on the default channel only;
    duplicates dropped; an invalid docker tag (`+build`, `/` in a channel)
    is a plan error. Builds also add OCI labels `image.version` and (when
    known) `image.revision`, as dim does. dim's `-warnaserror`,
    `--settings coverlet.runsettings` and trx logger are repo-specific:
    documented as stage overrides, not built in.
  - Yarn: `--immutable` for Yarn 2+ (package.json `packageManager`
    `yarn@>=2`, or `.yarnrc.yml` in the project or repo root), else
    `--frozen-lockfile`. Node scripts lint/build/test only when defined.
  - npm preset: no detection of `@semantic-release/npm` publishing (any
    config form may enable it, and it is in semantic-release's default
    plugins); documented instead.
  - NuGet key: an `Arg{SecretEnv}` resolved only when the step runs;
    Render shows `$NAME`; a missing/empty variable fails before the first
    stage (`Run` checks the publish stage's secrets too, except in a dry
    run). Limitation documented: it is on the `dotnet nuget push` argv
    while the push runs (visible to process listings).
  - csproj writer: every element outside comments replaced (attributes
    kept, `<Version />` opened), missing ones added at the top of the
    first unconditioned `<PropertyGroup>` (indented like its first
    child) or in a new group after `<Project>`; BOM and CRLF kept, notes
    written with the file's line ending, XML-escaped and MSBuild-escaped
    (`%`->`%25`, `$`->`%24`, `@`->`%40`). dim's bump script writes a
    release URL as the notes; what T6 passes is T6's choice.
- Open for T6: `WriteCsproj` at semantic-release's prepare runs after the
  pipeline built and packed, so the nupkg carries `-p:Version` but not the
  new `PackageReleaseNotes` unless T6 writes the csproj in the release
  worktree before the pipeline (the git plugin then commits it at
  prepare). Recommended: write it before the pipeline.
- RED: `go test ./internal/shiprun/` -> build failed (`unknown field Stage /
  Exec in struct literal of type Step`); `go test ./internal/shipplan/` ->
  `undefined: WriteCsproj`, then `undefined: CheckCoverage`, then
  `undefined: Context`, `undefined: Build` [build failed]. GREEN: each
  `ok` (one test-index correction in `TestBuild_DotnetAndDockerLikeDim`).
- Smoke (real tools, skipped under `-short` and without the tool):
  `TestSmoke_DotnetPipeline` (generated classlib + xunit + slnx, csproj
  written with notes `* fix <b> & $(Foo) at 50%`, restore, lint, build,
  test, coverage 100.00% >= 90, pack) PASS in 10.6s (setup 2.6s, pipeline
  8.0s; packages from the warm NuGet cache); the nupkg's nuspec has
  version `1.2.3-smoke.1` and `<releaseNotes>* fix &lt;b&gt; &amp; $(Foo)
  at 50%</releaseNotes>` (MSBuild escaping round-trips).
  `TestSmoke_DockerBuild` (FROM scratch, build arg, version + latest tags,
  labels checked with `docker image inspect`, image removed, never pushed)
  PASS in 0.3s build / 0.47s total.
- Checks: `go test -count=1 ./...` ok (27 packages), `go vet ./...` and
  `GOOS=windows go vet ./...` clean, `gofmt -l .` empty.
- Commits: `578501b` `feat(shiprun): run preset steps as argument lists
  without a shell` (2 files changed, 136 insertions(+), 15 deletions(-));
  `fa2720d` `feat(shipplan): write dotnet project version and release
  notes` (3 files changed, 481 insertions(+)); `4895d34`
  `feat(shipplan): check coverage thresholds from cobertura reports` (2
  files changed, 262 insertions(+)); `6510255` `feat(shipplan): plan
  pipeline stages from presets` (7 files changed, 1733 insertions(+), 4
  deletions(-): 839 tests, 745 code, 149 docs).
- Size: `git diff --shortstat 88562b0 6510255` 14 files changed, 2612
  insertions(+), 19 deletions(-). Above the ~400-line heuristic: about
  half is tests (table-driven plan tests, fixtures, real smoke runs); the
  four commits are separate work units (argv runner, csproj writer,
  coverage check, plan and presets with their docs).

### T5 review

- Native review of 88562b0..c9095d8 (high risk, 4 lenses, user granted):
  approved and acknowledged. Advisory: R1-nuget-key-argv (WARNING: the
  NuGet API key is on `dotnet nuget push` argv, readable by other local
  users through procfs on a shared host such as T2), R4-stale-coverage-
  reports / R4-stale-nupkg-push / R3-stale-artifacts (WARNING: nothing
  empties the artifacts directories, so old reports count toward coverage
  and old packages get pushed), R3-csproj-nonatomic (WARNING: os.WriteFile
  truncates first; the "unchanged on error" promise is false), R2-001
  (WARNING: coverage message formatting duplicated), R2-002 (`context`
  shadows the package), R2-003 (dead `NotesFile`), R2-004 (`writeJSON`
  misnamed), R3-exec-windows-shims (npm/pnpm/yarn are .cmd shims on
  Windows), R3-coverage-rounding-message.

### T5 follow-ups

All T5 review findings fixed (user authorized). Route: delegated (one
writer). Test-first: RED observed before each fix (a build failure, then a
behavioural failure once the new names existed, where a test can show it).

| Finding | Fix | Evidence |
|---|---|---|
| R1-nuget-key-argv | The requested temporary `NuGet.Config` was tried first and does not work on Linux. Observed with the .NET SDK 10.0.112 (NuGet 7.0.3) against a loopback HTTP feed that logs headers: a clear-text `<apikeys>` entry fails with "encryption is not supported on non-Windows platforms" (NuGet always DPAPI-decrypts it); `dotnet nuget push ... @push.rsp` delivers the key, but `/proc/<pid>/cmdline` of the child `dotnet exec .../NuGet.CommandLine.XPlat.dll ... --api-key <key>` shows it on argv; `dotnet exec NuGet.CommandLine.XPlat.dll ... @push.rsp` gives "file does not exist" (no response files); `dotnet nuget setapikey` does not exist; NuGet reads no API key from the environment (string scan of the SDK's NuGet DLLs). The argv-free mechanism chosen instead: `nuget:push` is an in-process step (`pushNuGet`, `nugetpush.go`) implementing NuGet's push protocol. A v3 index is read without the key and its `PackagePublish` / `SymbolPackagePublish` resources are used; any other source is a v2 push URL. Each package is a PUT to `<url>/`, multipart part `package` / `package.nupkg`, header `X-NuGet-ApiKey`, which matches what the real client sends (captured: same part name, filename and content type; the client uses chunked encoding, NERV sends a Content-Length). 409 means skipped (duplicate). Redirects are not followed; plain http is refused unless the host is loopback. A key a feed echoes is replaced by `***`. Step model: `Step.Exec` is now `[]string`, `Arg` removed, `Check` became `Func(ctx, lookupEnv)`, and `Step.Secrets` declares the variables checked before the run. Limitation (documented): a feed whose service index needs credentials (GitHub Packages, Azure Artifacts) needs a publish override. | RED: `undefined: pushNuGet` (build). GREEN: `TestPushNuGet_V3IndexAndSymbols`, `..._V2URLDuplicatesAndFailures` (409 skipped; a 403 echoing the key is redacted), `..._NeverSendsTheKeyElsewhere` (a 307 to another server: not followed, that server got 0 requests; non-loopback http, ftp and garbage URLs refused), `..._Errors`, `TestPlan_NuGetKeyNeverOnACommandLineOrInTheListing` (0 process steps, the key only in the feed's header, not in Render or output). Smoke: the real `Calc.1.2.3-smoke.1.nupkg` is pushed to a loopback fake feed (body byte-identical, key header present). |
| R4-stale-coverage-reports, R4-stale-nupkg-push, R3-stale-artifacts | `Plan.ResetDirs` (the coverage and nuget directories the plan uses); `Executor.Run` removes and recreates them after the secrets check and before the first stage, after checking each one is strictly inside `Context.ArtifactsDir` (lexical `Rel` + `IsLocal`; the root itself is refused too). Other projects' directories are untouched. | RED: `TestExecutor_LeftoversOfAnEarlierRunAreNotUsed` ("line coverage 8.64% (95 of 1100 lines, 2 reports)") and `TestExecutor_ResetsOnlyInsideTheArtifactsRoot` (no refusal). GREEN. `TestExecutor_CoverageCheckRunsInProcess` now writes its report from the test step. |
| R3-csproj-nonatomic | `replaceFile`: resolves symlinks, creates a temp file in the same directory (`createTemp` seam), sets the original's permission bits, then Write, Sync, Close and Rename; on any error the temp file is removed and the original is untouched. | RED (seam only): `TestWriteCsproj_AtomicReplace` write/sync/close failures returned nil; the call order was `[]`. GREEN: the original is byte-identical on each failure, no leftover file, calls `[write sync close]`, mode 0640 kept. `..._ThroughASymlinkKeepsTheLink` passed before and after. |
| R2-001, R3-coverage-rounding-message | `Coverage{..., Threshold}` with `String()` and `belowError()` built on one `measure()`. The percentage is truncated to hundredths in integer arithmetic, and the threshold is compared exactly (`covered*100 < threshold*valid`). The preset's summary is `c.String()`. | RED: `c.String undefined` (build); the old code printed 79.996% as `80.00%`. GREEN: `TestCoverage_TextNeverRoundsUpToTheThreshold` (79996/100000 vs 80 gives `79.99%`; 85.49 vs 85.5; 99.99 pass; 95.00). |
| R3-exec-windows-shims | `checkCmdSafe` refuses `& \| < > ^ % "`, CR and LF in `npm.dist_tag`, `npm.registry` and the channel used as the dist-tag, on every OS (the other node/npm arguments are fixed). Documented in the `shiprun.Step.Exec` comment and in the docs. | RED: 8 cases of `TestBuild_NpmRefusesCmdMetacharacters` built without an error. GREEN. |
| R2-002 | `context` local renamed `buildContext`. | Refactor; tests green. |
| R2-003 | `Context.NotesFile` removed (T6 passes the notes to `WriteCsproj`). | Refactor. |
| R2-004 | `writeJSON` renamed `writeFile`. | In `18a049f`. |

- Also fixed: the fake feed and the smoke test now ignore stray `GET /`
  requests. Another local process on this shared host probes newly opened
  loopback ports, which made the smoke test fail 1 run in 5 ("4
  requests"). `14715ea`.
- Process slip: `18a049f` was first committed with an unformatted test file
  and amended before anything else (unpushed). `9239f17` was committed
  while `go test` piped through `tail` hid a failure; that failure was the
  probe flake above, fixed in `14715ea`. Later checks capture the exit
  status.
- Docs: `docs/release-commands.md` (NuGet push protocol and why, redirects,
  http and index-credential limits, artifacts reset, atomic csproj write,
  Windows `.cmd` shims).
- Checks: `go test -count=1 ./...` exit 0 (27 packages ok, smoke tests
  included: dotnet 10.4s, docker 0.39s), `go vet ./...` and
  `GOOS=windows go vet ./...` clean, `gofmt -l .` empty.
- Commits: `18a049f` `fix(shipplan): push nuget packages without the api
  key on any command line` (9 files changed, 586 insertions(+), 124
  deletions(-)); `3dd25a0` `fix(shipplan): empty coverage and package
  directories before each run` (5 files, +116 -7); `7c84af5`
  `fix(shipplan): replace the dotnet project file atomically` (3 files,
  +155 -3); `9239f17` `fix(shipplan): format coverage results in one place
  without rounding up` (3 files, +77 -16); `14715ea` `test(shipplan):
  ignore stray loopback probes in the fake nuget feed` (2 files, +19 -2);
  `1a1e589` `fix(shipplan): refuse cmd.exe metacharacters in npm publish
  arguments` (5 files, +71 -6); `1d8edbf` `refactor(shipplan): rename the
  docker build context and drop the unused notes file` (2 files, +4 -6).
  Total `git diff --shortstat 288701f 1d8edbf`: 14 files changed, 1024
  insertions(+), 160 deletions(-).

### T5 follow-up review

- Native review of 288701f..7078e96 (high risk, 4 lenses, user granted):
  approved and acknowledged. Advisory: R4-nuget-partial-push-unreported and
  R3-partial-push-summary-lost (WARNING: a push that fails part-way does not
  name the packages already accepted), R3-atomic-test-close-leak (WARNING,
  test only: the injected close failure leaks the temp descriptor, which
  breaks cleanup on Windows), R1-nuget-partial-key-echo (redact before
  truncating the feed's error body), R4-nuget-put-body-not-rewindable
  (no GetBody for retried requests), R2-nugetpush-client-param-unused,
  R2-multipart-tail-handbuilt, R2-nuget-protocol-version-constant,
  R2-replacefile-closed-flag, R3-nupkg-glob-metachar.

### T5 final pass

Bounded final pass on the T5 follow-up review's findings (user authorized).
Route: delegated (one writer). Test-first; `go test` exit codes captured
directly.

| Finding | Fix | Evidence |
|---|---|---|
| R4-nuget-partial-push-unreported, R3-partial-push-summary-lost | `pushNuGet` lists every upload (packages and `.snupkg`) up front; on a failure the error is `pushed: ...; already on the feed: ...; failed on <file>: <cause>; not attempted: ...` (`nothing pushed; ...` when the first one fails). The success summary uses the same `pushed:` wording. Docs: the partial-push message and how to retry. | RED: `TestPushNuGet_FailurePartWayNamesWhatTheFeedAccepted` got `pushing C.1.0.0.nupkg: HTTP 500 ...`. GREEN (statuses 201, 201, 409, 500 with a fifth package left unpushed; plus the first-package failure). |
| R1-nuget-partial-key-echo | `errorText`: reads up to `errorBodyReadLimit` (64 KiB), redacts the whole key, cuts to `errorBodyShowLimit` (512), then replaces a trailing key prefix of 4 or more characters with `***`. | RED: `TestPushNuGet_KeyStraddlingTheErrorCutIsHidden` showed the key's first 4+ characters for a key at offsets 500 and 508. GREEN (offsets 500, 508, 511, 0). |
| R4-nuget-put-body-not-rewindable, R2-multipart-tail-handbuilt | New `uploadRequest`: `GetBody` reopens the file and rebuilds the same head + file + tail. A comment says why the closing boundary is hand-built: the body is streamed with a known ContentLength, and `multipart.Writer` only closes a part it wrote whole. | RED: `undefined: uploadRequest`. GREEN: `TestUploadRequest_BodyParsesAndRewinds` (ContentLength equals the body, `GetBody` gives identical bytes, `mime/multipart` parses one `package` / `package.nupkg` part and then `io.EOF`). |
| R3-nupkg-glob-metachar | `nupkgs`: `os.ReadDir` filtered to regular `*.nupkg` files (name order), no glob. | RED: `TestPushNuGet_DirectoryNameWithGlobCharacters` (`art[1]`) got "no *.nupkg". GREEN; a directory named `Dir.1.0.0.nupkg` is not pushed. |
| R3-atomic-test-close-leak | Test fake: `Close` calls the real Close first, then returns the injected error. | RED: the new assertion (the real file is closed, `Write` gives `os.ErrClosed`) failed for the close case. GREEN; the original stays byte-identical and no temp file remains. |
| R2-nugetpush-client-param-unused | Kept the parameter. `defaultPushClient()` (each request bounded by `nugetPushTimeout`, 10 min, documented in the code and the docs); a test injects a 200 ms client against a hanging feed. | RED: `undefined: defaultPushClient`. GREEN: fails as `failed on A.1.0.0.nupkg` well under 5s. |
| R2-nuget-protocol-version-constant | `serviceIndexLimit` (8 MiB), `errorBodyReadLimit` and `errorBodyShowLimit` are named with a rationale. For `"4.1.0"` no rationale could be verified: the real NuGet 7.0.3 push (captured in the follow-ups) sends `X-NuGet-ApiKey`, `X-NuGet-Client-Version` and `X-NuGet-Session-Id`, and no `X-NuGet-Protocol-Version`. So the header was removed (deviation from the request, to match the real client). | `TestUploadRequest_SendsOnlyTheHeadersTheNuGetClientSends` written with the change. |
| R2-replacefile-closed-flag | Renamed `closeAttempted`, with a comment (a failed Close has still released the file). | Refactor; tests green. |

- Checks: `go test -count=1 ./...` exit 0 (27 packages ok); smoke tests
  `go test -count=1 -run Smoke -v ./internal/shipplan/` exit 0 (dotnet
  10.56s: setup 2.7s, pipeline 7.9s; docker 0.29s); `go vet ./...` exit 0;
  `GOOS=windows go vet ./...` exit 0; `gofmt -l .` exit 0, no output.
- Commits: `76ef2ce` `fix(shipplan): name the packages a failed nuget push
  already delivered` (4 files, +79 -22); `2c06212` `fix(shipplan): redact
  the nuget key before cutting a feed's error text` (2 files, +49 -2);
  `6df7f4e` `fix(shipplan): let net/http resend a nuget upload body` (2
  files, +107 -26); `76e69a0` `fix(shipplan): list nuget packages without
  glob patterns` (2 files, +40 -5); `5523a53` `test(shipplan): close the
  real temp file on an injected close failure` (1 file, +15 -2);
  `d461038` `refactor(shipplan): name the nuget push limits and keep only
  the client's headers` (4 files, +61 -9). Total `git diff --shortstat
  19473b5 d461038`: 6 files changed, 348 insertions(+), 63 deletions(-).

### T5 final pass review

- Native review of 19473b5..bf7de86 (medium, user granted): approved and
  acknowledged. R3-nupkgs-skips-symlinked-packages (WARNING: `os.ReadDir`
  entries report a symlink's own type, so a linked `.nupkg` was silently
  skipped) and R3-partial-push-error-drops-cause-chain (the part-way error
  lost the cause, so a cancellation could not be told from a refusal) were
  fixed inline by the parent in `433d0cb`: packages are checked with
  `os.Stat` (links followed); the part-way error wraps the cause with `%w`,
  and the key-redacted error answers `errors.Is` for `context.Canceled` /
  `DeadlineExceeded` without unwrapping to text that holds the key. RED: the
  symlinked package was not pushed, and `errors.Is(err,
  context.DeadlineExceeded)` was false; GREEN after the fix (`go test
  ./...`, `GOOS=windows go vet ./...`, `gofmt -l .` clean). The first draft
  of the cancellation test hung because its fake server never saw the
  client disconnect; it now uses the release-channel pattern of the
  neighbouring test.

### T6

- Route: delegated (writer trigger: semrel real-run mode, a CLI command,
  shipplan additions, docs; 2+ non-trivial files).
- `internal/semrel`: `Workspace` (`NewWorkspace(ctx, WorkspaceOptions{RepoRoot,
  Branch, Env, CacheDir, FetchTimeout})`; `Dir`, `ArtifactsDir`, `NotesDir`,
  `OriginHead`, `PublicURL`, `Compute`, `CheckPush`, `Release`,
  `TagOnOrigin`, `Close`), `ReleaseRequest`, `ReleaseURL`, `ErrNoOrigin`,
  `ErrVersionMismatch`, `ErrPushDenied`. Refactor kept behaviour:
  `prepareNode` / `checkPlan` shared by Compute and Release; `fetchEnv` built
  on new `withGitConfig` + `promptFreeEnv`; `node()` can stream
  semantic-release's log.
- Release clone (decided): `git clone --quiet --shared --no-checkout
  --no-tags <user repo> <tmp>/<repo name>` (named like the repository so a
  sectionless project keeps its name), origin set to the push address,
  `git fetch --prune origin +refs/heads/*:refs/remotes/origin/*
  +refs/tags/*:refs/tags/* +refs/notes/*:refs/notes/*` (local-only branches
  and tags never reach it), `checkout -B <branch> refs/remotes/origin/<branch>`.
  Removed at the end on success and failure (path printed while running);
  artifacts and notes live in sibling temp dirs outside the clone.
- Credentials (decided): the push address is `publicURL(origin)` (no user
  info) or, for a local origin, its resolved path; when the real URL differs
  (it carries credentials) git gets `url.<real>.insteadOf=<push address>`
  through `GIT_CONFIG_COUNT` only, so semantic-release passes a
  credential-free URL on argv and git rewrites it. Prompt-free env (as the
  T2 fetch). `CheckPush` runs `git push --dry-run --no-verify origin
  HEAD:refs/heads/<branch>` before the first release, which also keeps
  semantic-release's getGitAuthUrl from building a token URL (it does so
  only when that check fails). GH_TOKEN: environment, else `gh auth token`,
  passed only in the runner's env; never to the pipeline.
- Compute on the clone runs offline (decision, deviation from "online"):
  the clone was just fetched from origin with prune, so the Compute mirror
  gets exactly origin's heads (`refs/remotes/origin/*`), tags and notes;
  origin is read once per ship run.
- Runner release mode: `{"mode":"release", ..., "expectedVersion"}` = one
  real `semanticRelease({dryRun: false, ci: false, repositoryUrl,
  tagFormat})` with the repository's own steps, `analyzeCommits` /
  `generateNotes` wrapped as in run mode, and `verifyRelease` wrapped: a next
  version other than `expectedVersion` throws `ENERVVERSIONMISMATCH` before
  prepare and the tag (no `semanticRelease` flag, so the repository's fail
  plugins are not called). Go maps it, a different released version, and
  "released nothing" to `ErrVersionMismatch`.
- `internal/shipplan` (small additive): `Executor.RunHooks(ctx, plan, key,
  vars)`, `StepFailure.Index`, `WriteCsprojVersion`; Render wording (publish
  runs after semantic-release tags; hook timing).
- `cmd/nerv/ship.go`: flags `--cwd --branch --project --dry-run --yes
  --fetch-timeout --json`; seams `OpenShipWorkspace`, `ShipSteps`,
  `GitHubToken` in `options`. Flow: config/project refusal, local branch
  sha, workspace, refuse when origin's head is missing or differs (exit 1,
  "push or pull first"), Compute (`--project` keeps only that project),
  plans from the clone's config, listing on stderr, confirmation (`[y/N]`
  on a terminal, `--yes`, refusal without a terminal, never in a dry run),
  push check + GH token (real runs only), then per project: notes file,
  csproj (dotnet preset: release link from `semrel.ReleaseURL`, or version
  only with a warning), pipeline, prepare hooks, [dry run stops], release,
  publish stage, publish hooks, success hooks. Failures after the release
  started run the fail hooks (skipped when interrupted) and set an action
  (tag on origin checked with `ls-remote`); later projects are
  `not-started`. Summary on stdout (text or `--json` outcomes). Exit 0 / 1
  (refusal) / 2 / 128+signal.
- RED: `go test ./internal/semrel/` -> build failed (`undefined:
  ReleaseURL`, `pushAddress`, `Workspace`, `NewWorkspace`, ...);
  `go test ./internal/shipplan/` -> build failed (`RunHooks undefined`,
  `f.Index undefined`, `undefined: WriteCsprojVersion`); `go test
  ./cmd/nerv/` -> build failed (`opts.OpenShipWorkspace undefined`,
  `undefined: shipWorkspace`, `undefined: shipOutcome`). GREEN: each `ok`;
  one cmd test fix after the first GREEN run (a case-sensitive "nothing was
  tagged" match).
- Integration (real git, node 24, npm, bash; local bare origins only):
  `TestIntegration_WorkspaceReleasesToOrigin` (semrel): a 9.9.9 expectation
  stops before the tag (origin tags still only `v1.0.0`); the real run
  pushes `v1.1.0` on origin's main, head `chore(release): 1.1.0`, CHANGELOG
  with the feat, semantic-release log streamed; user refs and checkout
  unchanged. First run RED on the fixture: a bare origin whose HEAD names a
  missing branch fails semantic-release's `git fetch --tags <url>`; the
  fixture sets HEAD to main (as on hosted origins).
  `TestIntegration_ShipReleasesWhatOriginHas` (cmd): `--dry-run` -> origin
  refs unchanged, marker `build ... dry=true`, `prepare ... dry=true`;
  without `--yes` and no terminal -> exit 1, no step ran; `--yes` -> tag
  `v1.1.0` = origin's main, `chore(release): 1.1.0`, CHANGELOG, marker
  build, prepare (after `test -s $NERV_RELEASE_NOTES_FILE`), publish-stage,
  publish-hook, success with version/tag/`dry=false`; user's refs and
  status unchanged; a second run (user's main now behind) -> exit 1 "push or
  pull". `TestIntegration_ShipMonorepoInOrder`: api (`feat`, path api) and
  web (`fix`, path web) -> `api-v1.1.0` and `web-v1.0.1` on origin, two
  release commits in order, docs "no new release", 10 marker lines api then
  web. Both pass (18.2s and 8.8s).
- Checks: `go test -count=1 ./...` exit 0 (27 packages); `go test -tags
  semrel_integration -run Integration -count=1 ./internal/semrel/` exit 0;
  `go test -tags semrel_integration -run Integration -count=1 ./cmd/nerv/`
  exit 0; `go vet ./...`, `GOOS=windows go vet ./...`, `go vet -tags
  semrel_integration ./cmd/nerv/ ./internal/semrel/` exit 0; `gofmt -l .`
  empty.
- Known limitations: the release clone does not initialise submodules or
  Git LFS objects; an origin whose HEAD names a missing branch fails
  semantic-release's own fetch; a `--project` release applies dependency
  bumps from dependencies it does not release (as `version next
  --project`); semantic-release's addChannel step (moving earlier releases
  to a channel) runs before the version guard; Windows paths are
  compile-checked only.
- Commits: `751fd4c` `feat(semrel): run semantic-release for real in a
  release clone` (7 files changed, 1005 insertions(+), 50 deletions(-));
  `8af9297` `feat(shipplan): run lifecycle hooks and name where a stage
  failed` (6 files changed, 156 insertions(+), 14 deletions(-)); `6347f25`
  `feat(cli): add nerv ship` (8 files changed, 1800 insertions(+), 26
  deletions(-)). Total `git diff --shortstat 910004e 6347f25`: 21 files
  changed, 2961 insertions(+), 90 deletions(-). Above the ~400-line
  heuristic: about 1450 lines are tests (unit and integration) and 170
  docs; the three commits are separate work units (engine real run,
  executor additions, CLI).

## Decisions before T6 (user, 2026-10-14)

- `<PackageReleaseNotes>` holds the release link
  (`https://<host>/<org>/<repo>/releases/tag/<tag>`, from origin's public URL),
  as dim.comprobantes.api's `bump-csproj.sh` does; without an origin the
  element is left unchanged with a warning.

### T6 review

- Native review of 910004e..6acd661 (high risk, 4 lenses, user granted).
  One corroborated CRITICAL, R3-project-filter-drops-depends-on: with
  `--project` the dependency results that drive `depends_on` were dropped
  before the real release, so a dependent raised by a dependency failed the
  version guard (safely, before any tag). Native correction (plan 60 lines,
  budget 200): fixed in `ef9be0b` (71 lines) by the T6 writer — ship computes
  every project and filters only what it releases; RED/GREEN with a unit
  test and a real integration test (`app-v1.1.0` released from a `core`
  minor bump, `core` not released). The first targeted-validator run was
  killed (`signal: killed`, nothing mutated); the exact-lineage STATUS
  re-offered the same slot and the rerun validated the correction: approved
  and acknowledged.
- Advisory findings left: R1-001 (WARNING: a credentialed origin URL sits
  in the workspace environment as an insteadOf key), R4-version-mismatch-
  recovery-advice (WARNING: recovery advice can be wrong when another
  version was released), R2-001 (WARNING: every `git remote get-url`
  failure becomes ErrNoOrigin), R2-002 (WARNING: the version guard is
  matched by error-text substring), R4-json-summary-missing-on-early-failure,
  R4-rerun-advice-refused-after-pushed-release-commit, R2-003 (duplicated
  csproj writer body).

### T6 follow-ups

All seven advisory findings of the T6 review fixed (user authorized).
Route: delegated (one writer). Test-first; `go test` exit codes captured
directly.

| Finding | Fix | Evidence |
|---|---|---|
| R1-001 | A credentialed origin URL is written only into the release clone's `.git/config` as `[url "<real>"] insteadOf = "<push address>"` (subsection and value quoted and escaped; line breaks refused; file chmod 0600 inside the 0700 temp dir removed at the end); the workspace environment holds only prompt-free settings. Manual real-git check: a clone-local rule rewrites both `git remote get-url origin` and an explicit URL. Docs: credentials (a credential helper is still recommended: git error output may echo the URL). | RED: `TestNewWorkspace_ClonesReadsOriginAndChecksOutOriginsBranch` (no rule in the config file; secret in the fetch env), `TestWorkspace_ReleaseIsARealRunGuardedByTheComputedVersion` (secret in the runner env, `noSecretAnywhere` over every command's argv and env). GREEN. |
| R4-version-mismatch-recovery-advice | When semantic-release released another version whose tag is on origin, the action says which version and tag exist, that the publish stage did not run (built for the confirmed version), how to publish or delete it, to pull, and not to rerun. The guard-stopped case keeps "stopped before the tag". | RED: build failure of the new tests, then `TestShip_ReleasedAnotherVersionSaysWhichTagExists` and the extra "stopped before the tag" assertion. GREEN. |
| R2-001 | `NewWorkspace` lists remotes (`git remote`): only a missing `origin` is `ErrNoOrigin`; a failing listing or `get-url` is returned with its cause (exit 2 in the CLI). | RED: `TestNewWorkspace_RemoteListingFailureIsAnEnvironmentError` (got ErrNoOrigin for `context.Canceled`). GREEN. |
| R2-002 | Runner lib `versionGuard(expected, actual)` throws a typed error (`code`, `expected`, `actual`, no `semanticRelease` flag); `failureResult` adds `versionMismatch {expected, actual}` to the result; Go checks `runnerResponse.VersionMismatch`, no text matching. | RED: node `does not provide an export named 'failureResult'`; Go `runner guard` case (structured field, text without the code) and `TestWorkspace_ReleaseErrorTextIsNotAMismatchSignal` (code only in a message -> must not be a mismatch). GREEN: node 13/13; the semrel integration 9.9.9 guard passes through the real runner. |
| R4-json-summary-missing-on-early-failure | `--json` always prints `{status, error, projects}` (`released`, `dry-run`, `nothing-to-release`, `refused`, `declined`, `failed`, `interrupted`); `projects` is `[]` before computing. Shape change from the T6 array (unreleased). Documented. | RED: build failure (`undefined: shipDocument`); GREEN: `TestShip_JSONDocumentOnEveryPath` (8 paths, exit codes 0/1/2/130). |
| R4-rerun-advice-refused-after-pushed-release-commit | `Workspace.BranchOnOrigin` (ls-remote of the branch); every "run again" advice says to `git pull` first when origin's branch moved during the run (or to check when it cannot be read). | RED: build failure; GREEN: `TestShip_RerunAdviceSaysPullWhenOriginMoved` (moved / unchanged). `BranchOnOrigin`'s unit check was written with the method (no separate RED). |
| R2-003 | `writeCsproj(path, version, props)` shared by `WriteCsproj` and `WriteCsprojVersion`. | Refactor; csproj tests green. |

- Checks: `go test -count=1 ./...` exit 0 (27 packages); `go test -tags
  semrel_integration -run Integration -count=1 ./internal/semrel/
  ./cmd/nerv/` exit 0; `go vet ./...`, `GOOS=windows go vet ./...`, `go vet
  -tags semrel_integration ./cmd/nerv/ ./internal/semrel/` exit 0; `gofmt
  -l .` empty.
- Commits: `443a71d` `refactor(shipplan): share one csproj writer between
  the version and notes writes` (1 file changed, 10 insertions(+), 21
  deletions(-)); `32f42df` `fix(semrel): keep origin credentials out of the
  environment and type the version guard` (7 files changed, 196
  insertions(+), 64 deletions(-)); `68eb48a` `fix(cli): print one json
  document, say which tag a wrong release left, and pull before a rerun` (5
  files changed, 260 insertions(+), 38 deletions(-)). Total `git diff
  --shortstat 3b486a7 68eb48a`: 10 files changed, 466 insertions(+), 123
  deletions(-).

### T6 follow-up review

- Native review of 3b486a7..52aa916 (medium, user granted): approved and
  acknowledged. Advisory: R3-rerun-advice-error-branch-untested (WARNING:
  no test for the BranchOnOrigin failure branch, also taken on interrupted
  runs), R3-rerun-advice-deleted-branch-says-moved (a branch deleted on
  origin is reported as "moved, git pull"), R3-origin-rewrite-linebreak-
  untested (the CR/LF/NUL refusal and the quoting of the insteadOf rule have
  no test).

## Next step

Review of T6 (910004e..6347f25), then the T6 pull request
(`feat/release-ship` -> develop), then T7 (`/nerv:ship` and the final docs
pass).
