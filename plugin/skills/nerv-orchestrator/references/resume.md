## Resume

On resuming an interrupted NERV change, in order:

1. **Lock check.** Read `nerv/.orchestrator.lock`. Held — fresh by age,
   or `waiting_on: user` at any age (see `## Orchestrator lock`) — and its
   `session_id` is not ours → do not resume; relay one blocking prompt,
   exactly two choices: wait (stop here, try later) or take over (only
   after the user confirms the other session is really dead; record
   `took_over_from: <session_id>`). Stale (`waiting_on` not `user` and
   `heartbeat_at` 15+ minutes old) or absent → proceed.
2. **Memory + change artifacts.** `mem_context` → `mem_search` scoped to
   `nerv/{change}` → `mem_get_observation` for each hit's full content →
   read the change directory: which of `proposal.md`, `specs/`,
   `design.md`, `tasks.md` exist and the `[x]` state of `tasks.md`.
3. **Artifacts.** Read `state.yaml`, every `nerv/*.md`, `votes.md`'s
   `frozen` flags, `waves.md`, and the commits since the branch point.
4. **Reconcile.** Frozen tasks are never re-voted; closed waves are never
   re-run; a `commit_recorded` event whose hash exists in `git log` is
   done. A launch recorded without its matching `envelope` event is the
   only step to redo.
5. **Take the lock.** Write it with our `session_id`, append a `resume`
   event `{from_step, took_over_from}`, continue at the next unfinished
   step.

Never infer active work from the newest global memory hit alone; always
confirm against the change's own artifacts.

