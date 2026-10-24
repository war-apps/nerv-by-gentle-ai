---
description: Re-dispatch the task events a NERV run could not deliver (blocked or failed with replay: true)
argument-hint: <feature or run id>
---

Load the `task` skill first: it is the contract every step below refers to (§ numbers). Read the merged configuration (§1). When `tasks.provider` is `none`, say that task tracking is off in this scope and stop.

Arguments: `$ARGUMENTS`.

1. **Read.** The argument is the name the run summary's hint printed: a FULL run's feature, or a DIRECT or LIGHT run's run id. Read `odd/{feature}/nerv/task-events.yaml` (a FULL run's folder) when it exists, otherwise `.nerv/runs/{run-id}/task-events.yaml` (a DIRECT or LIGHT run's runtime folder). When neither exists, list the features and run ids that have pending events in either location, then stop.
2. **Show.** Show the pending envelopes: event, provider, error.
3. **Confirm.** Ask once whether to replay them all.
4. **Replay.** On yes, dispatch each event again, in order, with its original payload. Use the `task:dispatcher` agent, or run its steps inline in this session so it can answer `needs_input` questions.
5. **Record.** Mark each envelope `replayed: <ISO time>` with its new result.

Report one line per envelope with its old and new results.
