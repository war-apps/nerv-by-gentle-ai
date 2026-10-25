# Extensions and providers

Objective: rename ship to release, call the plugins "extensions" and their implementations "providers", and make every extension talk to the core through one contract and every provider talk to its extension through one contract, all configured from `.ai-workflow.yml`.
Branch: `feat/extensions-providers` (from develop c74e6d3). Delivery: ask-on-risk, chain strategy feature-branch-chain (user, L5). Test runner: `go test ./...`.

## Specs

- **S1 Rename ship.** "vamos a cambiarle el nombre de ship -> release". Covers the extension, its CLI command, config section, slash commands, agents, skill, artifact schema and docs. Collisions to resolve: the hidden maintainer `nerv release` command, `internal/release`, and the legacy 4.2.0 `release:` section that `nerv migrate` reads today.
- **S2 Terminology.** "vamos a cambiar el nombre a la arquitectura, no van a ser mas plugins sino ahora se van a llamar extensiones. y a las implementaciones como proveedores."
- **S3 Task providers.** "si bien task ahora solo tiene teamwork como proveedor en el futuro podria llegar a tener jira, github projects, trello, etc." Teamwork stays the only provider built now; the contract must fit those future ones.
- **S4 Release providers.** "el nuevo release va a tener como proveedores a docker compose o bash por ej, el motor de versiones tambien va a tener proveedores y como default va a tener preinstalado el actual motor que se llamara conventional commits"
- **S5 Deploy providers.** "deploy es otra extension que va a tener como proveedor github, ssh, bash, etc."
- **S6 One contract per level.** "todos estos proveedores y extensiones deben comunicarse con el core a traves de un mismo contrato: las extensiones deben tener el mismo y los proveedores de cada extension tambien solo que estos ultimos ese contrato es entre la extension y el proveedor."
- **S8 Bundled providers.** "hagamos que los providers ya definidos de las extensiones este instaladas por defecto, excepto los de task". This is declared per extension in extension.yaml: release and deploy are bundled; task keeps suggesting its providers.
- **S9 `.ai-workflow/` folder.** "creemos la carpeta .ai-workflow y ahi dejemos el config de nerv como config.yaml, extension.yaml y provider.yaml. saquemos .ai-workflow.yml y aputnemos a config.yaml". Layout, the user's choice of "Una carpeta por cada uno (Recommended)":
  ```
  .ai-workflow/
    config.yaml
    extensions/<name>/extension.yaml
    providers/<extension>-<name>/provider.yaml (+ scripts, procedures)
  ```
  - Project extensions and providers are validated with the same contract and added to the ones nerv ships.
  - `.ai-workflow.yml` is removed; `nerv migrate` moves it.
- **S7 Configuration.** "todos deben poder ser configurados desde .ai-workflow.yml"

## Tasks

Order: T4 first (it touches every other surface), then T1, T2, T3, T5, T6, T7, T8, T9.

| ID | Specs | Task | Route | Status | Commit |
|---|---|---|---|---|---|
| T1 | S6 | Shared contract package: manifest loader, config schema, request/response envelope, statuses | delegated with T2-T3 (context backstop) | done; verifier PASS; review approved | 8f859d5 |
| T2 | S6, S2 | `extension.yaml` for task, release, deploy; discovery and requirements read from it instead of hard-coded maps | delegated with T1 | done; verifier PASS; review approved | 4840c1d |
| T3 | S6, S3, S5 | One `provider.yaml` shape for all providers; port task-teamwork, deploy-github, deploy-ssh-compose | delegated with T1 | done; verifier PASS; review approved | 4840c1d |
| T4 | S1 | Rename ship to release (code, CLI, config, extension, artifact schema, docs) | delegated (context backstop: ~95 files, keeps the parent thin for T1-T9) | done; verifier PASS | 277d5df |
| T5 | S4, S8 | Release slots: version (conventional-commits, preinstalled default) and build (docker-compose, bash) as providers; bundled/suggested install | delegated (context backstop) | done; verifier FAIL (B1, B2), corrected and rechecked (L19) | dda6006 + fix |
| T6 | S5 | Deploy bash target provider; deploy moves onto the shared `extension.Exec` runner (from T10) | delegated (parallel with T11) | done; verifier FAIL (B-A1) fixed inline (L21); review approved | e2e74cc, 805eb07 |
| T7 | S7 | Every extension and provider configurable from the project config | done with T5 and T11 (L20) | done | dda6006, c045ed3 |
| T8 | S1, S7 | `nerv migrate` from 4.2.0 and from 5.0.0-alpha.1 (`ship:` refused and migrated) | delegated (parallel with T9, T10) | done; verifier pending | see L26 |
| T10 | S6 | Follow-ups (delegated, parallel with T8, T9) from L11: help text, status validation, requires cycle, preflight cwd/stderr, toolRe dup, docs wording, deploy onto the shared extension.Exec runner and its `{build_mode}` variable renamed | delegated | done | 67c85f2 |
| T11 | S9 | `.ai-workflow/config.yaml` replaces `.ai-workflow.yml`; project extensions and providers from `.ai-workflow/extensions` and `.ai-workflow/providers` | delegated (parallel with T6) | done; verifier PASS | c045ed3 |
| T9 | S2 | Docs, install output and marketplace say extensions/providers; fix the stale repo marketplace.json; leftover old config paths (L18) | delegated (parallel with T8, T10) | done | 964fef2 |

