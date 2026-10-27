# Teamwork: `listTimelogs`

This implements the `listTimelogs` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*`, read as the task skill's §1 says (the project file, plus the `scope: user` keys of the user file). Report every failure with the exact error text, and never retry.

Call `twprojects-list_timelogs`:
- with `task_id`, or with `project_id` and `assigned_user_ids: [assignee_id]` for a listing. Use one call per project, never one per task;
- with `fields: [id, description, minutes, timeLogged, task]`, and `start_date`/`end_date` when the caller gives a window.

Return rows `{timelogRef, taskRef, start, minutes, description}`.
