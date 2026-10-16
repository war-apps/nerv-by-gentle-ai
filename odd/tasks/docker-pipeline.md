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
- [x] P4 — `release` step (reuse the ship workspace and version guard).
- [x] P5 — `docker` step (tags, computed build args, login, push, retry).
- [x] P6 — `sync` step.
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
- PR #107 (P3): `feat/pipeline-validate-test` → develop. Merged 33f14ac.
- PR #108 (side request, launch labels show the model): merged 6220788.
- PR #109 (P4): `feat/pipeline-release` → develop. Merged 65caeff.
- PR #110 (P5): `feat/pipeline-docker` → develop. Merged b2ea4b9.
- PR #111 (side request, dotnet runs without build servers — the T2 CLR 0x80131506 workaround built in): merged 42e90c7.
- P6 branch `feat/pipeline-sync` from 42e90c7.
- P5 branch `feat/pipeline-docker` from 65caeff.
- P4 branch `feat/pipeline-release` from 6220788.
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

### P3 follow-up review

- Native review of d839d70..02fe5e1 (medium, user granted): approved and
  acknowledged. Advisory: R3-json-silently-drops-cobertura-only-projects
  (WARNING: once any JSON report exists, a test project that wrote only
  Cobertura is left out of the gate), R3-smoke-fallback-nil-deref (WARNING,
  test only: a failed readCoverage panics the smoke test and hides the
  error; an unreadable JSON passes), R3-mkdir-before-symlink-check
  (`MkdirAll` on `.nerv/artifacts` runs before the real-path check, so a
  symlinked `.nerv` gets directories created outside).

### P3 final pass

Last bounded pass on the P3 follow-up review's findings (user authorized).
Route: delegated (one writer). Test-first; `go test` exit codes captured
directly.

| Item | Fix | Evidence |
|---|---|---|
| R3-json-silently-drops-cobertura-only-projects | `checkCoverage` groups the reports by run directory (one per test project) and reads each run's JSON, or its Cobertura when it has none or it cannot be read, into one model (`addJSON` / `addCobertura` read a file whole, then `merge`); a run with no readable report fails naming it. JSON module `Calc.dll` = Cobertura package `Calc`; methods keyed by class + name and signature in both formats. Branches per line: the JSON's known branches, with a Cobertura count for the same line as a floor (capped at the line's branches); a Cobertura-only line adds its count (documented conservative fallback). | RED: `TestCheckCoverage_ChoosesTheReportPerRun` (the Cobertura-only run was dropped: no error at line 100%), `TestCheckCoverage_UnreadableRunIsAnError` (a raw JSON error instead of falling back / naming the run). GREEN: both, plus `TestCheckCoverage_SameModuleFromJSONAndCoberturaIsOneModule` (passed before only because the Cobertura run was ignored; now meaningful). |
| R3-smoke-fallback-nil-deref | The smoke test stops with `t.Fatalf` on a `readCoverage` error and on a `ReadFile` error in the excluded-class loop. | Test-only; no RED (the bad path needs an unreadable report). Smoke passes. |
| R3-mkdir-before-symlink-check | `checkAncestorInside` resolves the deepest existing part of `.nerv/artifacts` before `MkdirAll`; then the existing results check. | RED: `TestTest_SymlinkedNervOrArtifactsOutsideAreRefusedBeforeCreatingAnything/.nerv` (`artifacts/` created in the link's target). The `.nerv/artifacts` subtest already passed (refused before writing). GREEN: both refused, the outside directory stays empty. |

- Docs: docs/pipeline.md (per-run report choice, module/method matching,
  the conservative Cobertura fallback).
- Checks: `go test -count=1 ./...` exit 0 (29 packages, smoke included,
  24.8s alone); `go vet ./...`, `GOOS=windows go vet ./...` exit 0;
  `gofmt -l .` empty.
- Commits: `f81ac02` `fix(pipeline): choose the coverage report per test
  run` (3 files changed, 275 insertions(+), 90 deletions(-)); `55f501d`
  `fix(pipeline): refuse a symlinked .nerv outside the repository before
  creating anything` (2 files, +62 -7); `0326131` `test(pipeline): stop the
  smoke test on unreadable coverage reports` (1 file, +10 -3). Total `git
  diff --shortstat a8af92a 0326131`: 6 files changed, 347 insertions(+),
  100 deletions(-).

### P4

- Route: delegated (one bounded writer; trigger: config field, semrel
  plumbing, outputs package, CLI, docs).
- Sharing with `nerv ship` (decided): `nerv pipeline release` builds a
  `shipRun` in a new release-only mode (`releaseOnly`, `versionFiles`,
  `released`) and calls the same `execute`: release clone, local-vs-origin
  check, version-file check on origin, Compute, `--expect`, listing,
  confirmation, push check, GH token, `ws.Release` and the same failure
  advice. In that mode the plan is empty (no stages, no hooks) and
  `ship()` delegates to `releaseOnlyStep` (dry run stops there; version
  files; release; a tagged failure gets advice without NERV's publish
  stage). Ship's own code paths and texts are unchanged (all ship unit
  and integration tests pass untouched). The shared messages say "nerv
  ship"; the pipeline command renames them on its own stderr (a line
  writer under the `[pipeline:release] ` PrefixWriter) and in its
  document's `error`/`action`.
