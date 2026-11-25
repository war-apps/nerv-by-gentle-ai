# Direct upgrade to 7.x from any earlier nerv

Objective: a user on nerv 4.x, 5.x or 6.x reaches 7.x in one step: `nerv upgrade` / `nerv migrate` chain every conversion (files, tasks, release, plugins) without passing through 6.6 first; the release and deploy conversions removed from nerv in 7.0 live in nerv-release as `nerv-release migrate`.
Repositories: nerv-release (`feat/migrate-command`, release 1.1.0), then nerv (`feat/direct-upgrade`, release 7.1.0). Delivery: ask-on-risk; work-unit commits. TDD strict, runner `go test ./...` in both.

## Specs

- **S1** (user, 2026-11-24) "cuando termines tenemos que crear la migracion desde versiones anteriores"; asked which one, chose "Upgrade directo a 7.x": "Que alguien en nerv 4.x, 5.x o 6.x llegue a 7.x en un solo paso: nerv upgrade/migrate encadena todo (archivos, tasks, release, plugins) sin tener que pasar por 6.6 primero."
- **S2** (user, 2026-11-24) Where the conversions live: "En nerv-release (Recomendado)": "nerv-release suma un `nerv-release migrate` que convierte las secciones viejas (release: 4.2.0, pipeline:, ship:, provider: bash, deploy-*) a su release.yaml, usando el motor que ya tiene. nerv migrate convierte lo suyo (archivos, tasks, ODD) y, si encuentra esas secciones, se las pasa a nerv-release; nerv upgrade instala la herramienta y encadena todo. nerv sigue sin código de release (S5 intacto). Requiere un release de nerv-release (1.1.0) y uno de nerv."
- **S3** The conversions are the ones nerv 6.6 ran (nerv `e27c72b` internal/migrate release.go, ship.go, deploy.go, script.go and their callers), with the same results, written to nerv-release's `release.yaml` instead of nerv's `config.yaml`; personal keys go to the global `release.yaml` only (as nerv-clean S2). Same guarantees as nerv migrate: dry run, backups, nothing overwritten, a second run changes nothing.
- **S5** (design, L2) Each tool writes only its own files. With nerv-release installed, nerv moves the legacy sections as they are (4.2.0 `release:` and `pipeline:` from `.nerv/nerv.yaml`, `ship:`, a 4.2.0-shaped or moved-keys `release:`, `deploy:` with `provider: bash`, from the project file; the user config's `deploy:`/`defaults.deploy`) into the repository or global `release.yaml`, then runs `nerv-release migrate` (`--user` for the global file), which converts them inside `release.yaml`: `ship:` → `release:`, moved engine keys, 4.2.0 `release:` → `build: bash` stages from the presets plus the semantic-release import, `pipeline:` → `deploy:` (ssh-compose targets, `host: github` when origin is GitHub), `provider: bash` → `script`, personal keys → the global `release.yaml`, and renames `.ai-workflow/providers/deploy-<name>` → `release-<name>`. `nerv-release migrate` has `--dry-run`, `--cwd`, `--user`; exit 0 done or nothing to do, 1 refused, 2 environment/usage. nerv-release's other commands point a legacy section to `nerv-release migrate`.
- **S4** This supersedes nerv-clean S7 for the release and deploy sections: nerv no longer refuses them when nerv-release can convert them; without nerv-release installed, nerv's refusal names the command that installs it.

## Tasks

| ID | Specs | Route | Status | Commit |
|---|---|---|---|---|
| R1 | S2, S3 | pending | pending | |
| R2 | S2 | pending | pending | |
| N1 | S1, S2, S4 | pending | pending | |
| N2 | S1 | pending | pending | |
| N3 | S1 | pending | pending | |

R1: `nerv-release migrate` (port of the 6.6 conversions to release.yaml). R2: nerv-release 1.1.0 release. N1: nerv migrate hands the legacy sections to `nerv-release migrate`. N2: nerv upgrade/install chain (offer nerv-release when legacy sections are found, then migrate). N3: docs and nerv 7.1.0 release.

## Log

- **L1** (2026-11-24, user) "cuando termines tenemos que crear la migracion desde versiones anteriores"; options asked: config vieja / upgrade directo / dentro de las herramientas → "Upgrade directo a 7.x"; then where: nerv-release / restore in nerv / chain with 6.6 → "En nerv-release (Recomendado)".
- **L2** (2026-11-25) Mapping (two explorers): every symbol the 6.6 conversions used exists in nerv-release (yamlsection, releaseconfig MoveKeys/LegacyKeys/ImportSection/AlphaSectionKey/EngineProvider, releaseplan DotnetBuildServerEnv/DisableBuildServers/MinBuildServersSDK/EngineYAML, versioning.Import, deployprovider manifests, config.Document) except nerv's `paths.ProjectConfig` (nerv-release's loaders read `.ai-workflow/release.yaml`) and `extension.ProjectProvidersDir` (`.ai-workflow/providers`); `extension.ProviderFile` is `provider.ProviderFile`. Model for the command: `setup --import` (refusal → 1, dry run, `--cwd`) and `initrepo` (atomicfile backups, symlink checks). nerv hooks: legacy.go refusals, repo.go planRepo/translateConfig, defaults.go, cmd/nerv migrate.go setToolFileTargets/migrateRun, ecosystem.go offerToolMigration (install, upgrade). 4.2.0 and 5.0 users have no `nerv upgrade` (added 5.1.0; Windows from 6.4.0): they reach 7.x through the install script, whose `nerv install` offers the migrate. Design S5 chosen so nerv keeps no release code.

