# nerv: hand release to nerv-release, then end clean of extensions

Objective: finish the nerv side of the ecosystem split (master plan `odd/tasks/ecosystem-split.md`, P4 nerv side and P5): nerv-release joins the tool catalogue and `nerv migrate` moves `release:`/`deploy:` to `release.yaml` (released as a 6.x first), then nerv loses every extension, its engine and their docs.
Branches: `feat/release-tool-migration` (N1-N4, release 6.6.0), then `feat/nerv-clean` (C1-C6). Delivery: ask-on-risk; work-unit commits. TDD strict, runner `go test ./...`.

## Specs

- **S1** (master S11, S5) nerv-release joins the tool catalogue: binary `nerv-release`, repository `war-apps/nerv-release-by-gentle-ai`, replacing `release`, `release-bash`, `release-conventional-commits`, `release-docker-compose`, `release-github`, `release-script`, `release-ssh-compose`. `nerv install`/`nerv upgrade` offer it like nerv-task; `/nerv:init` offers `nerv-release init` instead of `/release:setup` + `/release:deploy-setup`; docs updated.
- **S2** (master S4, S6, S12) `nerv migrate` moves release configuration to `release.yaml` when nerv-release is installed: user `config.yaml` `release:`/`deploy:` personal keys and `defaults.release`/`defaults.deploy` to the global `release.yaml`; a repository's `.ai-workflow/config.yaml` `release:`/`deploy:` to `.ai-workflow/release.yaml`. Unlike task.yaml, nerv-release keeps `release:` and `deploy:` as top-level sections and reads `defaults.release`/`defaults.deploy` under the same names, so keys keep their prefixes. The only personal (scope: user) release keys are `deploy.providers.ssh-compose.{infisical_domain, ssh_auth, ssh_key_file, ssh_known_hosts, ssh_connect_timeout}`; they never move repository → global. Same guarantees as the task move: existing keys never overwritten (conflicts reported), sections removed from `config.yaml` only after the new file is written, backups, dry run.
- **S3** (master S12) `nerv install` and `nerv upgrade` offer `nerv migrate` when a tool is installed and its sections are still in `config.yaml`; `nerv doctor` sees a pending task.yaml or release.yaml move.
- **S4** Release nerv 6.6.0 with S1-S3 before any deletion (S5-S8), so users migrate while nerv can still read their sections. Includes the pending "tested against gentle-ai 4.5.0" bump (5 places).
- **S5** (master S6) nerv ends clean: no `plugins/`, no `ExtensionsFS`, no extension-only Go code, tests or migrations, no `tasks:`/`release:`/`deploy:` handling except what `nerv migrate` needs to move them out; the personal-key lists migrate needs become frozen constants (no manifest reading).
- **S6** (user, 2026-11-20) Orphaned extension plugins: "Desactivar y avisar (Recomendado)": nerv upgrade sigue ofreciendo la herramienta; si la rechaza, desactiva los plugins *@nerv que ya no tienen fuente y le dice cómo instalar nerv-task / nerv-release. Pierde /task:* o /release:* hasta que la instale.
- **S7** (user, 2026-11-20) Legacy migrations: "Borrarlas (Recomendado)": se van con el motor. Quien tenga un config de 4.x/5.0-alpha recibe un error que dice qué sección mover a mano.
- **S8** The removed public contract is documented: `nerv release [setup --import]`, `nerv deploy`, `nerv version next`, the hidden `nerv provider`, `nerv install --with` extension selection, `release@nerv` and its 6 providers, `task@nerv`, `task-teamwork@nerv`; orchestrator prose that says "task extension" is reworded for the nerv-task tool (it keeps detecting `task:dispatcher`).

## Tasks

| ID | Specs | Route | Status | Commit |
|---|---|---|---|---|
| N1 | S1 | pending | pending | |
| N2 | S2 | pending | pending | |
| N3 | S3 | pending | pending | |
| N4 | S4 | pending | pending | |
| C1 | S5, S7 | pending | pending | |
| C2 | S5, S8 | pending | pending | |
| C3 | S5 | pending | pending | |
| C4 | S5, S6 | pending | pending | |
| C5 | S5, S6, S8 | pending | pending | |
| C6 | S8 | pending | pending | |

C1: frozen personal-key constants in migrate, migrate decoupled from ExtensionsFS, legacy migrations (release.go, ship.go, deploy.go, script.go) and checkLoads' engine loads removed. C2: cmd/nerv release, release setup, deploy (incl. hidden provider), version next and their wiring/tests. C3: extension-only packages (versioning, releaseplan, releaseconfig, deploy, deployrun, releaserun, releaseclone, artifact, deployprovider, deployconfig, taskprovider, proc). C4: core surgery (configure, wizard, doctor, install extension selection, plugin MaterializeBundle/RefreshCache, workflowconfig scope.go, internal/extension) plus the orphan cleanup of S6. C5: plugins/, ExtensionsFS, marketplace entries, prose tests. C6: docs and orchestrator prose.

## Log

- **L1** (2026-11-20, user) "si" to moving on to P5 after nerv-release 1.0.1; P5 found to depend on the unfinished nerv side of P4 (catalogue + release.yaml migration), so both are planned here in that order.
- **L2** (2026-11-20) Mapping (explorer): catalogue `internal/ecosystem/catalogue.go:31-41` (nerv-task only; release URLs derive from Repo/Binary); `cmd/nerv/migrate.go:141-153` hardcodes `nervTaskInstalled`; `internal/migrate/taskyaml.go` `migrateSection(dstPrefix)`, `removeMigrated`, `verifyTaskYAML` reusable; repo `personalKeys` (`tasks.go:123`) walks task providers only; no code offers migrate from install/upgrade (doctor only, and doctor's Compute never sets the task.yaml paths). P5 removal: ~13.8k production lines in 12 internal packages + ~2.2k in cmd/nerv with no core importer; `internal/extension`, `internal/plugin`, `workflowconfig/scope.go`, `configure`, `doctor`, `migrate` need surgery. nerv-release reads `defaults.release`/`defaults.deploy` (verified: nerv-release `internal/workflowconfig/scope.go:19-24,158`).
