# Jira Adapter (not implemented)

Implements the `nerv-tasks` port (`../SKILL.md`) against Jira. No operation
below has a working implementation yet — every row states
`status: not_implemented`. When `tasks.provider: jira` is selected and
Hyuga needs an operation from this table, Hyuga returns `status: blocked`
naming the operation, per the port's adapter contract (`../SKILL.md` §7).
It never improvises a Jira call outside this table.

`task_ref_prefix: JIRA`. A `taskRef` such as `JIRA-1234` would map to the
Jira issue key directly (Jira issue keys are already `{PROJECT_KEY}-{id}`,
so the port's `taskRef` and the native Jira key would need one clarified
mapping when this adapter is built — likely `taskRef` prefix `JIRA` wrapping
the native key rather than replacing it).

## Config keys this adapter will need

- `tasks.providers.jira.site` — the Jira Cloud site (e.g. `example.atlassian.net`).
- `tasks.providers.jira.project_key` — the Jira project key issues are created under.
- `tasks.providers.jira.task_ref_prefix` — `JIRA`.

## Op table

| Op | status | Design note |
|---|---|---|
| `start(taskRef)` | not_implemented | Would assign the issue and transition it via the configured "in progress"-equivalent workflow transition; local timer starts exactly as the Teamwork adapter does |
| `take(taskRef)` | not_implemented | Assign only, via a Jira MCP or REST v3 `PUT /issue/{key}/assignee`; create via `POST /issue` if it does not exist |
| `stop(taskRef, summary)` | not_implemented | Jira worklogs are the native equivalent of Teamwork timelogs (`POST /issue/{key}/worklog`); the coupled-vs-gap consolidation and 30-minute rounding rules stay unchanged and would apply to worklog entries the same way |
| `moveStage(taskRef, stageKey)` | not_implemented | Would map `stageKey` to a Jira workflow transition id via REST v3 transitions (`GET/POST /issue/{key}/transitions`), configured per `stageKey` the same way Teamwork's `stages` map works |
| `logTime(taskRef, start, end, note)` | not_implemented | `POST /issue/{key}/worklog` with `started` and `timeSpentSeconds` derived from real start/end, never a bare duration |
| `createTask({title, list, priority, description, source})` | not_implemented | `POST /issue` with `project.key` = `tasks.providers.jira.project_key`; `list` would map to an epic link or a component |
| `createSubtask(parentRef, ...)` | not_implemented | Jira has native sub-tasks (`issuetype: Sub-task`, `parent.key`) — this is expected to be simpler than the GitHub adapter's equivalent gap |
| `comment(taskRef, text)` | not_implemented | `POST /issue/{key}/comment` |
| `complete(taskRef)` | not_implemented | Workflow transition to the project's "Done"-equivalent status |
| `setPriority(taskRef, level)` | not_implemented | `PUT /issue/{key}` with the `priority` field, mapped from `high\|medium\|low` to the project's configured priority scheme |
| `list(scope, filters)` | not_implemented | JQL search (`GET /search` with `assignee = currentUser()` for `mine`, project-scoped JQL for `all`); would still merge with every enabled `tasks.sources` entry into the same unified table |
| `listTimers(taskRef?)` | not_implemented | Local timer store only — this would work identically to the Teamwork adapter once `start`/`stop` exist, since timers never touch the provider |

## Composite ops

`block`, `cancel`, `close`, `done` all compose from the primitives above per
`../SKILL.md` §4; none can be implemented until their constituent ops are.

## Why this adapter is not built yet

Jira maps onto the port more directly than GitHub (native worklogs,
transitions, and sub-tasks), but still needs two decisions settled before
implementation: which Jira MCP server (if any) is authorized for this
project, or whether this adapter goes straight to REST v3; and how
`taskRef`'s `{PREFIX}-{id}` shape coexists with Jira's own `{PROJECT_KEY}-{id}`
issue keys without ambiguity when both use dash-separated prefixes.
Selecting `tasks.provider: jira` today only proves the port is
provider-agnostic in shape, not that Jira is usable yet.
