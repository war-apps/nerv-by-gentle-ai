# Distribution: release channels and one-line installers

Branch `feature/distribution` from `develop` (`aaa3daf`, after the v0.1.0 back-merge). Started 2026-09-28.
Delivery strategy: `feature-branch-chain` (cached project choice). Forecast ~1400 authored changed
lines, shipped as three chained PRs (see Slices).
TDD: strict (global setting). Runners: `pwsh -NoProfile -File tests/release.test.ps1`,
`bash tests/get-nerv.test.sh`, `pwsh -NoProfile -File tests/get-nerv.test.ps1`, plus the existing
suites (`configure`, `configure-models`, `install-apply-models`, `install-skills`, `release-guard`,
the two hook `.sh` suites) which stay green.
Engram mirror: topic `odd/distribution/tasks` (project `nerv`).

## Objective

1. Three release channels driven by Gitflow branches: every push to `develop` publishes an
   `alpha` pre-release, every push to `release/*` publishes an `rc` pre-release, every merge to
   `main` publishes the stable release (already shipped in the release pipeline feature).
2. One-line installation with `curl` or `wget` on Linux and macOS, and with `irm | iex` or
   `curl` + `pwsh` on Windows, choosing a channel, and running the configuration wizard right
   after installing.

## Problem / why

- User request 2026-09-28: "que develop genere su propia versión alpha, release/rc genere la
  versión rc y main la versión estable" and "que se pueda instalar todo mediante curl o wget
  tanto para linux, macos o windows; el instalador debe correr la configuración después de
  instalarlo". The repository is public since 2026-09-28 and `main` is the default branch, so
  anonymous GitHub API and raw file access work.
- `claude plugin marketplace add` accepts no git ref (verified with `--help`), so channel
  selection cannot be expressed through the marketplace itself. The existing installer
  (`plugin/tools/install.ps1`) already registers a *directory* marketplace from a local checkout
  and enables the plugin, so the bootstrap only has to put the right ref on disk and delegate.
- `plugin/tools/install.ps1` defaults `SettingsPath` to `$env:USERPROFILE\.claude\settings.json`
  (`install.ps1:105`, `:121`): on Linux and macOS `USERPROFILE` is unset and the backslash becomes
  part of the file name. It must resolve from `Get-NervHomeDir` with forward slashes.
- Pre-release tags created by the workflow with `GITHUB_TOKEN` do not trigger other workflows,
  so the channel jobs must be branch-driven, not tag-driven.

## Scope and constraints

- Channel jobs never commit: they compute a tag and publish a GitHub pre-release. Only the
  `release/*` branch carries the `plugin.json` bump (Gitflow), so:
  - `alpha` base version = the next version computed from Conventional Commits since the last
    stable tag (`release.ps1 -PreRelease alpha`, base `next`); when nothing is releasable the job
    prints "nothing to pre-release" and succeeds without tagging.
  - `rc` base version = the version already in `plugin.json` on the release branch
    (`release.ps1 -PreRelease rc -PreReleaseBase current`).
  - `N` in `-alpha.N` / `-rc.N` = 1 + the highest existing tag for that base and label.
- Pre-release notes: `gh release create --prerelease --generate-notes --notes-start-tag <last
  stable tag>` (or without the start tag when none exists).
- `ci.yml` drops the `push: develop` trigger: the `alpha` job runs the full suite on that push
  already; pull requests keep running CI.
- Installers are repository files at the root (`get-nerv.sh`, `get-nerv.ps1`), never shipped
  under `plugin/`. They are downloaded from
  `https://raw.githubusercontent.com/war-apps/nerv-gentle-ai/main/<file>` and support
  `NERV_CHANNEL` (`stable` default | `alpha` | `rc`), `NERV_HOME` (checkout directory, default
  `~/.nerv/src`), `NERV_NO_CONFIGURE` (skip the wizard), and for the shell script the same as
  flags `--channel`, `--dir`, `--no-configure`. Every network location is overridable for tests:
  `NERV_REPO_URL` (clone source), `NERV_API_BASE` (GitHub API base).
