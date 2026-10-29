---
name: task
description: NERV task extension contract. Covers how providers are found (provider.yaml), the remote operations a provider implements, the composite operations, the local timer store, the pipeline events with their response envelope, fail-soft dispatch, the pending-event list, the listing table and the worktree offer. Loaded by the /task:* commands and by the task:dispatcher agent.
---

# NERV Task Extension — Contract

The task extension connects NERV to task trackers (Teamwork today). It does not talk to any tracker itself. Each tracker is a **provider** that ships a `provider.yaml` manifest plus one procedure file per operation. Every provider of every extension has the same `provider.yaml` shape, checked against the slot the extension's `extension.yaml` declares (`docs/extensions.md`); this file defines what the task side adds.

`nerv` core knows nothing about tasks. The core orchestrator emits pipeline **events** only when this extension is installed. The `task:dispatcher` agent turns each event into provider operations and answers with one envelope per provider (§6).

## 1. Configuration

Everything lives under `tasks:`, and it is read from the repository's `<repo>/.ai-workflow/config.yaml` only. The user config (the path `nerv configure --print --json` reports as `config_path`: `~/.config/ai-workflow/config.yaml` on Linux, `%AppData%\ai-workflow\config.yaml` on Windows) adds only the personal keys, and the repository still wins when it sets one:

- each `tasks.providers.<name>.<key>` whose provider manifest declares `scope: user` (such as `assignee_id` or `billable`);
- `tasks.timer_store`, `tasks.stale_after_hours` and the older `tasks.rounding_minutes`, which belong to the machine (an assumption of this contract: they are personal, like the `scope: user` keys).

Any other `tasks.*` key of the user config is ignored, `tasks.provider` included. The user config's `defaults.tasks` (same shape as `tasks:`) is never read here either: it is the seed that `/nerv:init`, `nerv configure --init-repo` and `/task:setup` copy into a repository being set up. `nerv doctor` warns about user keys that no longer apply, and `nerv migrate --user` moves them under `defaults.tasks`.

| Key | Default | Meaning |
|---|---|---|
| `tasks.provider` | the only installed provider | Provider for bare task ids (no `PREFIX-`) and for `/task:me`, `/task:all`. `none` turns the extension off in this scope: no events, no timers, no questions (S18). |
| `tasks.ask_when_missing` | `true` | At `run.started` with no task, ask for one. `false` runs without a task, silently. |
| `tasks.subtasks_per_wave` | `false` | Whether `wave.started` creates one subtask per FULL wave. |
| `tasks.timer_store` | `~/.claude/work/timers.json` | Local timer store (§4). |
| `tasks.stale_after_hours` | `12` | A running timer older than this is **stale** (§4.3). |
| `tasks.events.<event>` | `on` | Per-event switch (`on` or `off`), for example `tasks.events.verification.started: off` when stage moves stay manual (S10). |
| `tasks.sources` | none | Extra listing sources folded into `/task:me` and `/task:all` (sheets, etc.), each with its own reading and marking rules. |
| `tasks.providers.<name>.*` | from the provider's `provider.yaml` `config` block | Provider settings. The manifest declares each key's scope, type, default and the prompt `nerv configure` shows. |
| `tasks.providers.<name>.enabled` | `true` | `false` keeps the provider installed but stops sending it events. |

The `git.*` keys `branch_pattern`, `commit_ref_pattern`, `worktree`, `worktree_pattern` and `base_branch` belong to core. The worktree offer (§9) reads them.

## 2. Finding providers

A provider comes from one of two places, and its **root** is where its `provider.yaml` lives:

- **Installed:** the provider `task-<name>` is installed: its Claude Code id `task-<name>@nerv` appears in `~/.claude/plugins/installed_plugins.json`. The root is that entry's `installPath`.
- **Project:** the repository ships it as `.ai-workflow/providers/task-<name>/provider.yaml`, in the **main checkout** (`git rev-parse --path-format=absolute --git-common-dir`, then its parent), committed with the repository and never installed. The root is that folder. A project provider whose `task-<name>` is also an installed plugin's name is skipped and reported (`nerv` refuses that name collision), and so is one whose folder resolves outside the repository.

Then read `<root>/provider.yaml` and `<root>/<operation file>`; an operation file must stay inside the root.

Never read a copied procedure from another place, such as `~/.claude/commands/task/` or another folder of a repository checkout. The installed plugins and the repository's `.ai-workflow/providers/task-*/` folders are the only sources, from any repository or worktree (S11).

The manifest declares the common provider fields:

