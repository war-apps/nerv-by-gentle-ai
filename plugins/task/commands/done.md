---
description: Mark a task as implemented - moves it to implemented, stops this session's timer and consolidates the timelog (does NOT complete the task; use /task:close for that)
argument-hint: <taskRef>
---

Load the `task` skill first: it is the contract every step below refers to (§ numbers). Read the merged configuration (§1). When `tasks.provider` is `none`, say that task tracking is off in this scope and stop.

Arguments: `$ARGUMENTS`.

If the task is missing, ask which one, suggesting the task with this session's running timer (§4), then STOP and wait.

**Provider.** A `PREFIX-id` argument selects the provider whose `task_ref_prefix` matches. A bare id belongs to `tasks.provider`. Find that provider (§2) and run its tool check; when a tool is missing, report it and stop. Run every operation through the provider's procedure file.

Run the `done` composite (§7): `moveStage(implemented)`, then stop this session's timer (§4.2), when one exists. The task stays open.

Report: the task, the stage move and the time logged.
