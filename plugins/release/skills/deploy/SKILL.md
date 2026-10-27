---
name: deploy
description: NERV deploy contract (the deploy half of the release extension). Covers the deploy section of .ai-workflow/config.yaml (git host, targets, git), how deploy providers are found (provider.yaml), the operations a git host and a target implement, the step variables and the JSON request and response, the steps of nerv deploy with their statuses, preflight, re-runs and the dry-run-then-yes rule. Loaded by the /release:deploy* commands and by the release:deployer agent.
---

# NERV Deploy Extension — Contract

`nerv deploy`, the deploy half of the release extension, turns a local artifact that `nerv release` left (`release:release` skill, §4) into a release. It writes the release to git (release commit, tag, channel note, push), pushes the artifact's images to their registry (the `publish` step), creates the release on the **git host**, and then deploys to its **targets**, which are optional: without any, nothing connects to a server. Every git write happens here, so an artifact that is never deployed leaves no trace in the repository.

Hosts and targets are **providers** named `release-<name>`, each with a `provider.yaml` manifest: the shape every provider of every extension shares, checked against the `host` and `target` slots of `plugins/release/extension.yaml` (`docs/extensions.md`). `nerv deploy` talks to no host or target itself: `nerv deploy` runs the provider's operations.

If `nerv` is not on PATH, tell the user to install it — `curl -fsSL https://raw.githubusercontent.com/war-apps/nerv-by-gentle-ai/main/scripts/install.sh | bash`, or `go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest` — and stop.

## 1. Rules

- **Only `nerv deploy`.** Never run `git tag`, `git push`, `gh release`, `docker push`, `ssh` or a host's compose script yourself.
- **Dry run first, always.** A real deploy runs only after the user's explicit yes to the release the dry run showed (project, version, tag, targets), with `--yes`.
- Never add `--rebase` on your own: it tags code that was never built from that commit. Offer it, with that warning, when the deploy is refused because origin moved.
- Agents never ask the user anything. A question goes back to the session that launched them.

## 2. Configuration

The `deploy:` section of `<repo>/.ai-workflow/config.yaml`. Unknown keys are refused.

```yaml
deploy:
  host: github                    # git host provider: release-github
  publish: true                   # push the artifact's images (the default)
  targets:                        # optional; deployed in this order
    - name: staging
      provider: ssh-compose       # target provider: release-ssh-compose
      config:
        environment: staging
  git:
    commit_message: "chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}"
    remote: origin
```

| Key | Default | Meaning |
|---|---|---|
| `host` | none (no git host release) | The git host provider's name. |
| `publish` | `true` | Push the artifact's images in the `publish` step; `false` leaves it to the targets' `publish`. |
| `targets` | none | The servers to deploy to; none releases and publishes only. |
| `targets[].name` | required | The target's name, which `--target` selects. |
| `targets[].provider` | required | The target provider's name. |
| `targets[].config` | the provider's defaults | The provider's config keys. A key the provider does not declare is refused. |
| `git.commit_message` | `chore(release): ${nextRelease.version} [skip ci]`, a blank line and `${nextRelease.notes}` | Placeholders `${nextRelease.version}`, `${nextRelease.gitTag}`, `${nextRelease.channel}`, `${nextRelease.notes}` and the short forms `${version}`, `${tag}`, `${channel}`, `${notes}`. |
| `git.remote` | `origin` | The remote that is cloned and pushed to. |

A provider key's value comes from the target's `config`; when unset, from `deploy.providers.<provider>.<key>` in the user configuration (the path `nerv configure --print --json` reports as `config_path`); otherwise from the manifest's default. A `scope: user` key (credentials, local paths) belongs in the user configuration, never in the repository.

## 3. Finding providers

`nerv deploy` resolves a provider by name, `release-<name>`, from the providers this nerv build ships (today `release-github`, `release-ssh-compose` and `release-script`). A `host` or `provider` it does not know is refused before anything runs. Install the matching provider with `nerv install --with release-<name>` so the configuration prompts and its description are available in Claude Code.

The manifest declares:

- `name`, `extension: deploy`, `slot` (`host`: one git host, `deploy.host`; or `target`: one provider per `deploy.targets` entry), `contract_version` (1) and `runtime: exec`.
- `requires`: the executables that must be on PATH.
- `config`: `key`, `scope` (`user` or `repo`), `type` (`string`, `int`, `bool`), `default` and the `prompt` `/release:deploy-setup` and `nerv configure` show.
- `operations`: every operation of its slot. A host implements `releaseExists` and `createRelease`; a target implements `deploy`, and `preflight`, `publish` and `verify` or declares them `not_implemented`.

## 4. Operations