- Version files: `pipeline.release.version_files` (pipelineconfig,
  additive: validated, normalised, no duplicates) written in the clone
  before semantic-release with shipplan's csproj writer (`<Version>` +
  release link, or version only with a warning without a public origin),
  as ship writes the dotnet preset's file. Ship relies on the
  repository's own `@semantic-release/git` assets listing the file; the
  built-in configuration now takes `extraAssets` (semrel
  `ReleaseRequest.ExtraAssets` -> runner `defaultConfig(..., {assets})`),
  so the release commit holds CHANGELOG + the version files; a
  repository's own configuration keeps its own assets (documented, dim's
  lists its csproj).
- Outputs: `pipeline.ReleaseOutputs` (dim's published, version, channel,
  git_tag + git_head, last_version, project, branch, config source, the
  12 action outputs, error, action) written atomically to
  `.nerv/artifacts/pipeline/release.json` after the same symlink checks
  as the test results; `--json` prints the same document. Decisions: an
  earlier release.json is removed at the start (a refused, failed or
  interrupted run leaves none); `--dry-run` writes it with `dry_run: true`
  (later dry runs see the would-be version); no new release is
  `published: false`, exit 0; a `release:` section with several projects
  is refused (one project per pipeline; `nerv ship` for monorepos); an
  invalid `pipeline:` section is a refusal (exit 1).
- RED: `go test ./internal/pipelineconfig/` (`p.Release undefined`), node
  (`defaultConfig` without assets), `go test ./internal/semrel/`
  (`unknown field ExtraAssets`), `go test ./internal/pipeline/`
  (`undefined: ReleaseOutputs`), `go test ./cmd/nerv/` (the release step
  did not exist: 6 test functions failed). GREEN: all; one refusal fixed
  after GREEN (an invalid pipeline: section first exited 2).
- Integration (`TestIntegration_PipelineReleaseWithTheBuiltInConfiguration`,
  real semantic-release, local bare origin, no repo semantic-release
  config, untracked local `pipeline:` with `version_files:
  [src/Api/Api.csproj]`): `--dry-run` -> origin refs unchanged,
  release.json `dry_run: true`, published, `1.1.0-alpha.1`, config
  `default`; `--yes --json --expect widgets=1.1.0-alpha.1` -> tag
  `v1.1.0-alpha.1` = origin's develop, release commit holds exactly
  `CHANGELOG.md` and `src/Api/Api.csproj`, the csproj on origin has
  `<Version>1.1.0-alpha.1</Version>`, the warning (no public address for
  the link) is printed prefixed, release.json has the outputs and
  `git_head` = origin's develop; after `git pull`, a second run ->
  `published: false`, `last_version` `1.1.0-alpha.1`. 20.8s. The release
  link itself is proven by the unit test (GitHub origin fake): a real
  GitHub origin would also bring the GitHub plugin, not testable locally.
