# Deploy extension: `nerv deploy`

The deploy extension (`deploy`) releases what the [release extension](release-commands.md)
packaged. It requires `release`, and its git hosts and targets are providers
(`deploy-github`, `deploy-ssh-compose`, `deploy-bash`), installed with it. Install them with:

```sh
nerv install --with release,deploy,deploy-github
nerv install --with deploy-ssh-compose   # the SSH and compose target
```

| Slash command | What it does |
|---|---|
| `/deploy:run` | Picks the latest local artifact, or the one you name (`api`, `api@1.4.0`, a directory), runs `nerv deploy --dry-run`, and deploys only after your explicit yes. It reports every step and the pending ones. |
| `/deploy:ship` | Chains `/release:build` and `/deploy:run`: builds and packages (dry run, then your yes), then deploys each artifact it just packaged (dry run, then a second yes per artifact). It stops at the first refusal, failure or no, and says which artifacts are packaged but not deployed. |
| `/deploy:setup` | Writes the `deploy:` section (host, targets with their provider config, git), asks only what it cannot detect, and suggests the providers to install. |

The extension's agent, `deploy:deployer`, runs the same dry run and, with your
approved project, version and tag, the real deploy. Its skill, `deploy`, is
the contract below.

`nerv deploy` takes an artifact that `nerv release` left in
`.nerv/artifacts/<project>/<version>/` and turns it into a release. It writes
the release to git, pushes the artifact's images to their registry, creates
the release on the git host and, when you configure targets, deploys to them.
Targets are optional: rolling the version out to servers can stay with
another tool, and then `nerv deploy` connects to no server. `nerv release` never writes to git. Every
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
the tools their manifests `require` (`gh`, `docker`, `ssh`), and each
target's `preflight` operation, which refuses a bad configuration and
answers the tools that configuration implies (`sshpass` with
`ssh_auth: password`, the `infisical` CLI with `source: infisical`). A
missing tool or a refused configuration is a refusal (exit 1) with nothing
written, so a deploy never stops half-way for a missing binary after the
tag is on origin. The preflight also runs in a dry run.

The preflight runs in your repository, not in the temporary clone: it runs
before the clone exists (a refused configuration costs no clone), and the
clone has no working tree to check anyway. A provider that checks files,
such as `deploy-bash`'s scripts, therefore checks your working tree; it
checks them again in the release commit when it deploys.

## What it does

| Step | What happens |
|---|---|
| `verify-artifact` | Checks the sha256 of the notes, the CHANGELOG section, every output and every release file. A file the manifest does not list is refused. |
| `clone` | Clones the remote (`deploy.git.remote`) in a temporary directory, with its tags and `refs/notes/semantic-release*`. |
| `commit` | Writes the artifact's release files (CHANGELOG, version files) on the artifact's base commit and commits them with `deploy.git.commit_message`. |
| `tag` | Creates the lightweight tag, as semantic-release does (the artifact's tag, from the project's `tag_format`), so the channel note on the release commit is also the tag's note for semantic-release. |
| `notes` | Writes semantic-release's channel note on the release commit: ref `refs/notes/semantic-release-<tag>`, content `{"channels":["<channel>"]}` (`[null]` for the default channel). Without it, the next prerelease number of the channel would start over. |
| `push` | Pushes the branch, the tag and the note in one atomic push. It never force-pushes. |
| `publish` | Publishes the artifact's images: `docker load -i` for each image tar in `outputs/`, then `docker push` for each image of the manifest (the version, channel and `latest` tags `nerv release` recorded). Skipped when the artifact has no images, or with `deploy.publish: false`. |
| `release` | The git host's `createRelease`: a prerelease when the release is on a channel. It is skipped when `releaseExists` says the release is there. It runs before any target, so a failing server never leaves the release missing. |
| `publish:<target>` | Each target's `publish`, in `deploy.targets` order. |
| `deploy:<target>`, `verify:<target>` | Each target's `deploy`, then its `verify`, in order. |

The deploy stops at the first failure, and the report gives each step's
status: `done`, `skipped`, `failed`, `not-run` or `dry-run`.

