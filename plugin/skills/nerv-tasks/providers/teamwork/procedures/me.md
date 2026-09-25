---
description: List my pending tasks from all work sources (only assigned to me), with running timers and tracked time
---

<!-- Embedded NERV procedure for the Teamwork adapter (../../teamwork.md). Also installable verbatim as the /task:me slash command in ~/.claude/commands/task/me.md — see "Optional slash commands" in the adapter. -->

Config: read `~/.claude/nerv/nerv.yaml`, merged with `<repo>/.nerv/nerv.yaml` when present (project overrides user); all Teamwork ids, stage names and work sources come from there.

Read `tasks.sources` and list pending tasks from every enabled source — ONLY tasks assigned to me (for unassigned ones too, use `./all.md`).

For the `teamwork` source:
- Call `teamwork_list_tasks` with `assigned_to_me=true`.
- Optional filters parsed from `$ARGUMENTS`: a project name resolves to `project_id` via `teamwork_list_projects`; date expressions map to `due_before`/`due_after`; free text maps to `search_term`.
- Project name disambiguation: the default project is `tasks.providers.teamwork.default_project_id`. "gestion web" (any casing/accents) means the superseded project — only when named explicitly. The legacy project is retired — resolve to it only if the user gives that id explicitly (both in `tasks.providers.teamwork.known_projects`).

For EVERY other enabled source, follow the reading instructions in its `tasks.sources` entry — reading all enabled sources is MANDATORY, not best-effort. Dedupe across sources: a sheet row whose Teamwork-link column (J) already points to a task is the SAME item as that task — show it once, as the Teamwork task (id, stage, times), keeping the sheet as Origen only if the task row itself is not listed. If a source's primary mechanism is unavailable (e.g. the google-sheets MCP failed to connect), use the fallback documented in its `tasks.sources` entry (e.g. one-shot script with the service-account credentials) before giving up; if it still cannot be read, add a clearly visible warning line under the table naming the source and the error — NEVER silently omit a source.

For Teamwork tasks, include the workflow stage: `teamwork_list_tasks` returns `stageId` per task (absent = not on the board). Resolve stage names with ONE `teamwork_list_workflow_stages` call per project and render the stage name; show "—" for unstaged tasks. Also include the tasklist name: tasks carry `tasklistId` — resolve names with ONE `teamwork_list_tasklists` call per project; "—" for sheet-only rows.

## Timers and tracked time

Read the local timer store `tasks.timer_store` (shape: `timers` = running timers `{taskId, taskName, projectId, sessionId, startedAt}`; `openLogs` = per-task consolidation `{taskId, projectId, timelogId, blocks: [{start, end}], summaries}`). A missing file means no timers and no open logs.

Add two columns to the table:

- **Current Time**: if one or more running timers exist for the task, show the elapsed time of the OLDEST one as `⏱ Nh Nm` (now − `startedAt`); if more than one session has a timer on the same task, append `×N`. No running timer → `—`.
- **History Time**: total tracked time of the task = minutes logged in Teamwork + elapsed time of its running timers. Get the logged minutes with ONE `teamwork_list_timelogs` call per project (`project_id` filter, `mine=true`), aggregating entry minutes by task id — never one call per task. Round to `Nh Nm` (or `Nm` under an hour); nothing tracked → `—`.

If a running timer's task does not appear in the listing (filtered out, completed, or from another project), add a short note under the table listing it with its elapsed time — a running timer must never be invisible.

Present all results merged, sorted by (in this order):
1. **Assigned to me** first (relevant when a source yields unassigned items).
2. **Priority**: high → medium → low → none.
3. **Stage**, in the order given by `tasks.providers.teamwork.stage_sort_order`. Unstaged tasks ("—") take the Backlog slot. Match stage names by fragment (case- and accent-insensitive); a stage not in the list sorts after Pendiente.

Render the table with EXACTLY these columns, in this order and with these headers: `!` (priority emoji), `ID` (task id), `Own` (`Sí` = assigned to me, `—` = unassigned), `Project` (project name — resolve with ONE `teamwork_list_projects` call when a listed task's project isn't already known), `Task List` (tasklist name), `Task` (task name), `Stage` (workflow stage), `Current Time` (running timer), `History Time` (tracked time), `Source` (the task's source, e.g. `Teamwork` or a configured sheet source name from `tasks.sources`) — the Source column is ALWAYS present, even when only one source yielded tasks. Sheet-only rows show "—" in Project, Task List and Stage.
