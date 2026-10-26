# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [5.2.0] - 2026-10-26

Upgrading from 5.1.0: `nerv deploy` now pushes the artifact's images itself,
in a new `publish` step after `push`, and creates the git host release before
any target, so `deploy.targets` is optional (`deploy: { host: github }`
releases without touching a server). When an artifact carries images, the
machine that runs `nerv deploy` needs `docker` and credentials for each
image's registry in the docker config (`docker login <registry>`, a
`credHelpers` entry or a `credsStore`); set `deploy.publish: false` to keep
publishing from the targets only. `nerv upgrade` now also upgrades gentle-ai
when a newer 4.x release exists, never to a new major version.

### Added
- deploy: add /deploy:ship to chain /release:build and /deploy:run (a1971c2)
- upgrade: upgrade gentle-ai within its supported major version (2962c10)
- deploy: publish the artifact's images and create the release before any target (4115431)

### Fixed
- deploy: accept a global docker credsStore in the publish preflight (6fb6838)

## [5.1.0] - 2026-10-25

Upgrading from 5.0.0: `tasks`, `release` and `deploy` are now read from the
project's `.ai-workflow/config.yaml` only, apart from personal keys. If your
user configuration sets other `tasks:` or `deploy:` keys, `nerv doctor` says
so: run `nerv migrate --user` to move them under `defaults:`, then
`nerv migrate` in each repository to copy `tasks.provider` into it. Later
upgrades: `nerv upgrade` (`--alpha` or `--rc` for prereleases).

### Added
- cli: add nerv upgrade (46948b4)
- config: read tasks, release and deploy from the project only (f656c7f)
- configure: seed a new repository's sections from the user defaults (bd9c14d)

### Changed
- cli: name extensions by their bare name (ade0b6d)

### Fixed
- migrate: drop removed task providers before moving user keys under defaults (b6c8194)

## [5.0.0] - 2026-10-25

Upgrading from 4.2.0 or a 5.0.0 alpha: run `nerv doctor`, then `nerv migrate
--dry-run` and `nerv migrate` (`--user` for the user configuration). The
configuration now lives in `.ai-workflow/config.yaml`; the entries below that
name `ship` or `.ai-workflow.yml` are intermediate steps of this release.

### Breaking
- configure: remove task settings from core and prompt provider settings from manifests (4eb326a)
- orchestrator: emit task events to the task plugin instead of tracking tasks (8ecff2a)
- ship: package releases into a local artifact with compose or script builds (f0254d2)
- config: read configuration from .ai-workflow.yml and the ai-workflow user config (9c6032c)
- orchestrator: track NERV runs in gentle-ai's ODD document instead of OpenSpec (7af0059)
- ship: drop semantic-release and nerv pipeline for the native version engine (2e80414)
- rename the ship extension to release (b37b91f)
- extension: read every extension and provider from its manifest (1684958)
- release: run versioning and builds through release providers (31d860e)
- config: move the project config into .ai-workflow/ with project providers (a3d72e2)

### Added
- taskprovider: add the task provider contract and manifest validator (bba18f4)
- install: ship and install extension plugins from the nerv marketplace (5747eb2)
- task: add the task plugin with its contract, dispatcher and /task:* commands (29ac930)
- task-teamwork: add the Teamwork task provider (a7f142a)
- install: offer to remove old /task:* copies and refresh enabled extensions (c48900d)
- orchestrator: apply the lessons of real NERV runs (bfacb17)
- ship: add shared-host dotnet, submodule and per-metric coverage settings (4ac5d1e)
- versioning: compute releases natively with semantic-release parity (d30bbe5)
- deploy: deploy ship artifacts and make the release commit, tag and host release (367d066)
- ship: add the ship and deploy plugins with their agents and commands (16474d7)
- migrate: add nerv migrate to move a 4.2.0 repository and user config (9f6cf70)
- orchestrator: keep DIRECT and LIGHT runs out of the ODD document (487ccd0)
- ship: push the 4.2.0 channel and latest image tags again (06b7238)
- extension: add the shared extension and provider contract (5b770ca)
- deploy: add the bash target provider and share the exec runner (45bdce7)
- migrate: convert 5.0.0-alpha.1 ship: sections and refuse them elsewhere (c53745a)
- migrate: list the steps instead of diffs (a348146)
- doctor: add nerv doctor to check the machine and the repository (e54c5e5)

