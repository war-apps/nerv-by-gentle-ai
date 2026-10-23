---
description: List my pending tasks from all work sources (only assigned to me), with running timers and tracked time
argument-hint: [project] [date] [text]
---

Load the `task` skill first: it is the contract every step below refers to (§ numbers). Read the merged configuration (§1). When `tasks.provider` is `none`, say that task tracking is off in this scope and stop.

Arguments: `$ARGUMENTS`.

1. **Tasks.** `listTasks(mine)` on `tasks.provider`. A project name, a date expression or free text becomes a filter.
2. **Sources.** Read every enabled `tasks.sources` entry (§8).
3. **Table.** Render the listing table exactly as §8 describes.