## Log

- **L1 (2026-10-24, user request):** "unos cambios: * vamos a cambiarle el nombre de ship -> release * vamos a cambiar el nombre a la arquitectura, no van a ser mas plugins sino ahora se van a llamar extensiones. y a las implementaciones como proveedores. * si bien task ahora solo tiene teamwork como proveedor en el futuro podria llegar a tener jira, github projects, trello, etc. * el nuevo release va a tener como proveedores a docker compose o bash por ej, el motor de versiones tambien va a tener proveedores y como default va a tener preinstalado el actual motor que se llamara conventional commits * deploy es otra extension que va a tener como proveedor github, ssh, bash, etc. todos estos proveedores y extensiones deben comunicarse con el core a traves de un mismo contrato: las extensiones deben tener el mismo y los proveedores de cada extension tambien solo que estos ultimos ese contrato es entre la extension y el proveedor. todos deben poder ser configurados desde .ai-workflow.yml"
- **L2 (map):**
  - Task providers are run by the `task:dispatcher` agent through markdown procedures and MCP tools (`plugins/task/agents/dispatcher.md:27`).
  - Deploy providers are run by Go as shell-free argv steps with JSON on stdin and stdout (`internal/deployprovider/contract.go:73-102`, `internal/deployrun/run.go:54`).
  - The two packages share nothing and duplicate the config schema and `ContractVersion`.
  - Build modes are a hard-coded switch (`internal/shipplan/plan.go:157,280`), while `shipplan.VersionSource` is already an interface seam (`internal/shipplan/version.go:39`).
  - Requirements and suggestion order are fixed maps (`internal/plugin/extensions.go:16-35`).
  - `ssh-compose` is special-cased (`internal/deployrun/run.go:397`) through a hidden `nerv deploy-ssh-compose` command.
  - Rename size: about 54 non-test and 41 test Go files.
  - Already published in v5.0.0-alpha.1: `nerv ship`, `ship:`, `ship@nerv`, `/ship:*`, `nerv.ship-artifact/v1`.
- **L3 (2026-10-24, user decision):** asked about the provider execution model under one contract; the user chose "Un contrato, dos runtimes (Recommended)":
  - every provider uses the same `provider.yaml` and the same request/response envelope (`ok|skipped|blocked|failed|needs_input`);
  - each provider declares `runtime: exec` (Go runs a command with JSON) or `runtime: agent` (an agent follows procedures with MCP);
  - native providers (conventional-commits, docker-compose, ssh) are exec providers that call nerv itself, with no special cases.
