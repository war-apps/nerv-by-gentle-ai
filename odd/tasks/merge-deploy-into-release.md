# The deploy extension merges into release, with one command for version, build and deploy

Objective: one `release` extension (slots version, build, host, target) replacing `release` + `deploy`, and a command that runs version, build (lint and tests per the configured providers) and deploy, skipping the deploy when no deploy provider is configured. `task` is untouched.
Branch: `feat/merge-deploy-into-release` from `develop`. Delivery: ask-on-risk; forecast ~900 authored lines (mostly renames and docs). Test runner: `go test ./...`.

## Specs

- **S1** "la extension task la dejamos como esta"
- **S2** "fusiona las extensiones release y deploy". The merged extension is named `release` (user's answer: "release (Recommended)": "Absorbe a deploy: los providers pasan a ser release-github, release-ssh-compose, etc. Los usuarios de /release:* no cambian nada."). The `deploy` plugin and its `deploy-*` provider plugins no longer exist; their commands, skill and agent move into `release`.
- **S3** The deploy script target `bash` collides with the build provider `release-bash`; it is renamed `script` (user's answer: "script (Recommended)": "Carpeta release-script; en la config queda `targets[].provider: script`. `nerv migrate` reescribe `bash` → `script` en los targets.").
- **S4** "en esta nueva extension debe haber un command que ejecute tanto la obtencion de la version, el build como el deploy. deberia poder correr los tests (si hay), linters, etc. siempre en base a los providers configurados. si no se instalan ningun provider para los deploys omitir"
  - Assumption: the command is `/release:ship` (the former `/deploy:ship`); lint and tests are the build provider's pipeline stages (`lint`, `test`, `coverage`), run only when configured.
  - Assumption: every provider ships bundled, so "no deploy provider" means the project configures none: no `deploy:` section, or one with neither `host` nor `targets`. Then the deploy is skipped and the local artifact is the result.
- **S5** Assumption: the `deploy:` config section, `defaults.deploy`, and the `nerv deploy` CLI keep their names (config is not an extension name); user-scope keys of the host and target providers stay under `deploy.providers.<name>`.
- **S6** Assumption: `nerv migrate` rewrites `deploy.targets[].provider: bash` to `script` (and the user defaults), and `nerv install` unregisters the old `deploy`, `deploy-github`, `deploy-ssh-compose`, `deploy-bash` plugins from the settings.
- **S7** Help text, docs, README, CHANGELOG (Unreleased) follow the new names.

## Tasks

| ID | Specs | Route | Status | Commit |
|---|---|---|---|---|
| T1 | S2, S3, S5, S6 | inline | pending | |
| T2 | S2, S4, S7 | inline | pending | |

Route evidence: one rename touching shared code; T2 is the plugin prose and docs depending on T1's names; no parallel unit, no writer trigger. Risk: item 3 (plugin, command and provider names, `targets[].provider` values consumed by user repos), mitigated by `nerv migrate`; independent verifier at the end.

## Log

- **L1** (2026-10-27, user): "hagamos algunos cambios: * la extension task la dejamos como esta * fusiona las extensiones release y deploy * en esta nueva extension debe haber un command que ejecute tanto la obtencion de la version, el build como el deploy. deberia poder correr los tests (si hay), linters, etc. siempre en base a los providers configurados. si no se instalan ningun provider para los deploys omitir"
- **L2** Answers: name `release`; bash target renamed `script`.
- **L3** Exploration: provider folder must be `<extension>-<name>` (internal/extension/provider.go:419); deploy providers resolved as `deployprovider.Extension + "-" + name` (internal/deployrun/run.go:439, contract.go:28); `nativeProviders` keyed by folder (cmd/nerv/deploy.go:279); `SectionOf` maps extension to config section (internal/workflowconfig/scope.go:76) and `userProviderValue` reads `deploy.providers.*` (cmd/nerv/deploy.go:203); migrate suggests deploy plugins (internal/migrate/repo.go:333); install keeps stale settings entries (internal/install/extensions.go:91).
