---
description: Re-dispatch the task events a NERV run could not deliver (blocked or failed with replay: true)
argument-hint: <change>
---

Load the `task` skill first: it is the contract every step below refers to (§ numbers). Read the merged configuration (§1). When `tasks.provider` is `none`, say that task tracking is off in this scope and stop.

Arguments: `$ARGUMENTS`.

1. **Read.** Read `openspec/changes/<change>/nerv/task-events.yaml`. When the change is missing, list the changes that have pending events, then stop.
2. **Show.** Show the pending envelopes: event, provider, error.
3. **Confirm.** Ask once whether to replay them all.
4. **Replay.** On yes, dispatch each event again, in order, with its original payload. Use the `task:dispatcher` agent, or run its steps inline in this session so it can answer `needs_input` questions.
5. **Record.** Mark each envelope `replayed: <ISO time>` with its new result.

Report one line per envelope with its old and new results.
