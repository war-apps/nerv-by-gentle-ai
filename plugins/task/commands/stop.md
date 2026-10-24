---
description: Stop working on a task and log its time (coupled blocks merge into one timelog; a gap starts a fresh timelog dated at the new block)
argument-hint: <taskRef>
---

Load the `task` skill first: it is the contract every step below refers to (§ numbers). Read the merged configuration (§1). When `tasks.provider` is `none`, say that task tracking is off in this scope and stop.

Arguments: `$ARGUMENTS`.

If the task is missing, ask which one, suggesting the task with this session's running timer (§4), then STOP and wait.

**Provider.** A `PREFIX-id` argument selects the provider whose `task_ref_prefix` matches. A bare id belongs to `tasks.provider`. Find that provider (§2) and run its tool check; when a tool is missing, report it and stop. Run every operation through the provider's procedure file.

1. **Find the timer.** Find this session's timer for the task (§4.1). A legacy entry counts. With none, say so and stop: never log time without a timer.
2. **Stale timer.** When the timer is stale, propose the last-activity cut-off (§4.3) and wait for the user to confirm it.
3. **Stop.** Stop it (§4.2).

Report: this block's raw time, the consolidated total, the rounded time logged, and other sessions' timers on the task.
