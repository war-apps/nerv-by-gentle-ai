---
description: Stop tracking on ALL started tasks - stops every timer and logs time as /task:stop would for each
---

Load the `task` skill first: it is the contract every step below refers to (§ numbers). Read the configuration (§1: the project file, plus the personal keys of the user file). When `tasks.provider` is `none`, say that task tracking is off in this scope and stop.

Arguments: `$ARGUMENTS`.

1. **Collect.** Gather this session's timers, and legacy entries without a session. List the other sessions' timers as information only.
2. **Nothing running.** When there are no timers, say so and stop.
3. **Ask once.** Show the timers. Ask one grouped question covering both the missing summaries and the cut-offs of stale timers (§4.3).
4. **Stop each one.** Stop each timer as `/task:stop` would (§4.2), with the provider of its task ref. A failure on one task never leaves the others running.

Report a table: task, raw time, rounded time logged, and status (ok, or failed with the reason).
