# Teamwork: `getTask`

This implements the `getTask` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*` from the merged configuration. Report every failure with the exact error text, and never retry.

Call `teamwork_get_task`. Return:
- `taskRef`, `name`;
- `open`: false when it is completed;
- `assignees`: `assigneeUserIds`;
- `stage`: the stage name of `workflowStages[].stageId`, or `—` when it is 0;
- `projectId`: `tasklist.meta.projectId`;
- `tasklistId`, `priority`;
- `url`: `https://{domain}.teamwork.com/app/tasks/<id>`.

A missing task is `failed` with `task <taskRef> not found`.
