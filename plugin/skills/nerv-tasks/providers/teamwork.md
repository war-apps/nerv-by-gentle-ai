# Teamwork Adapter

Implements the `nerv-tasks` port (`../SKILL.md`) against Teamwork. This
adapter does not duplicate logic: each port op below names the exact
existing procedure file it executes. **The procedure files under
`teamwork/procedures/` are the executable reference** — this
adapter only maps port operations onto them and states which config values
and MCP tools each one uses. When a procedure file and this adapter appear
to disagree, the procedure file wins; report the discrepancy rather than
improvising.

`task_ref_prefix: TW`. A `taskRef` such as `TW-48888495` maps to Teamwork
task id `48888495` (strip the prefix and the dash).

## Config values used, by path

- `tasks.providers.teamwork.assignee_id` (user scope) — the id assigned in
  `start`/`take` and matched against `assigneeUserIds` in `list`.
- `tasks.providers.teamwork.project_id` (project scope) — default project
  for `createTask`/`createSubtask` and the `list` "all" scope.
- `tasks.providers.teamwork.tasklist_id` (project scope) — default tasklist
  for `createTask`.
- `tasks.providers.teamwork.stages` — map `{inDev, testing, implemented,
  blocked, canceled, pending, analysis}` → the case/accent-insensitive
  substring matched against `teamwork_list_workflow_stages` output for
  `moveStage`.
- `tasks.providers.teamwork.task_ref_prefix` — `TW`.
- `tasks.sources` — extra listing sources folded into `list` (the migrated
  successor of `~/.claude/work/sources.md`; see the note in §Deviations).

None of these values are hardcoded in this adapter or in `SKILL.md` — every
reference above is by config path.

## Op → procedure → MCP tools

| Op | Procedure file | MCP tools | Notes |
|---|---|---|---|
| `start(taskRef)` | `teamwork/procedures/start.md` | `teamwork_get_task`, `teamwork_update_task`, `teamwork_list_tasks`, `teamwork_create_task`, `teamwork_list_workflow_stages`, `teamwork_move_task_to_stage` | Local timer store write is part of the procedure, not a separate MCP call |
| `take(taskRef)` | `teamwork/procedures/take.md` | `teamwork_get_task`, `teamwork_update_task`, `teamwork_list_tasks`, `teamwork_create_task` | Never starts a timer or moves a stage |
| `stop(taskRef, summary)` | `teamwork/procedures/stop.md` | `teamwork_log_time`, `teamwork_update_timelog`, `teamwork_list_timelogs`; legacy path: `teamwork_list_timers`, `teamwork_complete_timer`, `teamwork_pause_timer` | Coupled-block union vs. gap-driven fresh timelog, 30-minute rounding, per the procedure |
| `moveStage(taskRef, stageKey)` | inlined in every procedure that needs it (`start.md`, `analysis.md`, `testing.md`, `pending.md`, `blocked.md`, `canceled.md`, `done.md`, `close.md`, all under `teamwork/procedures/`) | `teamwork_list_workflow_stages`, `teamwork_move_task_to_stage` | Stage substring comes from `tasks.providers.teamwork.stages[stageKey]`, not a literal in this adapter |
| `logTime(taskRef, start, end, note)` | `teamwork/procedures/stop.md` (direct-log path) | `teamwork_log_time` | Always real `date`/`start_time` (seconds required), never a bare duration |
| `createTask({title, list, priority, description, source})` | `teamwork/procedures/start.md` (task-creation branch) or `take.md` | `teamwork_create_task` | `list` resolves to `tasklist_id`, defaulting from `tasks.providers.teamwork.tasklist_id`; `source` (a sheet row) applies that source's task-creation rules |
| `createSubtask(parentRef, ...)` | not a `/task:*` command — direct v3 REST | `PATCH /projects/api/v3/tasks/{id}.json` with `parentTaskId` in the body | The MCP `teamwork_create_task`/`teamwork_update_task` tools cannot set a task's parent; this is a known gap (see Deviations) |
| `comment(taskRef, text)` | `teamwork/procedures/blocked.md` / `canceled.md` (comment step) | REST fallback, not an MCP tool: `POST https://{TEAMWORK_DOMAIN}.teamwork.com/tasks/{taskId}/comments.json`, basic auth `TEAMWORK_API_KEY` (user) / `x` (password) from the `teamwork` server env in `~/.claude.json` | The local Teamwork MCP has no comment tool at all |
| `complete(taskRef)` | `teamwork/procedures/close.md` | `teamwork_complete_task` | Reports and does not force completion if Teamwork rejects it (open dependencies) |
| `setPriority(taskRef, level)` | `teamwork/procedures/priority.md` | `teamwork_update_task` | `level` maps to `"high"\|"medium"\|"low"` |
| `list(scope, filters)` | `teamwork/procedures/me.md` (scope: mine) / `all.md` (scope: all) | `teamwork_list_tasks`, `teamwork_list_workflow_stages`, `teamwork_list_tasklists`, `teamwork_list_projects`, `teamwork_list_timelogs` | Includes every entry in `tasks.sources`, per the procedure's dedupe-by-link rule |
| `listTimers(taskRef?)` | `teamwork/procedures/timers.md` | none (local store only) | Read-only, never touches Teamwork |
| `discardTimer(taskRef)` | `teamwork/procedures/clear.md` | none (local store only) | Deletes this session's timer entry WITHOUT logging time; mandatory user confirmation; other sessions' entries are only cleared when the user picks them explicitly |
| `stopAll(summaries)` | `teamwork/procedures/stopAll.md` | `teamwork_log_time` per timer (via `./stop.md`) | Composite: `stop` for every timer THIS session holds, one summary per task; never logs or removes another session's entry |

