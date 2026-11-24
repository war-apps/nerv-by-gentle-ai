---
description: Bootstrap NERV for this repository — writes .ai-workflow/config.yaml and the skill registry, then offers the configuration flows (NERV, nerv-task, nerv-release)
---

# /nerv:init

Set up NERV in the current repository. Asks only for what is missing, never
overwrites silently, bootstraps the skill registry itself, and then offers
every configuration flow installed on this machine. Work is
tracked on gentle-ai's ODD feature documents (`odd/tasks/{feature}.md`),
which the orchestrator creates for FULL runs only (DIRECT and LIGHT runs
keep their files in the runtime folder `.nerv/runs/`); this command
creates no `odd/` file.

1. **Refuse if not a git repository.** Run `git rev-parse --is-inside-work-tree`.
   On failure, tell the user NERV requires a git repository and stop — do
   not create any file.

2. **Read existing config.** Read the user config (the path `nerv configure --print --json` reports as `config_path`: `~/.config/ai-workflow/config.yaml` on Linux, `%AppData%\ai-workflow\config.yaml` on Windows) (user scope) if
   present, and `<repo>/.ai-workflow/config.yaml` (project scope) if present. Merge
   per the standard precedence (project overrides user key by key). A
   `tasks:`, `release:` or `deploy:` section is not NERV's: those settings
   belong to the nerv-task and nerv-release tools, and `nerv migrate` moves
   them to the tools' own files. Never
   read the old `~/.claude/nerv/nerv.yaml` or `<repo>/.nerv/nerv.yaml` as
   configuration: if one exists without its new file, say once that
   `nerv migrate` moves it, and continue as if it were absent.

3. **Ask only for missing keys**, as ONE grouped blocking prompt (native
   `AskUserQuestion` when representable, plain-text fallback otherwise, per
   the Lossless Blocking Prompts contract). Do not ask about a key already
   resolved from either file. Candidate groups:
   - **Base branch**: free text, defaulting to the current branch's upstream
     default if detectable, else `main`. Omit if `git.base_branch` already
     resolves.
   - Skills per category (`testing`, `code`, `best-practices`,
     `architecture`, `audit`) may stay empty — do not force the user to fill
     them now; mention once that they can be added to `.ai-workflow/config.yaml`
     later.

   If every required key already resolves, skip the prompt entirely and
   proceed silently to step 4.

4. **Write `.ai-workflow/config.yaml`.** Compose the project file with `enabled: true`
   plus the resolved answers, following the schema in the approved NERV
   config (see `skills/nerv-orchestrator/SKILL.md` → Configuration
   resolution). Write no `tasks:`, `release:` or `deploy:` section.
   Append a fully commented-out `models:`
   block showing the syntax and today's plugin defaults, so the user can see
   how to override a role's model/effort without guessing a value:

   ```yaml
   # models:                           # per-role model and effort; project overrides user, key by key
   #   aoba: { model: sonnet, effort: low }
   #   asuka: { model: sonnet, effort: medium }
   #   balthasar: { model: sonnet, effort: medium }
   #   casper: { model: sonnet, effort: medium }
   #   fuyutsuki: { model: sonnet, effort: medium }
   #   gendo: { model: sonnet, effort: medium }
   #   hyuga: { model: sonnet, effort: medium }
   #   kaji: { model: opus, effort: high }
   #   kaworu: { model: sonnet, effort: medium }
   #   maya: { model: sonnet, effort: medium }
   #   melchior: { model: fable, effort: high }
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
   only). If `.ai-workflow/config.yaml` already exists, do **not** overwrite it
   without confirmation — show the diff between the existing file and the
   proposed one, and ask a single yes/no confirmation before writing. (The
   `nerv configure --init-repo` CLI never overwrites it either, and leaves
   any `tasks:` block in it untouched.) `.ai-workflow/config.yaml` is meant to be
   committed. Create `.nerv/.gitignore` with the single line `*` when it is
   missing, so the runtime folder (artifacts, the orchestrator lock, logs) is
   never committed.

5. **Bootstrap the skill registry.** Run `gentle-ai skill-registry refresh
   --cwd <repo>` to write `.atl/skill-registry.md`. If `gentle-ai` is not
   installed, say so and skip only this step. The TDD mode and test runner
   are not configured here: the orchestrator resolves them per feature
   from what the repo really has and records them in the ODD document's
   header (`skills/nerv-orchestrator/references/odd-document.md`).

6. **Offer the setup flows.** Ask one multi-select blocking prompt (native
   `AskUserQuestion` when representable, plain-text fallback otherwise, per
   the Lossless Blocking Prompts contract): which configuration to run now.
   The options, in this order, each with its command's one-line purpose:
   - `/nerv:configure` — NERV's user configuration: git, skills, models,
     repository. Always offered.
   - `nerv-task init` — the nerv-task tool's own repository setup (its
     `.ai-workflow/task.yaml`: task provider and connection check).
     Offered only when nerv-task is installed (its binary on `PATH` or in
     its default directory); when nerv-task is not installed it is not
     offered.
   - `nerv-release init` — the nerv-release tool's own repository setup
     (its `.ai-workflow/release.yaml`: release and deploy). Offered only
     when nerv-release is installed (its binary on `PATH` or in its
     default directory); when nerv-release is not installed it is not
     offered.
   - **None** — skip this step; always offered alongside the flows above.

   A flow whose tool is not installed is not offered; say once which tools
   are missing and that `nerv install --tools <tool>` installs them. NERV
   itself ships no task or release setup: the tools' own slash commands
   (`/task:setup`, `/release:setup`, `/release:deploy-setup`) do the same
   setup and are never offered here in addition to their tool's `init`.
   STOP after asking and wait. Selecting None skips this step. Run each
   selected flow in the order above, one after the other: Follow the
   selected command's file from its first step, with no arguments
   (`/nerv:configure` with `all`), or
   run the CLI command directly for `nerv-task init` and
   `nerv-release init`. A flow that stops or fails is reported and the
   next selected flow still runs.

7. **Summarize.** Print what was written (`.ai-workflow/config.yaml` path and its
   resolved keys), which
   setup flows ran and their result, and remind the user that no restart is required for the
   file itself, but the SessionStart hook only prints the activation
   header that loads the `nerv-orchestrator` skill on the **next** session
   start — this session keeps running under whatever routing was already
   active.

Do not modify any file this command or a selected setup flow does not own.
Do not launch any agent.