- `name`, `extension: task`, `slot: provider` (the task extension's only slot, several providers active at once), `contract_version` (1) and `runtime: agent` (this dispatcher follows its procedures).
- `mcp.server` and `mcp.tools`: the exact MCP server and tool names its procedures call.
- `config`: the provider's settings (§1).
- `operations`: one procedure file per operation (§3), or `not_implemented`. Every operation of the slot is declared, and every one is optional: the events decide what a provider needs.

And the task fields, which `plugins/task/extension.yaml` allows under `provider_fields`:

- `task_ref_prefix` (required, for example `TW`).
- `stages`: the default provider stage name for each canonical stage key (§5).
- `events`: the events it handles (§6).

`nerv` validates every manifest it ships. An operation an event needs can never be `not_implemented`.

**Tool check (S8).** Before the first operation of a dispatch, compare `mcp.tools` against the tools actually available to the running agent. A tool counts when its bare name, `mcp__<server>__<tool>`, or `mcp__plugin_task-<name>_<server>__<tool>` is available: the last is the server the provider's own plugin ships in its `.mcp.json` (Claude Code may spell the plugin's dash as `_`). For each claude.ai connector `mcp.connectors` names, `mcp__claude_ai_<connector>__<tool>` counts too, every character of the connector name but letters, digits, `_` and `-` read as `_` (`Teamwork.com` is `mcp__claude_ai_Teamwork_com__`). The same tool name from any other server, connector or plugin does not count. When more than one of these sources serves every tool, use one for the whole dispatch: `mcp__<server>__`, then the plugin's, then the first connector listed. When a tool is missing, run nothing for that provider and answer `blocked` with `error: "missing MCP tools: <names> (server <server>)"`. Never substitute another server's tools.

## 3. Operations a provider implements

`taskRef` is `{PREFIX}-{id}`, for example `TW-48888495`. It is the only identifier that crosses this boundary. The provider maps it to its own id. Every operation reports `ok`, or a failure with its exact error. It never retries on its own.

| Operation | Inputs | Output |
|---|---|---|
| `getTask` | taskRef | `{taskRef, name, open, assignees, stage, projectId, tasklistId, priority, url}` |
| `assign` | taskRef | `{assigned: bool}`: adds the configured user and keeps the other assignees. No-op when already assigned. |
| `moveStage` | taskRef, stageKey | `{stage}`: no-op when already there. When the provider has no matching stage, the result is `skipped` with the reason. |
| `logTime` | taskRef, start, end, minutes, description, billable | `{timelogRef}`: real start time, never a bare duration. |
| `updateTimelog` | timelogRef, minutes, description | `{timelogRef}` |
| `deleteTimelog` | timelogRef | none |
| `listTimelogs` | taskRef or project, mine | rows `{timelogRef, taskRef, start, minutes, description}` |
| `createTask` | title, list?, priority?, description?, source? | `{taskRef}`: unassigned unless the caller assigns it next. |
| `createSubtask` | parentRef, title, description? | `{taskRef}` |
| `comment` | taskRef, text | none |
| `complete` | taskRef | none. When the provider refuses (for example, open dependencies), report the refusal and never force it. |
| `setPriority` | taskRef, `high`, `medium` or `low` | `{priority}` |
| `listTasks` | scope (`mine` or `all`), filters | rows for the listing table (§8) |

## 4. Local timers (owned by this extension)

Timers are **always local**, never a provider-side timer. That way several tasks, and several sessions on one task, can be tracked in parallel.

### 4.1 Store

`tasks.timer_store` is shaped like this:

```json
{
  "timers":   [{"taskRef": "TW-1", "taskName": "...", "sessionId": "<uuid>", "startedAt": "<ISO-8601 with offset>"}],
  "openLogs": [{"taskRef": "TW-1", "timelogRef": "...", "blocks": [{"start": "...", "end": "..."}], "summaries": ["..."]}]
}
```

- Create the store as `{"timers": [], "openLogs": []}` when it is missing.
- Rewrite it atomically: read it, change it, write the whole JSON.
- A timer is keyed by **(taskRef, sessionId)**. `sessionId` is the UUID segment of this session's scratchpad path.
- Legacy entries carry `taskId` plus `projectId` and no `sessionId`. Read them as `{tasks.provider prefix}-{taskId}` belonging to no session, so any session may stop them.

Compute every time with a shell one-liner, never mentally.

### 4.2 Stopping (consolidating blocks)

One stop logs one **block**, from `startedAt` to the end.

1. **Summary.** It is a one-line summary of what was actually done in the block. When it is unknown, ask for it; never invent one. In the summary, write a pull request as `PR 34`, never `#34`, because some providers strip `#NN` tokens.
2. **First stop for the task** (there is no `openLogs` entry yet):
   - call `logTime` with the block's start and minutes = `ceil(blockMinutes / rounding) * rounding`, minimum one unit;
   - `rounding` and `billable` come from the provider config (`tasks.providers.<name>.rounding_minutes` and `.billable`). An older configuration's `tasks.rounding_minutes` is still read when the provider key is unset;
   - then create the `openLogs` entry.
