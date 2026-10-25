---
description: Discard a task's local timer WITHOUT logging time (asks for confirmation first)
argument-hint: <taskRef>
---

Load the `task` skill first: it is the contract every step below refers to (§ numbers). Read the configuration (§1: the project file, plus the personal keys of the user file). When `tasks.provider` is `none`, say that task tracking is off in this scope and stop.

Arguments: `$ARGUMENTS`.

If the task is missing, ask which one, suggesting the task with this session's running timer (§4), then STOP and wait.

This is the abort counterpart of `/task:stop`: the timer entry is deleted and **no time is logged**.

1. **Find.** Find this session's timer for the task. When there is none, list the other sessions' timers on it (they may be zombies), with the time that would be lost.
2. **Confirm.** Confirmation is mandatory. Show what will be discarded and ask confirm or cancel, letting the user pick when there are several timers. STOP and wait.
3. **Remove.** Remove only the confirmed entries. `openLogs` is untouched.

Report: what was discarded, and what is still running.
