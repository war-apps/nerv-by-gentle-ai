---
description: Configure Nerv by Gentle-AI (user config, models, skills, repository) through guided questions — no script invocation by hand
argument-hint: [git | skills | models | repo | all]
---

# /nerv:configure

Configure Nerv by Gentle-AI through guided questions instead of hand-editing
the configuration files or invoking `nerv configure` flags yourself. Reads the current
state first, asks only about the sections in scope, and writes exactly the
changes confirmed — never more.

1. **Resolve scope.** `$ARGUMENTS` names one or more sections —
   `git`, `skills`, `models`, `repo` — space- or
   comma-separated; default `all` when empty. An unrecognized section name
   stops here and lists the valid ones.

2. **Read current state.** Run:

   ```
   nerv configure --print --json
   ```

   If `nerv` is not on PATH, tell the user to install it — `curl -fsSL
   https://raw.githubusercontent.com/war-apps/nerv-by-gentle-ai/main/scripts/install.sh
   | bash`, or `go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest`
   — and stop. Do not fall back to reading or writing the configuration files by hand.

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

   - **git** (split across two grouped questions, asked back to back):
     1. `git.base_branch` (free text), `git.worktree` (`ask` | `always` |
        `never`), `git.worktree_pattern` as a `default` / `herdr` / `custom`
        choice, in that order (herdr's own repo/slug placeholders match
        NERV's `{repo}`/`{slug}`):
        - `default` — the catalogue default `.claude/worktrees/{slug}`,
          e.g. `<repo-root>/.claude/worktrees/feature-tw-123-add-button`.
        - `herdr` — offered only when `herdr` is on PATH: read
          `[worktrees] directory` from herdr's own config.toml
          (`%APPDATA%\herdr\config.toml` on Windows,
          `~/.config/herdr/config.toml` elsewhere; default
          `~/.herdr/worktrees` when the key is absent) and store
          `<directory>/{repo}/{slug}`, e.g.
          `D:\.worktrees\my-repo\feature-tw-123-add-button`.
        - `custom` — free text using `{repo}`, `{slug}` (also `{branch}`,
          `{prefix}`, `{id}`), pre-filled with the current value.
     2. `git.branch_pattern` (free text, e.g.
        `feature/{prefix}-{id}-{slug}`), `git.commit_ref_pattern` (free
        text, e.g. `({PREFIX}-{id})`).
   - **skills** (split across two grouped questions, 4 then 1):
     `skills.testing`, `skills.code`, `skills.best-practices`,
     `skills.architecture`, then `skills.audit` — each a free-text
     comma-separated list of skill names that must exist in
     `.atl/skill-registry.md`. Also ask `critical_paths` (free text,
     comma-separated) and `artifacts.commit` (`with-change` | `at-close` |
     `never`) in the second group.
   - **models**: first print the `models` rows from step 2 as a table —
     `role`, `display_name` (the role's full character name; `role` stays
     the ID to type), `purpose`, `gentle_ai_equivalent` (`-` when empty; a
     comma-separated list when the role covers several agents), `model`,
     `effort`, `source` — and a one-line legend for the group keywords:
     `magi` = the three voters (balthasar, melchior, casper), `pilots` =
     the implementers (kaworu, shinji, asuka, rei, toji), `audit-passes` =
     the audit compiler and its pass (kaji, gendo),
     `all` = every role. The equivalent is informational only and never a
     `from:<phase>` value (native `review-*` agents are not phase keys):
     when the user picks `from:` for a single role, suggest that role's
     `from_phase` first, and suggest nothing when it is empty. Neither
     changes resolution. A `from:`
     phase is a key of `claude_phase_assignments` in `~/.gentle-ai/state.json`
     that still has an agent in gentle-ai 4.x (`jd-judge-a`, `jd-judge-b`,
     `jd-fix-agent`); only melchior (`jd-judge-b`) and balthasar (`jd-judge-a`)
     have a `from_phase`.
     The pilots claim `jd-fix-agent` because fixes route through the owning
     pilot, but they get no `from_phase`: `jd-fix-agent` is sized for
     surgical fixes, while a pilot also implements whole work units, so
     inheriting that phase would undersize its main duty. Then ask one
     grouped question for which roles to override — free text, comma-separated role names or a group keyword (`magi`,
     `pilots`, `audit-passes`, `all`), pre-filled `skip`. For each role
     confirmed, ask a grouped question (at most 2 roles per question, 2
     sub-questions each: model, effort) with:
     - model: `sonnet` | `opus` | `haiku` | `fable` | a custom `claude-...`
       id | `from:<gentle-ai-phase>` | `default` (clears the override);
     - effort: `low` | `medium` | `high` | `xhigh` | `max` (only offered
       alongside an explicit model, not with `from:` or `default`).
   - **repo**: ask path (free text, default the current repo root) and
     base branch, as one grouped question. Before asking, run
     `git -C <path> rev-parse --is-inside-work-tree`; on failure, tell the
     user the path is not a git repository and skip this section instead
     of asking further.

   Task settings are not a section of this command: when the task plugin
   is installed, `nerv configure` asks for its providers' settings, read
   from each provider's `provider.yaml`.

4. **Apply.** `--set` and `--set-model` are repeatable flags — pass every
   changed key of a batch as its own `--set`/`--set-model` occurrence on one
   invocation. For each section with at least one changed answer:
   - **git / skills / critical_paths / artifacts**: collect every
     changed key from steps above into one call —
     `nerv configure --set key=value --set key2=value2 --json`.
     An unknown key anywhere in the batch exits 1 with nothing written —
     surface that verbatim and stop applying further keys from the same
     batch until fixed.
   - **models**: one call per confirmed batch —
     `nerv configure --set-model role=model[/effort] --set-model role2=from:jd-judge-a --json`.
   - **repo**: `nerv configure --init-repo <path> --repo-base <base> --json`.
     This never overwrites an existing `.ai-workflow/config.yaml`; if one is already
     present, the result carries a `warnings` entry saying so — report it
     and move on without asking again.

   Read each call's JSON result (`changed`, `changes`, `written`,
   `warnings`, `backup`, `config_path`) and hold it for the final summary. A section with no
   changed answers makes no call at all.

5. **Skills install.** Run `nerv skills --dry-run --json`
   and list any skill reported missing (`installed: false`). Ask once, as a single yes/no
   question, whether to install them now. On yes, run `nerv skills --json`;
   on no, leave it and mention it can be re-run later.
   Skip this step entirely when `skills` was not in scope.

6. **Finish.**
   - If any `--set-model` call changed something, or the user asks for it,
     run `nerv apply-models`.
   - Offer to also run `nerv install --no-configure --no-skills`
     (one yes/no question) when any plugin-cache-affecting change was made.
   - Print a final table of every applied change (section, key, old →
     new value) across all the JSON results from step 4, each result's
     `backup` path, and the line: "Restart Claude Code for the change to
     take effect."

   Never run `gentle-ai install`. Never edit the configuration files by hand. Never
   write to any path other than the ones `nerv configure`, `nerv skills`,
   `nerv apply-models`, or `nerv install` themselves report writing.

## Non-interactive use

Under `claude -p` this command cannot ask questions. In that mode, run only
steps 1–2, print the `--print` summary as-is, and print the exact `--set` /
`--set-model` / `--init-repo` syntax the user would
need to run themselves to make the same changes — do not guess an answer
for any question and do not apply anything. For `--set` and `--set-model`,
print the repeated-flag form shown in step 4 above.
