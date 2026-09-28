---
description: Mark a task as blocked - asks the mandatory cause, saves it as a task comment, moves it to Bloqueado, stops this session's timer and logs the time
argument-hint: <taskId> [tasklistId | tasklist name] [projectId | project name]
---

<!-- Embedded NERV procedure for the Teamwork adapter (../../teamwork.md). Also installable verbatim as the /task:blocked slash command in ~/.claude/commands/task/blocked.md — see "Optional slash commands" in the adapter. -->

Config: read `~/.claude/nerv/nerv.yaml`, merged with `<repo>/.nerv/nerv.yaml` when present (project overrides user); all Teamwork ids, stage names and work sources come from there.

Arguments: `$ARGUMENTS` — first token is the task id; trailing tasklist/project tokens (id or name) are optional context and normally unnecessary (derived from the task via `teamwork_get_task`). If the task id is missing, ask which task — suggest a default (the task with this session's running timer in `tasks.timer_store`) — then STOP and wait.

1. Verify the task exists and is open via `teamwork_get_task`; if not, report it and stop.
2. **Cause is MANDATORY**: ask the user for the blocking cause (unless they already stated it in this request). One question, then STOP and wait. Never proceed with an empty or invented cause.
3. **Save the cause as a task comment.** The local teamwork MCP has no comment tool; use REST per `tasks.providers.teamwork.comment_rest` (path `/tasks/{taskId}/comments.json` against the domain) with body `{"comment": {"body": "<comment_labels.blocked>: <cause>"}}` (label from `tasks.providers.teamwork.comment_labels`) — auth per `comment_rest.auth` (basic auth, `TEAMWORK_API_KEY` as user, `x` as password; in PowerShell parse `~/.claude.json` with `ConvertFrom-Json -AsHashtable`). Verify the response; if it fails, tell the user the comment must be added by hand and still continue.
4. Match stages by name CONTAINING the `blocked` stage substring from `tasks.providers.teamwork.stages` (case- and accent-insensitive — matches "Bloqueado" in workflow "Estados"). Resolve workflow and stage ids via `teamwork_list_workflow_stages` and move with `teamwork_move_task_to_stage` (skip if already there). If the project has no workflow or no matching stage, report it and continue.
5. If THIS session has a timer for the task in `tasks.timer_store` (`taskId` + this session's `sessionId`; a legacy entry counts), execute the full `./stop.md` procedure (coupled blocks merge into one timelog, a gap starts a fresh timelog dated at the new block; rounding per `tasks.rounding_minutes`, non-billable, verified before removing the timer). Mention the block cause in the summary if no better summary exists. If there is no timer for this session, say so and skip the logging.

Report: task name, id, the stage move, the comment posted (or the manual fallback), and the time logged (if any). Other sessions' timers are untouched.