### Fixed
- install: retry failed extension uninstalls and release extensions atomically (f4e4072)
- task: let the task plugin own the subtasks_per_wave check (6059a08)
- task: keep switched-off task plugins silent and list needs_input results (b591c8b)
- versioning: sort release notes with the CLDR root collation (26d33ef)
- config: resolve the user config per platform in prose and for fake homes (c5c0bf4)
- install: tell task and deploy providers apart and refuse unshipped requirements (322edc0)
- deploy: refuse release files inside .git and environments without an image version (ed99e5d)
- orchestrator: keep the lock out of git in fresh clones and the ODD document a file (d752c90)
- deploy: create lightweight release tags as semantic-release does (3690480)
- ship: keep hand-tuned engine keys, push script-mode images and fill --version notes (d25d8ff)
- deploy: refuse every git-directory alias and fail closed on unreadable paths (dc1a016)
- migrate: migrate transactionally, guard live locks and keep small runs out of ODD (771b0f9)
- deploy: extract deploy-bash trees without export attributes and add a timeout (bb4e8c6)
- point release docs at build providers and name alpha.1 in migrate usage (984339d)
- release: check build providers in version next and document bundled installs (9cf51d1)
- deploy: check deploy-bash scripts in the base commit at preflight (346645d)
- extension: enforce requirements, statuses and task events at discovery (9c22863)

## [4.2.0] - 2026-10-16

### Added
- pipelineconfig: `pipeline.dotnet.coverage` configures the test step's coverage gate: `mode` `enforce` (the
  default, as before), `report` (record and warn, never fail) or `off`, and optional `thresholds` (`line`,
  `branch`, `method`, `stat`) overriding the runsettings' ones key by key; the step's `--json` document and
  `test.json` record the mode, the thresholds with their source and the measured values.
- pipeline: the Docker pipeline, a standalone copy of dim.comprobantes.api's CI/CD for repositories that deploy a
  Docker image, configured by the `pipeline:` section of the local `.nerv/nerv.yaml` (docs/pipeline.md, with a
  Getting started guide): `nerv pipeline validate`, `test`, `release`, `docker`, `sync` and `deploy`, each a
  command of its own with its records in `.nerv/artifacts/pipeline/`, and `nerv pipeline run`, which runs them in
  that order (`--dry-run`, `--yes`, `--expect`, `--from`, `--only`), stops at the first step that does not pass
  (the rest reported not-run, the exit the failing step's) and writes `run.json`. The `/nerv:pipeline` command
  rehearses with a dry run, shows the plan and runs the real pipeline only after your explicit yes.
- pipeline: `nerv pipeline deploy` runs dim's `/update-deploy` for the release just made: it reads each
  `pipeline.deploy` environment from `NERV_DEPLOY_<ENVIRONMENT>_<KEY>` variables or from Infisical (update-deploy's
  layout), deploys those whose `DOCKER_IMAGE_VERSION` points at the release (`selection: target`, the default) or
  whose `branches` match the released branch (`selection: branches`), runs the host's compose script (`pull`, then
  `up`, bounded by `pipeline.deploy.update_timeout`) over SSH with strict host keys, keepalives and validated user
  and host (password through `sshpass -e` only, or a key), and verifies the container
  is healthy and carries the released version label; it stops at the first failure, requires this release's
  passing `docker.json`, asks before deploying (`--yes`), lists the deploys without connecting with `--dry-run`, and
  writes `.nerv/artifacts/pipeline/deploy.json`.
- pipelineconfig: `pipeline.deploy.selection` (`target` or `branches`) and per-environment `branches`
  (`environments: [{ name: staging, branches: [release/*] }]`).
