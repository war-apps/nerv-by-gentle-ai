# Teamwork: `deleteTimelog`

This implements the `deleteTimelog` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*`, read as the task skill's §1 says (the project file, plus the `scope: user` keys of the user file). Report every failure with the exact error text, and never retry.

Delete the timelog with `DELETE /projects/api/v3/time/{timelogId}.json`.

Teamwork's local MCP has no tool for this, so it uses REST, per `tasks.providers.teamwork.comment_rest`:
- `https://{domain}.teamwork.com`;
- basic auth with `TEAMWORK_API_KEY` as the user and `x` as the password, both from the `teamwork` server's `env` in `~/.claude.json`.

Never print the key. Check the HTTP status.

The caller has already checked that the replacement exists. When it has not, refuse with `failed`: never delete a timelog that has no checked replacement.
