# release-conventional-commits

The version engine of `nerv release` and `nerv version next` (Claude Code plugin
`release-conventional-commits`, provider name `conventional-commits`,
slot `version`, the slot's default). It has no commands of its own: the
release extension calls its operations from `provider.yaml`, the provider
contract (`docs/extensions.md`, "The release slots"), through the hidden
`nerv provider release-conventional-commits <operation>`.

It is nerv's native engine (`internal/versioning`): the version, channel,
tag, release notes and CHANGELOG section that semantic-release 24 with the
`conventionalcommits` preset gives, without node, npm or semantic-release.

- `validate` checks its keys and the projects (tag formats, paths,
  `depends_on`).
- `next` computes each project's next release on a branch, in the release
  clone (or the repository, offline).
- `branch` resolves a branch's prerelease identifier and channel.
- `import` translates a semantic-release configuration into its keys
  (`nerv release setup --import`).

## Configuration

Under `release.providers.conventional-commits` in `.ai-workflow/config.yaml`:
`branches`, `release_rules`, `note_keywords`, `tag_format` (default
`v${version}`), `repository_url` and `changelog` (`file`, `title`). See
`docs/release-commands.md#version-engine-keys`. Select it with
`release.version: conventional-commits` (the default).

## Install

It is installed with `release` (`nerv install --with release`), which
bundles its providers.
