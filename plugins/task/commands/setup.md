---
description: Set up the tasks section of this repository's .ai-workflow/config.yaml interactively — the task provider, its repository keys and the connection check — without nerv configure's wizard
argument-hint: [provider]
---

# /task:setup

Build the `tasks:` section of `<repo>/.ai-workflow/config.yaml` for the
task extension: pick the provider, propose its repository keys, ask only
what cannot be detected, write the section, and check that the provider's
tools are reachable. It is the task extension's part of a repository's
setup on its own: it does not run `nerv configure`, `/nerv:init` or any
other part of NERV's setup, and it never sets `enabled` (NERV's own switch,
which the task commands do not need). Load the `task` skill first: its §1
(configuration) and §2 (finding providers, the tool check) are the source
of truth for every key and rule.

**Arguments.** `$ARGUMENTS` may be empty or one provider name (`teamwork`):
then that provider is picked without asking. Anything else: say what is not
accepted and stop.

Talk to the user in their language; everything written to files is in
English.

## 1. Preconditions

1. Run `git rev-parse --show-toplevel`; on failure, say the task section
   belongs to a git repository and stop. Work on the **main checkout**
   (the skill's §2), so a worktree configures the repository itself.
2. Read `.ai-workflow/config.yaml` when it exists.
   - A `tasks:` key: show it and ask **edit it** (the default) or **start
     over**. Stop and wait.
   - `tasks.provider: none`: say task tracking is off in this repository
     and ask whether to turn it on. Stop and wait.
3. Read the user config (the path `nerv configure --print --json` reports
   as `config_path`; without `nerv` on PATH, the skill's §1 paths). Hold:
   - `defaults.tasks`, the user's seed for new sections (same shape as
     `tasks:`), never applied at run time;
   - the personal keys it already sets under `tasks.providers.<name>.`.

## 2. Provider

List the task providers the skill's §2 finds: the installed ones
(`task-<name>@nerv` in `~/.claude/plugins/installed_plugins.json`) and the
repository's own (`.ai-workflow/providers/task-<name>/provider.yaml`), each
with its `name`, its `task_ref_prefix` and where it comes from.

- None: say no task provider is installed, give the install line
  `nerv install --with task,task-<provider>` (`task-teamwork` ships with
  nerv), and stop.
- One: propose it.
- Several: the one `$ARGUMENTS` or `defaults.tasks.provider` names, else
  ask which one. Stop and wait.

## 3. Propose the section

Read the chosen provider's `provider.yaml` `config` block and show the
whole `tasks:` section as one YAML block:

- `provider: <name>`.
- `providers.<name>.<key>` for each `scope: repo` key the provider
  declares, with its `prompt` as a comment. Leave out a key whose value
  equals its `default`.
- **Seed.** Copy each key of `defaults.tasks` that the section does not set
  yet, once (the repository's values always win), leaving out any
  `scope: user` key. Mark every copied key in the proposal ("from your
  `defaults.tasks`").
- `ask_when_missing`, `subtasks_per_wave`, `events` and `sources` only when
  the user asks for other values than the skill's §1 defaults.

`scope: user` keys (such as `assignee_id`) never go into the repository:
they belong in the user configuration under `tasks.providers.<name>.`. List
the ones the user config does not set yet, with their `prompt`, and say
`nerv configure` edits them.

## 4. Ask only what cannot be detected

Ask one question at a time, with a recommended default, then STOP and wait.
When the provider's tools are reachable (§6), offer its own listings to
answer them: for `teamwork`, `twprojects-list_projects` for `project_id`
and `twprojects-list_tasklists` of that project for `tasklist_id`. Read
them only; create nothing in the provider.

Show the final section in full and ask once whether to write it. Write
only on an explicit yes.

## 5. Write

- No `.ai-workflow/config.yaml`: create it with the `tasks:` section only.
- Otherwise add or replace only the `tasks:` block, from the `tasks:` line
  up to the next column-0 key; every other byte stays as it is.

Then read the file back and check it parses as YAML and that `tasks:`
holds what was shown.

## 6. Connection

Run the skill's tool check (§2) for the chosen provider against the tools
available to this session.

- Every tool reachable: say through which source (a server, the
  provider's own plugin, a claude.ai connector).
- Missing tools: name them, and when the provider's root has a `README.md`
  (`task-teamwork` does), point to its connection section and summarize its
  options in one line each. Do not change any MCP configuration.

## 7. Next steps

Tell the user, without doing any of it: set the listed `scope: user` keys
with `nerv configure`, commit `.ai-workflow/config.yaml` when ready, then
run `/task:me`.

Never commit, push, create or change anything in the task provider, or edit
any file other than `.ai-workflow/config.yaml`.
