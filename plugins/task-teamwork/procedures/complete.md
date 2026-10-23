# Teamwork: `complete`

This implements the `complete` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*` from the merged `nerv.yaml`. Report every failure with the exact error text, and never retry.

Call `teamwork_complete_task`. When Teamwork refuses (open dependencies), return `blocked` with the dependencies it names. Never force it.
