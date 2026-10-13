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
- [x] T2 — node engine: embedded runner using semantic-release's API, pinned
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

- PR #91 (T1): `feat/release-commands` → develop, commits f2f7801..294f7b3. Merged 7a23170.
- T2 branch `feat/release-version-engine` from 7a23170.

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
- Not run: `nerv version` vs `npx semantic-release --dry-run` on a real
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

## Next step

T3 (`nerv version` CLI) on top of T2.
