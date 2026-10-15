# Docker pipeline configuration (`pipeline:`)

The `pipeline:` section of the project-scope `.nerv/nerv.yaml` configures
`nerv pipeline`, a standalone copy of dim.comprobantes.api's GitHub Actions
CI/CD (`.github/workflows/ci-cd.yml`) for repositories that deploy a Docker
image, plus the Docker deploy that dim runs with `/update-deploy`. Every step
runs locally, one process per step, without GitHub Actions; any GitHub
workflow in the repository is ignored.

> **Status.** `nerv pipeline validate` and `nerv pipeline test` are
> available ([Running the steps](#running-the-steps)); `release`, `docker`,
> `sync`, `deploy` and `nerv pipeline run` come in later releases.

| Step | What it does (dim's job) |
|---|---|
| `validate` | `dotnet restore` and `dotnet format --verify-no-changes` (validate) |
| `test` | `dotnet build -warnaserror`, `dotnet test` with coverage and the runsettings, then the runsettings' coverage thresholds (test) |
| `release` | semantic-release on origin's state: version, channel, tag (release) |
| `docker` | build, tag and push every image (docker) |
| `sync` | merge the released branch back into develop and main into `release/*` (sync-to-develop, sync-main-to-release) |
| `deploy` | update the containers of each environment over SSH and verify them (`/update-deploy`) |

## Contents

- [Running the steps](#running-the-steps)
- [Complete example](#complete-example)
- [Deploy from Infisical](#deploy-from-infisical)
- [Reference](#reference)
- [Deploy data](#deploy-data)
- [Validation](#validation)

## Running the steps

```
nerv pipeline <step> [--cwd <dir>] [--json]
```

Each step reads the `pipeline:` section of your local `.nerv/nerv.yaml`
(committed or not); without one it is refused (exit 1) pointing here.
`validate` and `test` run in your **working tree**, as CI runs on a
checkout: they check the code as it is, uncommitted changes included. Every
command runs without a shell, with `DOTNET_NOLOGO=true`,
`DOTNET_CLI_TELEMETRY_OPTOUT=true` (dim's workflow environment) and then
`pipeline.dotnet.env` over them; its output is streamed to stderr prefixed
`[pipeline:<step>] `. A step stops at the first failing command and names
it. Ctrl-C stops the command and every process it started.

### `nerv pipeline validate`

dim's validate job:

1. `dotnet restore <solution>`
2. `dotnet format <solution> --verify-no-changes`

### `nerv pipeline test`

dim's test job:

1. `dotnet restore <solution>`
2. `dotnet build <solution> --configuration <configuration> --no-restore`,
   plus `-warnaserror` unless `warnings_as_errors: false`
3. `dotnet test <solution> --configuration <configuration> --no-build
   --collect "XPlat Code Coverage"`, plus `--settings <runsettings>` when
   set, `--logger "trx;LogFileName=test-results.trx"` and
   `--results-directory <repo>/.nerv/artifacts/pipeline/test`

The results directory is emptied at the start of every run; it holds
`test-results.trx` and one `<run id>/coverage.cobertura.xml` per test
project, listed in the result (also after a failure, as dim uploads them
always). `.nerv/artifacts/` gets a `.gitignore` of `*`, so nothing there is
committed and the repository's own `.gitignore` is not touched.

**Coverage thresholds.** coverlet's **collector** (`XPlat Code Coverage`,
the `coverlet.collector` package dim uses) writes the reports but ignores
`<Threshold>` in the runsettings: verified with coverlet.collector 6.0.4, a
project at 50% line coverage against `100,80,100` passes `dotnet test`
with exit 0. So after `dotnet test` succeeds, `nerv pipeline test` enforces
the thresholds itself, with coverlet's own semantics (verified by calling
coverlet.core 6.0.4 directly):

| Setting (collector `Configuration`) | Meaning |
|---|---|
| `Threshold` | Percentages, comma-separated, in the order of `ThresholdType`; a single value applies to every type. |
| `ThresholdType` | Any of `line`, `branch`, `method` (default all three, in that order). |
| `ThresholdStat` | `total` (all modules together, dim's), `average` (the mean of each module's percentage) or `minimum` (the lowest module's; coverlet's default). |

- line: covered lines / lines of every method; branch: covered branches /
  branches; method: methods with at least one covered line / methods; a
  metric with nothing to cover counts as 100%.
- Percentages are truncated to hundredths, never rounded up; a threshold
  is missed when the percentage is lower than it.
- Modules are the reports' packages. Several reports (one per test
  project) are merged per line, as coverlet merges results, so a module in
  two reports is not counted twice.
- The collector's `Include` / `Exclude` / `ExcludeByFile` /
  `ExcludeByAttribute` filters were already applied when it wrote the
  reports; nothing is filtered again.
- No runsettings, or no `Threshold` in it: no check. A missed threshold
  fails the step (exit 2) naming each metric, e.g. `coverage below the
  thresholds of coverlet.runsettings (total): line 50% < 100%, branch 100%
  >= 80%, method 50% < 100%`; when met, the result carries a note such as
  `coverage (total): line 100% >= 100%, branch 85.71% >= 80%, method 100%
  >= 100%`.

### Result

stdout carries the result: `<step>: passed|failed|refused|interrupted`,
each command with its exit code, the results directory and the coverage
reports; with `--json`, one document:

```json
{"step":"test","status":"failed","commands":[{"argv":["dotnet","restore","App.slnx"],"exit":0},{"argv":["dotnet","build","App.slnx","--configuration","Release","--no-restore","-warnaserror"],"exit":1}],"artifacts":{"results_dir":"/repo/.nerv/artifacts/pipeline/test","trx":"/repo/.nerv/artifacts/pipeline/test/test-results.trx"},"error":"dotnet build: ..."}
```

`notes` holds the coverage check when one ran. Exit codes: `0`
passed, `1` refused (no or invalid `pipeline:` section, not a git
repository), `2` a command failed or a tool is missing, `128 + signal`
interrupted.

## Complete example

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

## Deploy from Infisical

The same pipeline reading the deploy data from Infisical, with dim's layout
(see [Deploy data](#deploy-data)):

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

## Reference

Paths are relative to the repository root, written with forward slashes
(backslashes are accepted and normalised); absolute paths and paths that
escape the repository are rejected. The file read is always the local
working-tree `.nerv/nerv.yaml`. The section holds names, never secret
values: a credential is given by the name of the environment variable that
holds it.

The source column cites where each default comes from in dim's
`.github/workflows/ci-cd.yml` (`ci-cd.yml`) or deploy helper
(`update-deploy.sh`).

### `pipeline.dotnet` (required)

| Field | Default | Meaning |
|---|---|---|
| `solution` | required | The `.sln`/`.slnx` every dotnet command runs against. |
| `configuration` | `Release` | Build configuration (ci-cd.yml: `--configuration Release`). |
| `runsettings` | none | Passed to `dotnet test --settings`: the coverage collector settings; its `<Threshold>` is enforced by nerv (see [`nerv pipeline test`](#nerv-pipeline-test)). |
| `warnings_as_errors` | `true` | Build with `-warnaserror` (ci-cd.yml, test job). |
| `env` | none | Environment variables for every dotnet command; literal values, never secrets. |

### `pipeline.branches`

The semantic-release branches, in semantic-release order. Each entry has a
`name` (a branch or a glob such as `release/*`, where `*` does not cross a
`/`; only `*`, `?` and `[...]` are supported: `**`, braces such as
`{a,b}` and extglobs such as `@(a|b)` are rejected, as NERV matches
patterns itself before semantic-release does), an optional `prerelease` id and an optional `channel` (default: the
prerelease id; empty on a release branch, meaning the default channel). The
channel is also the moving image tag. When `branches` is omitted:

| Branch | Prerelease | Channel | Source |
|---|---|---|---|
| `main` | none | default | ci-cd.yml trigger and `latest` tag |
| `develop` | `alpha` | `alpha` | dim's semantic-release `branches` |
| `release/*` | `rc` | `beta` | dim's `release/rc`, widened to every `release/*` |
| `hotfix/*` | `beta` | `beta` | dim's `hotfix/*` (fixed id, see below) |

`prerelease: true` (semantic-release's "use the branch name as the id") is
rejected: it breaks on branch names with a `/`, as ci-cd.yml itself warns
for `hotfix/*`. At least one and at most 3 release branches (without
prerelease) are required; the first one is the main branch, the only one
whose images get `latest`.

These branches also feed NERV's built-in semantic-release configuration,
used by `nerv version next`, `nerv ship` and the release step when the
repository has no semantic-release configuration of its own; a repository
with its own keeps its own branches (see
[release-commands.md](release-commands.md#built-in-semantic-release-configuration)).
A glob with a fixed prerelease id (`release/*` with `rc`) serves one
matching branch at a time: with two `release/x` branches on origin the
computation is refused (semantic-release requires one id per prerelease
branch).

### `pipeline.registry`

| Field | Default | Meaning |
|---|---|---|
| `host` | `ghcr.io` | Registry host (ci-cd.yml: `REGISTRY`). Every image name starts with it. |
| `username_env` | `GHCR_USER` | Name of the variable holding the login user (dim's CI uses `github.actor`). |
| `token_env` | `GHCR_TOKEN` | Name of the variable holding the login token (dim's CI uses `GITHUB_TOKEN`). |

### `pipeline.images[]` (at least one)

| Field | Default | Meaning |
|---|---|---|
| `name` | required | `<registry host>/<path>`, no tag or digest; lowercased on load, as ci-cd.yml does. Unique. |
| `title` | last path component of `name` | The `org.opencontainers.image.title` label. |
| `dockerfile` | `Dockerfile` | The Dockerfile. |
| `context` | `.` | The build context (ci-cd.yml: `context: .`). |
| `platform` | `linux/amd64` | `--platform`, comma separated for several (ci-cd.yml: `platforms`). |
| `build_args` | none | `--build-arg` values; they may use `${version}`, `${channel}` and `${tag}` and no other placeholder. |
| `computed_args` | none | Build arguments computed at build time: `run` (required) and `shell` (`node`, `pwsh` or `bash`; default `bash`). The value is the command's trimmed output. When an image has any, a failed build is retried once with them computed again (ci-cd.yml: the login-broker retry). |
| `tags` | `version`, `channel`, `latest` | `version` is the release version; `channel` is applied except on the default channel; `latest` only on the main branch (ci-cd.yml, Compute Docker tags). |
| `required` | `true` | A failure of this image fails the step. In dim both images are required: the broker's `continue-on-error` only lets its retry run, and a failed retry still fails the job. |

The images also receive the labels `org.opencontainers.image.version`,
`.revision` and `.source`, as in ci-cd.yml.

### `pipeline.sync`

After a release, the main branch and every branch matching `release` are
merged `--no-ff` into `develop`, and main is merged into every branch
matching `release` (a conflicting branch is aborted, the others continue,
and the step fails at the end), with `[skip ci]` messages.

| Field | Default | Meaning |
|---|---|---|
| `enabled` | `true` | Run the sync step. |
| `develop` | `develop` | The branch receiving the merges (ci-cd.yml: sync-to-develop); a single branch listed in `branches`. |
| `release` | `release/*` | The release-candidate branches (ci-cd.yml: sync-main-to-release). |

### `pipeline.deploy` (optional)

Without `deploy` there is no deploy step.

| Field | Default | Meaning |
|---|---|---|
| `source` | `env` | Where the deploy data comes from: `env` or `infisical`. |
| `infisical.project_id` | required by `infisical` | The Infisical project. |
| `infisical.app_path` | required by `infisical` | The application's folder, e.g. `/comprobantes-api`. |
| `infisical.domain` | the CLI session's | The Infisical URL; Infisical is read through the user's `infisical login` session. |
| `environments` | `production-green`, `production-blue`, `staging`, `development` | Deployed in this order: dim's processing order (update-deploy.sh, table). |
| `ssh.auth` | `password` | `password` (`DOCKER_SSH_PASS`) or `key` (update-deploy.sh uses sshpass). |
| `ssh.key_file` | required by `key` | The private key file. |
| `ssh.known_hosts` | `~/.ssh/known_hosts` | Host keys are always checked strictly. dim pins its hosts in `~/.config/update-deploy/known_hosts`. |
| `ssh.port` | `22` | The SSH port. |
| `ssh.connect_timeout` | `10s` | Connection timeout (update-deploy.sh: `ConnectTimeout=10`). |
| `verify.healthy` | `true` | Wait for the container's `(healthy)` status (update-deploy, step 8). |
| `verify.version_label` | `org.opencontainers.image.version` | The label compared with the deployed version (update-deploy.sh, verify). |
| `verify.timeout` | `120s` | How long to wait: dim's images declare `HEALTHCHECK --start-period=20s --interval=30s --retries=3`, at most 110s. |

## Deploy data

Both sources provide the same keys per environment. Each environment has
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
NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_SSH_PASS=...          # never in nerv.yaml
NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_COMPOSE_PATH=/srv/apps
NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_COMPOSE_SCRIPT=./manage.sh
NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_CONTAINER_NAME=comprobantes-api
NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_CONTAINER_VARIANT=blue
```

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

## Validation

`nerv.yaml` is read section by section: a YAML error elsewhere in the file
does not matter, and an alias inside `pipeline:` can only refer to an
anchor defined inside it (merge keys `<<:` work). Every problem is reported
at once, each with its path in the section, for example:

```
invalid pipeline configuration:
  pipeline.dotnet.solution: required
  pipeline.images[1].name: "ghcr.io/acme/app:1.0" is not a valid image repository (lowercase path components, no tag or digest)
  pipeline.registry.token_env: must be the name of an environment variable (A-Z, 0-9, _), not its value
```

Checked: unknown keys; the solution and every path; image names (on the
registry host, no tag, unique), platforms and tags; build-argument names and
placeholders (`${version}`, `${channel}`, `${tag}` only); computed arguments
(command, shell, not also in `build_args`); branches (names, duplicates,
prerelease ids, channels, the number of release branches); the sync branch;
the deploy source and its Infisical settings; environment names, duplicates
and variable prefixes; SSH auth, key file, port and durations. A field that
names an environment variable must be an upper-case name: a value that is
not one is reported as a probable secret, and never echoed.
