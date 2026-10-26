---
description: Set up the deploy section of .ai-workflow/config.yaml interactively — the git host provider, the targets with their provider config, and git (commit message, remote); suggests installing deploy-github and deploy-ssh-compose
argument-hint: [--target <name>]
---

# /deploy:setup

Build the `deploy:` section of `<repo>/.ai-workflow/config.yaml` for `nerv deploy`:
detect what the repository uses, propose a complete section, ask only what
cannot be detected (one question at a time), write it, and prove it with a
dry run when an artifact exists. It never commits, pushes or deploys. Load
the `deploy` skill first: its §2 to §4 are the source of truth for every
key, default and rule.

**Arguments.** `$ARGUMENTS` may be empty or `--target <name>`: then only
that target of an existing section is added or edited. Anything else: say
what is not accepted and stop.

Talk to the user in their language; everything written to files is in
English.

## 1. Preconditions

1. Run `nerv version`. If `nerv` is not on PATH, give the install line of
   the skill's §1 and stop.
2. Run `git rev-parse --show-toplevel`; on failure, say deploy needs a git
   repository and stop.
3. Read `.ai-workflow/config.yaml`.
   - No `release:` section: say deploy consumes release's artifacts and that
     `/release:setup` comes first; stop.
   - A `deploy:` key: show it and ask **edit it** (the default) or **start
     over**. Stop and wait.
   - An old `pipeline:` key: it is no longer read. Say `nerv migrate
     --dry-run` shows its translation, and offer to build `deploy:` here
     from its `pipeline.deploy` environments instead (one
     `deploy-ssh-compose` target each). Its `pipeline.images` are not a
     `deploy:` key: release builds the images now, from the `release:` project's
     the release project's `images` (`/release:setup` writes them), and `deploy-ssh-compose`
     pushes them.

4. Read the user config (the path `nerv configure --print --json` reports
   as `config_path`). Its `defaults.deploy`, when present, has the shape of
   `deploy:` and is the user's seed for new sections: it never applies at
   run time. Its other `deploy:` keys no longer apply either, except the
   `scope: user` provider keys under `deploy.providers.<provider>.`
   (`nerv migrate --user` moves the rest under `defaults.deploy`). Hold the
   seed for §3.

Only the `deploy:` block is added or replaced, with the same rule as
`/release:setup` (from the `deploy:` line up to the next column-0 key); every
other byte stays as it is.

## 2. Providers

Read `~/.claude/plugins/installed_plugins.json` for `deploy-*@nerv`
providers. Suggest what is missing, without installing it:

- **Git host.** `origin` on GitHub (`git remote get-url origin`): suggest
  `deploy-github` (needs the `gh` CLI and `gh auth login`). Another host:
  say no provider ships for it yet, and leave `host` unset (no host
  release).
- **Targets.** A compose file with images and a host compose script, an old
  `pipeline.deploy` section, or Infisical deploy data: suggest
  `deploy-ssh-compose` (needs `docker`, `ssh`, and `sshpass` or a key, and
  the `infisical` CLI with `source: infisical`).

The install line is `nerv install --with deploy,deploy-github,deploy-ssh-compose`
(only the ones missing; `deploy` requires `release`). After installing, the
user restarts Claude Code. Continue with the providers the user has or
will install.

The repository's own deploy providers, `.ai-workflow/providers/deploy-<name>/provider.yaml`
(docs/extensions.md, "Project extensions and providers"), are available too,
with no install: list them with their `slot` and `config` keys next to the
installed ones, and offer them for `host` or a target like any other.

## 3. Propose the section

Show the whole `deploy:` section as one YAML block:

- **Seed.** Start from the user's `defaults.deploy` (§1.4): copy each of
  its keys the section does not set yet, once (the repository's values and
  what detection shows always win), leaving out any `scope: user` provider
  key. Mark every copied key in the proposal ("from your
  `defaults.deploy`"); never re-copy a default into an existing section on
  later runs unless the user asks.
- `host`: the git host provider's `name` (`github`).
- `targets`: optional. One entry per environment nerv should deploy to,
  in deploy order (staging before production), each with `name`,
  `provider` and `config` holding only the `scope: repo` keys of the
  provider's manifest that differ from their default. When the servers
  are rolled out by another tool, leave `targets` out: `nerv deploy` then
  pushes the release, publishes the artifact's images and creates the git
  host release, and connects to no server.
- `publish`: leave it out (the default `true` pushes the artifact's
  images). Write `publish: false` only when the user wants a target to be
  the only one that publishes.
- `git.commit_message` and `git.remote` only when the repository needs
  other values than the defaults (an imported `@semantic-release/git`
  `message` goes here).

`scope: user` keys (`ssh_auth`, `ssh_key_file`, `ssh_known_hosts`,
`ssh_connect_timeout`, `infisical_domain`) never go into the repository:
they belong in the user configuration under `deploy.providers.<provider>.`,
which `nerv configure` edits. List the ones the user still has to set.

## 4. Ask only what cannot be detected

ONE question at a time, with a recommended default, then STOP and wait.
Typical questions: the environments and their order, `source` (`env` or
`infisical`) and its project id and app path, the SSH port, whether to
create a git host release.

Show the final section in full and ask once whether to write it. Write
only on an explicit yes.

## 5. Write and validate

1. Write the section.
2. When `.nerv/artifacts/` holds an artifact, run `nerv deploy --artifact
   <newest> --dry-run --json` and show the steps; a configuration error is
   path-qualified: fix the section and rerun, at most two rounds. A
   preflight refusal names a missing tool or value: say what to install or
   set. No artifact: say the section is validated by the first `/deploy:run`.

## 6. Next steps

Tell the user, without doing any of it: set the user-scope keys with `nerv
configure`, run `/release:build`, then `/deploy:run`.

Never commit, push, run `nerv deploy` without `--dry-run`, or edit any file
other than `.ai-workflow/config.yaml`.
