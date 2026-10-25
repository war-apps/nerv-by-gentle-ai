# Teamwork: `listTimelogs`

This implements the `listTimelogs` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*`, read as the task skill's §1 says (the project file, plus the `scope: user` keys of the user file). Report every failure with the exact error text, and never retry.

Call `teamwork_list_timelogs`:
- with `task_id`, or with `project_id` and `mine=true` for a listing. Use one call per project, never one per task.

Return rows `{timelogRef, taskRef, start, minutes, description}`.
