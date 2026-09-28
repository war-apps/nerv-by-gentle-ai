---
description: Mark a task as implemented - moves it to Implementado, stops this session's timer and consolidates the timelog (does NOT complete the task; use /task:close for that)
argument-hint: <taskId> [tasklistId | tasklist name] [projectId | project name]
---

<!-- Embedded NERV procedure for the Teamwork adapter (../../teamwork.md). Also installable verbatim as the /task:done slash command in ~/.claude/commands/task/done.md — see "Optional slash commands" in the adapter. -->

Config: read `~/.claude/nerv/nerv.yaml`, merged with `<repo>/.nerv/nerv.yaml` when present (project overrides user); all Teamwork ids, stage names and work sources come from there.

Arguments: `$ARGUMENTS` — first token is the task id; trailing tasklist/project tokens (id or name) are optional context and normally unnecessary (derived from the task). If the task id is missing, ask which task — suggest a default (the task with this session's running timer in `tasks.timer_store`) — then STOP and wait.

1. Verify the task exists and is open via `teamwork_get_task`; if not, report it and stop.
2. **Workflow stage**: match stages by name CONTAINING the `implemented` stage substring from `tasks.providers.teamwork.stages` (case- and accent-insensitive — matches "Implementado" in workflow "Estados"). Resolve workflow and stage ids via `teamwork_list_workflow_stages` and move with `teamwork_move_task_to_stage` (skip if already there). If the project has no workflow or no matching stage, report it and continue.
3. If THIS session has a timer for the task in `tasks.timer_store` (`taskId` + this session's `sessionId`; a legacy entry counts), execute the full `./stop.md` procedure (union of blocks, rounding per `tasks.rounding_minutes`, non-billable, verified before removing the timer). If there is no timer for this session, say so and skip the logging — never invent time.
4. The task stays OPEN in Teamwork — completion is `./close.md`. Other sessions' timers are untouched (each one stops its own).

Report: task name and id, the stage move, and the time logged (if any).