When the artifact has images to publish, `docker` must be on PATH and the
docker config (`$DOCKER_CONFIG/config.json`, else `~/.docker/config.json`)
must hold credentials for each image's registry, in `auths` or
`credHelpers`, or set a global `credsStore` (which may hold any registry's). Both are checked before anything is cloned or pushed; a
missing login is refused with the command to run, such as
`run docker login ghcr.io`.

### Origin moved

When origin's branch has commits after the artifact's base commit, the deploy
is refused. With `--rebase`, the release commit is cherry-picked onto the new
head. A conflict, for example someone else editing the CHANGELOG, is refused.
If origin no longer contains the base commit, release again.

`--rebase` warns that the tag then points at a commit whose code was never
built: the artifact was built from the base commit. Releasing again is the
safe path; `/deploy:run` never adds `--rebase` on its own.

### Re-runs

Running the same command again is safe:

- a tag already on origin is reused when its commit holds the same release
  files and message, and a tag with other content is refused;
- a release commit already on the branch is reused;
- only the refs origin lacks are pushed;
- the images are pushed again, which uploads nothing for a tag the
  registry already has with the same digest;
- an existing git host release is skipped.

After a partial failure, fix the cause and run the same command again. It
resumes after the steps that were done.

## Configuration

The `deploy:` section of `.ai-workflow/config.yaml`. Unknown keys are refused.

```yaml
deploy:
  host: github                    # git host provider: deploy-github
  publish: true                   # push the artifact's images (the default)
  targets:                        # optional; deployed in this order
    - name: staging
      provider: ssh-compose       # target provider: deploy-ssh-compose
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
| `publish` | `true` | Push the artifact's images in the `publish` step. `false` leaves publishing to the targets, as before 5.2.0. |
| `targets` | none | The servers to deploy to. Without any, `nerv deploy` releases and publishes only; `deploy: { host: github }` is a complete configuration. |
| `targets[].name` | required | The target's name, which `--target` selects. |
| `targets[].provider` | required | The target provider's name. |
| `targets[].config` | the provider's defaults | The provider's config keys. A key the provider does not declare is refused. |
| `git.commit_message` | `chore(release): ${nextRelease.version} [skip ci]` followed by a blank line and `${nextRelease.notes}` (the `@semantic-release/git` default) | Placeholders: `${nextRelease.version}`, `${nextRelease.gitTag}`, `${nextRelease.channel}`, `${nextRelease.notes}` and the short forms `${version}`, `${tag}`, `${channel}`, `${notes}`. |
| `git.remote` | `origin` | The remote that is cloned and pushed to. |

A provider key's value comes from the target's `config`. If the target does
not set it and the manifest declares the key `scope: user`, nerv reads
`deploy.providers.<provider>.<key>` in the user configuration
(`~/.config/ai-workflow/config.yaml`); a `scope: repo` key there no longer
applies (`nerv doctor` names it, `nerv migrate --user` moves it under
`defaults.deploy`). If neither sets it, the manifest's default applies. The
user file's `defaults.deploy` is only a seed `/deploy:setup` copies into a
project; see
[configuration.md](configuration.md#extension-sections-tasks-release-deploy).

## Providers

Git hosts and targets are providers named `deploy-<name>`. Each one
ships a `provider.yaml` manifest, the shape every provider shares
([extensions.md](extensions.md)), checked against the `host` and `target`
slots of the deploy extension's `extension.yaml`:

```yaml
name: github
extension: deploy
slot: host                  # host (deploy.host) or target (deploy.targets[])
contract_version: 1
runtime: exec               # nerv deploy runs exec providers
requires: [gh]              # executables that must be on PATH
config:                     # key, scope (user or repo), type (string, int, bool), default, prompt
  - key: repo
    scope: repo
    type: string
    prompt: The repository
operations:
  releaseExists:            # host: releaseExists, createRelease
    - argv: [gh, release, view, "{tag}"]
      absent_stderr: "release not found"
  createRelease:
    - argv: [gh, release, create, "{tag}", --notes-file, "{notes_file}", {arg: --prerelease, when: prerelease}]
```

- A target implements `deploy`, and `preflight`, `publish` and `verify` or
  declares them `not_implemented`. Every operation of the slot is declared.
- An operation is a list of steps. Go runs each step's `argv` directly,
  without a shell.
- An element is either a literal or one whole `{variable}`, and the variable
  is replaced as one argument. Values such as the release notes can never
  inject a command.
- The variables are `project`, `version`, `channel`, `tag`, `base_commit`,
  `branch`, `build_provider` (the release build provider that made the
  artifact), `prerelease`, `artifact_dir`, `notes_file`,
  `changelog_file`, `target`, `nerv` (the nerv binary), `config.<key>`, and
  `item` in a step with `for_each` (`images`, `image_tars` or `outputs`).
- `when: prerelease` keeps a step or an argument only for a release on a
  channel.
- Each step reads the common JSON request on stdin: `contract` (1),
  `extension` (`deploy`), `slot`, `provider`, `operation`, `config` and
  `payload` (`target`, `artifact`: the manifest, `artifact_dir` and `item`).
- A step may print the common JSON response on stdout:
  `{"status": "ok" | "skipped" | "blocked" | "failed" | "needs_input", "message": "...", "data": {...}}`.
  nerv deploy cannot ask you anything, so `blocked` and `needs_input` fail
  the operation like `failed`, with the message. A step that prints no JSON
  succeeds or fails by its exit code.
- `releaseExists` has one step. A `data.exists` answer decides. Otherwise
  exit 0 means the release exists, and a non-zero exit whose stderr matches
  `absent_stderr` means it does not. Any other failure is an error.
- A target's `preflight` runs before anything is cloned or pushed, also in a
  dry run. It refuses a bad config with `failed` and its message, and
  answers in `data.requires` the executables that config needs besides
  `requires`.
- A step answering a status that is none of these fails the operation: it
  speaks another contract.
- A native provider, such as `deploy-ssh-compose` or `deploy-bash`, is an
  exec provider whose steps run nerv's hidden
  `{nerv} provider deploy-<name> <operation>`.
- Provider steps run in the temporary clone, except `preflight`, which runs
  in your repository (see [Preflight](#preflight)). The clone holds origin's
  commits, the release commit and its tag, but no checked-out files. The
  `gh` CLI therefore works on origin's repository, with your
  `gh auth login` session.
- A refused preflight shows the provider's stderr under its message.

### deploy-github

Slot `host`, requires `gh`. `releaseExists` runs `gh release view <tag>`.
`createRelease` runs:

```sh
gh release create <tag> --title <tag> --notes-file <notes> [--prerelease] --verify-tag
```

### deploy-ssh-compose

Slot `target`, requires `docker` and `ssh`.

- `preflight` runs `nerv provider deploy-ssh-compose preflight`: it checks
  the config and answers `sshpass` for `ssh_auth: password` and `infisical`
  for `source: infisical`.
- `publish` runs `docker load -i` for each image tar of the artifact, then
  `docker push` for each image. Log in to the registry first. With
  `deploy.publish: true` (the default) the core `publish` step has already
  pushed the same images, so this re-push uploads nothing; set
  `deploy.publish: false` to publish only from the target.
- `deploy` and `verify` run `nerv provider deploy-ssh-compose deploy|verify`, the
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

### deploy-bash

Slot `target`, requires `bash` and `git`. It runs your repository's own
scripts, through `nerv provider deploy-bash <operation>`:

| Key | Scope | Default | Meaning |
|---|---|---|---|
| `script` | repo | required | The deploy script, relative to the repository root. |
| `publish_script` | repo | none (`publish` is skipped) | Run by `publish`, before any target deploys. |
| `verify_script` | repo | none (`verify` is skipped) | Run by `verify`, after the target's deploy. |
| `timeout` | repo | none (no limit) | A duration (`10m`, `90s`) bounding each script run; past it the script's process group is killed and the operation fails. |

```yaml
deploy:
  targets:
    - name: docs
      provider: bash
      config:
        script: deploy/deploy.sh
        verify_script: deploy/check.sh
```

- `preflight` checks, in your repository, that every configured script is a
  regular file inside it: a relative path without `..`, outside `.git`, and
  no symlink leading out of the repository.
- `publish`, `deploy` and `verify` run the script of the release commit (the
  artifact's tag), never your working tree's: the script must be a regular
  file there, not a symlink. That commit's files are extracted into a
  temporary directory, all of them (`export-ignore` attributes do not
  apply) with their executable modes, and `bash <script>` runs there.
- The script reads the common JSON request on stdin, and finds the deploy
  variables in its environment:

  | Variable | Value |
  |---|---|
  | `NERV_DEPLOY_OPERATION` | `publish`, `deploy` or `verify`. |
  | `NERV_DEPLOY_TARGET` | The target's name (`deploy.targets[].name`). |
  | `NERV_DEPLOY_PROJECT`, `NERV_DEPLOY_VERSION`, `NERV_DEPLOY_TAG` | The artifact's project, version and tag. |
  | `NERV_DEPLOY_CHANNEL`, `NERV_DEPLOY_PRERELEASE` | The release channel (empty for none) and `true` or `false`. |
  | `NERV_DEPLOY_BRANCH`, `NERV_DEPLOY_BASE_COMMIT`, `NERV_DEPLOY_COMMIT` | The release branch, the artifact's base commit and the release commit. |
  | `NERV_DEPLOY_BUILD_PROVIDER` | The build provider that made the artifact. |
  | `NERV_DEPLOY_ARTIFACT_DIR`, `NERV_DEPLOY_NOTES_FILE`, `NERV_DEPLOY_CHANGELOG_FILE` | The artifact's directory, its release notes and its CHANGELOG section. |
  | `NERV_DEPLOY_IMAGES` | The artifact's image references, space-separated. |

- The script may print the common JSON response on stdout: its `status`,
  `message` and `data` are the operation's, and `failed`, `blocked` or
  `needs_input` fail the deploy with its message. Without a response, exit
  0 is `ok` and a non-zero exit fails with the script's last stderr line.
  Its stderr is shown as it runs.

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
NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_SSH_PASS=...          # never in .ai-workflow/config.yaml
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

`nerv pipeline`, its Claude Code command and the `pipeline:` section were
removed.
Its release step ran semantic-release, which nerv no longer uses: the
version engine is native, and building and releasing are now two extensions
with a local artifact between them.

| Old step | Now |
|---|---|
| `validate` | A `lint` stage of the project in the `release:` section, run by `nerv release` ([release](release-commands.md)). |
| `test` | The `build`, `test` and `coverage` stages, with `coverage` floors. |
| `release` | `nerv release` (version, CHANGELOG section, version files, packages, into the artifact), then `nerv deploy` (release commit, tag, channel note, push, GitHub release with `deploy-github`). |
| `docker` | Building and saving the images: `nerv release` (`compose` mode, or a script `package` stage). Pushing them: the `publish` step of `nerv deploy`. |
| `sync` | Not ported. Merge the release back into `develop` and `release/*` by hand; an opt-in `deploy.sync` may come later. |
| `deploy` | The `deploy` and `verify` operations of the `deploy-ssh-compose` target ([deploy.md](#deploy-ssh-compose)). |

A repository that still has a `pipeline:` section is told so: the section
is no longer read, and the commands point to `/release:setup` and
`/deploy:setup`.

### Steps

1. **Install the extensions and providers**:

   ```sh
   nerv install --with release,deploy,deploy-github,deploy-ssh-compose
   ```

2. **See the whole migration** with `nerv migrate --dry-run`: it shows
   every change to the configuration files (the move to `.ai-workflow/config.yaml`,
   the translation of the 4.2.0 `release:` and `pipeline:` into today's
   `release:` and `deploy:`, the semantic-release import, the extensions to install) before
   writing anything. `nerv migrate` applies it, with backups.
3. **Or migrate by hand** with the two setup commands: `/release:setup`
   (offers `nerv release setup --import` for the semantic-release config in
   `package.json`, and asks each project's build provider), then
   `/deploy:setup` (the host, the targets, git).
4. **Rehearse**: `/release:build` (a dry run in a clone, then the artifact on
   your yes) and `/deploy:run` (a dry run, then the deploy on your yes).
5. **Delete** the `pipeline:` section, and the semantic-release config once
   `/release:version` gives the versions you expect.

### Before: the old `pipeline:` section

dim.comprobantes.api, deploying from environment variables:

```yaml
pipeline:
  dotnet:
    solution: Dim.Comprobantes.slnx
    runsettings: coverlet.runsettings      # coverage thresholds 100/80/100
    env:                                   # no MSBuild node reuse on a shared host
      MSBUILDDISABLENODEREUSE: "1"
  # branches: omitted, the defaults are dim's (main, develop, release/*, hotfix/*)
  registry:
    host: ghcr.io
    username_env: GHCR_USER
    token_env: GHCR_TOKEN
  images:
    - name: ghcr.io/dirm-s-a/dim.comprobantes.api
      title: dim.comprobantes.api
      dockerfile: docker/Dockerfile
      build_args: { APP_VERSION: "${version}" }
    - name: ghcr.io/dirm-s-a/dim.comprobantes.api-login-broker
      title: dim.comprobantes.login-broker
      dockerfile: docker/LoginBroker.Dockerfile
      build_args: { APP_VERSION: "${version}" }
      computed_args:
        # Google's apt index keeps only the latest Chrome: pin it at build
        # time; a failed build is retried once with the version resolved again.
        CHROME_VERSION:
          run: >-
            curl -fsSL https://dl.google.com/linux/chrome/deb/dists/stable/main/binary-amd64/Packages
            | awk '/^Package: google-chrome-stable$/ { found = 1 } found && /^Version:/ { print $2; exit }'
  deploy:
    source: env
    environments: [development, staging]
    ssh:
      auth: password
      known_hosts: ~/.config/update-deploy/known_hosts
```

The same pipeline reading the deploy data from Infisical:

```yaml
pipeline:
  dotnet:
    solution: Dim.Comprobantes.slnx
    runsettings: coverlet.runsettings
  images:
    - name: ghcr.io/dirm-s-a/dim.comprobantes.api
      dockerfile: docker/Dockerfile
      build_args: { APP_VERSION: "${version}" }
  deploy:
    source: infisical
    infisical:
      project_id: 5d9af001-6928-47cb-81ce-0789904385b7
      app_path: /comprobantes-api
      domain: https://infisical.dim.com.ar
    environments: [production-green, production-blue, staging, development]
    ssh:
      known_hosts: ~/.config/update-deploy/known_hosts
    verify:
      timeout: 3m
```

### After: `release:` and `deploy:`

The same repository. The images are built by a compose file
(`docker/compose.release.yml`, building both images with `APP_VERSION`), so
the docker-compose build saves them into the artifact and `deploy-ssh-compose` pushes
them; a script `package` stage running `docker build` and `docker save`
works too.

```text
release:
  providers:
    conventional-commits:
      branches:
        - main
        - { name: release/rc, prerelease: rc, channel: beta }
        - { name: develop, prerelease: alpha, channel: alpha }
        - { name: "hotfix/*", prerelease: beta, channel: beta }
  projects:
    - name: api
      path: .
      build: docker-compose
      providers:
        docker-compose: { file: docker/compose.release.yml, test_service: tests }
      images:
        - "ghcr.io/dirm-s-a/dim.comprobantes.api:${version}"
        - "ghcr.io/dirm-s-a/dim.comprobantes.api-login-broker:${version}"
      version_files: [src/Dim.Comprobantes.Api/Dim.Comprobantes.Api.csproj]
      coverage: { line: 89, branch: 80, method: 92 }
      stages:
        restore: { run: dotnet restore Dim.Comprobantes.slnx }
        lint: { run: dotnet format Dim.Comprobantes.slnx --verify-no-changes --no-restore }

deploy:
  host: github
  targets:
    - name: production-green
      provider: ssh-compose
      config:
        environment: production-green
        source: infisical
        infisical_project_id: 5d9af001-6928-47cb-81ce-0789904385b7
        infisical_app_path: /comprobantes-api
        verify_timeout: 3m
```

And in the user configuration (`deploy.providers.ssh-compose.*`, the
`scope: user` keys): `infisical_domain: https://infisical.dim.com.ar` and
`ssh_known_hosts: ~/.config/update-deploy/known_hosts`.

### Key by key

| Old key (default) | Now |
|---|---|
| `pipeline.dotnet.solution`, `configuration` (`Release`), `warnings_as_errors` (`true`) | The `restore`, `build` and `test` stage commands, written in full (`dotnet build <solution> -c Release -warnaserror`). There are no presets. |
| `pipeline.dotnet.env` | Each stage's `env`, or `providers.bash.shared_host: true` for the build-server variables. |
| `pipeline.dotnet.runsettings`, `coverage.mode` (`enforce`), `coverage.thresholds` | `--settings` in the test stage, and the project's `coverage` `{line, branch, method}`, always enforced. `report` and `off` have no equivalent: leave `coverage` out. |
| `pipeline.branches` | `release.providers.conventional-commits.branches` (same meaning; `prerelease: true` is accepted when the branch name is a valid id). |
| `pipeline.registry.host` (`ghcr.io`), `username_env` (`GHCR_USER`), `token_env` (`GHCR_TOKEN`) | Log in with `docker login` before `nerv deploy`: the target pushes with docker's own credentials. |
| `pipeline.images[].name`, `title`, `dockerfile` (`Dockerfile`), `context` (`.`), `platform` (`linux/amd64`), `build_args`, `computed_args` (shell `bash`), `tags`, `required` | The compose file's services (or a bash `package` stage), and the project's `images` with `${version}`. The pushed tags are exactly the `images` references; the moving tags (`channel`, `latest`) are not produced automatically. |
| `pipeline.sync.enabled`, `develop` (`develop`), `release` (`release/*`) | Not ported. |
| `pipeline.release.version_files` | the project's `version_files`. |
| `pipeline.deploy.source` (`env`) | The target's `source`. |
| `pipeline.deploy.infisical.project_id`, `app_path`, `domain` | `infisical_project_id`, `infisical_app_path` (repo), `infisical_domain` (user). |
| `pipeline.deploy.selection` (`target`) | Always `target`: an environment is deployed when its `DOCKER_IMAGE_VERSION` points at the release. `branches` has no equivalent. |
| `pipeline.deploy.environments` (`production-green`, `production-blue`, `staging`, `development`) | One target per environment, in deploy order. |
| `pipeline.deploy.update_timeout` (`15m`) | `update_timeout`. |
| `pipeline.deploy.ssh.auth` (`password`), `key_file`, `known_hosts` (`~/.ssh/known_hosts`), `port` (`22`), `connect_timeout` (`10s`) | `ssh_auth`, `ssh_key_file`, `ssh_known_hosts`, `ssh_connect_timeout` (user), `ssh_port` (repo). |
| `pipeline.deploy.verify.healthy`, `version_label` (`org.opencontainers.image.version`), `timeout` (`120s`) | `verify_healthy`, `version_label`, `verify_timeout`. |

The deploy data is unchanged: the same keys, read from the same
`NERV_DEPLOY_<ENVIRONMENT>_<KEY>` variables (prefix `NERV_DEPLOY`) or the
same Infisical layout. The host keys are `DOCKER_SSH_HOST`,
`DOCKER_SSH_USER`, `DOCKER_SSH_PASS`, `DOCKER_COMPOSE_PATH` and
`DOCKER_COMPOSE_SCRIPT`; the application keys are `DOCKER_CONTAINER_NAME`,
`DOCKER_CONTAINER_VARIANT`, `DOCKER_DEPLOY_ENABLED`,
`DOCKER_DEPLOY_STRATEGY`, `DOCKER_IMAGE` and `DOCKER_IMAGE_VERSION`
([deploy.md](#deploy-data)).

The records `nerv pipeline` left in `.nerv/artifacts/pipeline/` are not
read by anything; delete the folder.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Deployed, or a dry run finished. |
| 1 | Refused, and nothing was written: an invalid configuration or artifact, a preflight failure (a missing tool or deploy value), origin moved without `--rebase`, a conflict, or no confirmation. |
| 2 | A step failed. The report says which steps were done. |
| 128 + the signal | Interrupted. |
