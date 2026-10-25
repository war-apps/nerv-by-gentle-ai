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
- **S7 Configuration.** "todos deben poder ser configurados desde .ai-workflow.yml"

## Tasks

Order: T4 first (it touches every other surface), then T1, T2, T3, T5, T6, T7, T8, T9.

| ID | Specs | Task | Route | Status | Commit |
|---|---|---|---|---|---|
| T1 | S6 | Shared contract package: manifest loader, config schema, request/response envelope, statuses | inline | pending | |
| T2 | S6, S2 | `extension.yaml` for task, release, deploy; discovery and requirements read from it instead of hard-coded maps | inline | pending | |
| T3 | S6, S3, S5 | One `provider.yaml` shape for all providers; port task-teamwork, deploy-github, deploy-ssh-compose | inline | pending | |
| T4 | S1 | Rename ship to release (code, CLI, config, extension, artifact schema, docs) | delegated (context backstop: ~95 files, keeps the parent thin for T1-T9) | in progress | |
| T5 | S4 | Release slots: version (conventional-commits, preinstalled default) and build (docker-compose, bash) as providers | inline | pending | |
| T6 | S5 | Deploy bash target provider | inline | pending | |
| T7 | S7 | Uniform `.ai-workflow.yml` keys for every extension and provider | inline | pending | |
| T8 | S1, S7 | `nerv migrate` from 4.2.0 and from 5.0.0-alpha.1 | inline | pending | |
| T9 | S2 | Docs, install output and marketplace say extensions/providers; fix the stale repo marketplace.json | inline | pending | |

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