- An operation is a list of steps. Go runs each step's `argv` directly, without a shell, in the temporary clone.
- An `argv` element is a literal or one whole `{variable}`, replaced as one argument, so values such as the notes can never inject a command. `{arg: <flag>, when: prerelease}` keeps an argument only for a release on a channel; `when: prerelease` on a step does the same for the step.
- Variables: `project`, `version`, `channel`, `tag`, `base_commit`, `branch`, `build_provider` (`docker-compose` or `bash`), `prerelease`, `artifact_dir`, `notes_file`, `changelog_file`, `target`, `nerv` (the nerv binary), `config.<key>`, and `item` in a step with `for_each` (`images`, `image_tars` or `outputs`).
- Each step reads the common JSON request on stdin: `contract` (1), `extension` (`deploy`), `slot`, `provider`, `operation`, `config` (the provider's resolved config) and `payload`: `target`, `artifact` (the manifest), `artifact_dir`, `item`.
- A step may print the common response on stdout: `{"status": "ok" | "skipped" | "blocked" | "failed" | "needs_input", "message": "...", "data": {...}}`. `nerv deploy` cannot ask the user, so `blocked` and `needs_input` fail the operation like `failed`, with the message. Without JSON, its exit code decides.
- `releaseExists` has one step. A `data.exists` answer decides; otherwise exit 0 means it exists, and a non-zero exit whose stderr matches `absent_stderr` means it does not. Any other failure is an error.
- `preflight` runs once per target before anything is cloned or pushed (also in a dry run). It refuses a bad config with `failed` and its message, and answers in `data.requires` the executables that config needs besides `requires`.
- A **native** provider, such as `release-ssh-compose`, is an exec provider whose steps run the hidden `{nerv} provider release-<name> <operation>`.

Adding a host or a target, such as GitLab or Gitea, is another `release-<name>` provider with its manifest, with no change to `deploy`.

## 5. `nerv deploy`

```
nerv deploy --artifact <dir> [--target <name> ...] [--dry-run] [--rebase] [--yes] [--json]
```

**Preflight.** Before any push, `nerv deploy` runs every selected target's `preflight` and checks the host's and the targets' `requires` tools plus the tools each preflight answers (for `release-ssh-compose`: `sshpass` with `ssh_auth: password`, the `infisical` CLI with `source: infisical`). With images to publish, it also needs `docker` and credentials for each image's registry in the docker config (`auths`, `credHelpers`, or a global `credsStore`); without them it refuses with the login to run (`run docker login ghcr.io`). A missing tool or a refused config is a refusal (exit 1) with nothing written.

| Step | What happens |
|---|---|
| `verify-artifact` | Checks the sha256 of the notes, the CHANGELOG section, every output and every release file; a file the manifest does not list is refused. |
| `clone` | Clones `deploy.git.remote` in a temporary directory, with its tags and channel notes. |
| `commit` | Writes the release files on the artifact's base commit and commits them with `deploy.git.commit_message`. |
| `tag` | Creates the lightweight tag, as semantic-release does. |
| `notes` | Writes the channel note `refs/notes/semantic-release-<tag>`, so the next prerelease number continues. |
| `push` | Pushes the branch, the tag and the note in one atomic push; never forced. |
| `publish` | `docker load -i` of each image tar, then `docker push` of each image of the manifest; skipped without images or with `deploy.publish: false`. |
| `release` | The host's `createRelease` (a prerelease on a channel), skipped when `releaseExists` says it is there. It runs before the targets, so a failing server never leaves it missing. |
| `publish:<target>` | Each target's `publish`, in `deploy.targets` order (with `deploy.publish: true`, `release-ssh-compose`'s re-push uploads nothing). |
| `deploy:<target>`, `verify:<target>` | Each target's `deploy`, then its `verify`. |

- The deploy stops at the first failure. Each step's status is `done`, `skipped`, `failed`, `not-run` or `dry-run`. `not-run` steps are the **pending** ones.
- A dry run prepares the release in the clone and shows the steps; it pushes nothing and runs no provider operation but the targets' `preflight`.
- **Origin moved** past the base commit: refused, unless `--rebase` re-applies the release files on the new head (a conflict is refused). If origin no longer has the base commit, release again.
- **Re-runs are safe.** A tag already on origin with the same content, a release commit already on the branch and an existing host release are reused or skipped; only the refs origin lacks are pushed. After a failure, fix the cause and run the same command again.
- `--json` prints `{project, version, tag, channel, branch, dry_run, commit, steps: [{name, status, detail}], error}` on stdout; progress goes to stderr.
- Exit codes: 0 deployed or a dry run; 1 refused, nothing written (invalid configuration or artifact, preflight, origin moved, a conflict, no confirmation); 2 a step failed (the report says which were done); 128 + the signal interrupted.

## 6. Migration from `pipeline:`

`nerv pipeline` and the `pipeline:` section are removed. Its `docker` push is the `publish` step of `nerv deploy`, and its `deploy` step is the `release-ssh-compose` target (`deploy`, `verify`); `release` is `nerv release` then `nerv deploy` with `release-github`; the `sync` back-merge is not ported, so merge the release back into `develop` by hand. `nerv migrate` translates the section.
