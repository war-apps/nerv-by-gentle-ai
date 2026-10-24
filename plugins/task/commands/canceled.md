---
description: Cancel a task - asks the mandatory cause, saves it as a task comment, moves it to canceled, stops this session's timer and logs the time
argument-hint: <taskRef>
---

Load the `task` skill first: it is the contract every step below refers to (§ numbers). Read the merged configuration (§1). When `tasks.provider` is `none`, say that task tracking is off in this scope and stop.

Arguments: `$ARGUMENTS`.

If the task is missing, ask which one, suggesting the task with this session's running timer (§4), then STOP and wait.

**Provider.** A `PREFIX-id` argument selects the provider whose `task_ref_prefix` matches. A bare id belongs to `tasks.provider`. Find that provider (§2) and run its tool check; when a tool is missing, report it and stop. Run every operation through the provider's procedure file.

1. **Cause.** The cause is mandatory. Ask for it unless the user already gave it, then STOP and wait. Never invent one.
2. **Cancel.** Run the `cancel` composite (§7). When `comment` fails, tell the user to add the comment by hand, and go on. The task stays open; complete it only when the user asks.

Report: the task, the stage move, the comment (or the manual fallback) and the time logged.