- Prerequisites checked by the bootstrap: `git`, `pwsh` 7 (the whole toolchain is PowerShell;
  print the platform's install hint and exit 1 when missing), `claude` CLI. `gentle-ai` is checked
  by `install.ps1` itself. No prerequisite is installed on the user's behalf.
- Ref resolution per channel through the GitHub releases API: `stable` → `releases/latest`
  tag; `alpha`/`rc` → newest pre-release whose tag contains `-alpha.` / `-rc.`; `alpha` with no
  pre-release yet falls back to branch `develop`, `rc` with none falls back to the newest
  `release/*` branch or fails with a clear message.
- Checkout: `git clone --depth 1 --branch <ref> <url> <dir>` when the directory does not exist;
  otherwise `git -C <dir> fetch --depth 1 origin <ref>` and `checkout --detach FETCH_HEAD`
  (tags) or `checkout <branch>` + `pull` (branches). Re-running the installer is the update path.
- Delegation: after the checkout, run
  `pwsh -NoProfile -File <dir>/plugin/tools/install.ps1 -RefreshCache -Skills -Configure` (the
  script registers the directory marketplace, enables the plugin, refreshes the plugin cache,
  applies model assignments, installs skills, and runs `configure.ps1 -NoRefresh`). When
  `NERV_NO_CONFIGURE` is set, omit `-Configure` and print the command to run it later.
- `curl ... | bash` consumes stdin for the script itself, so the wizard would read from the pipe.
  The shell bootstrap re-attaches the terminal (`</dev/tty`) for the `pwsh` call when a tty is
  available; without one it behaves as `NERV_NO_CONFIGURE=1` and says so. `irm | iex` runs in the
  caller's console, so `get-nerv.ps1` needs no such handling, but must read its options from
  environment variables because `iex` passes no parameters.
- Artifacts in English. Never push without explicit OK: the branch push and the PR chain are asked
  for at the end.

## Tasks

- [x] T1 DONE 2026-09-28 (writer on sonnet; parent spot check re-ran release 134/134 and release-guard 54/54, actionlint clean, read the alpha and rc jobs). Commit `83689b1`. RED observed (unknown parameter `-PreReleaseBase`), GREEN 134 (+28). Bug caught in the cycle: `-match` is case-insensitive, so `RC` passed `^[a-z]+$`; fixed with `-cnotmatch`. Unverified: the alpha and rc jobs have not run on GitHub yet; the first push to `develop` after this merges exercises `alpha`. Release channels. `tools/release.ps1`: `-PreRelease <label>` accepts any lowercase
  label (`alpha`, `beta`, `rc`; validated `^[a-z]+$`), new `-PreReleaseBase next|current`
  (default `next`; `current` uses the `plugin.json` version as the base and does not require a
  releasable commit), JSON gains `tag` (`v<next>`) and `base`. `tests/release.test.ps1` covers
  alpha numbering, `current` base on a fixture with a bumped `plugin.json`, label validation and
  the JSON fields. `.github/workflows/release.yml`: jobs `alpha` (push `develop`), `rc` (push
  `release/**`), `stable` (push `main`, unchanged), each running the full suite first; the
  tag-driven `prerelease` job and the `tags:` trigger are removed; pre-release tags are annotated,
  pushed, then `gh release create --prerelease --generate-notes [--notes-start-tag <last
  stable>]`. `.github/workflows/ci.yml`: remove `push: develop`. Route: writer (writer trigger:
  4 non-trivial files). Checks: release suite green, `actionlint` clean, the six other `.ps1`
  suites unchanged.
- [x] T2 DONE 2026-09-28 (writer on sonnet; parent spot check re-ran get-nerv 20/20 and install-apply-models 28/28, shellcheck clean, read main/delegation and confirmed in install.ps1 that registration, -RefreshCache, -Skills and -Configure compose in that order). Commit `b673c22`. RED 0/20 then GREEN 20/20; install fix RED 26/2 then GREEN 28/28. Bugs caught: fetching a tag leaves no local tag ref (explicit refs/tags refspec now); /dev/tty unusable in the sandbox, so `NERV_TTY_OVERRIDE`/`NERV_TTY_DEVICE` seams exist for tests. Accepted deviation: the settings-path test compares against Join-Path output instead of asserting "no backslash" because Windows normalizes separators. `get-nerv.sh` + `tests/get-nerv.test.sh` + the `install.ps1` portability fix.
  Bash (`#!/usr/bin/env bash`, `set -euo pipefail`), POSIX tools only plus `curl` or `wget`
  (whichever exists) for the API call, `git`, `pwsh`. Behaviour per Scope. Tests use a temporary
  `PATH` with stub executables (`git` is real; `pwsh`, `claude`, `curl` are stubs that record
  their arguments and return canned API JSON from files), a local bare git repository with
  tags `v0.1.0`, `v0.2.0-alpha.1`, `v0.2.0-rc.1` as `NERV_REPO_URL`, and assert: channel → ref
  resolution (incl. the `develop` fallback), fresh clone vs update, the exact `pwsh` invocation
  with and without `-Configure`, prerequisite failures with exit 1 and a hint, `--help`. The
  `install.ps1` fix: `SettingsPath` default from `Get-NervHomeDir` with forward slashes, plus a
  test in `tests/install-apply-models.test.ps1` or a new small case group asserting the default
  contains no backslash and resolves under `HOME` when `USERPROFILE` is unset. Route: writer.
- [x] T3 DONE 2026-09-28 (writer on sonnet; parent spot check re-ran get-nerv.ps1 suite 29/29 and read the function list, guard, delegation and API seams). Commit `2297889`. RED 0/1 then GREEN 29/29. Incident: an early buggy run of the writer executed the real installer against the developer machine (clone into ~/.nerv/src, settings.json marketplace path rewritten, plugin cache reinstalled, cache verification failed); backup settings.json.bak-nerv-20260928-215744 holds the correct path; remediation asked to the user. Lesson recorded: bootstrap tests must set NERV_REPO_URL/NERV_HOME/NERV_API_FIXTURE_DIR before any invocation and assert them, never rely on parameter binding alone. `get-nerv.ps1` + `tests/get-nerv.test.ps1`. PowerShell 7, same behaviour and the same
  environment variables, parameters `-Channel`, `-Dir`, `-NoConfigure` when invoked as a file,
  environment fallbacks when piped to `iex`. Uses `Invoke-RestMethod` against `NERV_API_BASE`,
  `git` for the checkout, then the same `install.ps1` delegation. Tests mirror T2 with stub
  functions/executables and a local bare repository. Route: writer.
- [ ] T4 Docs. README: a new "Install" section at the top of Setup with the one-liners:
  Linux/macOS `curl -fsSL https://raw.githubusercontent.com/war-apps/nerv-gentle-ai/main/get-nerv.sh | bash`
  (and the `wget -qO-` form; `NERV_CHANNEL=alpha` prefix for channels), Windows
  `irm https://raw.githubusercontent.com/war-apps/nerv-gentle-ai/main/get-nerv.ps1 | iex` and the
  `curl -fsSL -o get-nerv.ps1 ... ; pwsh -File get-nerv.ps1 -Channel rc` form; what the installer
  does, prerequisites, update by re-running, `NERV_HOME`. "Releases" section: the three channels
  and what each branch publishes. Close this document. Route: inline or writer by size.

## Slices (feature-branch-chain)

- PR 1/3: T1 (channels).
- PR 2/3: T2 (shell installer, install.ps1 fix).
- PR 3/3: T3 + T4 (PowerShell installer, docs).

## Acceptance criteria

- A push to `develop` with at least one `feat`/`fix` since `v0.1.0` produces tag
  `v0.2.0-alpha.1` and a GitHub pre-release; the next such push produces `-alpha.2`.
- A push to `release/0.2.0` (with `plugin.json` at `0.2.0`) produces `v0.2.0-rc.1`.
- On a clean machine with `git`, `pwsh` and `claude`: the one-liner clones the channel ref into
  `~/.nerv/src`, registers and enables the plugin, and ends inside the configuration wizard;
  re-running it updates the checkout and refreshes the plugin cache.

## Decisions pending (user)

- None at creation time.

## Progress log

- 2026-09-28: T3 complete (`2297889`), PowerShell installer; T4 docs next. Writer incident on the developer machine recorded in T3.
- 2026-09-28: T2 complete (`b673c22`), shell installer and install.ps1 portability; T3 (PowerShell installer) next.
- 2026-09-28: T1 complete (`83689b1`), channels in release.ps1 and release.yml; T2 (shell installer) next.
- 2026-09-28: feature document created; branch `feature/distribution` from `develop` (`aaa3daf`).

## Next step

T4 (writer): README Install and Releases sections; then close the document and ask for the push and PR chain.
