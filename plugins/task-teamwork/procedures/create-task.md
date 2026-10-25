# Teamwork: `createTask`

This implements the `createTask` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*` from the merged configuration. Report every failure with the exact error text, and never retry.

1. **Where.** Pick the tasklist in this order: the caller's `list` (an id, or a name resolved with `teamwork_list_tasklists`), then `tasklist_id`, then `default_tasklist_id`. The project follows the same order (`project_id`, then `default_project_id`).

   Names in `known_projects` resolve as that map says: a superseded project only when it is named explicitly; a retired one only by explicit id.
2. **Create.** `teamwork_create_task` with the name, the description, the priority (default `high` for source rows), and no tags. It is unassigned: the caller assigns it next when it should be.
3. **Missing tasklist.** When no tasklist can be resolved, return `needs_input` asking for one, with the defaults as suggestions.

Return `{taskRef: "TW-<new id>"}`.
