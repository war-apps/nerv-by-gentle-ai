# release-script

The script target for `nerv deploy` (Claude Code plugin `release-script`, provider
name `bash`, slot `target`). It has no commands of its own: `nerv deploy`
runs its operations from `provider.yaml`, the provider contract
(`docs/extensions.md`) that the release extension's `deploy` skill details. It is a
native provider: every operation runs nerv's hidden `nerv provider
release-script <operation>`.

- `preflight` checks, in your repository and before `nerv deploy` writes
  anything, that `script` (and `publish_script` and `verify_script` when
  set) is a regular file inside the repository: a relative path without
  `..`, outside `.git`, and no symlink leading out of it.
- `publish`, `deploy` and `verify` run `publish_script`, `script` and
  `verify_script` with `bash`. The script is the release commit's (the
  artifact's tag), never your working tree's, and must be a regular file
  there, not a symlink; it runs in a temporary copy of all that commit's
  files (`export-ignore` attributes do not apply).
  `publish` and `verify` without their script are skipped.

The script reads the common JSON request on stdin and finds the deploy
variables in its environment: `NERV_DEPLOY_OPERATION`, `_TARGET`,
`_PROJECT`, `_VERSION`, `_TAG`, `_CHANNEL`, `_PRERELEASE`, `_BRANCH`,
`_BASE_COMMIT`, `_COMMIT` (the release commit), `_BUILD_PROVIDER`,
`_ARTIFACT_DIR`, `_NOTES_FILE`, `_CHANGELOG_FILE` and `_IMAGES`
(space-separated). It may print the common JSON response on stdout
(`{"status": "ok", "message": "...", "data": {...}}`): `failed`, `blocked`
or `needs_input` fail the deploy with its message. Without a response, its
exit code answers. It needs `bash` and `git` on PATH.

## Configuration

`deploy.targets[].config.script` (required), `.publish_script` and
`.verify_script` (optional), all repository-relative paths, and `.timeout`
(optional, a duration such as `10m`: past it the script's process group is
killed and the operation fails; no limit by default):

```yaml
deploy:
  targets:
    - name: docs
      provider: script
      config:
        script: deploy/deploy.sh
        verify_script: deploy/check.sh
```

`docs/deploy.md` ("release-script") has the full contract.

## Install

It is installed with `deploy`, which bundles its providers.
