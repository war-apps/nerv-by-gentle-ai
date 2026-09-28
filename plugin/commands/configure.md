---
description: Configure NERV Gentle-AI (user config, models, skills, repository) through guided questions — no script invocation by hand
argument-hint: [git | tasks | skills | models | repo | commands | all]
---

# /nerv:configure

Configure NERV Gentle-AI through guided questions instead of hand-editing
`nerv.yaml` or invoking `tools/configure.ps1` yourself. Reads the current
state first, asks only about the sections in scope, and writes exactly the
changes confirmed — never more.

1. **Resolve scope.** `$ARGUMENTS` names one or more sections —
   `git`, `tasks`, `skills`, `models`, `repo`, `commands` — space- or
   comma-separated; default `all` when empty. An unrecognized section name
   stops here and lists the valid ones.

2. **Read current state.** Run:

   ```
   pwsh -NoProfile -File "${CLAUDE_PLUGIN_ROOT}/tools/configure.ps1" -Print
   ```

   If the script is missing or the invocation fails to start (not merely a
   non-zero exit with parseable JSON), tell the user the plugin cache looks
   stale — suggest `pwsh tools/install.ps1 -RefreshCache` — and stop. Do
   not fall back to reading or writing `nerv.yaml` by hand.

   From the returned JSON print a compact summary: `config_path` and
   whether it `exists`; `prerequisites` (`gentle_ai`, `engram`, `claude`);
   and, for each section in scope, a small current-vs-default table built
   from `values`, `defaults`, `models`, and `skills_status`. This is the
   baseline every question below pre-fills from.

3. **Ask, one grouped question per section in scope.** Use native
   `AskUserQuestion` when representable, plain-text fallback otherwise, per
   the Lossless Blocking Prompts contract. Every sub-question pre-fills the
   current value (from step 2, falling back to the printed default) as the
   recommended option, and offers the documented allowed values for that
   key; use free text for an id, path, or pattern. A grouped question holds
   at most 4 sub-questions — split a section across multiple grouped
   questions, asked back to back, when it has more fields than that.
   **Never assume an answer for any sub-question; STOP after asking and
   wait.** A skipped or kept-as-shown answer produces no `-Set` for that
   key.

   - **git** (one grouped question, 4 sub-questions): `git.base_branch`
     (free text), `git.worktree` (`ask` | `always` | `never`),
     `git.branch_pattern` (free text, e.g.
     `feature/{prefix}-{id}-{slug}`), `git.commit_ref_pattern` (free text,
     e.g. `({PREFIX}-{id})`).
   - **tasks** (split across grouped questions, in this order — omit a
     later one whose condition doesn't hold):
     1. General: `tasks.provider` (`teamwork` | `github-projects` |
        `jira` | `none`), `tasks.ask_when_missing` (yes/no),
        `tasks.subtasks_per_wave` (yes/no), `tasks.timer_store` (free
        text path).
     2. General, continued: `tasks.rounding_minutes` (free text, e.g.
        `15`), and — only when the resolved provider is `teamwork` —
        `tasks.providers.teamwork.task_ref_prefix` (free text, default
        `tw`).
     3. Teamwork ids (only when the resolved provider is `teamwork`):
        `.assignee_id`, `.default_project_id`, `.default_tasklist_id`
        (all free text ids). When the Teamwork MCP is reachable, offer to
        list candidates first (`teamwork_list_projects`,
        `teamwork_list_tasklists`) so the user picks rather than types raw
        ids; fall back to free text otherwise.
     4. Teamwork stages (only when the resolved provider is `teamwork`),
        split across two grouped questions of at most 4 fields each:
        `.stages.inDev`, `.stages.testing`, `.stages.implemented`,
        `.stages.blocked`, then `.stages.canceled`, `.stages.pending`,
        `.stages.analysis` — each free text, defaulting to the current
        stage name.
   - **skills** (split across two grouped questions, 4 then 1):
     `skills.testing`, `skills.code`, `skills.best-practices`,
     `skills.architecture`, then `skills.audit` — each a free-text
     comma-separated list of skill names that must exist in
     `.atl/skill-registry.md`. Also ask `critical_paths` (free text,
     comma-separated) and `artifacts.commit` (`with-change` | `at-close` |
     `never`) in the second group.
   - **models**: ask one grouped question for which roles to override —
     free text, comma-separated role names or a group keyword (`magi`,
     `pilots`, `kaji-passes`, `all`), pre-filled `skip`. For each role
     confirmed, ask a grouped question (at most 2 roles per question, 2
     sub-questions each: model, effort) with:
     - model: `sonnet` | `opus` | `haiku` | `fable` | a custom `claude-...`
       id | `from:<gentle-ai-phase>` | `default` (clears the override);
     - effort: `low` | `medium` | `high` | `xhigh` | `max` (only offered
       alongside an explicit model, not with `from:` or `default`).
   - **repo**: ask path (free text, default the current repo root), base
     branch, task provider, and — only when the chosen provider is
     `teamwork` — project id and tasklist id, as one grouped question (4
     sub-questions; split the ids into a second question when the
     provider is `teamwork` and both are unset). Before asking, run
     `git -C <path> rev-parse --is-inside-work-tree`; on failure, tell the
     user the path is not a git repository and skip this section instead
     of asking further.
   - **commands**: one yes/no confirmation — install the Teamwork `/task:*`
     procedures into `~/.claude/commands/task/` (never overwriting an
     existing file there)?

4. **Apply.** For each section with at least one changed answer:
   - **git / tasks / skills / critical_paths / artifacts**: collect every
     changed key from steps above into one call —
     `pwsh -NoProfile -File "${CLAUDE_PLUGIN_ROOT}/tools/configure.ps1" -Set key=value [-Set key2=value2 ...] -Json`.
     An unknown key exits 1 with nothing written — surface that verbatim
     and stop applying further `-Set`s from the same batch until fixed.
   - **models**: one call per confirmed batch —
     `pwsh -NoProfile -File "${CLAUDE_PLUGIN_ROOT}/tools/configure.ps1" -SetModel role=model[/effort] [-SetModel role2=from:<phase>] -Json`.
   - **repo**: `pwsh -NoProfile -File "${CLAUDE_PLUGIN_ROOT}/tools/configure.ps1" -InitRepo <path> -RepoBase <base> -RepoProvider <provider> [-RepoProjectId <id> -RepoTasklistId <id>] -Json`.
     This never overwrites an existing `.nerv/nerv.yaml`; if one is already
     present, the result carries a `warnings` entry saying so — report it
     and move on without asking again.
   - **commands** (only on a yes): `pwsh -NoProfile -File "${CLAUDE_PLUGIN_ROOT}/tools/configure.ps1" -InstallCommands -Json`.

   Read each call's JSON result (`changed`, `changes`, `written`,
   `warnings`, `backup`, `config_path`) and hold it for the final summary. A section with no
   changed answers makes no call at all.

