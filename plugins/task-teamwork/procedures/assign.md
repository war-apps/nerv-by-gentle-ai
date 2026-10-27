# Teamwork: `assign`

This implements the `assign` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*`, read as the task skill's §1 says (the project file, plus the `scope: user` keys of the user file). Report every failure with the exact error text, and never retry.

1. **Check.** `twprojects-get_task` with `fields: [assignees]`. When `assignee_id` is already among the assigned users, return `{assigned: false}`.
2. **Assign.** Otherwise call `twprojects-update_task` with `assignees.user_ids` = the current assigned users plus `assignee_id`, keeping any `company_ids`, `team_ids` and `job_role_ids` it had. Never drop an existing assignee.

When `assignee_id` is unset, the result is `blocked` with `tasks.providers.teamwork.assignee_id is not set`.
