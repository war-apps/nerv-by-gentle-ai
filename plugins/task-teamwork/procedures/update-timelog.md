# Teamwork: `updateTimelog`

This implements the `updateTimelog` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*` from the merged configuration. Report every failure with the exact error text, and never retry.

Call `teamwork_update_timelog` with the new `hours` and `minutes` and the `description`, with `#NN` rewritten as `PR NN`.

Teamwork cannot move a timelog's start time. When the caller needs a different start, it calls `logTime` for the replacement and `deleteTimelog` for the old timelog (skill §4.2 step 5).
