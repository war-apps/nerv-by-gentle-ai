---
description: Show the local timers of a task (or all running timers), with their session
argument-hint: [taskRef]
---

Load the `task` skill first: it is the contract every step below refers to (§ numbers). Read the configuration (§1: the project file, plus the personal keys of the user file). When `tasks.provider` is `none`, say that task tracking is off in this scope and stop.

Arguments: `$ARGUMENTS`.

This command is read-only: it never changes the store and never logs time.

1. **Read.** Read the timer store (§4.1). When nothing is running, say so and stop.
2. **Table.** Render one table: task, task name, session (mark this session's entries as `esta sesión`), started at, elapsed `Nh Nm`, and a **stale** mark (§4.3).
3. **Open logs.** When a listed task has an `openLogs` entry, note its timelog and the total already logged there.
