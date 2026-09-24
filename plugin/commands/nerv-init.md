---
description: Bootstrap NERV for this repository — writes .nerv/nerv.yaml and ensures gentle-ai's own SDD bootstrap has run
---

# /nerv:init

Set up NERV in the current repository. Asks only for what is missing, never
overwrites silently, and delegates gentle-ai's own bootstrap unchanged.

1. **Refuse if not a git repository.** Run `git rev-parse --is-inside-work-tree`.
   On failure, tell the user NERV requires a git repository and stop — do
   not create any file.

2. **Read existing config.** Read `~/.claude/nerv/nerv.yaml` (user scope) if
   present, and `<repo>/.nerv/nerv.yaml` (project scope) if present. Merge
   per the standard precedence (project overrides user key by key).

3. **Ask only for missing keys**, as ONE grouped blocking prompt (native
   `AskUserQuestion` when representable, plain-text fallback otherwise, per
   the Lossless Blocking Prompts contract). Do not ask about a key already
   resolved from either file. Candidate groups:
   - **Task provider**: `teamwork` | `github-projects` | `jira` | `none`.
     Omit this group entirely if `tasks.provider` already resolves.
   - **Base branch**: free text, defaulting to the current branch's upstream
     default if detectable, else `main`. Omit if `git.base_branch` already
     resolves.
   - Skills per category (`testing`, `code`, `best-practices`,
     `architecture`, `audit`) may stay empty — do not force the user to fill
     them now; mention once that they can be added to `.nerv/nerv.yaml`
     later.

   If every required key already resolves, skip the prompt entirely and
   proceed silently to step 4.

4. **Write `.nerv/nerv.yaml`.** Compose the project file with `enabled: true`
   plus the resolved answers, following the schema in the approved NERV
   config (see `skills/nerv-orchestrator/SKILL.md` → Configuration
   resolution). If `.nerv/nerv.yaml` already exists, do **not** overwrite it
   without confirmation — show the diff between the existing file and the
   proposed one, and ask a single yes/no confirmation before writing.

5. **Bootstrap gentle-ai if needed.** If `openspec/config.yaml` or
   `.atl/skill-registry.md` is missing, delegate to the `sdd-init` agent
   unchanged — do not reimplement its logic. Tell the user plainly that this
   step is gentle-ai's own bootstrap (stack detection, persistence mode,
   `strict_tdd`, skill registry), not a NERV-specific step.

6. **Summarize.** Print what was written (`.nerv/nerv.yaml` path and its
   resolved keys) and remind the user that no restart is required for the
   file itself, but the SessionStart hook only injects the full
   `nerv-orchestrator` protocol on the **next** session start — this session
   keeps running under whatever routing was already active.

Do not modify any file this command does not own. Do not launch any agent
other than `sdd-init`, and only when step 5's condition is met.
