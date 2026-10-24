# Teamwork: `comment`

This implements the `comment` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*` from the merged `nerv.yaml`. Report every failure with the exact error text, and never retry.

Post the comment with `POST /tasks/{taskId}/comments.json` and the body `{"comment": {"body": "<text>"}}`.

Teamwork's local MCP has no tool for this, so it uses REST, per `tasks.providers.teamwork.comment_rest`:
- `https://{domain}.teamwork.com`;
- basic auth with `TEAMWORK_API_KEY` as the user and `x` as the password, both from the `teamwork` server's `env` in `~/.claude.json`.

Never print the key. Check the HTTP status.
