---
description: Take a task - assign it to me (creating it in Teamwork if needed) WITHOUT starting timers or moving stages
---

<!-- Embedded NERV procedure for the Teamwork adapter (../../teamwork.md). Also installable verbatim as the /task:take slash command in ~/.claude/commands/task/take.md — see "Optional slash commands" in the adapter. -->

Config: read `~/.claude/nerv/nerv.yaml`, merged with `<repo>/.nerv/nerv.yaml` when present (project overrides user); all Teamwork ids, stage names and work sources come from there.

Arguments: `$ARGUMENTS` — one of:
- A **numeric token** → a Teamwork task id.
- A **source-qualified row** → an external source name (or unambiguous fragment) configured in `tasks.sources`, plus a row number (e.g. `<source> 74`, `<source> fila 74`). Resolve the source via `tasks.sources`.
- A **quoted description** → free text naming a task. Trailing tokens (tasklist/project, id or name) are optional context; resolve names via `teamwork_list_tasklists` / `teamwork_list_projects` ("gestion web" in any casing/accents = the superseded project id in `tasks.providers.teamwork.known_projects`; the legacy one there is retired).

If empty, ask which task to take — suggest the most recently discussed unassigned task if there is one — then STOP and wait.

This command ONLY assigns. It NEVER starts a timer, NEVER moves workflow stages and NEVER offers a worktree — that belongs to `./start.md`. The ONE thing it shares with `./start.md` is the external-source association: when the task comes from a sheet, the row gets fully marked exactly as `./start.md` would mark it (see step 2).

## Resolution

1. **Teamwork task id**: verify it exists and is open via `teamwork_get_task`. If missing or completed, report it and stop. If already assigned to me (the assignee id from `tasks.providers.teamwork.assignee_id`), say so and stop. Otherwise assign via `teamwork_update_task` with `assignee_ids: [<assignee_id>]`, KEEPING any other existing assignees in the list.

2. **External source row**: read that row from the source per its `tasks.sources` entry (primary mechanism, or the documented fallback if unavailable — e.g. one-shot script with the service-account credentials).
   - If the row's Teamwork-link column (e.g. column J) already has a task, treat it as case 1 with that task id.
   - Otherwise create the Teamwork task with `teamwork_create_task`:
     - **name** = the row's task-text column, as documented in that source's `tasks.sources` entry. Trim/shorten sensibly if extremely long, keeping the meaning.
     - tasklist and priority: ask the user BEFORE creating — one grouped question — suggesting the defaults from `tasks.providers.teamwork` (`default_tasklist_id`, priority `high`, `default_project_id`). Pass both to `teamwork_create_task`.
     - `assignee_ids: [<assignee_id>]` — taking implies assigning (this is the explicit exception to the no-auto-assign rule).
     - Apply the source's task-creation rules: description with the link to the sheet row. NO tags.
   - **Associate the sheet row exactly as `./start.md` does**: apply the source's full "start marking" rules from its `tasks.sources` entry — the source's configured marker value (`tasks.sources[].start_marking`) and column layout, as documented in that source's `tasks.sources` entry — using the source's write mechanism or its documented fallback. Skip any marking another session already applied.

3. **Description**: search for an existing OPEN task via `teamwork_list_tasks` with `search_term` (unassigned or mine first; retry without filters if nothing matches).
   - Exactly one match: treat as case 1.
   - Multiple matches: list them with ids and ask which one; STOP and wait.
   - No match: create it with `teamwork_create_task` (name = the description, `assignee_ids: [<assignee_id>]`) in the tasklist from the arguments, or the defaults from `tasks.providers.teamwork` (`default_project_id`, `default_tasklist_id`) — if neither was given and the default feels wrong (e.g. the text clearly names a support issue), ask ONE question suggesting the default, then wait.

Report: task name, task id (say explicitly if it was just created), that it is now assigned to me, and — for a sheet-born task — the source row that was linked. Remind that `./start.md {id}` begins the timer/stage/worktree flow when development actually starts.
