# Teamwork: `createSubtask`

This implements the `createSubtask` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*`, read as the task skill's §1 says (the project file, plus the `scope: user` keys of the user file). Report every failure with the exact error text, and never retry.

1. **Parent.** `twprojects-get_task` on the parent, with `fields: [tasklist]`: the subtask goes in the parent's tasklist (Teamwork requires it).
2. **Create.** `twprojects-create_task` as `createTask` does, in that tasklist, with `parent_task_id` = the parent's id.

Return `{taskRef: "TW-<new id>"}`.
