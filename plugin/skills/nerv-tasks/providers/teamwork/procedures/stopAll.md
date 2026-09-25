---
description: Stop tracking on ALL started tasks - stops every timer and logs time as /task:stop would for each
---

<!-- Embedded NERV procedure for the Teamwork adapter (../../teamwork.md). Also installable verbatim as the /task:stopAll slash command in ~/.claude/commands/task/stopAll.md — see "Optional slash commands" in the adapter. -->

Config: read `~/.claude/nerv/nerv.yaml`, merged with `<repo>/.nerv/nerv.yaml` when present (project overrides user); all Teamwork ids, stage names and work sources come from there.

No arguments.

1. Read the local timer store `tasks.timer_store` and collect THIS session's entries only (`sessionId` equal to this session's id; a legacy entry without `sessionId` counts). Entries of other sessions are listed as information (task, session id, elapsed) and are NEVER logged or removed here — that is the per-session invariant every stop/close procedure keeps. Also check `teamwork_list_timers` for legacy Teamwork timers (running or paused) and include them.
2. If there are none, report that there is nothing to stop and stop.
3. Show the list of timers found (task name, task id, elapsed time) before doing anything.
4. For each timelog a description is required: derive a SHORT SUMMARY of what was developed for each task from the session context. For every task whose work you cannot summarize, ask the user for a one-line summary — gather all missing summaries in a single grouped question (`AskUserQuestion` when available), then wait. Never log time with an empty or invented description.
5. Then, for EACH timer, apply the exact per-task procedure from `./stop.md` (local-store steps for local timers, legacy procedure for Teamwork ones; rounding up per `tasks.rounding_minutes` with a one-unit minimum, `billable: false`, summary as description, remove the entry only after the timelog is verified). The result for each task must be identical to having run `./stop.md <taskId>` individually.
6. If one task fails, report the failure for that task and continue with the rest — one broken timer must not leave the others running.

Report a final table: task name, id, raw elapsed time, rounded time logged, and status (ok / failed with reason).
