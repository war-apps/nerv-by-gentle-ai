---
name: deploy
description: NERV deploy plugin contract. Covers the deploy section of .ai-workflow.yml (git host, targets, git), how deploy providers are found (deploy-provider.yaml), the operations a git host and a target implement, the step variables and the JSON request and response, the steps of nerv deploy with their statuses, preflight, re-runs and the dry-run-then-yes rule. Loaded by the /deploy:* commands and by the deploy:deployer agent.
---

# NERV Deploy Plugin — Contract

The deploy plugin turns a local artifact that `nerv ship` left (`ship:ship` skill, §4) into a release. It writes the release to git (release commit, tag, channel note, push), publishes and deploys the artifact to its **targets**, and creates the release on the **git host**. Every git write happens here, so an artifact that is never deployed leaves no trace in the repository. The plugin requires `ship`.

Hosts and targets are **provider plugins** named `deploy-<name>`, each with a `deploy-provider.yaml` manifest. The deploy plugin talks to no host or target itself: `nerv deploy` runs the provider's operations.

If `nerv` is not on PATH, tell the user to install it — `curl -fsSL https://raw.githubusercontent.com/war-apps/nerv-by-gentle-ai/main/scripts/install.sh | bash`, or `go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest` — and stop.

## 1. Rules

- **Only `nerv deploy`.** Never run `git tag`, `git push`, `gh release`, `docker push`, `ssh` or a host's compose script yourself.
- **Dry run first, always.** A real deploy runs only after the user's explicit yes to the release the dry run showed (project, version, tag, targets), with `--yes`.
- Never add `--rebase` on your own: it tags code that was never built from that commit. Offer it, with that warning, when the deploy is refused because origin moved.
- Agents never ask the user anything. A question goes back to the session that launched them.

## 2. Configuration

The `deploy:` section of `<repo>/.ai-workflow.yml`. Unknown keys are refused.

```yaml
deploy:
  host: github                    # git host provider: plugin deploy-github
  targets:                        # deployed in this order
    - name: staging
      provider: ssh-compose       # target provider: plugin deploy-ssh-compose
      config:
        environment: staging
  git:
    commit_message: "chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}"
    remote: origin
```

| Key | Default | Meaning |
|---|---|---|
| `host` | none (no git host release) | The git host provider's name. |
| `targets[].name` | required | The target's name, which `--target` selects. |
| `targets[].provider` | required | The target provider's name. |
| `targets[].config` | the provider's defaults | The provider's config keys. A key the provider does not declare is refused. |
| `git.commit_message` | `chore(release): ${nextRelease.version} [skip ci]`, a blank line and `${nextRelease.notes}` | Placeholders `${nextRelease.version}`, `${nextRelease.gitTag}`, `${nextRelease.channel}`, `${nextRelease.notes}` and the short forms `${version}`, `${tag}`, `${channel}`, `${notes}`. |
| `git.remote` | `origin` | The remote that is cloned and pushed to. |

A provider key's value comes from the target's `config`; when unset, from `deploy.providers.<provider>.<key>` in the user configuration (the path `nerv configure --print --json` reports as `config_path`); otherwise from the manifest's default. A `scope: user` key (credentials, local paths) belongs in the user configuration, never in the repository.

## 3. Finding providers

`nerv deploy` resolves a provider by name, `deploy-<name>`, from the providers this nerv build ships (today `deploy-github` and `deploy-ssh-compose`). A `host` or `provider` it does not know is refused before anything runs. Install the matching plugin with `nerv install --with deploy-<name>` so the configuration prompts and its description are available in Claude Code.

The manifest declares:

- `name`, `contract_version` (1) and `kind`: `git-host` or `target`.
- `requires`: the executables that must be on PATH.
- `config`: `key`, `scope` (`user` or `repo`), `type` (`string`, `int`, `bool`), `default` and the `prompt` `/deploy:setup` and `nerv configure` show.
- `operations`: a git host implements `releaseExists` and `createRelease`; a target implements `publish`, `deploy` and `verify` (`publish` and `verify` may be `not_implemented`).

## 4. Operations

