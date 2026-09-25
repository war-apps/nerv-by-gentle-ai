---
description: Discard a task's local timer WITHOUT logging time (asks for confirmation first)
argument-hint: <taskId>
---

<!-- Embedded NERV procedure for the Teamwork adapter (../../teamwork.md). Also installable verbatim as the /task:clear slash command in ~/.claude/commands/task/clear.md — see "Optional slash commands" in the adapter. -->

Config: read `~/.claude/nerv/nerv.yaml`, merged with `<repo>/.nerv/nerv.yaml` when present (project overrides user); all Teamwork ids, stage names and work sources come from there.

Arguments: `$ARGUMENTS` — the task id. If missing, ask which task — suggest a default (a task with a running timer in `tasks.timer_store`; if several, list them) — then STOP and wait.

The store is `tasks.timer_store` (see `./start.md` for its shape). This command is the "abort" counterpart of `./stop.md`: the timer entry is deleted and NO time is logged in Teamwork.

1. Read the store and find the timers for this task.
   - Prefer THIS session's entry (`taskId` + this session's `sessionId`; a legacy entry without `sessionId` counts).
   - If this session has none but OTHER sessions have timers for the task, list them (session id, started at, elapsed) — they may be clearable too (e.g. a zombie session), but make clear their elapsed time will be lost.
   - If the task has no timers at all, say so and stop.
2. **Confirmation is MANDATORY**: show what will be discarded (task name and id, session, start time, elapsed time — computed with a shell one-liner) and ask the user to confirm. One question (`AskUserQuestion` when available: confirm / cancel; with multiple candidate timers, let the user pick which to clear). STOP and wait. Never clear without explicit confirmation.
3. On confirmation, rewrite the store atomically removing ONLY the confirmed timer entries. `openLogs` is untouched (past consolidated timelogs are history, not part of the timer). Timers of other tasks and unconfirmed sessions stay.

Report: which timer(s) were discarded (task, session, elapsed time NOT logged) and which remain running for the task.
