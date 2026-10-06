---
description: Bootstrap NERV for this repository — writes .nerv/nerv.yaml, openspec/config.yaml and the skill registry
---

# /nerv:init

Set up NERV in the current repository. Asks only for what is missing, never
overwrites silently, and bootstraps `openspec/` and the skill registry itself.

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
   - **Task provider**: `teamwork` | `none`.
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
   #   kaji-resilience: { model: sonnet, effort: medium }
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
   proposed one, and ask a single yes/no confirmation before writing. (The
   `nerv configure --init-repo` CLI never overwrites it either; it only
   removes the removed task providers `github-projects` and `jira` from an
   existing file, with a backup.)

5. **Bootstrap `openspec/` and the skill registry.** NERV owns this step; it
   no longer delegates to a gentle-ai agent (gentle-ai 4.x removed
   `sdd-init`).
   - Create `openspec/specs/` and `openspec/changes/archive/` when missing.
   - Create `openspec/config.yaml` only when it is absent; an existing file
     and every value in it are kept untouched. Keep it minimal:

     ```yaml
     context: |
       <3-8 lines: stack, layout, conventions detected from the repo>
     strict_tdd: <true|false>
     testing:
       runner: <the workspace-level test command, or none>
     rules:
       apply:
         test_command: <same command>
     ```

   - Resolve the runner and `strict_tdd` from what the repo really has,
     never by guessing. Discover every project root from the repo root (the
     explicit workspace membership when declared, otherwise the root and at
     most two levels below it; skip `.git`, `node_modules`, `vendor`,
     `dist`, `build`, `out`, `target`, `.cache`, `__pycache__`, `.venv`,
     `venv` and nested repositories). `strict_tdd: true` only when the
     project set is non-empty and one explicit workspace-level test command
     (an existing script or target such as `go test ./...`, `npm test` or
     `dotnet test`, run from the repo root) covers every in-scope project.
     Otherwise write `strict_tdd: false`, set `testing.runner` and
     `rules.apply.test_command` to the single command when there is one (or
     `none`), and tell the user why. These rules decide only the values of a
     new file. When `openspec/config.yaml` already exists, never rewrite it:
     if it holds `strict_tdd: true` but no workspace-level command covers
     every in-scope project, leave the file as is and warn the user that
     strict TDD cannot be honored until they add such a command or set
     `strict_tdd: false` themselves.
   - Then run `gentle-ai skill-registry refresh --cwd <repo>` to write
     `.atl/skill-registry.md`. If `gentle-ai` is not installed, say so and
     skip only this sub-step.

6. **Summarize.** Print what was written (`.nerv/nerv.yaml` path and its
   resolved keys) and remind the user that no restart is required for the
   file itself, but the SessionStart hook only prints the activation
   header that loads the `nerv-orchestrator` skill on the **next** session
   start — this session keeps running under whatever routing was already
   active.

Do not modify any file this command does not own. Do not launch any agent.
