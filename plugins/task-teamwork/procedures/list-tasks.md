# Teamwork: `listTasks`

This implements the `listTasks` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*` from the merged configuration. Report every failure with the exact error text, and never retry.

1. **Fetch.**
   - `mine`: `teamwork_list_tasks` with `assigned_to_me=true`.
   - `all`: that call, plus `teamwork_list_tasks` on the default project (`project_id` or `default_project_id`) with no assignee filter, keeping only the unassigned tasks and those that include `assignee_id`. Merge them and dedupe by id.
2. **Filters.** A project name resolves through `teamwork_list_projects`, and `known_projects` applies. Date expressions become `due_before` and `due_after`. Free text becomes `search_term`.
3. **Names.** Resolve the stage names with one `teamwork_list_workflow_stages` call per project, the tasklist names with one `teamwork_list_tasklists` call per project, and the project names with one `teamwork_list_projects` call.

Return rows `{taskRef, name, own, project, tasklist, stage, priority, source: Teamwork}`. The listing table itself is built by the task extension (skill §8).
