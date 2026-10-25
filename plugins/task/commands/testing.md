---
description: Move a task to the testing stage
argument-hint: <taskRef>
---

Load the `task` skill first: it is the contract every step below refers to (§ numbers). Read the configuration (§1: the project file, plus the personal keys of the user file). When `tasks.provider` is `none`, say that task tracking is off in this scope and stop.

Arguments: `$ARGUMENTS`.

If the task is missing, ask which one, suggesting the task with this session's running timer (§4), then STOP and wait.

**Provider.** A `PREFIX-id` argument selects the provider whose `task_ref_prefix` matches. A bare id belongs to `tasks.provider`. Find that provider (§2) and run its tool check; when a tool is missing, report it and stop. Run every operation through the provider's procedure file.

1. **Check.** `getTask`. When the task is missing or closed, stop.
2. **Stage.** `moveStage(testing)`.

Timers are untouched. Report: the task and the stage move.
