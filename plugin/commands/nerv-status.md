---
description: Show NERV activation, config resolution and change status for this repo
---

# /nerv:status

Report NERV's state for the current repository. Read-only. Keep the output
short and imperative — this is a status check, not a narrative.

1. **Activation.** Check whether `.nerv/nerv.yaml` exists in the repo root
   and contains `enabled: true`. Report `NERV: active` or `NERV: inactive`
   accordingly. If inactive, stop here — nothing else applies.

2. **Config resolution.** Read `~/.claude/nerv/nerv.yaml` (user scope) and
   `<repo>/.nerv/nerv.yaml` (project scope). Print a merged summary: which
   keys came from the project file, which fell back to the user file, and
   which fell back to a built-in default. The project file always wins key
   by key. Also print one line: `tasks.provider: <resolved value>` and
   `session timer: running (task <taskId>)` or `session timer: none`
   (check `~/.claude/work/timers.json` for an entry matching this
   session's `sessionId`).

3. **gentle-ai version check.** Run `gentle-ai --version` and parse the
   first token as `MAJOR.MINOR.PATCH` (e.g. `3.7.0`). Compare against the
   version this NERV release was tested against — `3.7.0`, as stated in
   this plugin's `README.md`. Print both versions.
   - Major `!= 3`: print "NERV requires gentle-ai 3.x; found X.Y.Z" and
     mark status degraded.
   - Major `== 3` but minor/patch differ from `3.7.0`: print an
     informational note only.
   Never block on either case.

4. **Active changes.** If `openspec/changes/` exists in the repo, for each
   active change directory run `gentle-ai sdd-status --json` and list it
   alongside which NERV-owned artifacts are present under that change's
   `nerv/` subfolder (for example `nerv/run-summary.md`). If no changes
   exist, say so plainly.

5. **Phase 0 note.** Always print, verbatim: "Phases 1+ add loop counters
   and resume."

Do not modify any file. Do not launch any agent other than for the reads
above.
