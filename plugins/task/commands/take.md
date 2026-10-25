---
description: Take a task - assign it to me (creating it if needed) WITHOUT starting timers or moving stages
argument-hint: <taskRef | <source> <row> | description>
---

Load the `task` skill first: it is the contract every step below refers to (§ numbers). Read the configuration (§1: the project file, plus the personal keys of the user file). When `tasks.provider` is `none`, say that task tracking is off in this scope and stop.

Arguments: `$ARGUMENTS`.

**Provider.** A `PREFIX-id` argument selects the provider whose `task_ref_prefix` matches. A bare id belongs to `tasks.provider`. Find that provider (§2) and run its tool check; when a tool is missing, report it and stop. Run every operation through the provider's procedure file.

This command only assigns. It never starts a timer, never moves a stage and never offers a worktree.

1. **Resolve the task:**
   - task ref: `getTask`;
   - source row: read it per its `tasks.sources` entry. When the row already links a task, use that task. Otherwise `createTask` with the source's rules (one grouped question for tasklist and priority), then apply the source's full start marking;
   - description: `listTasks` search; on no match, `createTask`.
2. **Assign.** `assign`. When the task is already mine, say so.

Report: the task, whether it was created, and that `/task:start` begins the timer, stage and worktree flow.
