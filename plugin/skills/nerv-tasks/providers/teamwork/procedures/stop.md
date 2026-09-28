---
description: Stop working on a task and log its time (coupled blocks merge into one timelog; a gap starts a fresh timelog dated at the new block)
argument-hint: <taskId> [tasklistId | tasklist name] [projectId | project name]
---

<!-- Embedded NERV procedure for the Teamwork adapter (../../teamwork.md). Also installable verbatim as the /task:stop slash command in ~/.claude/commands/task/stop.md — see "Optional slash commands" in the adapter. -->

Config: read `~/.claude/nerv/nerv.yaml`, merged with `<repo>/.nerv/nerv.yaml` when present (project overrides user); all Teamwork ids, stage names and work sources come from there.

Arguments: `$ARGUMENTS` — first token is the task id; trailing tasklist/project tokens (id or name) are optional context and normally unnecessary (the task's project comes from the store or `teamwork_get_task`). If the task id is missing, ask which task to stop — suggest a default (the task with this session's running timer in `tasks.timer_store`; if several, list them) — then STOP and wait.

Timers are LOCAL: the store is `tasks.timer_store` (see `./start.md` for its shape). Timers are per (taskId, sessionId); each task tracks its CURRENT open Teamwork timelog in the store's `openLogs` section. Blocks that couple (overlap or touch) merge into that timelog; a GAP between the last block's end and the new block's start closes it and starts a FRESH timelog dated at the new block — so every timelog carries the real date/time of the work it contains. Time is still ALWAYS logged in Teamwork.

1. Find THIS session's entry for the task in the local timer store (`taskId` + this session's `sessionId` — the UUID segment of the scratchpad directory path). A legacy entry without `sessionId` may be adopted by any session. If there is none, check for a LEGACY Teamwork timer (`teamwork_list_timers`) and, if found, use the legacy procedure below. If neither exists, say so and stop — never log time without a timer.
2. This session's work block = `startedAt` → now (parse both as ISO-8601 with offset; compute with a shell one-liner, don't do date math mentally).
3. Build this session's one-line summary of what was actually developed during this block. Derive it from the session context; if you don't know what was done, ask the user for a one-line summary before logging.
4. Log the block, using the `openLogs` entry for this task (rounding unit = `tasks.rounding_minutes`):
   - **No `openLogs` entry (first stop for the task)**: log with `teamwork_log_time` — task id, `billable: false`, `date` and `start_time` from the block's start (`start_time` MUST include seconds, `HH:MM:SS`; the API rejects `HH:MM`), minutes = `ceil(blockMinutes / rounding_minutes) * rounding_minutes`, minimum one unit, description = this session's summary. Then create the `openLogs` entry `{taskId, projectId, timelogId, blocks: [block], summaries: [summary]}` with the returned timelog id.
   - **Existing `openLogs` entry — decide by GAP vs COUPLING** (compare the new block's `start` against the MAX `end` of the entry's blocks):
     - **Coupled (new start <= last end — the ranges overlap or touch)**: append the block to `blocks` and the summary to `summaries`, recompute the total as the **union of merged intervals** over the entry's blocks (overlapping time counts once). Rounded total = `ceil(unionMinutes / rounding_minutes) * rounding_minutes`, minimum one unit — rounding applies to the TOTAL, never per block. Update the existing timelog with `teamwork_update_timelog`: the new rounded total (hours + minutes) and description = the entry's summaries joined with `"; "`. Keep the timelog's original date/start time (earliest block).
     - **Gap (new start > last end)**: the previous timelog is CLOSED as it stands — do NOT touch it. Create a FRESH timelog with `teamwork_log_time` for this block alone (`date`/`start_time` from THIS block's start, minutes = `ceil(blockMinutes / rounding_minutes) * rounding_minutes`, minimum one unit, description = this session's summary), and RESET the `openLogs` entry to the new timelog: `{taskId, projectId, timelogId: <new>, blocks: [this block], summaries: [this summary]}`. This keeps every timelog dated on the day the work actually happened.
5. Verify the timelog exists with the expected duration (`teamwork_list_timelogs` for the task). Only then rewrite the store: remove THIS session's timer entry (leave other sessions' timers and other tasks untouched) and persist the updated `openLogs` entry. If logging/updating failed, keep the timer entry so no time is lost.

Report: task name and id, this block's raw elapsed time, the consolidated union total, and the rounded time actually logged. Mention any other sessions' timers still running for this task.

## Legacy Teamwork timers (pre-local-store only)

For a timer that still lives in Teamwork: complete it with `teamwork_complete_timer`. If it fails with 403 "timer must belong to a project", pause it (`teamwork_pause_timer`), compute elapsed from its `duration_seconds`, log the rounded time with `teamwork_log_time`, and tell the user a paused orphan timer remains to discard from the Teamwork UI. Otherwise reconcile so exactly ONE timelog with the ROUNDED duration exists: list today's timelogs; if completing created one with the raw duration, update it (`teamwork_update_timelog`); if none was created, create it (`teamwork_log_time`); never double-log.