- An operation is a list of steps. Go runs each step's `argv` directly, without a shell, in the temporary clone.
- An `argv` element is a literal or one whole `{variable}`, replaced as one argument, so values such as the notes can never inject a command. `{arg: <flag>, when: prerelease}` keeps an argument only for a release on a channel; `when: prerelease` on a step does the same for the step.
- Variables: `project`, `version`, `channel`, `tag`, `base_commit`, `branch`, `build_mode`, `prerelease`, `artifact_dir`, `notes_file`, `changelog_file`, `target`, `nerv` (the nerv binary), `config.<key>`, and `item` in a step with `for_each` (`images`, `image_tars` or `outputs`).
- Each step reads a JSON request on stdin: `contract_version`, `operation`, `target`, `artifact` (the manifest), `artifact_dir`, `config`, `item`.
- A step may print `{"status": "ok" | "skipped" | "failed", "message": "...", "exists": true | false}` on stdout. Without JSON, its exit code decides.
- `releaseExists` has one step. An `exists` answer decides; otherwise exit 0 means it exists, and a non-zero exit whose stderr matches `absent_stderr` means it does not. Any other failure is an error.

Adding a host or a target, such as GitLab or Gitea, is another `deploy-<name>` plugin with its manifest, with no change to `deploy`.

## 5. `nerv deploy`

```
nerv deploy --artifact <dir> [--target <name> ...] [--dry-run] [--rebase] [--yes] [--json]
```

**Preflight.** Before any push, `nerv deploy` checks every selected target's and the host's `requires` tools and the data they need (for `deploy-ssh-compose`: `sshpass` with `ssh_auth: password`, the `infisical` CLI with `source: infisical`, the environment's deploy data). A missing tool or value is a refusal (exit 1) with nothing written.

| Step | What happens |
|---|---|
| `verify-artifact` | Checks the sha256 of the notes, the CHANGELOG section, every output and every release file; a file the manifest does not list is refused. |
| `clone` | Clones `deploy.git.remote` in a temporary directory, with its tags and channel notes. |
| `commit` | Writes the release files on the artifact's base commit and commits them with `deploy.git.commit_message`. |
| `tag` | Creates the lightweight tag, as semantic-release does. |
| `notes` | Writes the channel note `refs/notes/semantic-release-<tag>`, so the next prerelease number continues. |
| `push` | Pushes the branch, the tag and the note in one atomic push; never forced. |
| `publish:<target>` | Each target's `publish`, in `deploy.targets` order. |
| `deploy:<target>`, `verify:<target>` | Each target's `deploy`, then its `verify`. |
| `release` | The host's `createRelease` (a prerelease on a channel), skipped when `releaseExists` says it is there. |

- The deploy stops at the first failure. Each step's status is `done`, `skipped`, `failed`, `not-run` or `dry-run`. `not-run` steps are the **pending** ones.
- A dry run prepares the release in the clone and shows the steps; it pushes nothing and runs no provider.
- **Origin moved** past the base commit: refused, unless `--rebase` re-applies the release files on the new head (a conflict is refused). If origin no longer has the base commit, ship again.
- **Re-runs are safe.** A tag already on origin with the same content, a release commit already on the branch and an existing host release are reused or skipped; only the refs origin lacks are pushed. After a failure, fix the cause and run the same command again.
- `--json` prints `{project, version, tag, channel, branch, dry_run, commit, steps: [{name, status, detail}], error}` on stdout; progress goes to stderr.
- Exit codes: 0 deployed or a dry run; 1 refused, nothing written (invalid configuration or artifact, preflight, origin moved, a conflict, no confirmation); 2 a step failed (the report says which were done); 128 + the signal interrupted.

## 6. Migration from `pipeline:`

`nerv pipeline` and the `pipeline:` section are removed. Its `docker` push and `deploy` steps are the `deploy-ssh-compose` target (`publish`, `deploy`, `verify`); `release` is `nerv ship` then `nerv deploy` with `deploy-github`; the `sync` back-merge is not ported, so merge the release back into `develop` by hand. `nerv migrate` translates the section.
