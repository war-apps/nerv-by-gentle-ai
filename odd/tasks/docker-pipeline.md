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
- [ ] P2 — built-in default semantic-release configuration (dim's, with
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

## Next step

P2 — built-in default semantic-release configuration.
