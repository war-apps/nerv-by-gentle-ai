## Task events

Loaded by the core's `## Task events` when the task plugin is installed.

Core never talks to a task tracker. When the task plugin is installed
(`task:dispatcher` is available), Ikari **emits events** to it by launching
`task:dispatcher` with `EVENT: <event>` and this payload, plus the merged
`tasks` and `git` blocks:

```yaml
task_ref: <from the run.started envelope; may be empty only on run.started>
repo: <repository root folder name>
change: <change name>
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
- **Recording.** Ikari appends every envelope to
  `openspec/changes/<change>/nerv/task-events.yaml` and logs one
  `task_event` entry `{event, provider, result, task_ref}` per envelope in
  `nerv/deliberation-log.md`.
- **Run summary.** When any envelope is pending (`blocked` or `failed`
  with `replay: true`), the run summary lists it under "Task events
  pending", with the hint `/task:replay <change>`.
- **Absent plugin.** Without `task:dispatcher`, none of the above happens:
  no event, no question, no log entry, no `task-events.yaml`.
