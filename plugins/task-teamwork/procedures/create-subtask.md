# Teamwork: `createSubtask`

This implements the `createSubtask` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*` from the merged `nerv.yaml`. Report every failure with the exact error text, and never retry.

1. **Create.** `createTask` in the parent's tasklist (from `teamwork_get_task` on the parent).
2. **Link.** Set the parent with `PATCH /projects/api/v3/tasks/{id}.json` and the body `{"task": {"parentTaskId": <parent id>}}`. The MCP tools cannot set a parent.

Teamwork's local MCP has no tool for this, so it uses REST, per `tasks.providers.teamwork.comment_rest`:
- `https://{domain}.teamwork.com`;
- basic auth with `TEAMWORK_API_KEY` as the user and `x` as the password, both from the `teamwork` server's `env` in `~/.claude.json`.

Never print the key. Check the HTTP status.

When the PATCH fails, return `failed` with the new task's ref in `data`, so the user can link it by hand.
