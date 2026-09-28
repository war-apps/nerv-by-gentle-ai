---
description: Close a task - stop this session's timer, consolidate the timelog, move it to Implementado and complete it in Teamwork
argument-hint: <taskId> [tasklistId | tasklist name] [projectId | project name]
---

<!-- Embedded NERV procedure for the Teamwork adapter (../../teamwork.md). Also installable verbatim as the /task:close slash command in ~/.claude/commands/task/close.md — see "Optional slash commands" in the adapter. -->

Config: read `~/.claude/nerv/nerv.yaml`, merged with `<repo>/.nerv/nerv.yaml` when present (project overrides user); all Teamwork ids, stage names and work sources come from there.

Arguments: `$ARGUMENTS` — first token is the task id; trailing tasklist/project tokens (id or name) are optional context and normally unnecessary (derived from the task). If the task id is missing, ask which task to close — suggest a default (the task with this session's running timer in `tasks.timer_store`) — then STOP and wait.

1. If THIS session has a timer for the task — an entry in `tasks.timer_store` matching `taskId` + this session's `sessionId` (a legacy entry without `sessionId` counts), or a legacy Teamwork timer — execute the full `./stop.md` procedure first (consolidated timelog: union of blocks, rounded per `tasks.rounding_minutes`, non-billable, summaries joined per session). EXCEPTION: if this session's block is already covered (a previous `./stop.md` run from this session already consolidated it — check the `openLogs` entry and `teamwork_list_timelogs`), do NOT log again; just clean up the leftover timer entry. If there is no timer at all for this session, warn the user that no additional time will be logged and continue.
2. If OTHER sessions still have running timers for this task, report them and ask whether to close the task anyway. Their time is not lost either way — each session stops its own timer with `./stop.md`, which updates the task's consolidated timelog even after the task is completed — but prefer stopping them first. NEVER log or remove another session's timer from here: this session cannot know what was developed there.
3. **Workflow stage**: move the task to the implemented stage BEFORE closing it. Match stages by name CONTAINING the `implemented` stage substring from `tasks.providers.teamwork.stages` (case- and accent-insensitive — matches "Implementado" in workflow "Estados" and the legacy label in `stage_variants`). Resolve workflow and stage ids via `teamwork_list_workflow_stages` and move with `teamwork_move_task_to_stage` (skip if already there). If the project has no workflow or no matching stage, report it and continue.
4. Close the task with `teamwork_complete_task`. If Teamwork rejects it because of open dependencies, report which dependencies block it and do not force anything.
5. After closing, remove the task's `openLogs` entry from the store ONLY if no timers remain for the task (any session); otherwise leave it so a later `./stop.md` run can still update the consolidated timelog.
6. If the task belongs to an external source (check `tasks.sources`), apply that source's "developed" marking rule.

Report: task name and id, time logged (if any), and confirmation that the task is closed.
