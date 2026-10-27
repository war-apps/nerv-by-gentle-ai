# The deploy extension merges into release, with one command for version, build and deploy

Objective: one `release` extension (slots version, build, host, target) replacing `release` + `deploy`, and a command that runs version, build (lint and tests per the configured providers) and deploy, skipping the deploy when no deploy provider is configured. `task` is untouched.
Branch: `feat/merge-deploy-into-release` from `develop`. Delivery: feature-branch-chain (user's answer): tracker `feat/merge-deploy-into-release` (this document), child 1 `feat/merge-deploy-into-release-1-rename` (T1), child 2 `feat/merge-deploy-into-release-2-ship` (T2); ~1200 authored lines, mostly renames and docs. Test runner: `go test ./...`.

## Specs

- **S1** "la extension task la dejamos como esta"
- **S2** "fusiona las extensiones release y deploy". The merged extension is named `release` (user's answer: "release (Recommended)": "Absorbe a deploy: los providers pasan a ser release-github, release-ssh-compose, etc. Los usuarios de /release:* no cambian nada."). The `deploy` plugin and its `deploy-*` provider plugins no longer exist; their commands, skill and agent move into `release`.
- **S3** The deploy script target `bash` collides with the build provider `release-bash`; it is renamed `script` (user's answer: "script (Recommended)": "Carpeta release-script; en la config queda `targets[].provider: script`. `nerv migrate` reescribe `bash` → `script` en los targets.").
- **S4** "en esta nueva extension debe haber un command que ejecute tanto la obtencion de la version, el build como el deploy. deberia poder correr los tests (si hay), linters, etc. siempre en base a los providers configurados. si no se instalan ningun provider para los deploys omitir"
  - Assumption: the command is `/release:ship` (the former `/deploy:ship`); lint and tests are the build provider's pipeline stages (`lint`, `test`, `coverage`), run only when configured.
  - Assumption: every provider ships bundled, so "no deploy provider" means the project configures none: no `deploy:` section, or one with neither `host` nor `targets`. Then the deploy is skipped and the local artifact is the result.
- **S5** Assumption: the `deploy:` config section, `defaults.deploy`, and the `nerv deploy` CLI keep their names (config is not an extension name); user-scope keys of the host and target providers stay under `deploy.providers.<name>`.
- **S6** Assumption: `nerv migrate` rewrites `deploy.targets[].provider: bash` to `script` (and the user defaults) and moves project deploy providers `.ai-workflow/providers/deploy-<name>` to `release-<name>` (`extension: release`); `nerv install` unregisters the old `deploy`, `deploy-github`, `deploy-ssh-compose`, `deploy-bash` plugins from the settings and the plugin cache; `nerv deploy` refuses a `provider: bash` target with what to do.
- **S7** Help text, docs and README follow the new names (the CHANGELOG section comes from the commits at release time, with an upgrade note then).

## Tasks

| ID | Specs | Route | Status | Commit |
|---|---|---|---|---|
| T1 | S1, S2, S3, S5, S6 | inline | done: suite and vet green; RED observed for the multiline target regex; differential vs develop: only the intended rename in `nerv deploy --help` | dfca1eb (rename), f1ef759 (migrate), 59d8e56 (install), 2490024 (docs) |
| T2 | S2, S4, S7 | inline | done: prose test pins /release:ship; suite green | 2c9501d (amended with this log) |

Route evidence: one rename touching shared code; T2 is the plugin prose and docs depending on T1's names; no parallel unit, no writer trigger. Risk: item 3 (plugin, command and provider names, `targets[].provider` values consumed by user repos), mitigated by `nerv migrate`; independent verifier at the end.

## Log

- **L1** (2026-10-27, user): "hagamos algunos cambios: * la extension task la dejamos como esta * fusiona las extensiones release y deploy * en esta nueva extension debe haber un command que ejecute tanto la obtencion de la version, el build como el deploy. deberia poder correr los tests (si hay), linters, etc. siempre en base a los providers configurados. si no se instalan ningun provider para los deploys omitir"
- **L2** Answers: name `release`; bash target renamed `script`.
- **L3** Exploration: provider folder must be `<extension>-<name>` (internal/extension/provider.go:419); deploy providers resolved as `deployprovider.Extension + "-" + name` (internal/deployrun/run.go:439, contract.go:28); `nativeProviders` keyed by folder (cmd/nerv/deploy.go:279); `SectionOf` maps extension to config section (internal/workflowconfig/scope.go:76) and `userProviderValue` reads `deploy.providers.*` (cmd/nerv/deploy.go:203); migrate suggests deploy plugins (internal/migrate/repo.go:333); install keeps stale settings entries (internal/install/extensions.go:91).
- **L4** (2026-10-27) Found during T1: project deploy providers (`.ai-workflow/providers/deploy-<name>`) would stop resolving; added to S6 (migrate moves them). The deploy request envelope's `extension` becomes `release` (documented in docs/extensions.md and docs/migration.md).
- **L5** (2026-10-27, user): delivery "feature-branch-chain".
- **L6** T1 committed as 4b712fd (`refactor(release)!`, a BREAKING CHANGE: the next version on develop is a major one unless the message changes). T2: /release:ship step 0 skips the deploy without `deploy.host` or `deploy.targets`; docs updated.
- **L7** (2026-10-27) Independent verifier: S1-S5, S7 pass; S6 blocker B1 (a user top-level `deploy.targets` with `provider: bash` was moved under defaults unrenamed, a second run changed it) fixed in T1 (amended to a4d9307, regression test `TestScriptTarget_UserDeployBlockIsRenamedBeforeItMoves`, RED observed); recheck: one run renames, the second is a no-op. Advisories left: migrate --help does not list the new steps; the user defaults rename touches any `provider: bash` under defaults; project deploy providers named like a bundled build provider (docker-compose, conventional-commits) are not refused; plain migrate does not hint --user for defaults; retired-plugin cache uninstall is not retried.
- **L8** (2026-10-27) Native review: the one-commit T1 exceeded the reviewer context budget (`lens_context_budget_exceeded`); user chose to split it: dfca1eb rename, f1ef759 migrate, 59d8e56 install, 2490024 docs (same tree as before). dfca1eb still exceeded the budget (moved files count whole); user chose to leave it to the independent verifier. f1ef759: granted, risk/resilience/reliability, approved and acknowledged (review-911b32a89a32d0d5), 7 advisories (symlinks in a project provider folder are dereferenced; CRLF provider.yaml not rewritten; ReadDir errors swallowed; no guard for build-provider names; empty dirs not carried). 59d8e56: granted, approved and acknowledged, 4 advisories (a failed cache uninstall is not retried; disabled entries uninstalled too; no test of the uninstall failure path; "Removed" printed even when nothing was written). 2490024 and T2: assessed medium, under_budget, no review.
