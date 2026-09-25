---
description: Move a task to the Testing stage
argument-hint: <taskId> [tasklistId | tasklist name] [projectId | project name]
---

<!-- Embedded NERV procedure for the Teamwork adapter (../../teamwork.md). Also installable verbatim as the /task:testing slash command in ~/.claude/commands/task/testing.md — see "Optional slash commands" in the adapter. -->

Config: read `~/.claude/nerv/nerv.yaml`, merged with `<repo>/.nerv/nerv.yaml` when present (project overrides user); all Teamwork ids, stage names and work sources come from there.

Arguments: `$ARGUMENTS` — first token is the task id; trailing tasklist/project tokens (id or name) are optional context and normally unnecessary (derived from the task via `teamwork_get_task`; resolve names via `teamwork_list_tasklists` / `teamwork_list_projects`). If the task id is missing, ask which task — suggest a default (the task with a running timer in `tasks.timer_store`, else the task most recently discussed, else the defaults from `tasks.sources`) — then STOP and wait.

1. Verify the task exists and is open via `teamwork_get_task`; if not, report it and stop.
2. Match stages by name CONTAINING the `testing` stage substring from `tasks.providers.teamwork.stages` (case- and accent-insensitive — e.g. "Testing" in workflow "Estados"). Resolve workflow and stage ids via `teamwork_list_workflow_stages` and move with `teamwork_move_task_to_stage` (skip if already there). If the project has no workflow or no matching stage, report it and stop.

Timers are untouched. Report: task name, id and the stage move.
