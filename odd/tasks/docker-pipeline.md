# Docker pipeline: `nerv pipeline`, a standalone copy of dim's CI/CD

## Objective

A standalone pipeline, `nerv pipeline <step>` and `nerv pipeline run`, that
reproduces dim.comprobantes.api's GitHub Actions CI/CD for projects that
deploy a Docker image, with one process per workflow step and an optional
deploy, run locally without GitHub Actions.

## Why

User request (2026-10-15): use dim.comprobantes.api's release setup as the
default when a repository has none, with a configurable workflow. Clarified:
only Docker-deployed projects for now; based on dim's workflow; one process
per step; runnable standalone; ignore any existing GitHub workflow;
configurable with the data it needs; deploy option (Docker only for now).

## Decisions (user, 2026-10-15)

- Scope: projects that deploy a Docker image.
- Model: dim.comprobantes.api's `.github/workflows/ci-cd.yml`; any existing
  GitHub workflow in the target repository is ignored.
- Default branches: `main` (stable), `develop` (alpha), `release/*` (rc,
  replaces dim's single `release/rc`), `hotfix/*` (beta).
- Default semantic-release configuration: dim's (conventionalcommits with
  `BREAKING CHANGE(S)` / `BREAKING` note keywords, changelog, git, github),
  used when the repository has none of its own.
- Deploy data source configurable: plain environment variables or Infisical
  (dim's `/update-deploy` layout: `DOCKER_SSH_*` at the environment root,
  `DOCKER_*` under `/<app>/`).

## Mapping of dim's workflow (evidence: odd exploration, 2026-10-15)

