# Tasks plugin split (core / nerv-tasks / providers)

Locator: `odd/tasks/tasks-plugin-split.md`. Engram mirror topic: `odd/tasks-plugin-split/tasks` (project `nerv-gentle-ai`).
Branch: `feature/tasks-plugin-split`, cut from `develop` at 5f0be02. Delivery: `ask-on-risk`, with chained PRs (forecast well over 400 lines). Test runner: `go test ./...`, plus the prose tests at the repo root.

## Specs

### A. Core / plugin split (user request L1)

- **S1. Core is only the gentle-ai adaptation.** The user asked: "hace que nerv sea puramente la parte de adaptar los agentes de gentle-ai a la estructura de nerv. saca la parte de tareas".
  - Core nerv (Go CLI and the `plugin/` tree) must not contain any task, timer, tracker, Teamwork or provider logic, config keys, wizard prompts, flags, agent tools or docs.
  - That means removing:
    - from Go: `tasks.*` in `internal/config` catalogue/managed/legacy, `internal/configure/installcommands.go`, `paths.CommandsDir`, the wizard's tasks/Teamwork/commands sections, and the `--repo-provider`, `--repo-project-id`, `--repo-tasklist-id`, `--install-commands` and `--skip-commands` flags;
    - from the plugin: `plugin/skills/nerv-tasks/**`, Hyuga's `DISPATCH: tracker` and its `mcp__teamwork__*` tools, the orchestrator's preflight Task group, the `tracker_event` log entry, and the evals under `evals/tracker-*`.
- **S2. Tasks are an optional nerv plugin.** The user asked: "las tareas deben ser un plugin de nerv, sugerir al usuario instalarlo o no."
  - Tasks ship as the plugin `nerv-tasks`, in this repo and the same marketplace, embedded in the binary (user decision on 2026-10-23: "Mismo repo").
  - `nerv install` and `nerv configure` ask whether to install it. Answering no leaves core fully working.
- **S3. Tasks consume providers.** The user asked: "las tareas deben consumir proveedores, cada proveedor debe brindar la configuracion necesaria para su consumicion desde las tareas."
  - Each provider is its own plugin and ships a manifest, `provider.yaml`, with:
    - `name` and `contract_version`;
    - the MCP server and the exact tool names it needs;
    - its config schema (keys, types, defaults, prompts, user or repo scope);
    - its canonical stage map;
    - the operations it implements;
    - the events it subscribes to.
  - `nerv-tasks` renders the provider's config prompts and validates the manifest. No provider name is hard-coded in `nerv-tasks` or in core.
- **S4. Tasks fire events that providers handle.** The user asked: "las tareas debe disparar eventos que el proveedor o los proveedores de tareas deben capturar, ejecutar y responder."
  - Core's orchestrator emits named events at its existing hook points: `run.started`, `verification.started`, `audit.ranked`, `issue.deferred`, `wave.started`, `run.closed` and `run.halted`.
  - `nerv-tasks` dispatches each event to every installed and enabled provider subscribed to it, then returns one response per provider with this shape: `{event, provider, result: ok|skipped|blocked|failed, task_ref?, data, error?}`.
  - When `nerv-tasks` is not installed, core emits nothing and logs nothing.
- **S5. Installing tasks suggests providers.** The user asked: "si se instala el plugin de tareas, esta debe sugerir instalar algunos proveedores, por ahora debe sugerir instalar teamwork que es el que usamos actualmente, con el tiempo se podria agregar mas respetando un contrato en comun".
  - After `nerv-tasks` is installed, the installer offers the known providers. Today that is only `nerv-tasks-teamwork`.
  - A new provider is added by shipping a plugin with a valid `provider.yaml`. It needs no change to core or to `nerv-tasks` code.
- **S6. Multi-plugin distribution.** `nerv install`, `nerv uninstall`, the materializer and the version check handle N plugins from one marketplace: `nerv` (required), `nerv-tasks` and providers (optional). Today `nerv@nerv` is hard-wired at `internal/install/install.go:19` and `internal/plugin/materialize.go:290`.
- **S7. Migration.** An existing `tasks:` block in `~/.claude/nerv/nerv.yaml` or `.nerv/nerv.yaml` keeps working:
  - with `nerv-tasks` installed, it is read under the new layout (`tasks.providers.<name>.*`, so Teamwork keys keep their paths);
  - without it, core ignores the block and does not delete it;
  - copies previously installed into `~/.claude/commands/task/` are reported, and removed only after the user confirms.

