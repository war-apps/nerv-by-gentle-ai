# Teamwork: `comment`

This implements the `comment` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*`, read as the task skill's §1 says (the project file, plus the `scope: user` keys of the user file). Report every failure with the exact error text, and never retry.

Call `twprojects-create_comment` with `object: {type: tasks, id: <id>}`, the text as `body` and `content_type: TEXT`. Leave `notify` out: the default notifies the task's followers.
