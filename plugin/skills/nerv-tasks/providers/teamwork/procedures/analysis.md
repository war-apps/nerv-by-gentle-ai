---
description: Move a task to the Análisis stage and start a local timer on it
argument-hint: <taskId> [tasklistId | tasklist name] [projectId | project name]
---

<!-- Embedded NERV procedure for the Teamwork adapter (../../teamwork.md). Also installable verbatim as the /task:analysis slash command in ~/.claude/commands/task/analysis.md — see "Optional slash commands" in the adapter. -->

Config: read `~/.claude/nerv/nerv.yaml`, merged with `<repo>/.nerv/nerv.yaml` when present (project overrides user); all Teamwork ids, stage names and work sources come from there.

Arguments: `$ARGUMENTS` — first token is the task id; trailing tasklist/project tokens (id or name) are optional context and normally unnecessary (derived from the task via `teamwork_get_task`; resolve names via `teamwork_list_tasklists` / `teamwork_list_projects`). If the task id is missing, ask which task — suggest a default (the task with a running timer in `tasks.timer_store`, else the task most recently discussed, else the defaults from `tasks.sources`) — then STOP and wait.

1. Verify the task exists and is open via `teamwork_get_task`; if not, report it and stop.
2. Match stages by name CONTAINING the `analysis` stage substring from `tasks.providers.teamwork.stages` (case- and accent-insensitive — matches "Análisis" in workflow "Estados"). Resolve workflow and stage ids via `teamwork_list_workflow_stages` and move with `teamwork_move_task_to_stage` (skip if already there). If the project has no workflow or no matching stage, report it and continue.
3. **Start a local timer** for this task exactly as `./start.md` does (steps 1–3 of its "Then" section: store `tasks.timer_store`, per (taskId, sessionId), `startedAt = now` with UTC offset, `projectId` from `tasklist.meta.projectId`, atomic rewrite, never `teamwork_start_timer`). If this session already has a timer for the task, report its elapsed time instead of duplicating it. No worktree offer and no assignment here — that is `./start.md`.

Report: task name, id, the stage move and the running timer.