### B. Learnings from consuming projects (user request L2)

Source: `~/projects/docs/nerv-learnings-from-projects-2026-10-23.md`. The numbers in brackets are that report's spec inputs, which cite their evidence.

- **S8. Provider tool check** [1]. At `run.started`, `nerv-tasks` checks that the dispatching agent really has the tools the manifest declares. If one is missing, it returns `blocked` with the missing names, and Teamwork's are named exactly.
- **S9. Fail-soft and replayable** [2, 3].
  - A dispatch never retries and never halts the pipeline.
  - Every `blocked`, `skipped` or `failed` operation goes into a pending-ops list per change. The run summary shows that list, and a command can replay it later.
- **S10. Per-event switches** [4]. Each event can be turned on or off per repo and per run, for example stage moves left manual.
- **S11. Single source for procedures** [5]. Provider procedures are read from the installed plugin from any consuming repo or worktree, never from a copy that can drift.
- **S12. Event payload** [6]. Every event carries `task_ref`, `repo`, `change`, `branch` and `worktree`.
- **S13. `run.closed` never completes the task** [7]. Completing the task stays an explicit user action.
- **S14. Canonical stages** [8]. Events use the canonical stage keys, and the provider config maps each one to the provider's name for it.
- **S15. Timers** [9, 11].
  - The local timer store stays keyed by (task, session).
  - Stale timers are closed at the last real activity (from the transcript and git log), with every cut-off confirmed in one grouped question.
  - Rounding and the billable default are provider config.
- **S16. Teamwork timelog quirks** [10]. The provider writes "PR 34" rather than ` #NN` in timelog descriptions. When a timelog's start has to move, it creates the replacement before deleting the old one.
- **S17. Worktrees from tasks** [12]. The worktree offer of `/task:start` uses the configured path strategy (default, herdr, custom). The skill registry, the KB repo key and the dependencies are resolved from the main checkout.
- **S18. Silent when tasks are off** [13]. With no tasks plugin, or no enabled provider, there are no tracker steps, questions or log entries.
- **S19. Engram flush at close** [14]. When sub-agent `mem_save` calls were rejected, the orchestrator flushes the pending KB mirrors itself at close.
- **S20. Launch discipline** [15, 16, 17].
  - Never launch two agents at once in one worktree when one of them is a gate.
  - Freezes and verbatim writes stay with the agent that owns them.
  - When an agent returns text instead of writing, the orchestrator appends it.
  - Artifact paths in briefs are absolute and scoped to the change.
  - Audit launches always include `nerv/audit/commits-round-N.txt`.
- **S21. Archive robustness** [18, 19].
  - When a requirement heading is renamed, the spec delta produces a RENAMED block.
  - LIGHT runs have an explicit close/archive step.
  - The orchestrator lock carries a heartbeat, so stale locks can be detected.
- **S22. Artifacts that git ignores** [20, 21].
  - The commit policy supports repos that git-ignore `.nerv/` and `openspec/`: the artifacts stay local and there is no archive commit.
  - `.orchestrator.lock` is never staged.
  - The RDD assess integration excludes NERV runtime folders without asking the user to edit `.git/info/exclude`.
- **S23. Telemetry** [22]. Run telemetry records the model, tokens in and out, and the duration of each launch.
- **S24. Release presets** [24, 25].
  - A shared-host dotnet profile (an SDK-pin hook or a no-build-server env) and a submodule-init hook are offered, instead of copying them into each stage.
  - `coverage_threshold` supports line, branch and method floors.
- Assumption, not in scope unless confirmed: [23] (a cheaper path for audit follow-ups) is an inference in the report and stays out.

## Tasks