- Checks: `go test -count=1 ./...` exit 0 (29 packages); `go test -tags
  semrel_integration -run Integration -count=1 ./internal/semrel/
  ./cmd/nerv/` exit 0; node runner tests 20/20; `go vet ./...`,
  `GOOS=windows go vet ./...`, `go vet -tags semrel_integration` exit 0;
  `gofmt -l .` empty.
- Commits: `f8bf4fd` `feat(pipelineconfig): add the release step's version
  files` (4 files changed, 49 insertions(+)); `0ef0a65` `feat(semrel):
  commit extra assets with the built-in configuration's changelog` (7
  files, +51 -7); `86aa2ce` `feat(pipeline): keep the release outputs for
  the later steps` (2 files, +248); `f4ae06b` `feat(cli): add nerv
  pipeline release` (8 files, +665 -13). Total `git diff --shortstat
  b41e0ba f4ae06b`: 21 files changed, 1013 insertions(+), 20
  deletions(-). Above the ~400-line heuristic: about 450 lines are tests
  and 120 docs; four work-unit commits.

### P4 review

- Native review of b41e0ba..2e84c5a (medium, user granted): approved and
  acknowledged. Advisory: R3-outputs-write-after-release (WARNING: a failed
  `release.json` write after the tag reports a bare failure, without saying
  the release is on origin), R3-own-config-version-files-silent (WARNING: with
  the repository's own config, version files its git assets do not list stay
  out of the release commit silently), R3-failed-doc-claims-published (a
  failed run's document reports the computed release as published),
  R3-prefixwriter-error-redup (a failed write is retried and duplicated).

### P4 follow-ups

All four P4 review advisories fixed (user authorized). Route: delegated
(one writer). Test-first; `go test` exit codes captured directly.

| Item | Fix | Evidence |
|---|---|---|
| R3-outputs-write-after-release | A failed release.json write (seam `options.WriteReleaseOutputs`) fails the step (exit 2) with `error` naming the write failure and an `action` saying the release is on origin (version, tag, commit), that docker/sync/deploy read release.json, and how to recover: the exact content (status passed) is printed on one stderr line after `release.json should hold: ` to save by hand; never a rerun ("Do not run nerv pipeline release again for this release"). | RED: build failure (`opts.WriteReleaseOutputs undefined`). GREEN: `TestPipelineRelease_OutputsWriteFailureAfterTheRelease` (exit 2, published true with version/tag/commit, the printed line parses back to a `passed` document with the tag, one release only). |
| R3-own-config-version-files-silent | The runner's plan mode returns `gitPlugin` / `gitAssets` of the repository's own configuration (`gitAssets(plugins)`: the plugin's default assets when unset, none for `false`, object paths); Compute carries them on `ProjectResult` (`json:"-"`, so `version next` output is unchanged); `nerv pipeline release` refuses after Compute, before listing, confirmation or any release (dry run included) when the config has no `@semantic-release/git` or `semrel.AssetsCover` (globby-style: `*`, `?`, `**`, `[...]`, `{a,b}`, leading `./`, `!` exclusions) leaves a version file out, naming the files and the fix. `ProjectResult` is no longer comparable with `==`: two tests switched to `reflect.DeepEqual`. | RED: node `gitAssets` export missing; Go build failures (`AssetsCover`, `GitPlugin`, `GitAssets`). GREEN: node 21/21, `TestAssetsCover` (10 cases), `TestCompute_ReportsTheRepositorysGitAssets`, `TestPipelineRelease_RepositoryConfigMustCommitTheVersionFiles` (covered by a glob proceeds; not in the assets and no git plugin refused, dry run and real, nothing released). Integration `TestIntegration_PipelineReleaseRepositoryConfigAssets` (real semantic-release, own package.json config): assets `CHANGELOG.md` only -> refused naming `src/Api/Api.csproj` and `package.json`, origin tags unchanged; assets with `src/**/*.csproj` -> `v1.1.0` released, the release commit holds `CHANGELOG.md` and `src/Api/Api.csproj`. |
| R3-failed-doc-claims-published | `published` and the release fields describe only what was released; a real run that computed but did not release reports `published: false` and `computed {version, channel, git_tag}`; a dry run keeps reporting the computation as before. | RED: build failure (`doc.Computed undefined`). GREEN: `TestPipelineRelease_FailureLeavesNoOutputs` extended (published false, no version/tag, computed `1.1.0-alpha.1` / `v1.1.0-alpha.1`). |
| R3-prefixwriter-error-redup | `PrefixWriter` consumes a line before writing it: a failed write returns its error and the line is not written again. | RED: `TestPrefixWriter_DropsTheLineThatFailed` got `> lost` written after the failure. GREEN. |

- Docs: docs/pipeline.md (the own-config assets check, `computed`, the
  write-failure recovery), committed with the assets fix in `a4175da`.
- Checks: `go test -count=1 ./...` exit 0 (29 packages); `go test -tags
  semrel_integration -run Integration -count=1 ./internal/semrel/
  ./cmd/nerv/` exit 0; node runner tests 21/21; `go vet ./...`,
  `GOOS=windows go vet ./...`, `go vet -tags semrel_integration` exit 0;
  `gofmt -l .` empty.
- Commits: `8a8f1a5` `fix(pipeline): drop a prefixed line whose write
  failed` (2 files); `397b2ae` `fix(cli): report a release.json write
  failure and only what was released` (4 files, +87 -5); `a4175da`
  `fix(cli): refuse version files the repository's release commit would
  not hold` (15 files, +302 -17). Total `git diff --shortstat 4a16bf4
  a4175da`: 19 files changed, 425 insertions(+), 25 deletions(-).

### P5

- Route: delegated (one bounded writer; trigger: new step package, CLI,
  additive shiprun/semrel helpers, docs).
- `internal/shiprun` (additive): `Runner.Capture` returns a command's
  stdout and streams only its stderr with the prefix (computed build
  arguments). `internal/semrel` (additive): `Workspace.CheckoutTag`
  (detached at origin's tag, returns the tagged commit;
  `ErrTagNotOnOrigin`).
- `internal/pipeline/docker.go`: `Docker.Run` (dim's docker job over
  `pipeline.images`): `ImageTags` (version always; channel unless empty or
  `default`; latest only on `MainBranch()`), build args with `${version}`,
  `${channel}`, `${tag}`, computed args via `ArgRunner.Capture` (trimmed
  stdout), labels title/version/revision/source, `docker build --platform
  -f --build-arg --label -t <context>` in the clone, one retry with the
  computed args computed again when the image has any, `docker push` per
  tag (first digest recorded), `required: false` failures reported without
  failing the step, every image tried. Login `docker login <host> -u <user>
  --password-stdin` with the token on stdin only; a dry run never logs in
  or pushes. `ReadReleaseOutputs` and `docker.json` share the artifacts'
  symlink checks (`writeArtifactJSON`, `removeArtifact`).
- `cmd/nerv/pipeline_docker.go`: `nerv pipeline docker [--cwd] [--dry-run]
  [--no-login] [--fetch-timeout] [--json]`; seams `OpenDockerWorkspace`,
  `DockerProc`, `DockerArgs`. Flow: pipeline: section (refusal without),
  stale docker.json removed, release.json read (missing or not passed ->
  refusal "run nerv pipeline release first"; `published: false` -> nothing
  built, exit 0, docker.json written with no images; a dry-run
  release.json -> only `--dry-run`), credentials (both env vars required
  unless `--no-login` or `--dry-run`), a fresh workspace clone of origin's
  release branch checked out at the release tag whose commit must equal
  `git_head` (a dry-run release builds origin's branch head: no tag
  exists), then the images; docker.json written only when passed.
- Decisions: login requires `username_env` and `token_env` unless
  `--no-login`; the build is at the release tag (dim checks out the
  branch ref, which is the release commit right after semantic-release);
  plain `docker build --platform` (BuildKit default since Docker Engine
  23; verified on 29.8.2) instead of buildx; `latest` follows
  `MainBranch()` against release.json's branch.
- RED: `go test ./internal/shiprun/` (`r.Capture undefined`),
  `./internal/semrel/` (`ws.CheckoutTag undefined`), `./internal/pipeline/`
  (`undefined: ImageTags`, `Docker`; then `ReadReleaseOutputs`,
  `WriteDockerResult`), `./cmd/nerv/` (`opts.OpenDockerWorkspace
  undefined`). GREEN: all; unit tests for tags per branch (main, develop,
  release/*, hotfix, selection), the build argv and labels, the broker
  retry with recomputed args, `required: false`, a required push failure,
  login via stdin (the token in no argv, env or output), dry run, login
  failure, refusals (no release.json, failed release, tag missing, tag
  moved, no credentials, no section), `--no-login`, stale docker.json.
- Integration (`TestIntegration_PipelineDockerPushesToALocalRegistry`,
  semrel_integration tag, real Docker Engine 29.8.2 and `registry:2`):
  local bare origin tagged `v1.0.0`, release.json written by the test,
  `nerv pipeline docker --no-login --json` -> exit 0 in 514ms; image
  `127.0.0.1:<port>/acme/app` pushed with tags `1.0.0` and `latest`
  (registry API `/v2/acme/app/tags/list`), digest recorded, labels
  revision = the tag's commit, version 1.0.0, title, `app.version` from
  `${version}` and `app.stamp` from the computed arg; containers and
  images removed. Environment finding: on this host a published port
  (`-p 127.0.0.1::5000`) accepted connections but never answered (the
  first runs hung in an un-timed `http.Get`, leaving registry containers,
  which were removed); the test now runs the registry on the host network
  bound to a free loopback port (`REGISTRY_HTTP_ADDR`), bounds every HTTP
  request (5s), and skips off Linux.
- Checks: `go test -count=1 ./...` exit 0 (29 packages); `go test -tags
  semrel_integration -run Integration -count=1 ./internal/semrel/
  ./cmd/nerv/` exit 0 (docker test included); `go vet ./...`,
  `GOOS=windows go vet ./...`, `go vet -tags semrel_integration` exit 0;
  `gofmt -l .` empty; no registry container left.
- Commits: `0404186` `feat(shiprun): capture a command's standard output`
  (2 files, +42 -1); `c15540e` `feat(semrel): check a release clone out at
  origin's tag` (2 files, +44); `4b8ebbc` `feat(pipeline): build, tag and
  push the pipeline's images` (4 files, +573 -4); `30c013b` `feat(cli):
  add nerv pipeline docker` (8 files, +783 -7). Total `git diff
  --shortstat ee084dd 30c013b`: 16 files changed, 1442 insertions(+), 12
  deletions(-). Above the ~400-line heuristic: about 700 lines are tests
  and 90 docs.

### P5 review

- Native review of ee084dd..794e08b (medium, user granted): approved and
  acknowledged. Advisory: R3-checkout-tag-error-collapse (WARNING: every
  rev-parse failure becomes ErrTagNotOnOrigin, a refusal with exit 1, even
  for timeouts or a broken clone), R3-empty-tag-selection (WARNING: an
  image whose tag selection yields no tags builds without `-t`, pushes
  nothing and is reported as pushed), R3-token-env-assertion-vacuous (docker
  children inherit the environment, so the token from `token_env` is still in
  their environment; the unit test only passes because the test process has
  no such variable).

### P5 follow-ups

All three P5 review advisories fixed (user authorized). Route: delegated
(one writer). Test-first; `go test` exit codes captured directly.

| Item | Fix | Evidence |
|---|---|---|
| R3-token-env-assertion-vacuous (security) | `pipeline.SafeEnv` removes `registry.username_env`, `registry.token_env` (case-insensitive) and every `NERV_DEPLOY_*` variable from the environment of every docker child (login, build, push) and of the computed-argument scripts (shiprun `Environ`); PATH, HOME, DOCKER_CONFIG, DOCKER_HOST and proxies are kept; the token reaches docker only on login's stdin. | RED: `TestDocker_ChildrenNeverInheritTheRegistrySecrets` (t.Setenv of `GHCR_USER`, `GHCR_TOKEN`, `NERV_DEPLOY_PRODUCTION_DOCKER_SSH_KEY`): every login/build/push command had them in its environment. GREEN, plus `TestSafeEnv` (`NERV_DEPLOYMENT` kept: only the `NERV_DEPLOY_` prefix goes). |
| R3-checkout-tag-error-collapse | `CheckoutTag` maps only rev-parse `--verify --quiet`'s exit 1 without output to `ErrTagNotOnOrigin` (refusal); any other failure is returned with its cause (exit 2). Verified with real git: a missing tag exits 1 printing nothing; an unusable directory exits 128. | RED: `broken` (exit 128) and `slow` (deadline) both became "the tag is not on origin". GREEN: their causes are kept, the missing tag is still the refusal. |
| R3-empty-tag-selection | An image whose selection gives no tag fails (`no tag selected for ...`, attempts 0) without building or pushing; `pipelineconfig` requires `version` in `pipeline.images[].tags` (the only tag every release produces). Docs: the tags row. | RED: the image was reported `pushed` with no tags; `tags: [channel, latest]` loaded. GREEN: both. |

- Checks: `go test -count=1 ./...` exit 0 (29 packages); `go test -tags
  semrel_integration -run Integration -count=1 ./internal/semrel/
  ./cmd/nerv/` exit 0, the docker test run (not skipped): exit 0 in
  468ms, passed; `go vet ./...`, `GOOS=windows go vet ./...`, `go vet
  -tags semrel_integration` exit 0; `gofmt -l .` empty; no registry
  container left.
- Commits: `9f65611` `fix(pipeline): keep the registry token out of
  docker's environment` (4 files, +80 -3); `2c2111c` `fix(semrel): tell a
  missing release tag from a failing git` (2 files, +32 -3); `7fa336f`
  `fix(pipeline): never build or report an image without a tag` (5 files,
  +36 -1). Total `git diff --shortstat 2262fd7 7fa336f`: 8 files changed,
  148 insertions(+), 7 deletions(-).

### P6

- Route: delegated (one bounded writer; trigger: additive semrel helpers,
  new step in internal/pipeline, CLI, docs).
- `internal/semrel` (additive, `merge.go`; `Workspace` keeps the user's
  top level): `UseCommitIdentity` (copies `user.name`/`user.email` as the
  user's repository resolves them into the clone, returns `git var
  GIT_COMMITTER_IDENT` without the timestamp), `Branches(pattern)`
  (origin's branches via for-each-ref, filtered with `path.Match`),
  `Merge` (checkout -B at origin's target, `merge --no-ff --no-edit -m`;
  up to date via `merge-base --is-ancestor`; a conflict is detected by
  MERGE_HEAD, its paths read, then `merge --abort`), `PreviewMerge`
  (`merge-tree --write-tree --name-only`, exit 1 = conflict), `PushBranch`
  (never forced); `ErrBranchNotOnOrigin`.
- `internal/pipeline/sync.go`: `Sync.Targets` (main or a
  `sync.release` match -> develop; main -> every origin `sync.release`
  branch in name order, main/develop excluded; anything else -> nothing),
  `Sync.Run` (per target pushed / up-to-date / conflict / failed, dry
  run would-merge / would-conflict / up-to-date; continues after a
  conflict or failure; failed at the end naming the branches, with the
  manual-merge action), `sync.json` (`WriteSyncResult`,
  `RemoveSyncResult`), `ReadDockerResult` (shared `readArtifactJSON` with
  release.json).
- `cmd/nerv/pipeline_sync.go`: `nerv pipeline sync [--cwd] [--dry-run]
  [--yes] [--fetch-timeout] [--json]`; seam `OpenSyncWorkspace`. Flow:
  section, stale sync.json removed, `sync.enabled: false` -> nothing (exit
  0), release.json missing / not passed -> refusal, `published: false` ->
  nothing, dry-run release.json -> only `--dry-run`, docker.json for the
  same version not passed -> refusal, clone of origin's released branch,
  targets (none -> nothing), identity printed, every merge listed, push
  check and confirmation (real run only), merges.
- Decisions: the commits use the user's identity, copied from the user's
  repository (an `includeIf` for its path would not match the clone);
  sync.json is written for every run that reached the merges or found
  nothing to do, failed merges included, never for a refusal; a dry run
  with predicted conflicts passes (exit 0), only a failed preview fails
  it; a develop conflict is recorded like a release/* one and the
  release branches still sync (dim's two jobs are independent);
  docker.json for another version means the docker step did not run for
  this release (ignored, noted).
- Decision (user, 2026-10-16): a failed or refused `nerv pipeline docker`
  never touches docker.json. Docker no longer removes an earlier one at its
  start and still writes it only on success, so a run that does not pass
  leaves the file byte-identical. Sync is unchanged: after a failed docker
  run it reads whatever docker.json an earlier run left (another version
  is ignored; an earlier success for the same version lets sync proceed),
  so docker must be rerun until it passes before syncing (documented in
  the docker and sync sections of docs/pipeline.md). RED:
  `TestPipelineDocker_FailureOrRefusalKeepsTheEarlierDockerJSON` (build
  failure exit 2, refusal without release.json exit 1, refusal with the
  tag not on origin exit 1): docker.json was gone in all three. GREEN
  after the change; `TestPipelineDocker_FailureLeavesNoDockerJSON` (which
  asserted the delete) is replaced by it, and the unused
  `RemoveDockerResult` is removed.
- RED: `go vet -tags semrel_integration ./internal/semrel/`
  (`ws.UseCommitIdentity undefined`), `go vet ./internal/pipeline/`
  (`undefined: Sync`), `go vet ./cmd/nerv/` with only the new test file
  on the previous commit (`opts.OpenSyncWorkspace undefined`). GREEN:
  all; unit tests for targets per released branch (main, release/a,
  develop, hotfix/x, configured names), conflict abort-and-continue,
  per-branch failures (missing target, rejected push keeps the commit),
  dry run, interruption, artifacts; CLI tests for main/release flows,
  nothing-to-sync cases, conflict exit 2, dry run, refusals (no section,
  no release.json, failed release, dry-run release, failed docker, no
  terminal, declined, push denied), confirmation, docker.json for another
  version.
- Integration (semrel_integration tag, real git, local bare origin):
  `TestIntegration_WorkspaceMergesBranchesAndPushes` (0.49s) and
  `TestIntegration_PipelineSyncMergesBackAfterAMainRelease` (0.78s): main
  with a `chore(release): 1.1.0 [skip ci]` commit and tag, develop,
  release/a and a conflicting release/b; `--dry-run` -> exit 0, origin
  refs unchanged, would-merge / would-merge / would-conflict; `--yes` ->
  exit 2, develop and release/a pushed with "chore(sync): merge main into
  <branch> [skip ci]", author `Sync User <sync@example.com>` from the
  repository's local config (GIT_* identity variables unset), two parents
  including main's release commit, release/b untouched and reported
  `conflict (CHANGELOG.md)`, main unchanged, sync.json written; a second
  run -> up-to-date, up-to-date, conflict. The release itself is simulated
  (commit, tag, release.json written by the test): semantic-release is
  proven by P4.
- Checks: `go test -count=1 ./...` exit 0 (29 packages); `go test -tags
  semrel_integration -run Integration -count=1 ./internal/semrel/
  ./cmd/nerv/` exit 0; `go vet ./...`, `GOOS=windows go vet ./...`, `go
  vet -tags semrel_integration ./...` exit 0; `gofmt -l .` empty.
- Commits: `50a1b63` `feat(semrel): merge and push origin's branches in a
  release clone` (3 files, +307 -3); `ce7cadf` `feat(pipeline): merge the
  released branch back as dim's sync jobs` (3 files, +487 -9); `8e5f7ac`
  `feat(cli): add nerv pipeline sync` (5 files, +749 -1); `46c4d81`
  `docs(pipeline): document nerv pipeline sync` (3 files, +67 -4). Total
  `git diff --shortstat 587a256 46c4d81`: 14 files changed, 1610
  insertions(+), 17 deletions(-). Above the ~400-line heuristic: about
  830 lines are tests and 70 docs. Follow-up (docker.json kept on
  failure): `22f329f` `fix(pipeline): keep docker.json when the docker
  step fails` (4 files, +45 -33), plus the docs/odd commit recording it.

## Next step

Review of P6; then P7 (`deploy` step).
