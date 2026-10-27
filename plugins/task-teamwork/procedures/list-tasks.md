# Teamwork: `listTasks`

This implements the `listTasks` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*`, read as the task skill's §1 says (the project file, plus the `scope: user` keys of the user file). Report every failure with the exact error text, and never retry.

1. **Fetch.**
   - `mine`: `twprojects-list_tasks` with `assignee_user_ids: [assignee_id]`.
   - `all`: that call, plus `twprojects-list_tasks` on the default project (`project_id` or `default_project_id`) with `only_unassigned: true`. Merge them and dedupe by id.
   - Ask for `fields: [id, name, priority, tasklist, assignees, workflowStages]`, and page with `page_size` up to 500.
2. **Filters.** A project name resolves through `twprojects-list_projects` (`search_term`), and `known_projects` applies. Date expressions become `due_before` and `due_after`, or `date_filter` (`overdue`, `today`, `thisweek`, `upcoming`). Free text becomes `search_term`.
3. **Names.** Resolve the stage names with one `twprojects-list_workflow_stages` call per workflow, the tasklist names with one `twprojects-list_tasklists` call per project, and the project names with one `twprojects-list_projects` call.

When `assignee_id` is unset, the result is `blocked` with `tasks.providers.teamwork.assignee_id is not set`.

Return rows `{taskRef, name, own, project, tasklist, stage, priority, source: Teamwork}`. The listing table itself is built by the task extension (skill §8).
