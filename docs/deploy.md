# Deploy plugin: `nerv deploy`

The deploy plugin (`deploy@nerv`) releases what the [ship plugin](release-commands.md)
packaged. It requires `ship`, and its git hosts and targets are provider
plugins (`deploy-github`, `deploy-ssh-compose`). Install them with:

```sh
nerv install --with ship,deploy,deploy-github
nerv install --with deploy-ssh-compose   # the SSH and compose target
```

| Slash command | What it does |
|---|---|
| `/deploy:run` | Picks the latest local artifact, or the one you name (`api`, `api@1.4.0`, a directory), runs `nerv deploy --dry-run`, and deploys only after your explicit yes. It reports every step and the pending ones. |
| `/deploy:setup` | Writes the `deploy:` section (host, targets with their provider config, git), asks only what it cannot detect, and suggests the provider plugins to install. |

The plugin's agent, `deploy:deployer`, runs the same dry run and, with your
approved project, version and tag, the real deploy. Its skill, `deploy`, is
the contract below.

`nerv deploy` takes an artifact that `nerv ship` left in
`.nerv/artifacts/<project>/<version>/` and turns it into a release. It writes
the release to git, publishes and deploys the artifact to its targets, and
creates the release on the git host. `nerv ship` never writes to git. Every
git write happens here, so an artifact that is never deployed leaves no trace
in the repository.

```sh
nerv deploy --artifact .nerv/artifacts/api/1.4.0 --dry-run
nerv deploy --artifact .nerv/artifacts/api/1.4.0
```

Run a dry run first. Without `--yes`, a real run shows the prepared release
and asks before it pushes anything.

### Preflight