| Step | dim job | Standalone behaviour |
|---|---|---|
| `validate` | validate | `dotnet restore <sln>`; `dotnet format <sln> --verify-no-changes` |
| `test` | test | `dotnet build <sln> -c Release --no-restore -warnaserror`; `dotnet test ... --no-build --collect "XPlat Code Coverage" --settings <runsettings> --logger trx`; coverage thresholds from the runsettings |
| `release` | release | semantic-release (repo config or the built-in default) on origin's state; outputs published, version, channel, tag |
| `docker` | docker (matrix) | per image: lowercase name, computed build args (e.g. Chrome version from Google's apt index), registry login, tags `<version>`, `<channel>` (not default), `latest` on main only, OCI labels, build + push `linux/amd64`, one retry with recomputed args |
| `sync` | sync-to-develop, sync-main-to-release | main or `release/*` merged `--no-ff` into develop; main merged into every `release/*` (abort and continue on conflict, fail at the end), "[skip ci]" messages, push |
| `deploy` | (outside CI: `/update-deploy`) | resolve hosts and containers (env or Infisical), SSH, `<script> pull/up --container <name>-<variant>`, verify `(healthy)` and the image version label |

## Technical design (parent's proposal)

- New command family `nerv pipeline validate|test|release|docker|sync|deploy`
  plus `nerv pipeline run` (all steps in order, stopping at the first
  failure; `--dry-run`; confirmation before irreversible steps), and a
  `/nerv:pipeline` slash command. `nerv ship` and `nerv version next` stay.
- A `pipeline:` section in the local `.nerv/nerv.yaml` with only the data
  needed: solution, runsettings, images (name, Dockerfile, context, build
  args incl. computed ones), registry and its credential env var, deploy
  (source env|infisical, environments, SSH auth, compose script, container
  name, health verification).
- Reuse: `internal/semrel` (version, real release on a throwaway clone),
  `internal/proc` / `internal/shiprun` (processes), `internal/shipplan`
  (dotnet steps, NuGet-style secret handling patterns).
- The built-in default semantic-release configuration also applies to
  `nerv version next` and `nerv ship` when the repository has none.

## Tasks

- [x] P1 — `pipeline:` configuration schema, validation and docs.
- [x] P2 — built-in default semantic-release configuration (dim's, with
  `release/*`), used by the engine when the repository has none.
- [x] P3 — `validate` and `test` steps.
- [ ] P4 — `release` step (reuse the ship workspace and version guard).
- [ ] P5 — `docker` step (tags, computed build args, login, push, retry).
- [ ] P6 — `sync` step.
- [ ] P7 — `deploy` step (env or Infisical; SSH; health and version check).
- [ ] P8 — `nerv pipeline run`, `/nerv:pipeline`, guide, CHANGELOG.

## Acceptance criteria

- On a copy of dim.comprobantes.api with only a `pipeline:` section (no
  semantic-release config, no workflow), `nerv pipeline run --dry-run`
  performs validate, test, the version computation and the image builds
  matching dim's CI (same version and tags).
- Each step runs on its own.
- Deploy works from env vars and from Infisical (verified against a fake
  SSH target / fake Infisical CLI in tests; no real server touched without
  the user's authorization).
- `go test ./...`, `go vet ./...`, `GOOS=windows go vet ./...` pass;
  `gofmt -l .` clean.

## Delivery

Branch `feat/pipeline-config` from fbe03dd. Forecast well above the ~400
line budget (roughly 3500+ lines across eight tasks). Strategy:
`ask-on-risk`; chain strategy (user, 2026-10-15): `stacked-to-main` (one PR
per task, merged into develop in order).

## Progress

- PR #105 (P1): `feat/pipeline-config` → develop. Merged f1a69de.
- PR #106 (P2): `feat/pipeline-default-semrel` → develop. Merged 87cf385.
- P3 branch `feat/pipeline-validate-test` from 87cf385.
- P2 branch `feat/pipeline-default-semrel` from f1a69de.

Branch created from fbe03dd.

### P1

- Route: delegated (one bounded writer; trigger: 2+ non-trivial files).
- Commit: this commit, `feat(pipelineconfig): add the pipeline configuration schema`.
- Shortstat (code + docs, without this document): 12 files changed,
  2247 insertions(+), 359 deletions(-). About 360 of the insertions are
  shipconfig's block-locating and strict-walk code moved, not new; ~640 are
  tests and ~300 docs.
- RED: `go test ./internal/pipelineconfig/` failed to build (package
  absent); then the branch-validation case failed (`prerelease: true`
  decoded as the id "true"), fixed by rejecting it explicitly.
- GREEN: `go test -count=1 ./...`, `go vet ./...`,
  `GOOS=windows go vet ./...` exit 0; `gofmt -l .` empty.
- Package `internal/pipelineconfig`; shared helpers extracted to
  `internal/shipconfig/yamlsection` (Locate, UnknownFields, DecodeProblems,
  NormalizePath, PathProblem); shipconfig now delegates to it, messages
  unchanged, its tests untouched and green.
- Decisions:
  - Deploy env namespacing for `source: env`:
    `NERV_DEPLOY_<ENVIRONMENT>_<KEY>`, environment upper-cased with `-` to
    `_`; keys are Infisical's (`DOCKER_SSH_*`, `DOCKER_COMPOSE_*`,
    `DOCKER_CONTAINER_*`, `DOCKER_DEPLOY_*`, `DOCKER_IMAGE*`); colliding
    prefixes rejected.
  - Deploy order default: dim's processing order production-green,
    production-blue, staging, development (update-deploy.sh table rank),
    not the discovery order of `UPDATE_DEPLOY_ENVS`.
  - `release/*` defaults to prerelease `rc` on channel `beta` (dim's
    package.json `release/rc`), so image tags match dim's CI; `hotfix/*`
    gets the fixed id `beta`; `prerelease: true` is rejected.
  - `dotnet` is required (the only supported stack); `deploy` optional.
  - Images: name lowercased (as ci-cd.yml does) and must start with
    `registry.host/`; `required` defaults to true and dim's broker is
    documented as required too (its `continue-on-error` only enables the
    retry; a failed retry fails the job).
  - `ssh.known_hosts` defaults to `~/.ssh/known_hosts` (dim's
    `~/.config/update-deploy/known_hosts` is shown in the example);
    `infisical.domain` defaults to the CLI session's; `verify.timeout`
    120s (HEALTHCHECK start 20s + 3 x 30s).
  - `*_env` fields must be upper-case names; anything else is reported as a
    probable secret and never echoed.
  - Added `sync.develop` / `sync.release` (defaults develop, release/*) and
    image `title` / `tags`, needed by P5/P6.
- Open: semantic-release behaviour of a glob `release/*` with a fixed
  prerelease id when several release branches exist (P2 to verify).

### P1 review

- Native review of fbe03dd..45c32a8 (medium, user granted): approved and
  acknowledged. R3-001 (WARNING: the first release branch, used as the main
  branch, could be a glob) and R3-002 (a solution with surrounding spaces
  passed validation) fixed inline by the parent in `749fa64`; RED: both
  new validation cases returned no error; GREEN after the fix.
- Release branches keep dim's model: `release/*` uses prerelease id `rc`
  (versions `X.Y.Z-rc.N`) and channel/image tag `beta`, as dim's
  `release/rc` does today.

### P2

- Route: delegated (one bounded writer; trigger: engine, node runner, CLI,
  docs: 2+ non-trivial files).
- Detection is semantic-release's own: the runner's plan mode returns the
  `configFile` cosmiconfig("release") finds (cosmiconfig 9.0.2, strategy
  `none`: 21 search places in the repository root only, package.json only
  with a `release` key); "" means none. Not re-implemented in Go (the list
  includes `.config/releaserc*` and `.ts` forms and a meta config).
- Built-in configuration (`defaultConfig(branches)`, runner lib): dim's
  package.json `release` (read-only) — commit-analyzer and
  release-notes-generator with `conventionalcommits` and note keywords
  `BREAKING CHANGE`, `BREAKING CHANGES`, `BREAKING`; changelog
  `CHANGELOG.md` (project changelogs rewrite it as before); git with assets
  the changelog and its default message (dim sets none); github. Left out:
  dim's `@semantic-release/npm` (`npmPublish: false`) and
  `@semantic-release/exec` (bump-csproj, set-release-outputs). Packages:
  bundled + default pins only. Applied as API options (`branches`,
  `plugins`) over the empty repository configuration.
- Branches: Go always sends them: the local `pipeline:` section's
  (`pipelineconfig.Load`, the user's working tree; in a Workspace the
  user's top level), or `pipelineconfig.DefaultBranches()` when there is no
  section — one source of truth. An invalid `pipeline:` section only fails
  when the default is used. No pipelineconfig change was needed.
- Item 3 evidence (semantic-release 24.2.3, local temp repo, dry run,
  branches as the default): with `release/1.1` and `release/1.2` on origin,
  a fixed `prerelease: rc` on `release/*` fails on either branch with
  `EPRERELEASEBRANCHES` ("Each pre-release branch ... must have a unique
  prerelease property"); with one release branch it computes `1.1.0-rc.1`;
  a template id `rc-${name...}` works with two branches but yields
  `1.1.0-rc-1-1.1`, breaking dim's `X.Y.Z-rc.N` versions and image tags.
  Choice: keep dim's fixed `rc` and refuse first, before semantic-release,
  when a fixed-id glob of the built-in branches matches more than one
  branch on origin (`ErrFixedPrereleaseGlob`, naming them; Compute checks
  the mirror's heads, Release the clone's `refs/remotes/origin/*`). A
  repository with its own configuration keeps semantic-release's own error.
- Source: `ProjectResult.SemanticReleaseConfig` (`repository` / `default`)
  and `SemanticReleaseConfigFile`; `nerv version next` text line
  `semantic-release config: repository (<file>)` / `built-in default
  (dim)`, JSON fields; `nerv ship` stderr line and `--json` document field
  `semantic_release_config`. The env format keeps the 12 action outputs.
- RED: node `does not provide an export named 'defaultConfig'`; Go build
  failures (`UseDefault`, `Branches`, `defaultBranch` undefined); CLI build
  failure (`shipDocument.SemanticReleaseConfig`). One fixture correction
  (a test `pipeline:` section without `develop` failed its sync
  validation). `TestVersionNext_JSONSingleProjectIsAnObject` now expects
  the new key (15).
- GREEN: node 18/18; Go unit tests (default on no config, pipeline
  branches replace the defaults, repository config kept with its file, the
  fixed-id glob refused before semantic-release runs, the glob matcher
  table); CLI source tests.
- Integration (real semantic-release, local bare origin):
  `TestIntegration_BuiltInDefaultWhenTheRepositoryHasNone` — no config:
  develop `1.1.0-alpha.1` (alpha), `release/1.0` `1.0.1-rc.1` (beta, tag
  `v1.0.1-rc.1`), source `default`; with `release/1.1` pushed too,
  `release/1.0` is refused naming `release/1.0, release/1.1`.
  `TestIntegration_ConfigDetectionForms` — package.json `release`,
  `.releaserc`, `.releaserc.{json,yaml,yml,js,cjs,mjs}`,
  `.config/releaserc.json`, `release.config.{js,cjs,mjs}` found;
  package.json without `release` and an unrelated file: none.
  `TestIntegration_SingleProjectPrerelease` now also asserts source
  `repository` (`package.json`).
- Checks: `go test -count=1 ./...` exit 0 (28 packages); `go test -tags
  semrel_integration -run Integration -count=1 ./internal/semrel/
  ./cmd/nerv/` exit 0; node runner tests 18/18; `go vet ./...`,
  `GOOS=windows go vet ./...`, `go vet -tags semrel_integration` exit 0;
  `gofmt -l .` empty.
- Commits: `aebc254` `feat(semrel): use dim's semantic-release
  configuration when a repository has none` (12 files changed, 601
  insertions(+), 25 deletions(-)); `db3c048` `feat(cli): say which
  semantic-release configuration was used` (4 files changed, 76
  insertions(+), 5 deletions(-)). Total `git diff --shortstat 1c9ba15
  db3c048`: 16 files changed, 677 insertions(+), 30 deletions(-). Above the
  ~400-line heuristic: about 300 lines are tests and 80 docs; the engine
  and the CLI line are separate commits.

### P2 review

- Native review of 1c9ba15..b2c0412 (medium, user granted): approved and
  acknowledged. Advisory: R3-release-default-path-untested (WARNING: the real
  release path with the built-in default — UseDefault, Branches, and the
  fixed-id glob check reading `refs/remotes/origin/` rather than the mirror's
  heads — has no test), R3-default-project-changelog-unproved (a project
  changelog together with the built-in default is not tested),
  R3-glob-matcher-divergence (`path.Match` vs semantic-release's micromatch:
  `**` and braces are not handled and match errors are ignored).

### P2 follow-ups

All three P2 review advisories addressed (user authorized). Route:
delegated (one writer). Test-first; `go test` exit codes captured
directly.

| Item | Fix | Evidence |
|---|---|---|
| R3-release-default-path-untested | New end-to-end test found a real defect: with the built-in configuration, a real release on an origin outside GitHub stopped with `EINVALIDGITHUBURL; ENOGHTOKEN` (nothing tagged). `@semantic-release/github` is now included only when origin is on GitHub (`github.com`, or `GH_HOST`); `defaultConfig(branches, {github})`, request field `githubRelease`, `remoteIsGitHub`. The fixed-id check is one helper (`checkPrereleaseBranches`) for both paths: Compute reads the mirror's `refs/heads/` (set to origin's by `planHeads`), Release the clone's `refs/remotes/origin/` (fetched with prune); both are origin's branches as last fetched, so they did not differ — now one code path. | RED: `TestIntegration_ShipWithTheBuiltInConfiguration` (exit 2, the GitHub error); node and Go build/test failures for the new option and `remoteIsGitHub`. GREEN: `nerv ship --yes --json` on develop -> `v1.1.0-alpha.1` on origin's develop, head `chore(release): 1.1.0-alpha.1`, release commit holds only `CHANGELOG.md`, `semantic_release_config: default`; `TestIntegration_ReleasePathRefusesTwoReleaseBranches` (release/1.1 and release/1.2 on origin -> refused, origin tags unchanged); unit `TestWorkspaceRelease_BuiltInConfigurationRequest` (GitHub / other origin / two release branches), `TestRemoteIsGitHub`. |
| R3-default-project-changelog-unproved | No code change needed: the changelog rewrite applies to the built-in plugin list (it rewrites the loaded options' plugins). | `TestIntegration_BuiltInConfigurationMonorepoChangelogs`: api and web, no repo config -> `api-v1.1.0` holds only `api/CHANGELOG.md`, `web-v1.0.1` only `web/CHANGELOG.md`. It could only run once the GitHub fix let real releases work on a local origin. |
| R3-glob-matcher-divergence | `pipelineconfig` rejects `**`, `{`/`}` and extglobs (`@(`, `!(`, `+(`, `*(`, `?(`) in branch names: "uses glob syntax nerv does not support (only *, ? and [...] are)", path-qualified. `checkFixedPrereleaseGlobs` returns `path.Match` errors. Docs: supported syntax in docs/pipeline.md. | RED: 8 unsupported patterns loaded without error; GREEN; `release/*`, `release/v?`, `hotfix/[a-z]*` still load. `TestCheckFixedPrereleaseGlobs_InvalidPatternIsAnError`. |

- Checks: `go test -count=1 ./...` exit 0 (28 packages); `go test -tags
  semrel_integration -run Integration -count=1 ./internal/semrel/
  ./cmd/nerv/` exit 0; node runner tests 19/19; `go vet ./...`,
  `GOOS=windows go vet ./...`, `go vet -tags semrel_integration` exit 0;
  `gofmt -l .` empty.
- Commits: `e08ade1` `fix(pipelineconfig): reject branch glob syntax beyond
  *, ? and classes` (3 files changed, 39 insertions(+), 1 deletion(-));
  `a409dcd` `fix(semrel): release with the built-in configuration on
  origins outside GitHub` (11 files changed, 248 insertions(+), 19
  deletions(-)). Total `git diff --shortstat f4c9b9d a409dcd`: 14 files
  changed, 287 insertions(+), 20 deletions(-).

### P3

- Route: delegated (one bounded writer; trigger: new package, CLI, docs).
- `internal/pipeline` (new): `Steps{Repo, Config, Run}` with `Validate` and
  `Test`, returning `Result{step, status, commands[{argv, exit}],
  artifacts{results_dir, trx, coverage}, warnings, error}`. Commands are
  `shiprun.Step` Exec steps (no shell, whole-tree kill), output prefix
  `[pipeline:<step>] ` (Vars.Project `pipeline`), stop at the first
  failure naming it (`dotnet build: ...`). `internal/shiprun`,
  `internal/proc` and `internal/pipelineconfig` unchanged.
- `cmd/nerv/pipeline.go`: `nerv pipeline <step> [--cwd] [--json]`, steps
  `validate`, `test` (only the implemented ones are listed); seam
  `options.PipelineSteps`; loads the local `pipeline:` section (none ->
  refusal pointing to docs/pipeline.md; invalid -> refusal listing the
  problems); exit 0 / 1 refused / 2 failed / 128 + signal; text or one
  JSON document on every path past the flags (status `refused` added).
- Decisions:
  - Both steps run in the user's working tree (like CI on a checkout).
  - Every dotnet command gets dim's workflow env `DOTNET_NOLOGO=true`,
    `DOTNET_CLI_TELEMETRY_OPTOUT=true`, then `pipeline.dotnet.env` over it.
  - Test results: `--results-directory <repo>/.nerv/artifacts/pipeline/test`
    (dim passes none; a fixed place is needed to report them), emptied per
    run; `.nerv/artifacts/.gitignore` = `*` so it is never committed without
    touching the repository's `.gitignore` (smoke: `git status --porcelain
    -- .nerv` empty). Coverage reports listed even after a failure.
  - Flags follow the step (`nerv pipeline <step> [flags]`).
- **Finding (verified, contradicts the brief's premise):**
  `coverlet.collector` 6.0.4 (dim's package, `XPlat Code Coverage`) ignores
  `<Threshold>` in the runsettings. Experiment (dotnet 10.0.112, classlib
  with `Add` covered and `Sub` uncovered, runsettings `100,80,100`
  line,branch,method, total): line-rate 0.5, `Sub` line-rate 0, `dotnet
  test` exit 0, no threshold message. So dim's CI does not enforce its
  coverage gate today. As instructed, NERV does not re-check; `nerv
  pipeline test` warns when the runsettings declares a `<Threshold>`
  ("not enforced"), and the smoke test asserts the real behaviour (an
  uncovered method passes, warned) instead of the brief's expected
  failure. Open decision for the user: enforce the runsettings thresholds
  in NERV (read them and check the cobertura reports, line/branch/method),
  or switch dim to `coverlet.msbuild`, or keep as is.
- RED: `go test ./internal/pipeline/` -> build failed (`undefined: Steps`,
  `StatusPassed`); `go test ./cmd/nerv/` -> build failed
  (`opts.PipelineSteps undefined`). GREEN: both `ok`; one test fix (flags
  after the step).
- Smoke (`TestSmoke_ValidateAndTest`, real dotnet 10.0.112, skipped under
  `-short` or without dotnet): generated classlib + xunit + slnx +
  runsettings: validate passed in 4.83s; test passed in 5.04s with one
  coverage report and the trx; artifacts git-ignored; an uncovered method
  passes with the warning (coverage line-rate 0.5); a failing test fails
  at `dotnet test` (exit 1); misformatted code fails at `dotnet format`.
  Whole smoke 23.9s (22.2s in the full suite).
- Checks: `go test -count=1 ./...` exit 0 (29 packages, smoke included);
  `go vet ./...`, `GOOS=windows go vet ./...` exit 0; `gofmt -l .` empty.
- Commits: `7adda17` `feat(pipeline): run dim's validate and test jobs` (3
  files changed, 591 insertions(+)); `cd0d19a` `feat(cli): add nerv
  pipeline validate and test` (6 files changed, 459 insertions(+), 6
  deletions(-)). Total `git diff --shortstat 8ae0406 cd0d19a`: 9 files
  changed, 1050 insertions(+), 6 deletions(-). Above the ~400-line
  heuristic: about 500 lines are tests (unit, CLI, smoke) and 130 docs.

### P3 coverage gate

User decision (2026-10-15): enforce the coverage threshold in nerv. Route:
delegated (one writer). Test-first.

- Semantics verified, not assumed: a throwaway console app called
  coverlet.core 6.0.4 (the assembly shipped in coverlet.collector 6.0.4)
  through reflection on a two-module model. Observed: percentages floored
  to hundredths (2/3 -> 66.66); nothing to cover -> 100%; method covered
  when any of its lines was hit; branch = covered/total; `total` over all
  modules, `average` = floored mean of module percentages (line 66.66,
  branch 75, method 75), `minimum` = lowest module (line 33.33); a
  threshold is missed when `actual < threshold`. The full
  GetThresholdTypesBelowThreshold table has 15 rows; at `17368a6` the
  test replayed 10 of them (corrected in the P3 follow-ups: `630956a` adds
  the other 5, so `TestCheckThresholds_EachStatisticAsCoverlet` now
  replays all 15).
- `internal/pipeline/coverage.go`: `parseThresholds` (the collector named
  "XPlat code coverage"; `Threshold` comma list in `ThresholdType` order,
  default `line,branch,method`, one value for all; `ThresholdStat` default
  `minimum` as coverlet; errors for non-numbers, out of 0-100, count
  mismatch, unknown type or statistic, malformed XML); `readCoverage`
  (Cobertura packages = modules; method lines; `condition-coverage
  "x% (c/t)"` for branches); `checkThresholds`; `checkCoverage` (note or
  error). Runs after `dotnet test` succeeds when a runsettings is set; no
  runsettings or no `Threshold` -> no check; the P3 warning was removed
  (`Result.Warnings` replaced by `Result.Notes`).
- Decisions:
  - Several reports are merged per line (hits added, best branch
    coverage of a line kept), as coverlet merges results, instead of
    summing each report's covered/valid totals as the brief proposed:
    summing would count a module present in two test projects' reports
    twice. Tested by `TestMeasure_ReportsAreMergedPerLine`.
  - The collector's Include/Exclude filters are already applied in the
    reports; nothing is re-filtered (documented).
  - shipplan's coverage formatter was not reused: it formats line coverage
    of summed reports only; the gate formats hundredths with the same
    integer truncation (`percentText`, `hundredthsOf`).
  - Messages use `>=` / `<`.
- RED: build failure (`undefined: Thresholds`, `StatTotal`, ...). GREEN
  after the implementation; one existing test fixed (it named a runsettings
  file it never created, which the check now reads).
- Smoke (real dotnet 10.0.112): validate passed in 4.71s; test passed in
  3.33s with the note `coverage (total): line 100% >= 100%, branch 100%
  >= 80%, method 100% >= 100%`; the uncovered method now fails:
  `coverage below the thresholds of coverlet.runsettings (total): line 50%
  < 100%, branch 100% >= 80%, method 50% < 100%` (dotnet test itself exit
  0); a failing test and misformatted code fail as before. 21.8s.
- docs/commands.md: not updated in `17368a6` (outside that pass's
  surfaces); the parent updated it in `ff53bdc`.
- Checks: `go test -count=1 ./...` exit 0 (29 packages, smoke included);
  `go vet ./...`, `GOOS=windows go vet ./...` exit 0; `gofmt -l .` empty.
- Commit: `17368a6` `feat(pipeline): enforce the runsettings coverage
  thresholds` (8 files changed, 724 insertions(+), 66 deletions(-); about
  330 lines of tests and 50 of docs).

### P3 review

- Native review of 8ae0406..ff53bdc (high risk, 4 lenses, user granted):
  approved and acknowledged. Advisory: R4-branch-merge-undercount /
  R3-branch-merge-max / R2-merge-comment-misleading (WARNING: with several
  test projects covering different branches of one line, the per-line merge
  keeps max covered / max total — 1/2 instead of coverlet's 2/2 — because
  Cobertura's condition-coverage has counts, not which branches; the
  comment claims it matches coverlet), R1-symlinked-results-dir (the
  results reset checks the path lexically only), R2-empty-coverage-zero (no
  modules → 0% instead of the 100% rule), R2-tasklog-stale-commands-doc and
  R2-tasklog-row-count (task log notes outdated / overstated: the coverlet
  table test has 10 rows).

### P3 follow-ups

All P3 review advisories addressed (user authorized). Route: delegated
(one writer). Test-first; `go test` exit codes captured directly.

| Item | Fix | Evidence |
|---|---|---|
| R4-branch-merge-undercount, R3-branch-merge-max, R2-merge-comment-misleading | The gate now reads coverlet's JSON reports, requested for the run only with `--collect "XPlat Code Coverage;Format=<the runsettings' formats>,json"` when a threshold is checked; metrics merged per line and per branch (branch key Line/Offset/EndOffset/Path/Ordinal, hits added), as coverlet merges; Cobertura stays the artifact and the fallback, documented as unable to tell which branches (the misleading comment rewritten). Mechanism verified with the .NET 10 SDK before coding: the `-- DataCollectionRunSettings.DataCollector.Configuration.Format=json,cobertura` override combined with `--settings` is rejected by VSTest ("unexpected XML element DataCollector"); `--collect "XPlat Code Coverage;Format=json,cobertura"` with `--settings` writes both reports and keeps the file's `Exclude` (a control run without the file includes the excluded class). The runsettings' own formats are kept (json added). | RED: build failure (`CoverageJSONFile`, `readJSONCoverage`, `readGate` undefined). GREEN: `TestMeasureJSON_TwoProjectsCoveringDifferentBranchesOfALine` (one project 50%, two merged 100% line/branch/method), `TestMeasureJSON_MatchesCoverlet` (the probe model in JSON: same 9 values as coverlet), `TestCollectArg` (5 cases incl. user formats), `TestTest_PrefersTheJSONReports`. Smoke: two test projects each covering one branch of `Sign` -> `coverage (total): line 100% >= 100%, branch 100% >= 100%, method 100% >= 100%`; the Cobertura fallback on the same run gives branch 50%; the JSON holds no `Skipped` (excluded by the runsettings). No coverlet.core cross-check of the merge itself (optional; not done). |
| R1-symlinked-results-dir | `resetResults` resolves symlinks of the repository, `.nerv/artifacts` and the deepest existing part of the results path, and refuses when the real path leaves `.nerv/artifacts` (or that leaves the repository) before removing anything. | RED: `TestTest_ResultsBehindASymlinkOutsideTheArtifactsAreRefused` — the old code emptied the symlink target (`keep.txt` outside the repository deleted). GREEN: refused, nothing run, the file kept. |
| R2-empty-coverage-zero | No module -> 100% under every statistic, with a comment. | RED: `TestMeasure_NoModulesIsFullCoverage` (0 hundredths). GREEN. |
| R2-tasklog-stale-commands-doc, R2-tasklog-row-count | The two notes above corrected; the table test extended to the full 15 rows instead of shrinking the claim. | `TestCheckThresholds_EachStatisticAsCoverlet` has 15 rows, passes. |

- A stale smoke assertion (`line-rate="0.5"` in whichever Cobertura report
  sorted first) was removed: with two test projects the order depends on
  the run ids; the coverage error message asserts the merged numbers
  (`line 66.66% < 100%, branch 100% >= 100%, method 66.66% < 100%`). The
  smoke test then passed twice in a row and in the full suite.
- Smoke timings: validate 4.95s, test 3.70s, whole smoke ~25s.
- Checks: `go test -count=1 ./...` exit 0 (29 packages, smoke included);
  `go vet ./...`, `GOOS=windows go vet ./...` exit 0; `gofmt -l .` empty.
- Commits: `f1761a4` `fix(pipeline): never empty a results directory
  linked outside the repository` (2 files changed, 79 insertions(+), 1
  deletion(-)); `e817714` `fix(pipeline): count a run without coverage
  modules as full coverage` (2 files, +15 -1); `4f68fd8` `fix(pipeline):
  merge branch coverage per branch from coverlet's JSON reports` (5 files,
  +461 -111); `630956a` `test(pipeline): replay coverlet's whole threshold
  table` (1 file, +7 -1). Total `git diff --shortstat d839d70 630956a`: 6
  files changed, 562 insertions(+), 114 deletions(-).

## Next step

Review of P3 (with the coverage gate); then P4 (`release` step).
