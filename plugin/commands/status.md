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
   session's `sessionId`). Also print `artifacts.commit: <resolved value>`
   (`with-change` | `at-close` | `never`; built-in default `at-close` when neither file sets it).

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
   exist, say so plainly. For each active change, also read
   `nerv/.orchestrator.lock` if present (read-only — never refresh or
   delete it from this command) and print `lock: session <session_id>,
   step <step>, waiting_on <waiting_on> — fresh (heartbeat <N.N>m ago)`,
   `… — held: parked on a user gate (heartbeat <N.N>m ago)` when
   `waiting_on` is `user` whatever the age, `… — stale (heartbeat <N.N>m
   ago)`, or `lock: none` when the file is absent; an unreadable or
   partial lock, or one without a parseable `heartbeat_at`, prints `lock:
   unreadable — treated as held`. Fresh means `heartbeat_at` is under 15
   minutes old and `waiting_on` is not `user` (see the Orchestrator lock
   section of `nerv-orchestrator/SKILL.md`).

5. **Model/effort table.** Resolve each of the 18 `nerv:<role>` launches'
   model and effort per `nerv-orchestrator/SKILL.md`'s Configuration
   resolution → **Model and effort per role** (project `models.<role>` >
   user `models.<role>` > `from` phase in `~/.gentle-ai/state.json` >
   plugin default). Locate the installed cache at
   `~/.claude/plugins/cache/nerv/nerv/<version>/` (the `installPath` of
   `nerv@nerv` in `~/.claude/plugins/installed_plugins.json`, or the single
   version directory present there) and look up each role's cached
   frontmatter effort at `<installPath>/agents/<role>.md`; if that cache
   directory is missing, print `cache: not found` once and leave `cached
   effort` blank for every row. Print one table:

   | role | purpose | gentle-ai equivalent | model | effort | source | cached effort |
   |---|---|---|---|---|---|---|
   | ... | ... | ... | ... | ... | project\|user\|gentle-ai:\<phase\>\|default | ... |

   Take `purpose` and `gentle-ai equivalent` (`-` when none) per role from
   `nerv configure --print --json` (`models[].purpose` and
   `models[].gentle_ai_equivalent`); they are informational and never
   affect resolution.

   Then print a closing line: `models: in sync` when every role's resolved
   effort matches its cached frontmatter effort, or `models: N role(s)
   drift — run nerv apply-models` otherwise (`N` = the
   count of mismatched roles). Read-only — never write `nerv.yaml`,
   `state.json`, or any cached agent file.

6. **Phase note.** Always print, verbatim: "Phase 5 adds the orchestrator
   lock and safe resume."

Do not modify any file. Do not launch any agent other than for the reads
above.
