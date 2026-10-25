---
description: List all pending tasks from all work sources (assigned to me AND unassigned), with running timers and tracked time
argument-hint: [project] [date] [text]
---

Load the `task` skill first: it is the contract every step below refers to (§ numbers). Read the configuration (§1: the project file, plus the personal keys of the user file). When `tasks.provider` is `none`, say that task tracking is off in this scope and stop.

Arguments: `$ARGUMENTS`.

1. **Tasks.** `listTasks(all)` on `tasks.provider`: tasks assigned to me plus unassigned ones, never tasks that belong only to others. A project name, a date expression or free text becomes a filter.
2. **Sources.** Read every enabled `tasks.sources` entry (§8).
3. **Table.** Render the listing table exactly as §8 describes.