- pipeline: `nerv pipeline sync` runs dim.comprobantes.api's sync-to-develop and sync-main-to-release jobs after
  the release step: in a fresh clone of origin it merges main or a `release/*` release into develop, and main into
  every `release/*` branch on origin (`--no-ff`, "chore(sync): merge <source> into <branch> [skip ci]"), pushes
  with your credentials and commits as you; a conflict is aborted and reported, the other branches still sync and
  the step fails at the end. It syncs nothing unless every earlier part passed for this release: a passing
  `test.json` on a clean tree at the commit the release was made from (`release.json`'s new `base_commit`) and a
  passing `docker.json` of the release's version, tag and commit; it asks before pushing (`--yes`), previews with
  `--dry-run` (conflicts included, same checks) and writes `.nerv/artifacts/pipeline/sync.json` (also after an
  interruption, with the targets not run). It refuses when git has no commit identity configured instead of
  letting git guess one.
- pipeline: `nerv pipeline test` records a passing run in `.nerv/artifacts/pipeline/test.json` (the tested `HEAD`
  and any uncommitted changes outside `.nerv/`); a failed run leaves an earlier record untouched.
- pipeline: `nerv pipeline docker` runs dim.comprobantes.api's docker job: from the release step's outputs, in a
  clone of origin at the release tag, it builds every `pipeline.images` entry (build and computed arguments, OCI
  labels, `--platform`), tags it with the version, the channel and `latest` on main, pushes it after a
  `docker login` that receives the token on stdin only, retries a failed build once with recomputed arguments,
  and writes `.nerv/artifacts/pipeline/docker.json` when it passes (a failed or refused run leaves an earlier one
  untouched); `--dry-run` builds without logging in or pushing.
- pipeline: `nerv pipeline release` runs dim.comprobantes.api's release job: it releases what is on origin as
  `nerv ship` does (same checks, configuration, version guard, `--expect`, `--dry-run`), without building or
  publishing, versions `pipeline.release.version_files` (`<Version>` and the release link) in the release commit,
  and writes the release outputs to `.nerv/artifacts/pipeline/release.json` for the later steps, including
  `base_commit`, the commit the release was made from.
- pipeline: `nerv pipeline validate` and `nerv pipeline test` run dim.comprobantes.api's validate and test jobs
  locally from the `pipeline:` section (dotnet restore, format check, build with `-warnaserror`, tests with coverage
  and a trx logger), in the working tree, with streamed output and a `--json` result; test results go to the
  git-ignored `.nerv/artifacts/pipeline/test`. The runsettings' coverage thresholds, which coverlet's collector
  ignores, are enforced by nerv with coverlet's semantics (line, branch, method; total, average or minimum).
- release: a built-in semantic-release configuration (dim.comprobantes.api's: `main`, `develop` alpha, `release/*`
  rc on beta, `hotfix/*` beta; conventionalcommits, changelog, git, github) for repositories without one of their
  own; a `pipeline:` section's `branches` replace its branches. `nerv version next` and `nerv ship` say which
  configuration was used (`semantic_release_config` in JSON). A fixed-id prerelease glob matching several branches
  on origin is refused with the branch names.

### Changed
- orchestrator: Ikari now routes every request through three tiers that mirror gentle-ai's Work Routing Ladder.
  DIRECT (direct inline) covers one mechanical, already-understood file: Ikari edits it, runs the focused check and
  commits it after you validate. LIGHT (delegated direct) launches an explorer only when the mapping trigger fires,
  one domain pilot in `MODE: solo` that writes and observes its own failing test, and Maya only when gentle-ai's
  Delegated Verification Gate asks for an independent verifier; Ikari commits (after your validation) and writes the
  run summary. FULL is unchanged. The delegation triggers use gentle-ai's current thresholds, the classification
  event records the route declaration, the ratchet runs DIRECT → LIGHT → FULL, and `state.yaml` accepts
  `path: direct`. New `classification-direct` eval case.
- orchestrator: agent launch labels and deliberation log `launch` lines show the role's model, as
  `({model}) {Display name}: {action}` (e.g. `(sonnet) Shinji Ikari: Retrying dotnet format verification`).

### Fixed
- pipeline: the dotnet commands of `nerv pipeline validate` and `test` no longer inherit the registry token, the
  `NERV_DEPLOY_*` data or the GitHub tokens; the release, docker and sync clones' git and semantic-release lose the
  registry and deploy secrets too.
- pipeline: `nerv pipeline release` refuses a computed release whose tag is already on origin (stale or missing
  semantic-release notes), dry run or not; semantic-release errors are reported with their details, secrets hidden.
- pipeline: a failed `nerv pipeline docker` names each failed image with the docker error and the build's last
  output lines in its result (and in `run.json`); `nerv pipeline sync` no longer clones origin for a release that
  syncs nothing.
- dotnet: every dotnet command nerv runs (the `dotnet` ship preset and `nerv pipeline validate`/`test`) runs
  without build servers: `MSBUILDDISABLENODEREUSE=1`, `UseSharedCompilation=false`,
  `DOTNET_CLI_USE_MSBUILD_SERVER=0`, and `--disable-build-servers` on restore, build, test and pack when the
  SDK (`dotnet --version`, once per run and directory) is 7.0.100 or later (never on `dotnet format`, which
  rejects it; left out with a note when the version cannot be detected). On shared hosts reused servers made dotnet die mid-release with "Internal
  CLR error (0x80131506)". `pipeline.dotnet.env` still wins; stage overrides' `run:` are not rewritten, so
  repositories that overrode every stage for this can drop those overrides.

## [4.1.1] - 2026-10-15

### Changed
- release: `nerv ship` now uses your local `.nerv/nerv.yaml` `release:` section (committed or not), as `nerv version
  next` does, instead of origin's copy; the code and the semantic-release configuration still come from origin's
  branch. A path the section names that is not on origin's branch is refused before any version is computed.

## [4.1.0] - 2026-10-14

### Added
- release: local release commands for your own repositories, a replacement for the CI release where it cannot or
  should not run (guide: `docs/release-commands.md`).
  - `nerv version next` computes the next semantic version of any repository (or of each project of a monorepo)
    with the repository's own semantic-release configuration and origin's tags and notes, as text, JSON or GitHub
    Actions output lines (the outputs of `cycjimmy/semantic-release-action`); `--offline` uses local refs only.
    `/nerv:version` shows it in Claude Code. Plain `nerv version` still prints the binary version.
  - `nerv ship` releases what is on origin from your machine, in a throwaway clone: pipeline and presets (dotnet,
    node, docker, NuGet, npm), semantic-release for real with the repository's own plugins, publish, and `prepare` /
    `publish` / `success` / `fail` hooks, per project in dependency order; `--dry-run` builds without releasing,
    `--yes` skips the confirmation, `--expect <project>=<version>` refuses any release other than the approved one,
    and a failure reports what was tagged and published and what to do. `/nerv:ship` always rehearses with
    `--dry-run` and releases only after your explicit yes, bound to the dry run's versions.
  - The `release:` section of `.nerv/nerv.yaml` declares the projects (paths, shared paths, scopes, tag formats,
    `depends_on` bumps), presets, stage overrides and hooks (bash, pwsh or node), validated with every problem
    reported at once.
  - `/nerv:release-setup` writes the `release:` section interactively: it detects the repository's semantic-release
    configuration, projects, Dockerfiles, scripts and monorepo layout, asks only what it cannot detect, writes the
    section without touching the rest of `.nerv/nerv.yaml`, and validates it with `nerv version next --offline`.
  - In a monorepo each project's release writes its own `changelog` (default `<path>/CHANGELOG.md`): the
    repository's `@semantic-release/changelog` and `@semantic-release/git` entries are pointed at it, so each
    release commit holds only that project's CHANGELOG. A single project keeps the repository's configured
    changelog unless `changelog` is set explicitly; without a changelog plugin the field has no effect (one warning).

## [4.0.0] - 2026-10-09

### Breaking
- config: rename melchor to melchior and kaji-audit to gendo (31a6c90)
  - Anything that launches the agents directly must use the subagent types
    `nerv:melchior` and `nerv:gendo` instead of `nerv:melchor` and `nerv:kaji-audit`; the
    audit pass files are `pass-melchior-round-N.json` and `pass-gendo-round-N.json`.
  - The group `kaji-passes` is now `audit-passes` (`kaji`, `gendo`).
  - The old names stay accepted as legacy aliases: `models.melchor` and `models.kaji-audit`
    in `nerv.yaml` keep applying to the renamed roles, and `nerv configure --set-model` and the
    wizard accept the old role and group names. Every write stores the new ID once; when a
    file holds both keys, the new one wins.

### Added
- config: give every role its full character name for display (31a6c90)
  - Role IDs stay short lowercase slugs (agent names, `nerv:<id>` subagent types, `models:`
    keys); each role now also has a display name, such as `misato` = Misato Katsuragi and
    `fuyutsuki` = Kōzō Fuyutsuki.
  - The name is shown as `display_name` on each `nerv configure --print` models row, as the
    NAME column of the wizard's models table, in `/nerv:configure` and `/nerv:status`, in the
    roles table of `docs/configuration.md`, and at the start of each agent's description.
- orchestrator: label each agent launch `{Display name}: {action}` (e.g.
  `nerv:melchior(Melchior-Magi 1: security audit, round 2)`), and open its deliberation-log
  `launch` entry with the same label (ca15ec4, 6d53de8)

### Fixed
- configure: `--set-model <role>=default` clears a stale override left for a role removed
  from the catalogue (b8f2267)
- audit: an audit round started before the rename resumes with its pre-rename pass files
  (af0bd9b)
- orchestrator: a project `nerv.yaml` holding a legacy `models.melchor` or
  `models.kaji-audit` key applies it to the renamed role (97b2920)

## [3.0.0] - 2026-10-08

### Breaking
- release: re-publish 2.1.0 under a major version, since it removes roles
  - 2.1.0 removed the `kaji-security`, `kaji-coverage`, `kaji-resilience` and `kaji-refuter`
    roles and changed the Phase 3 audit to four passes. That breaks existing `models:` overrides
    and audit artifact names, so it needed a major bump instead of a minor one.
  - 3.0.0 has the same code and plugin content as 2.1.0. Read the 2.1.0 section below for the
    full list of changes and how to migrate leftover overrides.

## [2.1.0] - 2026-10-08

### Breaking
- config: drop the `kaji-security` role and give each MAGI one gentle-ai equivalent (2f686de)
  - The Phase 3 audit runs five passes. Melchor's audit pass now carries the full security lens
    (injection, authz, secrets, data exposure, unsafe defaults, dependency risk, crypto, infra
    hardening) and claims `review-risk` next to `jd-judge-b`.
  - Balthasar keeps `jd-judge-a` only. Casper takes over the readability audit lens and
    `review-readability`, and no longer suggests a `from:` phase.
  - A `models.kaji-security` override left in `nerv.yaml` is not an error, but it no longer
    applies to any agent: `nerv apply-models` skips it, and `nerv configure --print`,
    `/nerv:status` and the wizard show it as a row with no purpose. `nerv configure --set-model
    kaji-security=default` refuses the unknown role, so delete the line by hand (or move the
    value to `melchor`).
- config: merge the kaji audit passes and move the refuter to fuyutsuki (868da54)
  - Supersedes the pass count and the casper readability move above: the Phase 3 audit runs four
    passes (`kaji-audit`, `melchor`, `balthasar`, `casper`), and no role judges work it authored
    or compiles.
  - `kaji-audit` merges `kaji-coverage` and `kaji-resilience` with every check of both, always at
    full RDD scope, and claims `review-reliability` and `review-resilience`. Its findings land in
    `nerv/audit/pass-kaji-audit-round-N.json`.
  - Balthasar takes the readability audit lens back and claims `jd-judge-a` and
    `review-readability`. Casper keeps the process lens only and claims no equivalent.
  - Fuyutsuki gains a read-only `MODE: refute` with the whole `kaji-refuter` contract (no
    writes, no `mem_save`, no new findings) and claims `review-refuter`. The `kaji-passes` group
    is now `kaji` and `kaji-audit`.
  - `models.kaji-coverage`, `models.kaji-resilience` and `models.kaji-refuter` overrides behave
    like a stale `models.kaji-security` one: `nerv apply-models` skips them, `--print`,
    `/nerv:status` and the wizard show them as rows with no purpose, and `--set-model
    <role>=default` refuses them, so delete the lines by hand (or move the values to
    `kaji-audit` or `fuyutsuki`).

### Added
- agents: extend balthasar and kaji-coverage to cover gentle-ai readability and reliability (0b90206)
- agents: add kaji-resilience audit pass for resilience and performance (b826c89)
- config: list every gentle-ai equivalent per role and guard v4 agent parity (8dc7ad3)

## [2.0.1] - 2026-10-05

### Fixed
- specs: refuse a requirement both modified and removed in one delta (d321c9a)
- atomicfile: refuse to replace a dangling symlink (8faa5a0)
- config: suggest only real phase keys as from: hints (3141729)
  - `nerv configure --print` rows gain a `from_phase` field; `gentle_ai_equivalent` is now
    informational only.
- config: flag a from: phase missing from gentle-ai state (66a4bf9)
  - The model source reads `gentle-ai:<phase> (missing; plugin default)`, and `--set-model` warns
    with the available phases.

## [2.0.0] - 2026-10-05

### Breaking
- gentleai: require gentle-ai 4.x (ebdc5ba)
  - gentle-ai 4.0.0 retired SDD/OpenSpec, and NERV no longer depends on it. gentle-ai 3.x is
    refused by `nerv install --require-gentle-ai` and flagged as unsupported elsewhere.
  - To upgrade, run `go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@latest`
    (or `brew upgrade gentle-ai`), then `gentle-ai sync`. On go installs, `gentle-ai upgrade` from
    3.x cannot reach 4.x.
  - The v4 sync leaves the v3 `~/.claude/skills/sdd-*`, `~/.claude/agents/sdd-*.md` and
    `~/.claude/commands/gentle-sdd-*.md` in place. It also keeps a `PreToolUse` hook calling the
    removed `gentle-ai sdd-preflight-hook`. NERV does not use any of them, so they are safe to
    remove.

### Added
- cli: add nerv spec-compose ported from gentle-ai (20d00bc). It replaces
  `gentle-ai sdd-archive-compose` for Aoba's archive merges.
- plugin: bootstrap openspec config in /nerv:init without sdd-init (366fcbd)

### Changed
- config: map role equivalents to gentle-ai v4 agents (dcbdacb)

### Fixed
- plugin: read change artifacts instead of sdd-status (c38f182)
- specs: accept CRLF headings in spec-compose (47babd0)
- specs: refuse a requirement repeated within one delta section (b1f23f1)
- atomicfile: write through a symlink to its target (bb29e87)
- plugin: never rewrite an existing openspec config in /nerv:init (45f69f0)
- specs: ignore fenced headings when parsing a delta (7f217c4)

## [1.0.0] - 2026-10-02

### Breaking
- config: drop the GitHub Projects and Jira task providers (cdf8cc7)
  - `tasks.provider` accepts only `teamwork` or `none`. Existing configs are cleaned on the next
    write: `nerv` removes `tasks.provider: github-projects|jira` (the default then applies) and
    the `tasks.providers.github-projects` / `tasks.providers.jira` blocks; run
    `nerv configure --init-repo` in a repository to clean its project `nerv.yaml`.

### Added
- config: describe each role and its gentle-ai equivalent (ba9fbce)
- wizard: explain each role and suggest its gentle-ai equivalent (6b7d766)
- config: strip the removed task providers from existing configs (ae0e388)
- configure: strip the removed providers on every config write (4b31fa6)
- configure: clean removed task providers from existing project configs (8228784)

### Fixed
- wizard: size the models table columns to their longest value (51986fa)
- wizard: size the purpose column to the longest role purpose (720477b)
- config: keep foreign comments and strip every legacy provider form (bdb2fc0)
- config: treat in-scalar quotes as text and keep --init-repo non-fatal on unreadable configs (3fcc9e0)

## [0.3.0] - 2026-10-02

### Breaking
- move the repository and Go module to war-apps/nerv-by-gentle-ai (3228875)
  - Install with `go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest`; the old
    `github.com/war-apps/nerv-gentle-ai` module path no longer resolves to new versions.

### Added
- skills: ask before running the gentle-ai sync (4fba6c6)
- skills: run gentle-ai sync for missing gentle-ai skills (8afa5cd)
- tasks: focus the herdr workspace of a newly opened worktree (774004a)

### Fixed
- install: sync the nerv marketplace before refreshing the plugin cache (cb9a1a9)
- install: remove the nerv marketplace registration on uninstall (5da904f)
- install: tolerate only the marketplace's own not-found message on uninstall (5139e65)
- skills: never prompt for the sync when printing --json (88decdc)
- skills: refresh statuses after sync and surface its diagnostics (9737cf8)

## [0.2.0] - 2026-10-01

### Added
- alpha and rc release channels driven by develop and release branches (83689b1)
- get-nerv.sh one-line installer for Linux and macOS with channel selection (b673c22)
- get-nerv.ps1 one-line installer for Windows with channel selection (2297889)
- Go module with the embedded plugin, materialization and nerv version (d02eb0d)
- internal/config ports the nerv.yaml editor, catalogues and model specs to Go (4c7797d)
- environment packages for the nerv binary (runner, gentle-ai, models, skills) (0e477cc)
- nerv configure replaces configure.ps1 and configure-models.ps1 (1b2c944)
- nerv install, uninstall, apply-models and skills replace install.ps1 and install-skills.ps1 (3c94749)
- interactive configuration wizard as nerv configure and the end of nerv install (3a3c044)
- nerv release replaces release.ps1 and release-guard.ps1; goreleaser publishes the binaries (77a73fe)
- plugin commands call the nerv binary and scripts/install.sh installs it (4f6e56e)
- configurable worktree location through git.worktree_pattern (98a700c)
- initial claude plugin eval corpus for the NERV plugin (1a708d1)
- herdr: detect herdr and resolve its worktrees directory (732cef9)
- configure: offer default / herdr / custom worktree path pattern menu (07d5f4b)

### Changed
- release use case, refusal type, atomic writes, paths and store extracted (9081a45)
- remove the port's residue and unify the duplicated helpers (9d99eec)
- skills: split the orchestrator protocol into a core and reference files (14e1d97)

### Fixed
- wizard safety after the spot-check incident (8051784)
- installer suite keeps the interpreter's directory on usrmerge systems (d69747a)
- hooks: print a short activation header instead of the full protocol (c2e0c85)
- evals: tolerate markdown emphasis and grade the header body line (17077cb)

## [0.1.0] - 2026-09-28

### Added

- NERV plugin overlay on gentle-ai: the Ikari orchestrator protocol and 18
  Evangelion-named agents (Misato, Hyuga, Balthasar, Melchor, Casper,
  Fuyutsuki, Ritsuko, Shinji, Asuka, Rei, Toji, Kaworu, Maya, Kaji and the
  kaji-security/kaji-coverage/kaji-refuter audit passes, and Aoba).
- LIGHT and FULL delegation paths, with FULL adding a per-task MAGI vote
  (Balthasar, Melchor, Casper), a governance veto on new
  skills/scripts/commands (Fuyutsuki), a mandatory quality gate before any
  audit (Maya), a multi-pass ranked audit compiler (Kaji), and an
  end-of-run summary reporting tokens, time, and model per agent.
- Per-repository activation via `.nerv/nerv.yaml`, layered over user-scope
  defaults in `~/.claude/nerv/nerv.yaml` with the same schema.
- The setup wizard (`tools/configure.ps1`) and its non-interactive mode
  (`-Print`, `-Set`, `-SetModel`, `-InitRepo`, `-InstallCommands`) for
  scripted or tested configuration.
- Slash commands `/nerv:init`, `/nerv:status`, and `/nerv:configure` for
  guided repository activation, status reporting, and reconfiguration.
- Per-role model and effort assignment (`models:` in `nerv.yaml`,
  `tools/configure-models.ps1`), including inheriting a role's assignment
  from gentle-ai's own phase table.
- A self-contained Teamwork adapter and the `/task:*` command family
  (start, stop, done, close, blocked, take, priority, timers, listings)
  for provider-agnostic task tracking.
- Skills installation and management for the agent stacks consumed by
  NERV's roles (testing, code, best-practices, architecture, audit).
- The Engram project hook (SessionStart) and knowledge base integration
  for cross-session memory of decisions, conventions, and run history.
- Tag-driven release process: `tools/release.ps1` computes the version and
  changelog from Conventional Commits, CI runs every suite on pull requests,
  and the Release workflow tags, publishes the GitHub Release and opens the
  back-merge on each merge to `main`.
