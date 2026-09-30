---
description: Start working on a task - local timer, assigns it to me, moves it to En Desarrollo, offers a worktree (creates the task in Teamwork if needed)
argument-hint: <taskId | description> [tasklistId | tasklist name] [projectId | project name]
---

<!-- Embedded NERV procedure for the Teamwork adapter (../../teamwork.md). Also installable verbatim as the /task:start slash command in ~/.claude/commands/task/start.md — see "Optional slash commands" in the adapter. -->

Config: read `~/.claude/nerv/nerv.yaml`, merged with `<repo>/.nerv/nerv.yaml` when present (project overrides user); all Teamwork ids, stage names and work sources come from there.

Arguments: `$ARGUMENTS`

Parse them as follows:
- If the first token is numeric, it is a **task id**; otherwise the input starts with a **description** (e.g. an observation coming from a Google Sheet).
- Trailing tokens (after the id or the quoted description) are optional **tasklist** and **project**, each an id or a name — resolve names via `teamwork_list_tasklists` / `teamwork_list_projects` (project name "gestion web" in any casing/accents = the superseded project id in `tasks.providers.teamwork.known_projects`; the legacy one there is retired).
- One of task id or description is REQUIRED. If empty, ask which task to start — suggest a default (a task with a running timer, or the task most recently discussed) — then STOP and wait.

Resolution:
1. **Task id given**: verify it exists and is open via `teamwork_get_task`. If it does not exist or is completed, report it and stop. If the task is not assigned to me (the assignee id from `tasks.providers.teamwork.assignee_id`), assign it now via `teamwork_update_task` (`assignee_ids: [<assignee_id>]`, keeping any other existing assignees) — tasks are created unassigned and get assigned on start.
2. **Description given**: search for an existing open task matching it via `teamwork_list_tasks` with `search_term` (assigned_to_me first; retry without the filter if nothing matches).
   - Exactly one match: use it (and assign it to me as in step 1 if needed).
   - Multiple matches: show them with ids and ask which one; stop.
   - No match: the task must be created. Use the tasklist/project from the arguments; if none were given, ask the user, suggesting the defaults from `tasks.providers.teamwork` (`default_project_id`, `default_tasklist_id`) — one grouped question, then wait. Create the task with `teamwork_create_task` (name = the description, assigned to me — starting implies assigning) in that tasklist. If the task comes from an external source (sheet row), apply that source's task-creation rules from `tasks.sources` instead: sheet link in the description, NO tags, and ask tasklist (default `default_tasklist_id`) and priority (default `high`) — one grouped question — passing both to `teamwork_create_task`.

Then:
1. Timers are LOCAL, not Teamwork timers (Teamwork allows only one running timer per user; local timers allow tracking multiple tasks in parallel). The store is `tasks.timer_store` with shape:
   ```json
   {
     "timers":   [{"taskId": <int>, "taskName": "<string>", "projectId": <int>, "sessionId": "<string>", "startedAt": "<ISO-8601 with offset>"}],
     "openLogs": [{"taskId": <int>, "projectId": <int>, "timelogId": <int>, "blocks": [{"start": "<ISO-8601>", "end": "<ISO-8601>"}], "summaries": ["<string>"]}]
   }
   ```
   If the file does not exist, create it with `{"timers": [], "openLogs": []}`. If it exists without `openLogs`, add the empty array when rewriting.
   - **Timers are per (taskId, sessionId)**: the same task may have one timer per session working on it. `sessionId` = the UUID segment of this session's scratchpad directory path (from the system prompt).
   - **`openLogs`** is the per-task consolidation record used by `./stop.md` — one Teamwork timelog per task that grows as each session stops (see `./stop.md`). This procedure never touches it.
   - Legacy entries without `sessionId` may exist; treat them as belonging to no particular session (any session may stop them) and never duplicate them for the same task in the same session.
