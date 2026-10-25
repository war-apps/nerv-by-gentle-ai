# release-bash

The script build of `nerv release` (Claude Code plugin `release-bash`, provider
name `bash`, slot `build`). It has no commands of its own: the release
extension calls its operations from `provider.yaml`, the provider contract
(`docs/extensions.md`, "The release slots"), through the hidden `nerv
provider release-bash <operation>`.

Every pipeline stage (`restore`, `lint`, `build`, `test`, `coverage`,
`package`) runs the project's own command under its `shell` (`bash` by
default, `pwsh` or `node`), as `docs/release-commands.md` ("How a command
runs") describes; the images the commands build are saved into the
artifact after the `package` stage.

- `validate` accepts any stage.
- `plan` has no command of its own; with `shared_host` it gives the
  `restore`, `lint`, `build`, `test` and `package` commands the
  build-server-safe dotnet environment under their own `env`.

## Configuration

`release.providers.bash.shared_host` (default `false`), overridden per
project under `release.projects[].providers.bash`. Select it with
`release.build: bash` or `release.projects[].build: bash`.

## Install

It is installed with `release` (`nerv install --with release`), which
bundles its providers.
