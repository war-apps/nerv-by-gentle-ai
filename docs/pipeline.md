# Migrating from `nerv pipeline` (`pipeline:`)

`nerv pipeline`, its Claude Code command and the `pipeline:` section were
removed.
Its release step ran semantic-release, which nerv no longer uses: the
version engine is native, and building and releasing are now two plugins
with a local artifact between them.

| Old step | Now |
|---|---|
| `validate` | A `lint` stage of the project in the `release:` section, run by `nerv release` ([release](release-commands.md)). |
| `test` | The `build`, `test` and `coverage` stages, with `build.coverage` floors. |
| `release` | `nerv release` (version, CHANGELOG section, version files, packages, into the artifact), then `nerv deploy` (release commit, tag, channel note, push, GitHub release with `deploy-github`). |
| `docker` | Building and saving the images: `nerv release` (`compose` mode, or a script `package` stage). Pushing them: the `publish` operation of the `deploy-ssh-compose` target. |
| `sync` | Not ported. Merge the release back into `develop` and `release/*` by hand; an opt-in `deploy.sync` may come later. |
| `deploy` | The `deploy` and `verify` operations of the `deploy-ssh-compose` target ([deploy.md](deploy.md#deploy-ssh-compose)). |

A repository that still has a `pipeline:` section is told so: the section
is no longer read, and the commands point to `/release:setup` and
`/deploy:setup`.

## Steps

1. **Install the plugins**:

   ```sh
   nerv install --with release,deploy,deploy-github,deploy-ssh-compose
   ```

2. **See the whole migration** with `nerv migrate --dry-run`: it shows
   every change to the configuration files (the move to `.ai-workflow.yml`,
   the translation of the 4.2.0 `release:` and `pipeline:` into today's
   `release:` and `deploy:`, the semantic-release import, the plugins to install) before
   writing anything. `nerv migrate` applies it, with backups.
3. **Or migrate by hand** with the two setup commands: `/release:setup`
   (offers `nerv release setup --import` for the semantic-release config in
   `package.json`, and asks each project's `build.mode`), then
   `/deploy:setup` (the host, the targets, git).
4. **Rehearse**: `/release:build` (a dry run in a clone, then the artifact on
   your yes) and `/deploy:run` (a dry run, then the deploy on your yes).
5. **Delete** the `pipeline:` section, and the semantic-release config once
   `/release:version` gives the versions you expect.

## Before: the old `pipeline:` section

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

## After: `release:` and `deploy:`

The same repository. The images are built by a compose file
(`docker/compose.release.yml`, building both images with `APP_VERSION`), so
compose mode saves them into the artifact and `deploy-ssh-compose` pushes
them; a script `package` stage running `docker build` and `docker save`
works too.

```text
release:
  branches:
    - main
    - { name: release/rc, prerelease: rc, channel: beta }
    - { name: develop, prerelease: alpha, channel: alpha }
    - { name: "hotfix/*", prerelease: beta, channel: beta }
  projects:
    - name: api
      path: .
      build:
        mode: compose
        file: docker/compose.release.yml
        test_service: tests
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

## Key by key

| Old key (default) | Now |
|---|---|
| `pipeline.dotnet.solution`, `configuration` (`Release`), `warnings_as_errors` (`true`) | The `restore`, `build` and `test` stage commands, written in full (`dotnet build <solution> -c Release -warnaserror`). There are no presets. |
| `pipeline.dotnet.env` | Each stage's `env`, or `build.shared_host: true` for the build-server variables. |
| `pipeline.dotnet.runsettings`, `coverage.mode` (`enforce`), `coverage.thresholds` | `--settings` in the test stage, and `build.coverage` `{line, branch, method}`, always enforced. `report` and `off` have no equivalent: leave `build.coverage` out. |
| `pipeline.branches` | `release.branches` (same meaning; `prerelease: true` is accepted when the branch name is a valid id). |
| `pipeline.registry.host` (`ghcr.io`), `username_env` (`GHCR_USER`), `token_env` (`GHCR_TOKEN`) | Log in with `docker login` before `nerv deploy`: the target pushes with docker's own credentials. |
| `pipeline.images[].name`, `title`, `dockerfile` (`Dockerfile`), `context` (`.`), `platform` (`linux/amd64`), `build_args`, `computed_args` (shell `bash`), `tags`, `required` | The compose file's services (or a script `package` stage), and `build.images` with `${version}`. The pushed tags are exactly the `build.images` references; the moving tags (`channel`, `latest`) are not produced automatically. |
| `pipeline.sync.enabled`, `develop` (`develop`), `release` (`release/*`) | Not ported. |
| `pipeline.release.version_files` | `build.version_files`. |
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
([deploy.md](deploy.md#deploy-data)).

The records `nerv pipeline` left in `.nerv/artifacts/pipeline/` are not
read by anything; delete the folder.