- **L4 (parent decisions, defaults):**
  - **Alpha names:** v5.0.0-alpha.1 has no binaries and no adopted repo, so the rename is a clean break with no aliases. `nerv migrate` reads 4.2.0 and alpha.1 files.
  - **Telling the two `release:` sections apart:** migrate recognizes a 4.2.0 `release:` section by its legacy keys.
  - **Maintainer command:** the hidden `nerv release preview|apply|guard`, used by `.github/workflows/release.yml:57,148,228`, moves to `nerv maintainer-release`. This frees `nerv release` for the extension. The workflow is updated in the same commit (Risk item 5).
  - **Folder names:** `<extension>-<provider>`.
  - **Slots:** release has `version` (default `conventional-commits`, installed with release) and `build` (`docker-compose`, `bash`); deploy has `host` (`github`) and `target` (`ssh-compose`, `bash`); task has `provider` (`teamwork`, several active at once).
  - **Config:** slot selection is `<extension>.<slot>`, provider config is `<extension>.providers.<provider>.<key>`, and deploy keeps `targets[]`.
- **L5 (2026-10-24, user decision):** asked how to deliver the chain; the user chose "feature-branch-chain (Recommended)".
- **L6 (design for T1-T3, parent; assumptions, open to change):**
  - **Two manifests, one schema package** (`internal/extension`): a shared `ConfigKey` schema (`key`, `scope` user|repo, `type`, `default`, `prompt`), name rules, `contract_version`, and the envelope:
    - Request: `{contract, extension, slot, provider, operation, event?, config{}, payload{}}`.
    - Response: `{status: ok|skipped|blocked|failed|needs_input, message, data{}}`.
  - **`extension.yaml` (core <-> extension)**, at the extension root:
    - `name`, `contract_version`, `requires` (extensions);
    - `runtime` agent|exec, plus the entry: the agent name, or the argv;
    - `events` (core events it subscribes to; task: the 7 pipeline events);
    - `config` (keys under `<extension>.<key>`);
    - `slots[]`: `{name, cardinality: one|many|per-target, default, operations{name: required|optional}}`.
    - It replaces the fixed maps in `internal/plugin/extensions.go:16-35`.
  - **`provider.yaml` (extension <-> provider)**, for every provider:
    - `name`, `extension`, `slot`, `contract_version`, `runtime` exec|agent;
    - exec: `requires` (executables); agent: `mcp{server,tools}`;
    - `config` (keys under `<extension>.providers.<name>.<key>`);
    - `operations{op: exec steps | agent procedure .md | not_implemented}`.
    - It is validated against the slot's operations from the owning `extension.yaml`.
    - It replaces both today's `provider.yaml` (task) and `deploy-provider.yaml`.
  - **Native providers** are exec providers whose argv calls a hidden `nerv provider <extension>-<name> <operation>`. This removes the `ssh-compose` special case.
- **L7 (T4 evidence):**
  - The writer reported gofmt, vet, build and `go test ./...` clean, and the parent re-ran build and test as a spot check.
  - `tests/install-sh.test.sh` fails with exit 127 at the base too, so it is environmental.
  - Commit 277d5df is 127 files and about 4.8k changed lines, almost all a mechanical rename. This slice is `size:exception`.
  - `gentle-ai review assess` returned high, review due, escalated on item 5.
  - The independent verifier was launched before the native review.
- **L8 (T4 verifier):**
  - PASS, with no blockers. S1, the item 3 maintainer command (byte-identical output) and the item 5 workflow all hold.
  - Advisory 1: an alpha.1 `ship:` section is silently ignored by `version next` and `migrate`. Taken into T8, which refuses `ship:` with a `nerv migrate` hint and migrates it.
  - Advisory 2 (out of domain): a 4.2.0 `release:` that uses only `stages` or `hooks` is not detected as legacy.
- **L9 (T1-T3 evidence):**
  - The writer reported gofmt, vet, build, `go test ./...` and both hook tests clean, and the parent re-ran the suite.
  - Differential against 277d5df: install order, `install --help`, the usage text, 4 `deploy --dry-run` cases and 9 configure paths are identical. `deploy --help` changed only for the new preflight.
  - Design additions accepted by the parent:
    - an optional `preflight` operation on the deploy `target` slot replaces the ssh-compose special case;
    - `provider_fields` in extension.yaml lets an extension add its own fields (task: `task_ref_prefix`, `stages`, `events`);
    - the hidden `nerv provider <extension>-<name> <op>` replaces `nerv deploy-ssh-compose`.
  - Risk: item 3 (provider manifest, deploy step JSON shape) and item 5 (install discovery refuses invalid trees).
