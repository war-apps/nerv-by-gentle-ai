# Upgrade command, project-scoped extension config, bare extension names

Objective: add `nerv upgrade`, make task/release/deploy configuration project-only with user-level seeds, and show extensions by their bare name.
Branches: one per task from `develop` (`feat/nerv-upgrade`, `feat/project-scoped-extension-config`, `refactor/bare-extension-names`), one PR each to `develop`. Delivery: ask-on-risk, three independent PRs. Test runner: `go test ./...`.

## Specs

- **S1** `nerv upgrade`: "agregame un comando de upgrade que chequee la funeta y descargue el ultimo binario y lo ejecute. que incluyan los flags de rc, beta y alpha" — corrected by L3: no `--beta` flag ("no le pongas el flag beta").
  - It checks the GitHub releases of `war-apps/nerv-by-gentle-ai`, picks the newest release of the channel, downloads `nerv_<version without v>_<os>_<arch>.tar.gz` and `checksums.txt` (the same assets `scripts/install.sh` uses), verifies the SHA-256, replaces the running binary, and then runs the new binary's `install` (as `scripts/install.sh` does after installing).
  - Channels: default stable; `--alpha` (tags `-alpha.N`); `--rc` (tags `-rc.N`). `--alpha` and `--rc` together are a usage error.
  - Assumption: a prerelease channel also considers stable releases, so it never stays on an older prerelease when a newer stable exists.
  - Already on the newest version (or newer): it says so, changes nothing, exit 0.
  - A failed download or checksum leaves the current binary untouched.
- **S2** Extension configuration scope: "la configuracion de task, deploy, release es por proyecto unicamente. sin embargo podemos tener una configuracion por default que sea por usuario".
  - L2 decision "Semilla al configurar": the user file holds defaults that the setup flows copy into the project's `.ai-workflow/config.yaml`; at run time only the project counts, so changing a default never changes an already configured repository.
  - L4 decision "Siguen en el usuario": provider keys declared `scope: user` (personal, e.g. `assignee_id`, `billable`, `ssh_key_file`, `ssh_auth`, `infisical_domain`) keep being read from the user file at run time.
  - Everything else under `tasks`, `release` and `deploy` (provider choice, `scope: repo` keys, the sections themselves) is read from the project only.
- **S3** Names: "a las extensiones no lo llames task@nerv, etc. simplemente llamalo task".
  - Every user-facing string (CLI output, prompts, help, docs, plugin markdown) names extensions and providers by bare name (`task`, `release`, `deploy`, `task-teamwork`, ...).
  - The Claude Code identifiers (`name@nerv` in `enabledPlugins`, `claude plugin install/uninstall` arguments, `installed_plugins.json` lookups) stay as they are.

## Tasks

| ID | Specs | Route | Status | Commit |
|---|---|---|---|---|
| T1 | S1 | delegated (parallel units, worktree) | verifier: no blockers, advisories 1-3 fixed inline (stable tag validated, channel wording); suite green; native review granted, approved and acknowledged (lineage review-6a5dc3098c7a74b2; 7 non-blocking advisories: no fsync before rename, no rollback after replace, timeouts, test notes) | 46948b4 on feat/nerv-upgrade |
| T2 | S2 | delegated (parallel units, worktree) | verifier: no blockers; advisory fixed inline (removed provider dropped before moving to defaults); native review granted, approved, acknowledged (lineage review-65998e5d4fedbf5b; 11 non-blocking advisories) | f656c7f, bd9c14d, afeb101, b6c8194 on feat/project-scoped-extension-config |
| T3 | S3 | delegated (parallel units, worktree) | done (tests green, spot check ok) | ade0b6d on refactor/bare-extension-names |

Route evidence: three independent units of behavior, each several files and 100+ lines, overlapping only in shared docs (`README.md`, `docs/commands.md`), so each runs in its own worktree.

## Log

- **L1** (2026-10-25, user): "hagamos algunos cambios:
  * agregame un comando de upgrade que chequee la funeta y descargue el ultimo binario y lo ejecute. que incluyan los flags de rc, beta y alpha
  * la configuracion de task, deploy, release es por proyecto unicamente. sin embargo podemos tener una configuracion por default que sea por usuario
  * a las extensiones no lo llames task@nerv, etc. simplemente llamalo task"
- **L2** (2026-10-25, user answer): user default mode = "Semilla al configurar (Recommended)".
- **L3** (2026-10-25, user answer on the beta channel, which no workflow publishes): "no le pongas el flag beta".
- **L4** (2026-10-25, user answer on personal `scope: user` provider keys): "Siguen en el usuario (Recommended)".
- **L6** (2026-10-25, user): "si" to pushing the three branches and opening one PR each to develop: #154 (upgrade), #155 (config scope), #156 (names).
- **L7** (2026-10-25, user): "mergea los prs". #154 merged as 8c9c7e0 (v5.1.0-alpha.1), #155 as 1446f0d (alpha.2), #156 after resolving a docs/configuration.md conflict with #155 (73ea202, CI green) as 75e6e63 (alpha.3).
- **L5** Exploration: no Go code talks to GitHub; `scripts/install.sh` holds the asset and checksum logic; `releaseconfig.Load`/`deployconfig.Load` read only the project; `tasks.*` is merged user+project by the `task` skill §1 and `cmd/nerv/doctor.go`; `name@nerv` is built in `internal/install` as `ext.Name+"@"+MarketplaceName`.
