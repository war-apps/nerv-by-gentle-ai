# release-ssh-compose

The SSH and docker compose target for `nerv deploy` (Claude Code plugin
`release-ssh-compose`, provider name `ssh-compose`, slot `target`). It
has no commands of its own: `nerv deploy` runs its operations from
`provider.yaml`, the provider contract (`docs/extensions.md`) that the
release extension's `deploy` skill details. It is a native provider: `preflight`,
`deploy` and `verify` run nerv's hidden `nerv provider release-ssh-compose
<operation>`.

- `preflight` checks the configuration and answers the tools it needs,
  before `nerv deploy` writes anything.
- `publish` runs `docker load -i` for each image tar of the artifact, then
  `docker push` for each image. Log in to the registry first. With
  `deploy.publish: true` (the default) `nerv deploy` has already pushed the
  same images in its own `publish` step, so this re-push uploads nothing.
- `deploy` reads the environment's deploy data (the
  `NERV_DEPLOY_<ENVIRONMENT>_<KEY>` variables with `source: env`, or
  Infisical with `source: infisical`) and runs the host's compose script over
  SSH: `pull`, then `up`.
- `verify` waits until the container is healthy and checks that its version
  label is the released version.

An environment whose `DOCKER_DEPLOY_ENABLED` is off, or whose
`DOCKER_IMAGE_VERSION` points at other releases, is skipped; one with no
`DOCKER_IMAGE_VERSION` is refused. It needs `docker` and `ssh`, plus `sshpass`
with `ssh_auth: password` and the `infisical` CLI with `source: infisical`;
`nerv deploy` checks them before any push. The configuration keys and their
scopes are in `provider.yaml` and in `docs/deploy.md`.

## Install

```sh
nerv install --with release
```

Then add a `deploy.targets[]` entry with `provider: ssh-compose` to the
project configuration, or run `/release:deploy-setup`.