- **L10 (native review, T4):**
  - The user granted review of 277d5df. The first START was refused because the concurrent writer had left untracked files.
  - It was rerun in a detached worktree at 277d5df with the same target identity. The native review stopped there with `lens_context_budget_exceeded` (terminal; the 9.4k-line rename is above the lens budget).
  - Outcome: unavailable (budget). The independent verifier's PASS (L8) stands.
  - The commits holding T1-T3 are 8f859d5 (T1) and 4840c1d (T2+T3). Both were assessed high, review due; their verifier was launched before review.
  - T5 (with the release part of T7) was delegated for the context backstop.
- **L11 (T1-T3 verifier and native reviews):**
  - Verifier: PASS, no blockers.
  - 8f859d5: granted, approved and acknowledged (lineage review-01664f936d1f5cc8). Advisories: contract.go:136-143 (ParseResponse accepts an unknown status as ok), manifest.go:112, manifest.go:67-74, provider.go:73-75, extensiontest.go:14-31.
  - 4840c1d: granted, 4 lenses, approved and acknowledged. Advisories:
    - the garbled `deploy --help` preflight sentence (cmd/nerv/deploy.go:41-42);
    - preflight stderr is silenced (deployprovider/run.go:190-192);
    - `toolRe` is duplicated (run.go:215);
    - deployprovider/manifest.go:11 does not import the extension package;
    - preflight runs in the user's repo (deployrun/run.go:403-405);
    - preflight status coverage;
    - discovery blast radius (plugin/extensions.go:85-88).
  - Verifier advisories:
    - a `requires` cycle would recurse in selectExtensions;
    - task's event-to-operation rule is not enforced at discovery;
    - an unknown response status counts as ok;
    - the docs wording on `default`.
  - All of these are follow-ups in T10.
- **L12 (2026-10-24, user correction):** "hagamos que los providers ya definidos de las extensiones este instaladas por defecto, excepto los de task". S8 added and forwarded to the running T5 writer, as an extension.yaml field (bundled or suggested) with no extension names hard-coded in Go.
- **L13 (T5 evidence):**
  - The writer reported gofmt, vet, build, `go test ./...` and the hook tests clean, and the parent re-ran the suite.
  - Slots: `version` (one, default conventional-commits; ops next and branch required, validate and import optional) and `build` (one, per-project override; plan required, validate optional).
  - Config moved to `release.version`, `release.build`, `release.projects[].build` and `release.providers.<p>.*`. Old keys are refused by name.
  - The artifact field `build_mode` is now `build_provider`.
  - extension.yaml gains `providers: bundled|suggested`.
  - Differential against 4840c1d: identical apart from the plan-header label and the moved keys.
  - Gap: deploy still has its own exec runner; added to T10.
  - Risk items 3 and 5.
- **L14 (2026-10-24, user decision):** asked what "fusiona extension.yaml y provider.yaml en .ai-workflow.yml" means. The user answered: "hagamnos esto si te parece, creemos la carpeta .ai-workflow y ahi dejemos el config de nerv como config.yaml, extension.yaml y provider.yaml. saquemos .ai-workflow.yml y aputnemos a config.yaml"
- **L15 (2026-10-24, user decision):** for the layout inside `.ai-workflow/`, the user chose "Una carpeta por cada uno (Recommended)", so S9 was added.
  - Parent defaults: the user config file becomes `config.yaml` (from `config.yml`) for consistency, and migrate renames it.
  - A project extension or provider whose name collides with a shipped one is refused.
  - Project exec providers run project scripts, like today's script stages, so Risk item 2 applies and gets a verifier.
- **L16 (T5 verifier): FAIL, 2 blockers.** The correction batch runs after T6 and T11 land, because their surfaces overlap.
  - **B1:** `nerv version next` no longer validates the build part of the config. An unknown build provider (`build: maven`), docker-compose without a file, and unknown provider keys now exit 0. On base they exit 1, and `/release:setup` step 6 relies on `version next` to check what it wrote.
    - Repro: `release:\n  build: maven\n  projects: [{ name: app, path: . }]`, then `nerv version next --offline` gives exit 0.
  - **B2:** `nerv install --help` (cmd/nerv/install.go:30-31) still says the deploy providers are offered once deploy is installed, and does not mention bundled providers.
  - Advisories:
    - an artifact that still has `build_mode` is accepted and the field is silently ignored;
    - "build mode" wording remains in plugins/release/commands/build.md:46 and agents/builder.md:38;
    - only the first pass's type errors are reported.
