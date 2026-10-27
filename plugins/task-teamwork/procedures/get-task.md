# Teamwork: `getTask`

This implements the `getTask` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*`, read as the task skill's §1 says (the project file, plus the `scope: user` keys of the user file). Report every failure with the exact error text, and never retry.

Call `twprojects-get_task` with `fields: [id, name, priority, status, completedAt, assignees, tasklist, workflowStages]`. Return:
- `taskRef`, `name`;
- `open`: false when it is completed (`status` `completed`, or a `completedAt`);
- `assignees`: the assigned user ids;
- `stage`: the stage name of `workflowStages[].stageId` (from `twprojects-list_workflow_stages` on its `workflowId`), or `—` when it has none;
- `projectId`: the tasklist's project id;
- `tasklistId`, `priority`;
- `url`: `https://{domain}.teamwork.com/app/tasks/<id>`.

A missing task is `failed` with `task <taskRef> not found`.