2. Read the store:
   - If a timer for this task **and this session** already exists, report it (with its elapsed time) and do nothing else.
   - If timers for this task exist from **other sessions**, mention them briefly (task is being worked in parallel) and CONTINUE: this session still starts its own timer and runs the full flow below, including the worktree offer.
   - Timers running for OTHER tasks are fine — parallel tracking is the point. Mention them briefly; never block or ask to stop them.
3. Append an entry for this task with this session's `sessionId`, `startedAt = now` (local time with UTC offset) and the `projectId` from the task's `tasklist.meta.projectId` (returned by `teamwork_get_task`) — it is needed later by `teamwork_log_time`. Rewrite the file atomically (read, modify, write the whole JSON). Do NOT call `teamwork_start_timer`.
4. If the task belongs to an external source (check `tasks.sources`), apply that source's "in development" marking rule (skip if another session already applied it — e.g. the sheet row is already marked).
5. **Workflow stage**: the task must be in the development stage while in development. Match stages by name CONTAINING the `inDev` stage substring from `tasks.providers.teamwork.stages` (case- and accent-insensitive — e.g. "En Desarrollo" in workflow "Estados"; board names may carry prefixes or typos). Check the task's current stage (from `teamwork_get_task`, `workflowStages[].stageId`; 0 = unstaged); if it already matches, do nothing. Otherwise resolve workflow and stage ids via `teamwork_list_workflow_stages` and move with `teamwork_move_task_to_stage`. If the project has no workflow or no matching stage, report it and continue without failing.
6. **Git worktree setup (global rule)** — runs for EVERY session that starts the task, even when other sessions already have timers for it. Only if the current working directory is inside a git repository (skip silently otherwise):
   a. Build a proposed working branch name from `git.branch_pattern` (gitflow + task id, e.g. `feature/tw-{taskId}-short-description`; use `bugfix/` or `hotfix/` in place of `feature/` when the task is a bug or hotfix).
   b. If a branch or worktree for this task id already exists in THIS repo (another session created it — check `git branch --list "*tw-{taskId}*"` and `git worktree list`, and also look for a worktree at the path resolved from `git.worktree_pattern` per step f below), offer to ENTER the existing worktree (`EnterWorktree` with `path`) instead of creating a new one; also offer working in place. Do not create a second branch for the same task in the same repo unless the user explicitly asks for one.
   c. Otherwise ask the user, BEFORE creating anything: the **base branch** (suggest `git.base_branch` per gitflow, listing existing local branches as alternatives), whether to **create the worktree** (yes/no), and **confirmation of the new working branch name** (accept the proposal or type another). Use a single grouped `AskUserQuestion` when available; otherwise ask one question at a time and wait for each answer.
   d. If the user declines the worktree, continue working in the current directory on the current branch — do not create branches or worktrees.
   e. If the user accepts, FIRST update the base branch from the remote: `git fetch origin {base-branch}:{base-branch}` (fast-forwards the local branch without checkout); if `{base-branch}` is checked out in the current directory use `git pull --ff-only origin {base-branch}` instead. If the update cannot fast-forward (diverged local base) or the fetch fails (no remote/offline), report it and ask whether to continue from the local state or abort — do not create the worktree silently on a stale/diverged base.
   f. Then resolve the worktree path from `git.worktree_pattern` (default `.claude/worktrees/{slug}`), substituting `{slug}` (the task slug used in `git.branch_pattern`), `{branch}` (the confirmed branch name, slashes replaced with dashes), `{prefix}`, `{id}`, and `{repo}` (the name of the repository root directory, from `git rev-parse --show-toplevel`); a relative pattern resolves from the repository root, an absolute one is used as-is (e.g. `~/worktrees/{repo}/{slug}` keeps every repository's worktrees apart). Create the worktree with `git worktree add "{resolved-path}" -b {confirmed-branch} {base-branch}`, then switch the session into it (`EnterWorktree` with `path`). Report the worktree path, the branch, and the base commit it started from.

Confirm to the user: task name, task id (mention it explicitly if the task was just created), assignment (if it was assigned now), that this session's timer is running (plus any other sessions' timers on the same task), and the worktree/branch in use (if one was created or entered).
