---
description: Start working on a task - local timer, assigns it to me, moves it to the inDev stage, offers a worktree (creates the task if needed)
argument-hint: <taskRef | description> [tasklist] [project]
---

Load the `task` skill first: it is the contract every step below refers to (§ numbers). Read the configuration (§1: the project file, plus the personal keys of the user file). When `tasks.provider` is `none`, say that task tracking is off in this scope and stop.

Arguments: `$ARGUMENTS`.

**Provider.** A `PREFIX-id` argument selects the provider whose `task_ref_prefix` matches. A bare id belongs to `tasks.provider`. Find that provider (§2) and run its tool check; when a tool is missing, report it and stop. Run every operation through the provider's procedure file.

1. **Resolve the task.**
   - **Task ref or id:** `getTask`. When it is missing or closed, report it and stop.
   - **Description:** search the open tasks with `listTasks`, using the description as the search term (mine first, then all).
     - One match: use it.
     - Several matches: list them and stop.
     - No match: create the task. Ask one grouped question for tasklist and project, suggesting the provider defaults, then `createTask`.
     - A row from a `tasks.sources` entry follows that source's task-creation rules.
2. **Assign.** `assign` (starting implies assigning).
3. **Timer.** Start this session's local timer (§4.1):
   - when this session already has one for the task, report its elapsed time and skip;
   - mention other sessions' timers on the task, and any stale timers (§4.3), without blocking.
4. **Source marking.** For a task from a `tasks.sources` entry, apply that source's "in development" marking, unless another session already did.
5. **Stage.** `moveStage(inDev)`.
6. **Worktree.** Run the worktree offer (§9).

Report: the task (say so when it was just created), the assignment, this session's timer, other sessions' timers, and the worktree or branch in use.
