# GitHub Projects Adapter (not implemented)

Implements the `nerv-tasks` port (`../SKILL.md`) against GitHub Projects.
No operation below has a working implementation yet — every row states
`status: not_implemented`. When `tasks.provider: github-projects` is
selected and Hyuga needs an operation from this table, Hyuga returns
`status: blocked` naming the operation, per the port's adapter contract
(`../SKILL.md` §7). It never improvises a GitHub call outside this table.

`task_ref_prefix: GH`. A `taskRef` such as `GH-142` would map to issue
number `142` in the configured repository.

## Config keys this adapter will need

- `tasks.providers.github-projects.owner` — org/user that owns the project.
- `tasks.providers.github-projects.project_number` — the Projects (v2) board number.
- `tasks.providers.github-projects.task_ref_prefix` — `GH`.

## Op table

| Op | status | Design note |
|---|---|---|
| `start(taskRef)` | not_implemented | Would assign the issue (`gh issue edit --add-assignee`), set the project's status field to an "in progress"-equivalent, and start the local timer exactly as the Teamwork adapter does |
| `take(taskRef)` | not_implemented | `gh issue edit --add-assignee`; create via `gh issue create` if the issue does not exist yet |
| `stop(taskRef, summary)` | not_implemented | GitHub has no native time tracking — would need to log the consolidated block as an issue comment or an external time-tracking field; local timer store logic stays unchanged |
| `moveStage(taskRef, stageKey)` | not_implemented | Would map `stageKey` to a Projects v2 single-select "Status" field option via `gh project item-edit --field-id ... --single-select-option-id ...`, configured per `stageKey` the same way Teamwork's `stages` map works |
| `logTime(taskRef, start, end, note)` | not_implemented | Same gap as `stop` — no native GitHub time field |
| `createTask({title, list, priority, description, source})` | not_implemented | `gh issue create` then `gh project item-add` to attach it to the configured project; `list` would map to a milestone or a second Projects field |
| `createSubtask(parentRef, ...)` | not_implemented | GitHub issues have "tracked-by" relationships, not a first-class parent field; would need the GraphQL API's sub-issues relationship |
| `comment(taskRef, text)` | not_implemented | `gh issue comment` |
| `complete(taskRef)` | not_implemented | `gh issue close` |
| `setPriority(taskRef, level)` | not_implemented | Would map to a Projects v2 "Priority" single-select field, if configured on the board |
| `list(scope, filters)` | not_implemented | `gh project item-list` plus `gh issue list --assignee`; would still merge with every enabled `tasks.sources` entry into the same unified table |
| `listTimers(taskRef?)` | not_implemented | Local timer store only — this would work identically to the Teamwork adapter once `start`/`stop` exist, since timers never touch the provider |

## Composite ops

`block`, `cancel`, `close`, `done` all compose from the primitives above per
`../SKILL.md` §4; none can be implemented until their constituent ops are.

## Why this adapter is not built yet

GitHub Projects (v2) has no native concept of stages equivalent to a
Teamwork workflow, no native time tracking, and no first-class subtask
relationship — three of the port's invariants (`moveStage`, `stop`/`logTime`,
`createSubtask`) need a deliberate mapping decision before any op can be
implemented safely, not just a tool substitution. Building this adapter is
future work; selecting `tasks.provider: github-projects` today only proves
the port is provider-agnostic in shape, not that GitHub is usable yet.
