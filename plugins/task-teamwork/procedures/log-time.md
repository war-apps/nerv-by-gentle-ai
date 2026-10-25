# Teamwork: `logTime`

This implements the `logTime` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*`, read as the task skill's §1 says (the project file, plus the `scope: user` keys of the user file). Report every failure with the exact error text, and never retry.

Call `teamwork_log_time`:
- `task_id`;
- `date` and `start_time` from the block's real start. `start_time` must be `HH:MM:SS`: the API rejects `HH:MM`;
- `hours` and `minutes` from `minutes`;
- `billable` from the caller, default `billable`;
- `description`. Teamwork strips ` #NN` tokens, so rewrite any `#NN` as `PR NN` before sending.

Return `{timelogRef}`, the new timelog id.