- **L17 (T6 evidence):**
  - deploy-bash is a native exec provider (`deploy.RunBashOp`, internal/deploy/bash.go) with config keys `script` (required), `publish_script` and `verify_script` (optional).
    - Preflight checks the scripts in the user's repo.
    - publish, deploy and verify run the script from the release commit, extracted with git archive, with the envelope on stdin and `NERV_DEPLOY_*` in the environment.
  - Deploy now uses the shared `extension.Exec`. `{build_mode}` is now `{build_provider}`. Preflight stderr is surfaced; `extension.ValidTool` is shared; `Exec` refuses an unknown status.
  - Differential: 7 deploy cases identical.
  - Pending parent edits, once T11 lands (both are in T11's surfaces):
    - add the `deploy-bash` entry to `nativeProviders` in cmd/nerv/deploy.go;
    - update the fixtures in cmd/nerv/install_test.go:321, cmd/nerv/deploy_test.go:86 and internal/plugin/families_test.go:21,50,73;
    - fix `build_mode` in plugins/deploy/skills/deploy/SKILL.md:64;
    - optionally make `ParseResponse` refuse unknown statuses.
  - Risk: items 2 and 3.
- **L18 (T11 evidence):**
  - The writer reported gofmt, vet, build, `go test ./...` (36 packages) and the hook tests (19 and 17 passed) clean. The parent re-ran the full suite after wiring deploy-bash; e2e74cc also builds and tests clean on its own.
  - What changed:
    - the paths are now `.ai-workflow/config.yaml` and the user `config.yaml`;
    - a leftover `.ai-workflow.yml` or `config.yml` is refused with the migrate hint;
    - migrate moves both files;
    - `extension.WithProject` merges project manifests under the same contract, with `{provider_dir}` and folder-escape checks and collisions refused.
  - Differential against dda6006 with the config at the old path: identical apart from the temp clone names and the migrate help text.
  - Leftover mentions of the old paths are in plugins/release-conventional-commits/README.md, bench/README.md, bench/journeys.md, CONTRIBUTING.md and three code comments. They are now part of T9.
  - Risk: items 1, 2 and 3.
- **L19 (T5 correction batch, inline):**
  - **B1 fixed:** a shared `checkBuilds` in cmd/nerv/release.go is used by both `nerv release` (build provider required) and `nerv version next` (projects without a build provider are skipped).
    - RED was observed in `TestVersionNext_RefusesAnInvalidBuildProvider` (exit 2 with the fixture engine called) before GREEN.
    - Recheck on the built binary: `build: maven` gives exit 1 with `release.build: unknown build provider "maven" (want bash, docker-compose)`.
  - **B2 fixed:** `install --help` now describes the bundled providers; the test is updated.
  - "build mode" wording in the release build command and the builder agent became "build provider", and docs/commands.md lists deploy-bash.
  - The full suite passes.
- **L20 (dda6006 review, T7 scope):**
  - The user granted review of dda6006. The native START stopped with `lens_context_budget_exceeded` (4.8k lines), so the outcome is unavailable (budget). The verifier (L16) plus the correction and recheck (L19) stand.
  - T7: every extension and provider is now configurable from `.ai-workflow/config.yaml`: `tasks.providers.*`, `release.*` with `release.providers.*`, and `deploy.*` with `deploy.providers.*`.
  - The `tasks:` key is kept rather than renamed to `task:`. Renaming it was not requested, and 4.2.0 repos already use it.
- **L21 (T6/T11 verifier):**
  - **Unit B (c045ed3): PASS** on every aspect and named risk.
  - **Unit A (e2e74cc): FAIL, one blocker (B-A1).** Preflight checked only the working tree, so a symlinked or uncommitted script passed it and then failed after the tag push.
    - Fixed inline in the parent's correction commit: preflight also checks the script in the artifact's `base_commit` when the repo has it.
    - RED: `TestRunBashOp_PreflightChecksTheScriptInTheBaseCommit` returned ok for both cases. GREEN after the fix; the deploy and deployrun suites pass.
  - Advisories:
    - project exec providers run working-tree `{provider_dir}` scripts, as documented;
    - `configure --help` still says "embedded provider.yaml" for `--repo-provider-config`;
    - a bare `{provider_dir}` element is accepted and fails only when run;
    - non-JSON stdout with exit 0 counts as ok, as documented.
- **L22 (native reviews of e2e74cc and c045ed3):** the user granted both.
  - **e2e74cc:** approved and acknowledged (3 lenses). Advisories to follow up:
    - `git archive` honours `export-ignore`, so a script marked that way is missing (bash.go:233-236, 276);
    - no timeout on the deploy script (bash.go:238-239);
    - the full tree is extracted for each operation (bash.go:270-276);
    - the `build_mode` payload field is removed (deployprovider/contract.go:73);
    - Exec refusing unknown statuses changes behaviour for every exec provider (exec.go:58-60);
    - the preflight working-tree check, already fixed in 805eb07.
  - **c045ed3:** the native START stopped with `lens_context_budget_exceeded` (2.6k lines), so the outcome is unavailable (budget). The verifier's PASS (L21) stands.
- **L23 (follow-ups queued after T10):**
  - deploy-bash should extract the release tree with export-ignore disabled (or with `git checkout-index` into the temp dir) and get an optional `timeout` config key;
  - `configure --help` should say "provider.yaml (shipped or project)";
  - a bare `{provider_dir}` argv element should be refused at load.
- **L24 (T9 evidence):**
  - Terminology sweep across docs, prose, bench and plugin.json descriptions. The repo marketplace.json now lists all 11 plugins, pinned by `TestRepoMarketplace_ListsEveryShippedPlugin` (RED, then GREEN).
  - Old config paths are fixed. `go test ./...` passes on 964fef2 in a clean worktree.
  - Leftovers:
    - the `cmd/nerv/install.go` help still says "extension plugins" (parent);
    - comments in internal/install and internal/plugin (sent to T10);
    - migrate messages that say "task plugin" (sent to T8);
    - the `plugin/evals/task-plugin-absent-no-events` case.yaml and scaffold.sh text;
    - comments and printed text in internal/wizard/providers.go:105, assets.go, internal/configure, internal/releaseplan, internal/taskprovider and internal/deployrun, plus the core plugin.json (parent, final pass).
  - Risk: item 3 (renamed doc anchors, and the `/nerv:status` printed line is now "Task extension").
- **L25 (T10 evidence):**
  - Discovery refuses an unknown `requires` or a `requires` cycle. One `CheckStatus` serves `Exec` and `ParseResponse`/`DecodeResponse`.
  - Slot `events` (event to needed operations) lives in extension.yaml, and provider `events` is a common field checked against it, so task's event rule runs at discovery.
  - The `deploy --help` sentence is rewritten, and docs/extensions.md is updated by the parent.
  - RED, then GREEN, for items 1-4. Items 5 and 6 confirmed existing behaviour.
  - `go test ./...` passes on 67c85f2 in a clean worktree.
  - Risk: item 3 (extension.yaml slot `events`, three printed messages) and item 5 (discovery refuses bad requires).
- **L26 (T8 evidence):**
  - `ship:` is refused by `version next`, `release` and `release setup --import`, with a `nerv migrate` hint, exit 1 and the file untouched.
  - migrate converts alpha.1 `ship:` (in `.ai-workflow.yml` or in `config.yaml`) and the old-shape `release:` through one shared mapping (`releaseconfig.MoveKeys`, the data behind the moved-key refusals).
  - A `nerv.ship-artifact/v1` artifact is refused with "rebuild it with nerv release". docs/migration.md has a "From 5.0.0-alpha.1" section.
  - Differential: the 4.2.0 fixtures produce identical migrated files; stdout differs only in the reworded note.
  - Departures from the brief:
    - project-level `tag_format` and `changelog` stay on the project, because they are project keys today;
    - an old-shape `release:` is also converted.
  - Risk: item 1 (rewrites stored config) and item 3.