Before any push, `nerv deploy` checks every selected target and the host:
the tools their manifests `require` (`gh`, `docker`, `ssh`), the tools
their configuration implies (`sshpass` with `ssh_auth: password`, the
`infisical` CLI with `source: infisical`), and the data they need (each
environment's deploy data). A missing tool or value is a refusal (exit 1)
with nothing written, so a deploy never stops half-way for a missing
binary after the tag is on origin.

## What it does

| Step | What happens |
|---|---|
| `verify-artifact` | Checks the sha256 of the notes, the CHANGELOG section, every output and every release file. A file the manifest does not list is refused. |
| `clone` | Clones the remote (`deploy.git.remote`) in a temporary directory, with its tags and `refs/notes/semantic-release*`. |
| `commit` | Writes the artifact's release files (CHANGELOG, version files) on the artifact's base commit and commits them with `deploy.git.commit_message`. |
| `tag` | Creates the annotated tag (the artifact's tag, from the project's `tag_format`). |
| `notes` | Writes semantic-release's channel note on the release commit: ref `refs/notes/semantic-release-<tag>`, content `{"channels":["<channel>"]}` (`[null]` for the default channel). Without it, the next prerelease number of the channel would start over. |
| `push` | Pushes the branch, the tag and the note in one atomic push. It never force-pushes. |
| `publish:<target>` | Each target's `publish`, in `deploy.targets` order. |
| `deploy:<target>`, `verify:<target>` | Each target's `deploy`, then its `verify`, in order. |
| `release` | The git host's `createRelease`: a prerelease when the release is on a channel. It is skipped when `releaseExists` says the release is there. |

The deploy stops at the first failure, and the report gives each step's
status: `done`, `skipped`, `failed`, `not-run` or `dry-run`.

### Origin moved

When origin's branch has commits after the artifact's base commit, the deploy
is refused. With `--rebase`, the release commit is cherry-picked onto the new
head. A conflict, for example someone else editing the CHANGELOG, is refused.
If origin no longer contains the base commit, ship again.

`--rebase` warns that the tag then points at a commit whose code was never
built: the artifact was built from the base commit. Shipping again is the
safe path; `/deploy:run` never adds `--rebase` on its own.

### Re-runs

Running the same command again is safe:

- a tag already on origin is reused when its commit holds the same release
  files and message, and a tag with other content is refused;
- a release commit already on the branch is reused;
- only the refs origin lacks are pushed;
- an existing git host release is skipped.

After a partial failure, fix the cause and run the same command again. It
resumes after the steps that were done.

## Configuration

The `deploy:` section of `.ai-workflow.yml`. Unknown keys are refused.

```yaml
deploy:
  host: github                    # git host provider: plugin deploy-github
  targets:                        # deployed in this order
    - name: staging
      provider: ssh-compose       # target provider: plugin deploy-ssh-compose
      config:
        environment: staging
    - name: production
      provider: ssh-compose
      config:
        environment: production
        source: infisical
        infisical_project_id: 0f3c...
        infisical_app_path: /my-api
  git:
    commit_message: "chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}"
    remote: origin
```

| Key | Default | Meaning |
|---|---|---|
| `host` | none (no release is created) | The git host provider's name. |
| `targets[].name` | required | The target's name, which `--target` selects. |
| `targets[].provider` | required | The target provider's name. |
| `targets[].config` | the provider's defaults | The provider's config keys. A key the provider does not declare is refused. |
| `git.commit_message` | `chore(release): ${nextRelease.version} [skip ci]` followed by a blank line and `${nextRelease.notes}` (the `@semantic-release/git` default) | Placeholders: `${nextRelease.version}`, `${nextRelease.gitTag}`, `${nextRelease.channel}`, `${nextRelease.notes}` and the short forms `${version}`, `${tag}`, `${channel}`, `${notes}`. |
| `git.remote` | `origin` | The remote that is cloned and pushed to. |

A provider key's value comes from the target's `config`. If the target does
not set it, nerv reads `deploy.providers.<provider>.<key>` in the user
configuration (`~/.config/ai-workflow/config.yml`). If neither sets it, the
manifest's default applies.

## Providers

Git hosts and targets are provider plugins named `deploy-<name>`. Each one
ships a `deploy-provider.yaml` manifest:

```yaml
name: github
contract_version: 1
kind: git-host              # git-host or target
requires: [gh]              # executables that must be on PATH
config:                     # key, scope (user or repo), type (string, int, bool), default, prompt
  - key: repo
    scope: repo
    type: string
    prompt: The repository
operations:
  releaseExists:            # git-host: releaseExists, createRelease
    - argv: [gh, release, view, "{tag}"]
      absent_stderr: "release not found"
  createRelease:
    - argv: [gh, release, create, "{tag}", --notes-file, "{notes_file}", {arg: --prerelease, when: prerelease}]
```

- A target implements `publish`, `deploy` and `verify`. `publish` and
  `verify` may be declared `not_implemented`.
- An operation is a list of steps. Go runs each step's `argv` directly,
  without a shell.
- An element is either a literal or one whole `{variable}`, and the variable
  is replaced as one argument. Values such as the release notes can never
  inject a command.
- The variables are `project`, `version`, `channel`, `tag`, `base_commit`,
  `branch`, `build_mode`, `prerelease`, `artifact_dir`, `notes_file`,
  `changelog_file`, `target`, `nerv` (the nerv binary), `config.<key>`, and
  `item` in a step with `for_each` (`images`, `image_tars` or `outputs`).
- `when: prerelease` keeps a step or an argument only for a release on a
  channel.
- Each step reads a JSON request on stdin: `contract_version`, `operation`,
  `target`, `artifact` (the manifest), `artifact_dir`, `config` and `item`.
- A step may print a JSON response on stdout:
  `{"status": "ok" | "skipped" | "failed", "message": "...", "exists": true | false}`.
  A step that prints no JSON succeeds or fails by its exit code.
- `releaseExists` has one step. An `exists` answer decides. Otherwise exit 0
  means the release exists, and a non-zero exit whose stderr matches
  `absent_stderr` means it does not. Any other failure is an error.
- Provider steps run in the temporary clone. The `gh` CLI therefore works on
  origin's repository, with your `gh auth login` session.

### deploy-github

Kind `git-host`, requires `gh`. `releaseExists` runs `gh release view <tag>`.
`createRelease` runs:

```sh
gh release create <tag> --title <tag> --notes-file <notes> [--prerelease] --verify-tag
```

### deploy-ssh-compose

Kind `target`, requires `docker` and `ssh`.

- `publish` runs `docker load -i` for each image tar of the artifact, then
  `docker push` for each image. Log in to the registry first.
- `deploy` and `verify` run `nerv deploy-ssh-compose deploy|verify`, the
  update-deploy flow of the removed `nerv pipeline deploy`.
  - It reads the environment's deploy data, either from the
    `NERV_DEPLOY_<ENVIRONMENT>_<KEY>` variables (`source: env`) or from
    Infisical (`source: infisical`, which needs the `infisical` CLI); see
    [Deploy data](#deploy-data).
  - `deploy` runs the host's compose script over SSH: `pull`, then `up`.
    `ssh_auth: password` needs `sshpass`.
  - `verify` waits until the container is healthy and checks that its
    version label is the released version.
  - An environment whose `DOCKER_DEPLOY_ENABLED` is off, or whose
    `DOCKER_IMAGE_VERSION` points at other releases, is skipped. One that
    sets no `DOCKER_IMAGE_VERSION` is refused, so a prerelease never
    reaches an environment by default.

| Key | Scope | Default |
|---|---|---|
| `environment` | repo | required |
| `source` | repo | `env` |
| `infisical_project_id`, `infisical_app_path` | repo | required by `source: infisical` |
| `infisical_domain` | user | the CLI session's |
| `ssh_auth` | user | `password` |
| `ssh_key_file` | user | required by `ssh_auth: key` |
| `ssh_known_hosts` | user | `~/.ssh/known_hosts` |
| `ssh_port` | repo | `22` |
| `ssh_connect_timeout` | user | `10s` |
| `update_timeout` | repo | `15m` |
| `verify_healthy` | repo | `true` |
| `version_label` | repo | `org.opencontainers.image.version` |
| `verify_timeout` | repo | `120s` |

## Deploy data

The `deploy-ssh-compose` target reads one environment's deploy data
(`config.environment`). Both sources provide the same keys per environment. Each environment has
one Docker host:

| Key | Meaning |
|---|---|
| `DOCKER_SSH_HOST` | The Docker host. |
| `DOCKER_SSH_USER` | The SSH user. |
| `DOCKER_SSH_PASS` | The SSH password (auth `password`). |
| `DOCKER_COMPOSE_PATH` | The directory of the compose script. |
| `DOCKER_COMPOSE_SCRIPT` | The script run as `<script> pull --container <c>` and `<script> up --container <c>`, e.g. `./manage.sh`. |

and the application's own keys:

| Key | Meaning |
|---|---|
| `DOCKER_CONTAINER_NAME` | Base container name; the container is `<name>-<variant>`. |
| `DOCKER_CONTAINER_VARIANT` | The variant (`alpha`, `beta`, `blue`, `green`, ...). |
| `DOCKER_DEPLOY_ENABLED` | `false`, `0`, `no` or `off` skips the environment; default enabled. |
| `DOCKER_DEPLOY_STRATEGY` | `rolling` or `blue-green` (a blue-green live container is warned about). |
| `DOCKER_IMAGE` | The image the environment runs. |
| `DOCKER_IMAGE_VERSION` | The target version: a version, or a moving tag such as `alpha`. |

**Source `env`.** Each key is read from the variable
`NERV_DEPLOY_<ENVIRONMENT>_<KEY>`, where `<ENVIRONMENT>` is the environment
name upper-cased with `-` turned into `_`; the prefix is `NERV_DEPLOY`. For
`production-blue`:

```bash
NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_SSH_HOST=10.0.0.10
NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_SSH_USER=deploy
NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_SSH_PASS=...          # never in .ai-workflow.yml
NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_COMPOSE_PATH=/srv/apps
NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_COMPOSE_SCRIPT=./manage.sh
NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_CONTAINER_NAME=comprobantes-api
NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_CONTAINER_VARIANT=blue
NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_IMAGE_VERSION=latest   # required
```

An environment without `DOCKER_IMAGE_VERSION` is refused: nerv cannot
tell whether the release is its target.

Two environments whose names map to the same prefix (`production-blue` and
`production_blue`) are rejected.

**Source `infisical`.** dim's single-folder layout, read with the
`infisical` CLI (raw values: no imports, no expansion, no personal
overrides):

```
[environment: development | staging | production-blue | production-green]
  /                 DOCKER_SSH_HOST, DOCKER_SSH_USER, DOCKER_SSH_PASS,
                    DOCKER_COMPOSE_PATH, DOCKER_COMPOSE_SCRIPT
  /<app_path>/      DOCKER_CONTAINER_NAME, DOCKER_CONTAINER_VARIANT, DOCKER_IMAGE,
                    DOCKER_IMAGE_VERSION, DOCKER_DEPLOY_STRATEGY, DOCKER_DEPLOY_ENABLED
```

## Migrating from `nerv pipeline`

The `pipeline:` section's `docker` push and `deploy` steps are this
plugin's `deploy-ssh-compose` target, and its `release` step is `nerv ship`
then `nerv deploy` with `deploy-github`. The key-by-key translation is in
[pipeline.md](pipeline.md); `nerv migrate --dry-run` shows it for your
repository.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Deployed, or a dry run finished. |
| 1 | Refused, and nothing was written: an invalid configuration or artifact, a preflight failure (a missing tool or deploy value), origin moved without `--rebase`, a conflict, or no confirmation. |
| 2 | A step failed. The report says which steps were done. |
| 128 + the signal | Interrupted. |
