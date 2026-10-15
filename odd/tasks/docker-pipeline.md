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
- [ ] P3 — `validate` and `test` steps.
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

## Next step

Review of P2, then its pull request; then P3 (`validate` and `test`
steps).