| ID | Specs | Route | Status | Commit |
|---|---|---|---|---|
| T1 | S6 | inline | pending | — |
| T2 | S3, S5, S8, S12, S14 | inline | Go part done (RED: build failed; GREEN: go test ./internal/taskprovider ok). Prose contract goes with T3 | see L7 |
| T3 | S2, S4, S9, S10, S11, S13, S15, S17, S18 | inline | pending | — |
| T4 | S3, S14, S15, S16 | inline | pending | — |
| T5 | S1, S7 | inline | pending | — |
| T6 | S19, S20, S21, S22, S23 | inline | pending | — |
| T7 | S24 | inline | pending | — |

- **T1:** multi-plugin embed, materialize, install, uninstall and version check, plus the suggestion prompts. Risk item 5 (installer).
- **T2:** the provider contract (the `provider.yaml` schema, the event list, the response envelope, `contract_version`) and its Go validator.
- **T3:** the `nerv-tasks` plugin, covering the dispatcher agent (which takes over Hyuga's tracker), the generic `/task:*` commands, timers and pending-ops replay.
- **T4:** the `nerv-tasks-teamwork` provider, covering its manifest, the procedures moved over, the stage map and the timelog quirks.
- **T5:** removal from core, with the orchestrator emitting events instead, the config migration, and updates to docs, evals and tests. Risk items 1 and 3 (persisted config and CLI flags).
- **T6:** the core pipeline learnings.
- **T7:** the release preset learnings.

The order is T2 → T1 → T3/T4 → T5 → T6 → T7. Every task runs inline: the units depend on each other, so no Writer-trigger reason applies.

## Log

- **L1** (2026-10-23, user, verbatim):
  > necesitamos separar algunas cosas.
  > * hace que nerv sea puramente la parte de adaptar los agentes de gentle-ai a la estructura de nerv. saca la parte de tareas
  > * las tareas deben ser un plugin de nerv, sugerir al usuario instalarlo o no.
  > * las tareas deben consumir proveedores, cada proveedor debe brindar la configuracion necesaria para su consumicion desde las tareas.
  > * las tareas debe disparar eventos que el proveedor o los proveedores de tareas deben capturar, ejecutar y responder.
  > * si se instala el plugin de tareas, esta debe sugerir instalar algunos proveedores, por ahora debe sugerir instalar teamwork que es el que usamos actualmente, con el tiempo se podria agregar mas respetando un contrato en comun
- **L2** (2026-10-23, user, verbatim):
  > agrega los cambios que aprendimos en los diferentes proyectos que implemente nerv, esa es informacion valiosa para esta nueva version
- **L3** (2026-10-23): the user chose "Mismo repo (Recommended)" for where the plugins live.
- **L4** (2026-10-23): mapping evidence. Distribution is single-plugin. The Go config is hard-wired to Teamwork. The tasks contract already exists as prose in `plugin/skills/nerv-tasks/SKILL.md`. Hyuga's tracker is its only caller.
- **L5** (2026-10-23): the learnings report was written to `~/projects/docs/nerv-learnings-from-projects-2026-10-23.md`. Two repos had real runs: dim.comprobantes.api and dim-factura-electronica-service. Its key finding is that no tracker update ever reached Teamwork from the pipeline, because the tool names did not match.
- **L6** (2026-10-23): the user chose "Plugin llamado task (Recommended)". The tasks plugin is registered as `task` (`task@nerv`), so Claude Code serves `/task:start` and the rest natively, with no copies. The working names "nerv-tasks" and "nerv-tasks-teamwork" in these specs stand for the plugins `task` and `task-teamwork`.
- **Next step:** T2, the provider contract.
- **L7** (2026-10-23): T2 contract design.
  - A provider implements remote primitives only: `getTask, assign, moveStage, logTime, updateTimelog, deleteTimelog, listTimelogs, createTask, createSubtask, comment, complete, setPriority, listTasks`.
  - The task plugin owns timers, timelog consolidation and the composite ops (block, cancel, close, done, stopAll).
  - `Events` maps each event to the ops it needs. A manifest that subscribes to an event whose op is `not_implemented` is invalid. `run.closed` needs `moveStage` and `logTime` only, never `complete` (S13).
  - `MissingTools` accepts a bare tool name or `mcp__<server>__<tool>`, never a same-named tool from another server (S8).
  - Code is in `internal/taskprovider`.
