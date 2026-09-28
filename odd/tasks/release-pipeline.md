# Release pipeline: tag-driven versioning and GitHub Releases

Branch `feature/release-pipeline` from `develop` (`c9a4b1f`). Started 2026-09-28.
Delivery strategy: `feature-branch-chain` (cached project choice). Forecast ~850 authored
changed lines, so the feature ships as two chained PRs (see Slices).
TDD: strict (global setting), runner `pwsh -NoProfile -File tests/release.test.ps1`; the
existing suites (`configure`, `configure-models`, `install-apply-models`, `install-skills`,
hook `.sh` tests) stay green.
Engram mirror: topic `odd/release-pipeline/tasks` (project `nerv`).

## Objective

Give the plugin a versioning and release process that (a) keeps `plugin/.claude-plugin/plugin.json`
`version`, the git tag `vX.Y.Z` and `CHANGELOG.md` in agreement, (b) publishes a GitHub Release
per version, and (c) fits Gitflow without any bot commit on a base branch.

## Problem / why

- Claude Code detects plugin updates by comparing the `version` string of `plugin.json`
  (docs: plugins/loading, "Versions and updates"). A release that does not bump it is invisible
  to every installed user.
- The repo has no tags, no CI and no changelog; the version is hand-set to `0.1.0`.
- Gitflow rules (CLAUDE.md) forbid direct commits to `main`/`develop`, which rules out the
  default semantic-release flow (bot commit of the bump on `main`). Decision 2026-09-28 (user):
  tag-driven releases, version derived from Conventional Commits, no semantic-release.
- gentle-ai itself releases tag-driven (goreleaser) with `-rc.N` pre-releases; aligning the
  convention keeps the two projects readable side by side.

## Scope and constraints

- Release tooling is repository tooling, not plugin runtime: it lives in `tools/release.ps1`
  and is NOT shipped under `plugin/` (no forwarder).
- Conventional Commits drive the bump: `feat` → minor; `fix`, `perf` → patch; `!` or a
  `BREAKING CHANGE:` footer → major; `docs`, `chore`, `test`, `refactor`, `ci`, `build`,
  `style` → no bump on their own. No releasable commit → exit 1 "nothing to release".
- The bump lands on a `release/X.Y.Z` (or `hotfix/X.Y.Z`) branch, exactly where Gitflow puts
  it; the PR to `main` carries the bump and the changelog section; CI on `main` tags and
  publishes. Back-merge `main` → `develop` is opened as a PR by CI, never pushed directly.
