# Teamwork: `assign`

This implements the `assign` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*` from the merged configuration. Report every failure with the exact error text, and never retry.

1. **Check.** `teamwork_get_task`. When `assignee_id` is already among the assignees, return `{assigned: false}`.
2. **Assign.** Otherwise call `teamwork_update_task` with `assignee_ids` = the current assignees plus `assignee_id`. Never drop an existing assignee.

When `assignee_id` is unset, the result is `blocked` with `tasks.providers.teamwork.assignee_id is not set`.