## Composite ops

`block`, `cancel`, `close`, `done` map directly to
`teamwork/procedures/blocked.md`, `canceled.md`, `close.md`,
`done.md` respectively — each procedure already implements the exact
composition defined in `SKILL.md` §4 (comment + stage move + stop, or stage
move + stop, or stop + stage move + complete).

## Stage resolution mechanics

`moveStage` never matches on a literal stage name. It reads the target
substring for the given `stageKey` from
`tasks.providers.teamwork.stages[stageKey]`, calls
`teamwork_list_workflow_stages` for the task's project, and matches case-
and accent-insensitively by substring against the returned stage names —
exactly as `start.md` step 5 and the other stage-moving procedures already
do (each procedure file's own configured substring matches its Spanish
stage name, including legacy naming variants on older boards). If the
project has no workflow, or no stage name contains the configured
substring, the op reports the miss and continues rather than failing the
whole run — this mirrors every procedure file's existing "report it and
continue" behavior for an unmatched stage.

## Worked example: `start(taskRef)`

1. Hyuga calls `start("TW-48888495")`. This adapter strips the prefix,
   getting Teamwork task id `48888495`.
2. It resolves `teamwork/procedures/start.md` as the procedure
   and runs its "task id given" branch: `teamwork_get_task` to verify the
   task is open; if not already assigned to
   `tasks.providers.teamwork.assignee_id`, `teamwork_update_task` assigns
   it, keeping any existing assignees.
3. It writes the local timer entry per the procedure's step 3 (keyed by
   this task id and the session's `sessionId`); this write never goes
   through an MCP tool.
4. It resolves the `inDev` stage from
   `tasks.providers.teamwork.stages.inDev`, calls
   `teamwork_list_workflow_stages`, and moves the task with
   `teamwork_move_task_to_stage` if it is not already there.
5. It returns `{taskId: 48888495, taskRef: "TW-48888495", stage: "En Desarrollo"}`
   to Hyuga. The worktree/branch offer (`start.md` step 6) is executed by
   Aoba, not by this adapter — this adapter's contribution ends at the
   tracker-side state.

## Deviations from the generic port

- **`comment` has no MCP tool.** It uses the REST fallback the
  `blocked.md`/`canceled.md` procedures already document (`POST
  /tasks/{id}/comments.json`). If the REST call fails, the procedure falls
  back to telling the user to add the comment by hand and continues rather
  than blocking the run.
- **`createSubtask` cannot use the MCP.** The MCP's `teamwork_update_task`
  does not accept a parent-task field; setting `parentTaskId` requires the
  Teamwork v3 REST `PATCH /projects/api/v3/tasks/{id}.json` endpoint
  directly.
- **`list` sources.** `teamwork/procedures/me.md` and `all.md`
  read extra sources from `tasks.sources` in `nerv.yaml` (the Phase 4
  config migration's successor to the old `~/.claude/work/sources.md`);
  this adapter always names `tasks.sources` as the source of truth.

## Optional slash commands

Every procedure under `teamwork/procedures/` can also be
installed verbatim as a `/task:*` slash command in
`~/.claude/commands/task/` — the setup wizard (`nerv configure`)
offers to do this, and it never overwrites a file that already exists
there. When both copies exist, the embedded procedure and the installed
command must stay identical; the plugin copy under
`teamwork/procedures/` is canonical, so a fix always lands there
first and is then re-copied to `~/.claude/commands/task/` if a personal
copy is kept.
