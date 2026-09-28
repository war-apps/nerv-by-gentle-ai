---
description: Show the local timers of a task (or all running timers), with their session
argument-hint: [taskId]
---

<!-- Embedded NERV procedure for the Teamwork adapter (../../teamwork.md). Also installable verbatim as the /task:timers slash command in ~/.claude/commands/task/timers.md — see "Optional slash commands" in the adapter. -->

Config: read `~/.claude/nerv/nerv.yaml`, merged with `<repo>/.nerv/nerv.yaml` when present (project overrides user); all Teamwork ids, stage names and work sources come from there.

Arguments: `$ARGUMENTS` — optional task id: with it, show only that task's timers; without it, show ALL running timers (no question needed — the no-arg form is the overview).

1. Read the local timer store `tasks.timer_store` (see `./start.md` for its shape). A missing file or empty `timers` array means nothing is running — say so and stop.
2. Compute each timer's elapsed time (now − `startedAt`) with a shell one-liner, never mentally.
3. Render one table: task id, task name, **session** (the `sessionId`; mark THIS session's entries — the UUID of this session's scratchpad path — as `esta sesión`; a legacy entry without sessionId shows `—`), started at (local time), elapsed (`Nh Nm`).
4. If the task (or any listed task) also has an `openLogs` entry, add a short note: the open consolidated timelog id and the total already logged there — helps see what a `./stop.md` run would couple with.

Read-only: this command never modifies the store nor logs time.
