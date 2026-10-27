# Teamwork: `logTime`

This implements the `logTime` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*`, read as the task skill's §1 says (the project file, plus the `scope: user` keys of the user file). Report every failure with the exact error text, and never retry.

Call `twprojects-create_timelog`:
- `task_id`;
- `date` and `time` from the block's real start. `time` must be `HH:MM:SS`: the API rejects `HH:MM`;
- `hours` and `minutes` from `minutes` (`minutes` below 60);
- `billable` from the caller, default `billable`;
- `description`. Teamwork strips ` #NN` tokens, so rewrite any `#NN` as `PR NN` before sending.

Return `{timelogRef}`, the new timelog id.