3. **The new block couples** with the entry (its start ≤ the latest end):
   - append the block and the summary;
   - total = the union of the merged intervals, rounded once on the total;
   - call `updateTimelog` with that total and the summaries joined with `; `;
   - the timelog keeps its original start.
4. **A gap** (the new block's start > the latest end):
   - leave the old timelog alone;
   - call `logTime` for this block alone;
   - reset the entry to the new timelog.

   Every timelog is dated on the day its work happened.
5. **Moving a timelog's start.** A provider may be unable to change a timelog's start time. In that case, `logTime` the replacement, check it with `listTimelogs`, and only then `deleteTimelog` the old one. Never delete first (S16).
6. **Check before removing.** Check the timelog with `listTimelogs` before removing this session's timer. When logging failed, keep the timer so no time is lost.

### 4.3 Stale timers (S15)

A timer whose age passes `tasks.stale_after_hours` is stale. Never log a stale timer up to "now". Instead:

1. Find the **last real activity** for the task:
   - the newest `git log` commit on the task's branch or worktree;
   - the session transcript's last modification for that session, under `~/.claude/projects/**/<sessionId>.jsonl`.

   Take the later of the two.
2. Propose that time as the block's end. Show every stale timer's proposed cut-off in **one grouped question**, so the user can confirm, edit or discard each one.
3. Stop each confirmed timer at its confirmed end, following §4.2.

`/task:timers` marks stale timers. `/task:stopAll` and `run.started` mention them.

## 5. Canonical stages (S14)

Events and commands use only the stage keys `pending`, `analysis`, `inDev`, `testing`, `implemented`, `blocked` and `canceled`. The provider resolves each key in two steps:

1. from `tasks.providers.<name>.stages.<key>`;
2. otherwise, from the manifest's `stages.<key>`.

The resolved value is a case- and accent-insensitive substring of the provider's real stage name.

## 6. Events

The core orchestrator emits these events. Each one lists the operations it needs and what the dispatcher does with it:

| Event | Emitted when | Operations | The dispatcher |
|---|---|---|---|
| `run.started` | A NERV run classified and its task is known | `getTask`, `assign`, `moveStage` | Checks the task is open, assigns it, moves it to `inDev`, starts this session's local timer (§4). With no `task_ref` and `ask_when_missing`, it returns `needs_input` with the task question (§6.3). |
| `verification.started` | The quality gate (FULL) or the work unit's check (LIGHT, DIRECT) starts | `moveStage` | Moves the task to `testing`. |
| `audit.ranked` | The FULL issue ranking is accepted | `comment` | Posts the ranked audit summary. |
| `issue.deferred` | The user accepts a DEFER issue | `createTask` | Creates one task per deferred issue, linked from the parent in its description. |
| `wave.started` | Every FULL wave starts | `createSubtask` | Creates the wave's subtask, but only when `tasks.subtasks_per_wave` is `true`. Otherwise it answers `skipped` with `reason: subtasks_per_wave is off`. Core emits the event every time; this extension owns the check. |
| `run.closed` | A run ends complete | `moveStage`, `logTime` | Stops this session's timer (§4.2) and moves the task to `implemented`. **It never calls `complete`.** One task can span several changes and repositories; completing it stays an explicit `/task:close` (S13). |
| `run.halted` | A run halts before completion | `comment`, `moveStage`, `logTime` | Comments the halt cause, moves the task to `blocked` and stops this session's timer. |

### 6.1 Payload (S12)

Every event carries:

```yaml
event: run.started
task_ref: TW-49076279        # may be empty only on run.started
repo: dim.comprobantes.api   # repository root folder name
feature: <ODD feature name in FULL; the run id in DIRECT and LIGHT>
branch: feature/tw-49076279-...
worktree: /abs/path/or/empty
session_id: <uuid>
data: {}                     # event-specific: summary, cause, issues, wave
disabled_events: []          # per-run switches chosen by the user (S10)
```

### 6.2 Dispatch and fail-soft (S9)

For each event, the dispatcher works through these steps:

1. **Event switch.** When the extension is off in this scope (`tasks.provider: none`), or the event is `off` in config or in `disabled_events`, answer one envelope with `result: skipped` and `reason: switched off`. Nothing else runs.
2. **Provider choice.** Pick every installed provider that is enabled and subscribes to the event. When `task_ref` is set, only the provider whose `task_ref_prefix` matches it is picked.
3. **Run each provider:**
   - first the tool check (§2), then the event's operations through that provider's procedure files;
   - one try only: **never retry**;
   - **never halt the pipeline**: a failure is a result, not an exception.
4. **Answer** with one envelope per provider:

```yaml
- event: run.started
  provider: teamwork
  result: ok | skipped | blocked | failed | needs_input
  task_ref: TW-49076279
  data: {stage: "En Desarrollo", timer: started}
  error: "<exact error text>"   # when blocked or failed
  replay: true                   # when re-dispatching later can still apply it
```

When no provider matches, answer one envelope with `provider: none`, `result: skipped` and `reason: no provider handles <event>`.

### 6.3 Questions

The dispatcher never asks the user anything itself. When a choice is needed, it answers `result: needs_input` with the complete question. Examples: which task to start, the halt cause, a summary, stale cut-offs. The orchestrator relays the question unchanged and dispatches again with the answer in `data`.

### 6.4 The pending-event list (S9)

The orchestrator appends every envelope to the run folder's `task-events.yaml`, one list item per envelope: `odd/{feature}/nerv/task-events.yaml` for a FULL run, `.nerv/runs/{run-id}/task-events.yaml` for a DIRECT or LIGHT run (which has no ODD document). An envelope with `result` `blocked` or `failed` and `replay: true` stays **pending**.

- The run summary lists the pending envelopes under "Task events pending", along with the `/task:replay {feature}` command (the run id for DIRECT and LIGHT).
- `/task:replay {feature}` finds the file in either location and dispatches every pending envelope again, in order, and marks each one `replayed: <ISO time>` with its new result.

## 7. Composite operations (owned by this extension)

| Composite | Steps |
|---|---|
| `block(taskRef, cause)` | `comment("<blocked label>: " + cause)` → `moveStage(blocked)` → stop (§4.2). The cause is mandatory. |
| `cancel(taskRef, cause)` | `comment("<canceled label>: " + cause)` → `moveStage(canceled)` → stop. The cause is mandatory. The task stays open. |
| `done(taskRef)` | `moveStage(implemented)` → stop. The task stays open. |
| `close(taskRef)` | stop → `moveStage(implemented)` → `complete`. |
| `stopAll` | Stop every timer **this session** holds, with one grouped question for the missing summaries. Other sessions' timers are listed, never touched. |

The comment labels come from `tasks.providers.<name>.comment_labels` and default to `BLOQUEADA` and `CANCELADA`.

## 8. Listing table

`/task:me` and `/task:all` call `listTasks` on `tasks.provider` and read every enabled `tasks.sources` entry. Reading every source is mandatory: when one cannot be read, show a visible warning under the table that names the source and its error.

- **Dedupe.** A source row that links to a provider task is shown once, as that task.
- **Columns, exactly these:** `!`, `ID`, `Own`, `Project`, `Task List`, `Task`, `Stage`, `Current Time`, `History Time`, `Source`.
  - `Current Time`: the oldest running local timer as `⏱ Nh Nm`, with `×N` when several sessions are running one.
  - `History Time`: logged minutes from one `listTimelogs` call per project, plus the running time.
- **Sort order:**
  1. assigned to me;
  2. priority;
  3. stage, in the order Bloqueado → Implementado → Testing → En Desarrollo → Análisis → Pendiente → Backlog → Cancelado. An unstaged task counts as Backlog.

A running timer whose task is not in the listing gets a note under the table.

## 9. Worktree offer (`/task:start` only)

The offer runs only inside a git repository.

1. **Proposed branch.** Build it from `git.branch_pattern`, using `bugfix/` or `hotfix/` for bugs.
2. **Existing branch.** When a branch or worktree for the task already exists (`git branch --list "*{prefix}-{id}*"`, `git worktree list`, the resolved `git.worktree_pattern` path), offer to enter it or to work in place.
3. **New worktree.** Otherwise ask one grouped question: the base branch (default `git.base_branch`), whether to create a worktree, and the branch name.
4. **On yes:**
   - fast-forward the base branch first. When it has diverged or the remote is unreachable, ask before going on;
   - resolve the path from `git.worktree_pattern` (`{repo}`, `{slug}`, `{branch}`, `{prefix}`, `{id}`; relative patterns resolve from the repository root);
   - run `git worktree add`, then `EnterWorktree`.
5. **Main-checkout resources.** Resolve the skill registry, the Engram project key and the installed dependencies from the **main checkout**, never from the new worktree (S17):
   - read `.atl/skill-registry.md` from the main checkout;
   - keep the main checkout's Engram project name;
   - link or install `node_modules` and similar dependency folders when the worktree has none.
6. **herdr.** When `herdr` is on PATH and `herdr status server` reports running, run `herdr worktree open --workspace "$HERDR_WORKSPACE_ID" --path "<path>" --label "<branch without its type prefix>" --focus`. When `HERDR_WORKSPACE_ID` is empty, pass `--cwd "<repo root>"` instead. Skip silently when herdr is absent.
