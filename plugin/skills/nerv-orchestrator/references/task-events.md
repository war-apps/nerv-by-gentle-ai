## Task events

Loaded by the core's `## Task events` when the task plugin is installed.

Core never talks to a task tracker. Ikari **emits each event** to every
installed extension whose `extension.yaml` lists it under `events` (the
extension contract, `docs/extensions.md`): an extension with
`runtime: agent` is reached by launching its `agent` entry. Today only the
task extension subscribes, through `task:dispatcher`. When the task plugin
is installed (`task:dispatcher` is available), Ikari launches
`task:dispatcher` with `EVENT: <event>` and this payload, plus the merged
`tasks` and `git` blocks:

```yaml
task_ref: <from the run.started envelope; may be empty only on run.started>
repo: <repository root folder name>
feature: <feature name in FULL; the run id in DIRECT and LIGHT>
branch: <current branch>
worktree: <absolute worktree path, or empty>
session_id: <this session's UUID>
data: {}             # event-specific: summary, cause, issues, wave
disabled_events: []  # per-run switches the user chose
```

| Event | Emitted when |
|---|---|
| `run.started` | Preflight, after classification |
| `verification.started` | Maya's full gate starts (FULL), or the work unit's verification starts (LIGHT, DIRECT) |
| `audit.ranked` | The user accepts Hyuga's issue ranking (FULL) |
| `issue.deferred` | Once per accepted `DEFER` issue (FULL) |
| `wave.started` | A FULL implementation wave starts; the task plugin's own config decides whether it creates a subtask |
| `run.closed` | The run closes complete, before the run summary is written. It never completes the task: completing it stays an explicit `/task:close` |
| `run.halted` | The run halts before completion, with the cause in `data` |

- **Fail-soft.** An envelope with `result` `blocked` or `failed` never
  halts the run and is never retried by Ikari. A `needs_input` envelope is
  relayed verbatim and dispatched again with the answer in `data`.
- **Recording.** Ikari appends every envelope to the run folder's
  `task-events.yaml` — `odd/{feature}/nerv/task-events.yaml` in FULL,
  `.nerv/runs/{run-id}/task-events.yaml` in DIRECT and LIGHT — and logs one
  `task_event` entry `{event, provider, result, task_ref}` per envelope in
  `nerv/deliberation-log.md`. There is one exception (S18). An envelope
  with `result` `skipped` whose provider is `provider: none` (no provider
  handles the event), or whose reason is `switched off` (the plugin is off
  in this scope, or the event is turned off), is not recorded: no
  `task-events.yaml` line and no log entry. When every envelope of
  `run.started` is such a skip, Ikari dispatches no further event in this
  run, so a plugin that is installed but off or has no provider behaves
  like an absent one.
- **Run summary.** When any envelope is pending (`blocked` or `failed`
  with `replay: true`), the run summary lists it under "Task events
  pending", with the hint `/task:replay {feature}` (the run id in DIRECT
  and LIGHT).
- **Absent plugin.** Without `task:dispatcher`, none of the above happens:
  no event, no question, no log entry, no `task-events.yaml`.
