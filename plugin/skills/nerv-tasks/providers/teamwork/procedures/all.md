---
description: List all pending tasks from all work sources (assigned to me AND unassigned), with running timers and tracked time
---

<!-- Embedded NERV procedure for the Teamwork adapter (../../teamwork.md). Also installable verbatim as the /task:all slash command in ~/.claude/commands/task/all.md — see "Optional slash commands" in the adapter. -->

Config: read `~/.claude/nerv/nerv.yaml`, merged with `<repo>/.nerv/nerv.yaml` when present (project overrides user); all Teamwork ids, stage names and work sources come from there.

Read `tasks.sources` and list pending tasks from every enabled source — tasks assigned to me PLUS unassigned tasks. Tasks assigned exclusively to OTHER people are excluded.

For the `teamwork` source, two calls, then merge and dedupe by task id:
1. `teamwork_list_tasks` with `assigned_to_me=true` (my tasks across all projects).
2. `teamwork_list_tasks` with `project_id` = the default project (`tasks.providers.teamwork.default_project_id`) and NO assignee filter; keep only tasks whose `assigneeUserIds` is empty or contains the assignee id (`tasks.providers.teamwork.assignee_id`).

Optional filters parsed from `$ARGUMENTS`: a project name resolves to `project_id` via `teamwork_list_projects` (and replaces the default project in call 2); date expressions map to `due_before`/`due_after`; free text maps to `search_term`. Project name disambiguation: "gestion web" (any casing/accents) means the superseded project — only when named explicitly; the legacy project is retired — only by explicit id (both in `tasks.providers.teamwork.known_projects`).

For EVERY other enabled source, follow the reading instructions in its `tasks.sources` entry — reading all enabled sources is MANDATORY, not best-effort. Dedupe across sources: a sheet row whose Teamwork-link column (J) already points to a task is the SAME item as that task — show it once, as the Teamwork task (id, stage, times), keeping the sheet as Origen only if the task row itself is not listed. If a source's primary mechanism is unavailable (e.g. the google-sheets MCP failed to connect), use the fallback documented in its `tasks.sources` entry (e.g. one-shot script with the service-account credentials) before giving up; if it still cannot be read, add a clearly visible warning line under the table naming the source and the error — NEVER silently omit a source.

For Teamwork tasks, include the workflow stage: `teamwork_list_tasks` returns `stageId` per task (absent = not on the board). Resolve stage names with ONE `teamwork_list_workflow_stages` call per project; show "—" for unstaged tasks. Also include the tasklist name: tasks carry `tasklistId` — resolve names with ONE `teamwork_list_tasklists` call per project; "—" for sheet-only rows.

## Timers and tracked time

Same as `./me.md`: read `tasks.timer_store` and add the **Current Time** (elapsed of the oldest running timer, `×N` if several sessions) and **History Time** (Teamwork minutes via ONE `teamwork_list_timelogs` call per project with `mine=true`, plus running elapsed) columns. A running timer whose task is not in the listing gets a note under the table.

Present all results merged, sorted by (in this order):
1. **Assigned to me** first, unassigned after.
2. **Priority**: high → medium → low → none.
3. **Stage**, in the order given by `tasks.providers.teamwork.stage_sort_order`. Unstaged tasks ("—") take the Backlog slot. Match stage names by fragment (case- and accent-insensitive); a stage not in the list sorts after Pendiente.

Render the table with EXACTLY these columns, in this order and with these headers: `!` (priority emoji), `ID` (task id), `Own` (`Sí` = assigned to me, `—` = unassigned), `Project` (project name — resolve with ONE `teamwork_list_projects` call when a listed task's project isn't already known), `Task List` (tasklist name), `Task` (task name), `Stage` (workflow stage), `Current Time` (running timer), `History Time` (tracked time), `Source` (the task's source, e.g. `Teamwork` or a configured sheet source name from `tasks.sources`) — the Source column is ALWAYS present, even when only one source yielded tasks. Sheet-only rows show "—" in Project, Task List and Stage.
