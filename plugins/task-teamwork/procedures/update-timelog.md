# Teamwork: `updateTimelog`

This implements the `updateTimelog` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*`, read as the task skill's §1 says (the project file, plus the `scope: user` keys of the user file). Report every failure with the exact error text, and never retry.

Call `twprojects-update_timelog` with the timelog `id`, the new `hours` and `minutes`, and the `description`, with `#NN` rewritten as `PR NN`.

The start time stays where it is: when the caller needs a different start, it calls `logTime` for the replacement and `deleteTimelog` for the old timelog (skill §4.2 step 5).
