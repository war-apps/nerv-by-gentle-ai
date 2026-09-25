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
   by key.

3. **gentle-ai version check.** Run `gentle-ai --version` and compare it
   against the version this NERV release was tested against — `3.7.0`, as
   stated in this plugin's `README.md`. Print both versions. Warn on any
   mismatch; do not block on it.

4. **Active changes.** If `openspec/changes/` exists in the repo, for each
   active change directory run `gentle-ai sdd-status --json` and list it
   alongside which NERV-owned artifacts are present under that change's
   `nerv/` subfolder (for example `nerv/run-summary.md`). If no changes
   exist, say so plainly.

5. **Phase 0 note.** Always print, verbatim: "Phases 1+ add loop counters
   and resume."

Do not modify any file. Do not launch any agent other than for the reads
above.
