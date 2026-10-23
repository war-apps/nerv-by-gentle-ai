---
description: Change a task's priority (High, Medium or Low)
argument-hint: <taskRef> <High | Medium | Low>
---

Load the `task` skill first: it is the contract every step below refers to (§ numbers). Read the merged configuration (§1). When `tasks.provider` is `none`, say that task tracking is off in this scope and stop.

Arguments: `$ARGUMENTS`.

**Provider.** A `PREFIX-id` argument selects the provider whose `task_ref_prefix` matches. A bare id belongs to `tasks.provider`. Find that provider (§2) and run its tool check; when a tool is missing, report it and stop. Run every operation through the provider's procedure file.

**Arguments.** One token is the priority: `high`, `medium` or `low`, in any casing. The Spanish `alta`, `media` and `baja` are accepted too.

- When the task is missing, ask which one and stop.
- When the priority is missing or invalid, ask which one, showing the current priority, and stop.

1. **Check.** `getTask`. When the task already has that priority, say so and stop.
2. **Update.** `setPriority`.

Report: the change, old → new, with 🔴 high, 🟡 medium and 🟢 low.
