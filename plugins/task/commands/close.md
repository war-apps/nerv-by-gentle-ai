---
description: Close a task - stop this session's timer, consolidate the timelog, move it to implemented and complete it
argument-hint: <taskRef>
---

Load the `task` skill first: it is the contract every step below refers to (§ numbers). Read the merged configuration (§1). When `tasks.provider` is `none`, say that task tracking is off in this scope and stop.

Arguments: `$ARGUMENTS`.

If the task is missing, ask which one, suggesting the task with this session's running timer (§4), then STOP and wait.

**Provider.** A `PREFIX-id` argument selects the provider whose `task_ref_prefix` matches. A bare id belongs to `tasks.provider`. Find that provider (§2) and run its tool check; when a tool is missing, report it and stop. Run every operation through the provider's procedure file.

1. **Stop the timer.** Stop this session's timer for the task (§4.2). When this session's block was already logged, only remove the leftover entry. With no timer, warn that no more time will be logged.
2. **Other sessions.** When other sessions still have timers on the task, list them and ask whether to close anyway. Never stop them from here.
3. **Stage.** `moveStage(implemented)`.
4. **Complete.** `complete`. When the provider refuses (for example, open dependencies), report the refusal and do not force it.
5. **Clean up.** Remove the task's `openLogs` entry only when no session holds a timer for it.
6. **Source marking.** For a task from a `tasks.sources` entry, apply that source's "developed" marking.

Report: the task, the time logged, and that it is completed.