- Pre-releases: `vX.Y.Z-rc.N` tags from `release/*` publish a GitHub pre-release.
- Never rewrite `plugin.json` formatting beyond the `version` value (2-space JSON, LF, no BOM).
- Artifacts in English. Never push without explicit OK (branch pushes for PRs are authorized
  by the user's "arranquemos").
- Out of scope: changing the GitHub default branch to `main` and creating the baseline tag
  `v0.1.0` on the current `main` are user decisions recorded under Decisions pending.

## Tasks

- [x] T1 DONE 2026-09-28 (writer on sonnet; parent spot check re-ran the suite 106/106, the real-repo `-Preview -Json` (current 0.1.0, no tag, bump minor, next 0.2.0, 90 commits inspected) and read back the guard, strict mode and encoding). Commit `706b631`. RED observed on `release-script-exists` (0/1), GREEN 106/106; existing suites 169/25/26/29. Writer deviations accepted: fixtures use `git init -b main` and `commit -F` for multi-line bodies; "apply twice" idempotence is tested as apply, tag, apply again (exit 1, files byte-unchanged) because `-Apply` never tags. One real bug caught in GREEN: PowerShell empty-pipeline unwrapping made `@($null)` a one-element array; fixed at the call site and in both consumers. 1257 lines, above the heuristic as forecast. `tools/release.ps1` + `tests/release.test.ps1`. Pure functions dot-sourced by the
  tests behind the same interactive guard pattern as `configure.ps1`: `Get-NervLastReleaseTag`
  (highest `vX.Y.Z` tag, ignoring pre-releases unless `-IncludePreRelease`),
  `Get-NervCommitsSince`, `Get-NervBumpKind` (per the rules above), `Get-NervNextVersion`
  (from current `plugin.json` version + bump; `-PreRelease rc` appends `-rc.N` with N = next
  free number for that base), `Set-NervPluginVersion` (rewrites only the `version` value),
  `New-NervChangelogSection` (Keep-a-Changelog groups: Breaking, Added, Changed, Fixed;
  newest on top; commit subject + short sha; scope kept), `Update-NervChangelog` (inserts the
  section under `## [Unreleased]`). CLI modes: `-Preview` (default; prints next version,
  bump kind and the section), `-Apply` (writes `plugin.json` and `CHANGELOG.md`), `-Version
  X.Y.Z` (explicit override, still validated as semver > current), `-PreRelease rc`, `-Json`.
  Tests build real temporary git repositories with `git init`/`commit`/`tag` (no mocks of
  git), cover every bump rule, no-release exit 1, pre-release numbering, `plugin.json`
  byte-preservation, changelog insertion and idempotence. Route: writer (writer trigger:
  2 non-trivial files). Checks: `pwsh -NoProfile -File tests/release.test.ps1` green plus
  the four existing `.ps1` suites unchanged.
- [ ] T2 GitHub Actions. `.github/workflows/ci.yml`: on pull_request to `develop` and `main`,
  run the four `.ps1` suites on `ubuntu-latest` (pwsh preinstalled) and the two hook `.sh`
  suites with bash. `.github/workflows/release.yml`: on push to `main`, read `plugin.json`
  version, fail if the tag `v<version>` already exists or `CHANGELOG.md` has no `## [<version>]`
  section, run the suites, create the annotated tag, create the GitHub Release with the
  changelog section as notes (`gh release create`), then open the back-merge PR `main` →
  `develop` (`gh pr create`, skip if one is open); on push of a tag matching `v*-rc.*`, create a
  GitHub pre-release from the tag. Permissions `contents: write`, `pull-requests: write`;
  actions pinned by SHA. Route: writer, loads the `github-actions-templates` skill first.
  Checks: `actionlint` if available, otherwise structural readback; a dry `pwsh` run of the
  version/tag/changelog guard script extracted to `tools/release-guard.ps1` with its test.
- [ ] T3 Docs and baseline files. `CHANGELOG.md` with `## [Unreleased]` and a `## [0.1.0]`
  baseline entry summarizing what is on `main` today; README "Releases" section (how to cut a
  release, pre-releases, what the CI does, the `main`-as-default-branch note for marketplace
  consumers); `odd/tasks/release-pipeline.md` closed. Route: inline or writer depending on
  size.

## Slices (feature-branch-chain)

- PR 1/2: T1 (`tools/release.ps1`, `tests/release.test.ps1`).
- PR 2/2: T2 + T3 (workflows, guard script, changelog, README).

## Acceptance criteria

- `pwsh tools/release.ps1 -Preview` on this repo prints the next version derived from the
  commits since the last tag (or since the root when no tag exists) and the changelog section.
- `-Apply` changes exactly one line of `plugin.json` and inserts one section in `CHANGELOG.md`;
  running it twice is a no-op with exit 0 when nothing new is releasable.
- The release workflow refuses a `main` push whose version already has a tag or lacks a
  changelog section, and otherwise produces tag + Release + back-merge PR.

## Decisions pending (user)

- Set `main` as the GitHub default branch (`gh repo edit --default-branch main`) so a
  marketplace added from GitHub serves released content, not `develop`.
- Create the baseline tag `v0.1.0` on the current `main` (`c9a4b1f`) so the first computed
  release starts from it.

## Progress log

- 2026-09-28: T1 complete (`706b631`), release script and suite; T2 (workflows) and T3 (docs) next.
- 2026-09-28: feature document created; branch `feature/release-pipeline` from `develop`.

## Next step

T2 (writer): CI and release workflows plus `tools/release-guard.ps1` with its suite, RED first; then T3.
