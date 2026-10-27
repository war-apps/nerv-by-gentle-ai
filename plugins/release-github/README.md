# release-github

The GitHub git host for `nerv deploy` (Claude Code plugin `release-github`, provider
name `github`, slot `host`). It has no commands of its own: `nerv deploy`
runs its operations from `provider.yaml`, the provider contract
(`docs/extensions.md`) that the release extension's `deploy` skill details.

- `releaseExists` runs `gh release view <tag>`.
- `createRelease` runs `gh release create <tag> --title <tag> --notes-file
  <notes> [--prerelease] --verify-tag`, a prerelease for a release on a
  channel, after `nerv deploy` has pushed the tag.

`gh` runs in `nerv deploy`'s temporary clone of origin, so it releases
origin's repository with your `gh auth login` session. It needs the `gh` CLI
on PATH and no configuration keys.

## Install

```sh
nerv install --with release
```

Then set `deploy.host: github` in the project configuration, or run
`/release:deploy-setup`.
