# /nerv:init offers every setup, and install keeps gentle-ai current

Objective: `/nerv:init` initializes the repository and then offers the configuration flows (NERV, tasks, release, deploy); `nerv install` and `nerv upgrade` upgrade gentle-ai to its newest 4.x release with `gentle-ai upgrade`, on every platform.
Branch: `feat/init-setup-and-gentle-ai-upgrade` from `develop`, one PR. Delivery: ask-on-risk; forecast ~350 authored lines. TDD: strict (session configuration); runner `go test ./...`.

## Specs

- **S1** (user, 2026-11-02) "necesito que el comando /nerv:init no solo inicie el repositorio sino que pregunte configurar el release y el deploy y todos los comandos para configurar".
- **S2** `/nerv:init`, after the skill registry step, asks one multi-select blocking prompt with the setup flows that exist on this machine: `/nerv:configure` (always), `/task:setup` (task extension installed), `/release:setup` and `/release:deploy-setup` (release extension installed). Each selected flow runs in that order by following its command file, as `/release:ship` follows `/release:build`. Selecting none ends the command as today. Flows whose extension is missing are not offered.
- **S3** (user, 2026-11-02) "cuando instalo/actualizo/upgrade nerv no esta actualizando gentle-ai ... deberia utilizar la ultima version [4].x.x de gentle-ai (se deberia correr con gentle-ai upgrade)". Confirmed: major 4 (`gentleai.SupportedMajor`), not 3.
- **S4** `nerv install` upgrades gentle-ai before laying NERV over its files: when the installed gentle-ai is 4.x and the newest GitHub release is newer and still 4.x, run `gentle-ai upgrade gentle-ai`, then `gentle-ai sync`. A newer major is reported and not installed. A failed release lookup or upgrade is a warning; install continues with the installed gentle-ai.
- **S5** `nerv upgrade` keeps upgrading gentle-ai when the nerv binary cannot be replaced (Windows has no prebuilt binary, `cmd/nerv/upgrade.go:85-89` returned before `upgradeGentleAI`). gentle-ai is upgraded once per `nerv upgrade`, not twice (upgrade and the install it runs).
- **S7** (user, 2026-11-02) "cambia un poco la funcionalidad del provider teamwork: cuando no existe un workflow, agregar como cambio de estado un comentario en la tarea, asi queda registrado que se cambio de estado". `moveStage` on a task without a workflow posts a comment `<comment_labels.stage>: <target>` (default label `ESTADO`) and returns `{stage}` noting it was recorded as a comment. A task with a workflow but no matching stage stays `skipped`.
- **S6** Docs follow the behavior: `docs/commands.md`, README, troubleshooting, CHANGELOG via the release.

## Tasks

| ID | Specs | Route | Status | Commit |
|---|---|---|---|---|
| T1 | S1, S2, S6 | inline (one command file plus docs) | done: RED observed (init_setup_prose_test), GREEN root package, `go vet ./...`; releaseplan/releaserun failures pre-exist on develop (Windows paths, pwsh) | fac81d7 |
| T3 | S7 | inline (move-stage procedure, task skill contract row) | done: RED observed (teamwork_stage_comment_prose_test), GREEN root package | d19ecea |
| T2 | S3, S4, S5, S6 | delegated writer (cmd/nerv upgrade + install, tests, docs) | done: RED observed (TestRunInstall_UpgradesGentleAIBeforeLayingFiles, TestUpgrade_UnsupportedPlatformStillUpgradesGentleAIOnce, ...), GREEN; `go vet`, `go build` clean; parent spot check reran `go test ./...` | 1e51d18 |

Route evidence: T1 touches `plugin/commands/init.md` and docs only, mechanical once the chaining pattern (`plugins/release/commands/ship.md`) is known. T2 touches 2+ non-trivial Go files with tests: writer trigger.

## Log

- **L1** (2026-11-02, user) Requests S1 and S3; work without a Teamwork task ("no, esto es sin tarea").
- **L2** Exploration: setup commands are `/nerv:configure`, `/task:setup`, `/release:setup`, `/release:deploy-setup`; `nerv upgrade` replaces the binary, then `upgradeGentleAI`, then the new binary's install; `nerv install` only prints `upgradeHint` for a non-4.x gentle-ai.
- **L3** (2026-11-02) T1 committed fac81d7. Known environmental failures on Windows, also on develop: internal/releaseplan (absolute `\tmp` paths), internal/releaserun (pwsh real-exec tests).
- **L4** (2026-11-02, user) Adds S7 mid-run (Teamwork stage change as a comment without a workflow).
- **L5** (2026-11-02) T3 committed d19ecea.
- **L6** (2026-11-02) T2 committed 1e51d18: `nerv install` runs the shared `upgradeGentleAI` before laying files; `nerv upgrade` no longer returns before it on a replace failure, upgrades gentle-ai itself and passes the hidden `--skip-gentle-ai-upgrade` to the install it runs (one upgrade per run); exit 1 when the replace or the gentle-ai upgrade failed. `testOptions` now uses a no-network HTTP transport.
- **L7** (2026-11-02) Verification: failures in cmd/nerv (TestIntegration_Release*, TestUpgrade_StableReplacesBinaryAndRunsInstall chmod mode), internal/artifact, atomicfile, deploy, deployrun, releaseplan, releaserun also fail on develop on this Windows machine (parent reran them in a develop worktree); internal/artifact showed a transient `Access is denied` on the test binary. RDD off (global); assess: risk medium, review_due false (under_budget). No native review.
