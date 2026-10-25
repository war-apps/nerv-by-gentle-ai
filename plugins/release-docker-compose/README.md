# release-docker-compose

The docker compose build of `nerv release` (Claude Code plugin
`release-docker-compose`, provider name `docker-compose`, slot
`build`). It has no commands of its own: the release extension calls its
operations from `provider.yaml`, the provider contract
(`docs/extensions.md`, "The release slots"), through the hidden `nerv
provider release-docker-compose <operation>`.

- `validate` refuses a missing or escaping `file`, a `test_service` that
  is not a compose service name, and a project command for the `build`,
  `test` or `package` stage (compose runs them).
- `plan` gives the build stage `docker compose -f <file> build` and, with a
  `test_service`, the test stage `docker compose -f <file> run --rm
  <test_service>`; `nerv release` then saves the project's `images` into
  the artifact with `docker tag` and `docker save`. The project's own
  `restore`, `lint` and `coverage` commands still run.

## Configuration

`release.providers.docker-compose.file` (required) and `test_service`,
overridden per project under `release.projects[].providers.docker-compose`.
Select it with `release.build: docker-compose` or
`release.projects[].build: docker-compose`. It needs `docker` with the
compose plugin on PATH.

## Install

It is installed with `release` (`nerv install --with release`), which
bundles its providers.
