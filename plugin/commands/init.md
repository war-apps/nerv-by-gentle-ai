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
   - **Teamwork project/tasklist** (only when the chosen or already-
     resolved provider is `teamwork` and `tasks.providers.teamwork.
     project_id` or `.tasklist_id` is still missing from the project
     file): project and tasklist, each by id or name. When the Teamwork
     MCP is reachable, offer to list candidates first
     (`teamwork_list_projects`, `teamwork_list_tasklists`) so the user
     picks rather than types raw ids; fall back to free text otherwise.
     Omit this group when the provider is not `teamwork`, or when both
     ids already resolve.
   - **Base branch**: free text, defaulting to the current branch's upstream
     default if detectable, else `main`. Omit if `git.base_branch` already
     resolves.
   - Skills per category (`testing`, `code`, `best-practices`,
     `architecture`, `audit`) may stay empty — do not force the user to fill
     them now; mention once that they can be added to `.nerv/nerv.yaml`
     later.

   User-scope-only keys — `tasks.providers.teamwork.assignee_id` and
   `.stages` — are read from `~/.claude/nerv/nerv.yaml` and are never
   asked here.

   If every required key already resolves, skip the prompt entirely and
   proceed silently to step 4.

4. **Write `.nerv/nerv.yaml`.** Compose the project file with `enabled: true`
   plus the resolved answers, following the schema in the approved NERV
   config (see `skills/nerv-orchestrator/SKILL.md` → Configuration
   resolution) — never the user-scope-only keys
   (`tasks.providers.teamwork.assignee_id`, `.stages`), which stay in
   `~/.claude/nerv/nerv.yaml` alone. Append a fully commented-out `models:`
   block showing the syntax and today's plugin defaults, so the user can see
   how to override a role's model/effort without guessing a value:

   ```yaml
   # models:                           # per-role model and effort; project overrides user, key by key
   #   aoba: { model: sonnet, effort: low }
   #   asuka: { model: sonnet, effort: medium }
   #   balthasar: { model: sonnet, effort: medium }
   #   casper: { model: sonnet, effort: medium }
   #   fuyutsuki: { model: sonnet, effort: medium }
   #   hyuga: { model: sonnet, effort: medium }
   #   kaji: { model: opus, effort: high }
   #   kaji-coverage: { model: sonnet, effort: medium }
   #   kaji-refuter: { model: sonnet, effort: medium }
   #   kaji-security: { model: sonnet, effort: medium }
   #   kaworu: { model: sonnet, effort: medium }
   #   maya: { model: sonnet, effort: medium }
   #   melchor: { model: fable, effort: high }
   #   misato: { model: fable, effort: high }
   #   rei: { model: sonnet, effort: medium }
   #   ritsuko: { model: opus, effort: high }
   #   shinji: { model: sonnet, effort: medium }
   #   toji: { model: sonnet, effort: medium }
   ```

   Uncomment and edit only the roles the user wants to override — an absent
   role keeps the plugin default shown above. Mention that `from:
   <gentle-ai-phase>` is also accepted in place of an explicit `model`/
   `effort` pair (see README → Configuration schema → `models:`), and that
   `effort` overrides only take effect after `nerv apply-models`
   (user scope only — project-scope `models:` applies `model`
   only). If `.nerv/nerv.yaml` already exists, do **not** overwrite it
   without confirmation — show the diff between the existing file and the
   proposed one, and ask a single yes/no confirmation before writing.

5. **Bootstrap gentle-ai if needed.** If `openspec/config.yaml` or
   `.atl/skill-registry.md` is missing, delegate to the `sdd-init` agent
   unchanged — do not reimplement its logic. Tell the user plainly that this
   step is gentle-ai's own bootstrap (stack detection, persistence mode,
   `strict_tdd`, skill registry), not a NERV-specific step.

6. **Summarize.** Print what was written (`.nerv/nerv.yaml` path and its
   resolved keys) and remind the user that no restart is required for the
   file itself, but the SessionStart hook only prints the activation
   header that loads the `nerv-orchestrator` skill on the **next** session
   start — this session keeps running under whatever routing was already
   active.

Do not modify any file this command does not own. Do not launch any agent
other than `sdd-init`, and only when step 5's condition is met.
