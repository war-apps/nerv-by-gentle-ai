---
name: nerv-tasks
description: NERV task-tracking port: provider-agnostic operations (start, take, stop, moveStage, logTime, createTask, createSubtask, comment, complete, setPriority, list, listTimers, discardTimer) with composite ops block/cancel/close/done/stopAll; Hyuga loads it for DISPATCH: tracker; providers under providers/.
---

# NERV Tasks — Provider-Agnostic Port

This skill defines the task-tracking port that NERV runs through. It is a
contract, not an implementation: every operation below is implemented once
per provider in `providers/<provider>.md`. Nothing in this file talks to a
tracker API directly.

## 1. Purpose and ownership

Hyuga is the only NERV agent that calls this port (dispatch d, "tracker" —
see `nerv-orchestrator/SKILL.md`'s dispatch table). No other agent invokes a
port operation or a provider file directly:

- **Aoba** owns git (worktree creation, branch checkout, commits). Hyuga
  passes Aoba the confirmed branch name and base branch; Aoba never calls a
  port operation itself.
- **Ikari** relays the preflight question verbatim (task choice, worktree,
  branch name, base branch — see `nerv-orchestrator/SKILL.md` "Preflight
  when configuration or an active task is missing") and, once the user
  answers, passes the resulting `task_ref` and `commit_ref` to Hyuga and
  Aoba. Ikari never calls a port operation itself.

## 2. Provider selection

Resolve the provider from the merged configuration (user `~/.claude/nerv/nerv.yaml`
overridden key-by-key by project `<repo>/.nerv/nerv.yaml`) at `tasks.provider`:

- A concrete value (`teamwork`, `github-projects`, `jira`) selects the
  adapter file `providers/<value>.md`. Hyuga loads exactly that one file.
- `none` means no tracker calls of any kind: no port operation runs, and no
  local timer is started or stopped either — `start`/`stop`/`logTime` are
  timer-and-tracker operations together (see §5), so `none` disables the
  whole layer, not just the remote calls. A NERV run still executes; the
  preflight and dispatch d simply produce no tracker/timer side effects.
- Absent with `tasks.ask_when_missing: true` (the default) means Ikari asks
  the provider as part of the grouped preflight question, then the answer is
  offered to be persisted into the project `nerv.yaml` per the orchestrator's
  preflight rules. Hyuga never guesses a provider and never falls back to a
  default silently.
- Absent with `ask_when_missing: false` and no answer resolvable: Hyuga
  returns `status: blocked` naming the missing `tasks.provider` key.

## 3. Port operations

`taskRef` has the shape `{PREFIX}-{id}` (e.g. `TW-48888495`), where `PREFIX`
comes from the selected provider's `task_ref_prefix` config key. It is the
one identifier that crosses the port boundary; callers never pass or expect
a provider-internal id shape.

| Op | Inputs | Outputs | Side effects | Notes |
|---|---|---|---|---|
| `start(taskRef)` | taskRef | `{taskId, taskRef, stage}` | Assigns the task to the configured user if unassigned; moves it to the `inDev` stage; starts a local timer for `taskRef` keyed by (task, session) | No-op on the timer if this session already has one running for the task; other sessions' timers are untouched |
| `take(taskRef)` | taskRef | `{taskId, taskRef}` | Assigns the task if unassigned; creates it first if `taskRef` names a not-yet-existing task | Never starts a timer, never moves a stage, never offers a worktree |
| `stop(taskRef, summary)` | taskRef, summary (required, one-line, never invented) | `{loggedMinutes, timelogRef}` | Stops this session's local timer; consolidates the timelog (coupled blocks merge, a gap starts a fresh timelog); logs the rounded time to the provider with real start/end times | Rounding and coupling rules are provider-agnostic (§5); never a bare duration |
| `moveStage(taskRef, stageKey)` | taskRef, `stageKey ∈ pending\|analysis\|inDev\|testing\|implemented\|blocked\|canceled` | `{stage}` | Moves the task to the stage resolved through `tasks.providers.<provider>.stages[stageKey]` | No-op if already on that stage; reports and continues if the provider has no matching stage |
| `logTime(taskRef, start, end, note)` | taskRef, real start/end timestamps, note | `{timelogRef}` | Logs time directly, bypassing the local timer (used for corrections, never as a substitute for `stop`) | `start`/`end` are always real clock times, never a bare duration |
| `createTask({title, list, priority, description, source})` | title, list (id or name), priority, description, optional `source` (e.g. an external sheet row) | `{taskId, taskRef}` | Creates an unassigned task unless the calling op implies assignment (e.g. via `take`/`start`) | `list` and `priority` default from `tasks.providers.<provider>` config when omitted |
| `createSubtask(parentRef, {title, ...})` | parentRef, subtask fields | `{taskId, taskRef}` | Creates a task linked as a child of `parentRef` | Used when `tasks.subtasks_per_wave: true` |
| `comment(taskRef, text)` | taskRef, text | none | Posts a comment on the task | |
| `complete(taskRef)` | taskRef | none | Marks the task complete/closed in the provider | Task stays open if the provider rejects completion (e.g. open dependencies); reported, not forced |
| `setPriority(taskRef, high\|medium\|low)` | taskRef, level | `{priority}` | Updates task priority | No-op if already at that priority |
| `list(scope: mine\|all, filters)` | scope, optional filters (project, date, free text) | unified rows | Read-only | Merges the provider's tasks with every enabled `tasks.sources` entry into one table (§4) |
| `listTimers(taskRef?)` | optional taskRef | timer rows | Read-only | Reads the local timer store only; never calls the provider |
| `discardTimer(taskRef)` | taskRef | none | Deletes this session's local timer entry without logging | Mandatory user confirmation; never touches the provider |

## 4. Composite ops

Defined once here; adapters never redefine them, only the primitives they compose:

- `block(taskRef, reason)` = `comment(taskRef, "BLOQUEADA: " + reason)` + `moveStage(taskRef, blocked)` + `stop(taskRef, summary)`. `reason` is mandatory — never proceed with an empty or invented one.
- `cancel(taskRef, reason)` = `comment(taskRef, "CANCELADA: " + reason)` + `moveStage(taskRef, canceled)` + `stop(taskRef, summary)`. `reason` is mandatory. The task stays open in the provider; the stage marks the cancellation.
- `close(taskRef)` = `stop(taskRef, summary)` + `moveStage(taskRef, implemented)` + `complete(taskRef)`.
- `stopAll(summaries)` = `stop(taskRef, summary)` for every timer THIS session holds (one summary per task, all gathered in one grouped question when missing). Other sessions' timers are listed as information only and never logged or removed.
- `done(taskRef)` = `moveStage(taskRef, implemented)` + `stop(taskRef, summary)`. The task stays open — `complete` is not called; that is what distinguishes `done` from `close`.

## 5. Provider-agnostic invariants

Adapters implement these operations against a specific provider, but they
never change the following — these are properties of the port itself:

- **Local timer store contract.** Timers are always local (never a
  provider-side timer), keyed by (taskId, sessionId), so the same task can
  be worked by multiple sessions in parallel. `stop` consolidates: coupled
  (overlapping or touching) blocks merge into one timelog as the union of
  intervals; a gap after the last block closes that timelog and starts a
  fresh one dated at the new block. The rounded total is always
  `ceil(minutes / 30) * 30`, minimum 30, applied to the union total, never
  per block.
- **Unified listing table.** `list` always renders the columns `!`, `ID`,
  `Own`, `Project`, `Task List`, `Task`, `Stage`, `Current Time`,
  `History Time`, `Source`, sorted assigned-to-me first, then priority, then
  stage (Bloqueado → Implementado → Testing → En Desarrollo → Análisis →
  Pendiente → Backlog → Cancelado; unstaged counts as Backlog).
- **`taskRef` format and how it feeds git.** `{PREFIX}-{id}` (lowercased
  `{prefix}` feeds `git.branch_pattern`, e.g. `feature/{prefix}-{id}-{slug}`;
  the full uppercase `{PREFIX}-{id}` feeds `git.commit_ref_pattern`, e.g.
  `({PREFIX}-{id})` appended to the commit subject).
- **No coding without an active task.** Ikari's preflight enforces this
  before dispatch d runs `start`/`createTask` — see
  `nerv-orchestrator/SKILL.md`.
- **Time logs always carry real start and end times.** `stop` and `logTime`
  never send a bare duration with a made-up or defaulted time of day — the
  actual block boundaries are always used.

## 6. In a NERV run

What Hyuga does in `DISPATCH: tracker` (dispatch d), across a FULL or LIGHT
run:

1. **Preflight**: after Ikari's grouped question is answered, run
   `createTask` if the user chose to create one, then `start(taskRef)`.
   Aoba creates the worktree and branch with the confirmed names, using the
   task's `taskRef` to build `git.branch_pattern`, and the path resolved
   from `git.worktree_pattern`.
2. **Per-wave subtasks** (FULL only, optional): `createSubtask(parentRef, ...)`
   for each wave when `tasks.subtasks_per_wave: true`.
3. **Testing gate**: `moveStage(taskRef, testing)` when Maya's full quality
   gate starts (FULL phase 13) or the LIGHT reduced gate starts.
4. **Audit summary** (FULL only): `comment(taskRef, <ranked audit summary>)`
   after the issue-ranking gate (FULL phase 15), plus `createTask` for each
   DEFER issue the user accepted.
5. **Close**: at the end of the run, `close(taskRef)` or `done(taskRef)` as
   the user chooses (FULL phase 19 / LIGHT close).
6. **Halt**: `block(taskRef, reason)` when a run is halted before
   completion, with the mandatory cause.

Every op Hyuga runs through this port is logged by Ikari as a
`tracker_event` entry in `nerv/deliberation-log.md`, carrying `{op, taskRef,
result}`.

## 7. Adapter contract

Each `providers/<provider>.md` file implements every operation in §3 and §4,
or explicitly declares it `not_implemented`. Hyuga never improvises a
provider-specific workaround for an unimplemented op: when a run needs an op
the selected adapter marks `not_implemented`, Hyuga returns `status: blocked`
naming that exact operation and provider, and stops — it never substitutes a
different op, silently skips the step, or guesses at the provider's native
API.
