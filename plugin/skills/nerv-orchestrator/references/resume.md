## Resume

On resuming an interrupted NERV run, in order. The run folder is
`odd/{feature}/nerv/` for FULL and `.nerv/runs/{run-id}/` for DIRECT and
LIGHT (`odd-document.md`); a run with no ODD document is DIRECT or LIGHT.

1. **Lock check.** Read `.nerv/locks/{run-id}.lock`. Held — fresh by age,
   or `waiting_on: user` at any age (see `## Orchestrator lock`) — and its
   `session_id` is not ours → do not resume; relay one blocking prompt,
   exactly two choices: wait (stop here, try later) or take over (only
   after the user confirms the other session is really dead; record
   `took_over_from: <session_id>`). Stale (`waiting_on` not `user` and
   `heartbeat_at` 15+ minutes old) or absent → proceed.
2. **ODD memory + document (FULL only).** gentle-ai's ODD resume order:
   `mem_context` → `mem_search` scoped to this project and feature
   (`odd/{feature}/tasks`, then `nerv/{feature}`) → `mem_get_observation`
   for each hit's full content → the file `odd/tasks/{feature}.md` itself:
   its `## Specs`, the `Status`, `MAGI` and `Commit` columns of `## Tasks`,
   and the last `## Log` entries. Reconcile the file with its mirror; on
   an irreconcilable difference keep both and ask the user about that
   conflict only.
3. **Run folder.** Read its `state.yaml`, every `nerv/*.md`, and in FULL
   `votes.md`'s `frozen` flags and `waves.md`, plus the commits since the
   branch point.
4. **Reconcile.** Frozen tasks are never re-voted; closed waves are never
   re-run; a `commit_recorded` event whose hash exists in `git log` is
   done. A launch recorded without its matching `envelope` event is the
   only step to redo.
5. **DIRECT and LIGHT state.** A run whose `.nerv/runs/{run-id}/state.yaml`
   says `path: direct` or `path: light` has no document, votes or waves:
   it resumes from that file's `request`, the diff and the log. Its progress is
   one work unit that stays uncommitted in the working tree until Ikari's
   user-validated commit. Rebuild it from `git status --porcelain`, `git
   diff` (staged and unstaged) and the log since the last
   `commit_recorded` event:
   - **Committed unit**: a `commit_recorded` hash present in `git log` and
     no uncommitted changes in the unit's paths → the unit is done;
     continue at the RDD hook, or at the run summary when its
     `rdd_assess`/`rdd_receipt` is logged.
   - **Finished, uncommitted edit**: changes in the tree plus the pilot's
     `envelope` (LIGHT) or Ikari's logged focused check (DIRECT) for them
     → re-run the verification step (spot check, or the focused check),
     then relay the commit validation as usual.
   - **Ambiguous**: changes in the tree with no matching envelope or
     focused check (an interrupted pilot launch or inline edit), changes
     outside the unit's paths, or a tree that does not match the logged
     evidence → show the user `git diff --stat` and relay one blocking
     prompt with exactly three choices: keep the changes and verify them
     as they are; keep them as the starting state and relaunch the pilot
     (LIGHT) or finish the edit inline (DIRECT); stop here.

   Never discard the working tree: no `git checkout --`, `git reset`,
   `git restore`, `git clean` or `git stash` on resume, in any tier;
   uncommitted changes the user did not ask to drop are always kept.
6. **Take the lock.** Write it with our `session_id`, append a `resume`
   event `{from_step, took_over_from}`, continue at the next unfinished
   step.

Never infer active work from the newest global memory hit alone; always
confirm against the run's own folder and, in FULL, its ODD document.

