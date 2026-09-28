---
description: Change a task's priority (High, Medium or Low)
argument-hint: <taskId> <High | Medium | Low> [tasklistId | tasklist name] [projectId | project name]
---

<!-- Embedded NERV procedure for the Teamwork adapter (../../teamwork.md). Also installable verbatim as the /task:priority slash command in ~/.claude/commands/task/priority.md — see "Optional slash commands" in the adapter. -->

Config: read `~/.claude/nerv/nerv.yaml`, merged with `<repo>/.nerv/nerv.yaml` when present (project overrides user); all Teamwork ids, stage names and work sources come from there.

Arguments: `$ARGUMENTS` — first token is the task id; one token must be the priority (`high`, `medium` or `low`, any casing, Spanish accepted: alta/media/baja); remaining tasklist/project tokens (id or name) are optional context and normally unnecessary (derived from the task via `teamwork_get_task`).

- If the task id is missing, ask which task — suggest a default (the task with a running timer in `tasks.timer_store`, else the task most recently discussed) — then STOP and wait.
- If the priority is missing or not one of the three values, ask which one, showing the task's current priority. One question, then STOP and wait.

1. Verify the task exists and is open via `teamwork_get_task`; if not, report it and stop.
2. If the task already has that priority, say so and stop.
3. Update with `teamwork_update_task` (`priority`: `"high"` | `"medium"` | `"low"`).

Report: task name, id, and the priority change (old → new, with the emoji convention 🔴 high / 🟡 medium / 🟢 low).
