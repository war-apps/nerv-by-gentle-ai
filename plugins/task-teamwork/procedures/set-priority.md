# Teamwork: `setPriority`

This implements the `setPriority` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*` from the merged configuration. Report every failure with the exact error text, and never retry.

Call `teamwork_update_task` with `priority` set to `high`, `medium` or `low`. Return `{priority}`.