5. **Skills install.** Run
   `pwsh -NoProfile -File "${CLAUDE_PLUGIN_ROOT}/tools/install-skills.ps1" -DryRun -Json`
   and list any skill reported missing (`installed: false`). Ask once, as a single yes/no
   question, whether to install them now. On yes, run the same command
   without `-DryRun`; on no, leave it and mention it can be re-run later.
   Skip this step entirely when `skills` was not in scope.

6. **Finish.**
   - If any `-SetModel` call changed something, or the user asks for it,
     run `pwsh -NoProfile -File "${CLAUDE_PLUGIN_ROOT}/tools/install.ps1" -ApplyModels`.
   - Offer to also run
     `pwsh -NoProfile -File "${CLAUDE_PLUGIN_ROOT}/tools/install.ps1" -RefreshCache`
     (one yes/no question) when any plugin-cache-affecting change was made.
   - Print a final table of every applied change (section, key, old →
     new value) across all the JSON results from step 4, each result's
     `backup` path, and the line: "Restart Claude Code for the change to
     take effect."

   Never run `gentle-ai install`. Never edit `nerv.yaml` by hand. Never
   write to any path other than the ones `configure.ps1`,
   `install-skills.ps1`, or `install.ps1` themselves report writing.

## Non-interactive use

Under `claude -p` this command cannot ask questions. In that mode, run only
steps 1–2, print the `-Print` summary as-is, and print the exact `-Set` /
`-SetModel` / `-InitRepo` / `-InstallCommands` syntax the user would need
to run themselves to make the same changes — do not guess an answer for
any question and do not apply anything.
