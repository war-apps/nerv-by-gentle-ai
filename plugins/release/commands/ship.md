---
description: Version, build and deploy in one go — the next version, the build with the lint and test stages the project's build provider configures, then /release:deploy with the artifact it just packaged (skipped when no deploy provider is configured); every dry run first, and each real run only after its own explicit yes
argument-hint: [--branch <name>] [--project <name>] [--version <semver>] [--target <name> ...] (nothing else is accepted)
---

# /release:ship

Run the whole release extension in one go: compute the next version, build,
lint, test and package what is on origin into a local artifact, then release
and deploy exactly that artifact. Nothing new happens here: the version and
the build are `/release:build`, the deploy is `/release:deploy`, each with the
same dry run first and the same explicit yes before the real run, and every
step comes from the configured providers. Load the `release` skill and the
`deploy` skill first: they are the contracts both commands refer to (§
numbers).

**Accepted arguments.** Check `$ARGUMENTS` before running anything: it may
hold only `--branch <name>`, `--project <name>` and `--version <semver>`,
each at most once, and any number of `--target <name>`, each followed by a
non-empty value that does not start with `-`. Anything else (including
`--dry-run`, `--yes`, `--force`, `--expect`, `--rebase`, `--cwd`, `--json`,
an artifact selector, extra words) is not accepted: tell the user which part
is not accepted and stop without running `nerv`. Below, `<build args>` means
the `--branch`, `--project` and `--version` given, and `<targets>` the
`--target` ones.

0. **Is there a deploy?** Read the `deploy:` section of
   `.ai-workflow/config.yaml`. The deploy runs only when a deploy provider is
   configured: `deploy.host` (a git host, such as `github`) or at least one
   `deploy.targets` entry.
   - No `deploy:` section, or one with neither `host` nor `targets`: the
     deploy is skipped. Tell the user up front that this run versions,
     builds and packages only, and that `/release:deploy-setup` configures a
     deploy. With a `--target`, say it is not accepted because nothing
     deploys, and stop without running `nerv`.
   - Otherwise tell the user up front that a second question, the deploy
     one, comes after packaging.

1. **Version and build.** Follow the `/release:build` command, steps 1 to 6, with `<build args>`: `nerv release --dry-run --json <build
   args>`, the plan, the explicit yes to package, then `nerv release --yes
   --json <build args> --expect <project>=<version> ...`. The version is the
   version engine's (`release.version`, `conventional-commits` by default),
   and the stages are each project's build provider's: `restore`, `lint`,
   `build`, `test`, `coverage` and `package`, each only when the project
   configures it (the `docker-compose` provider's `test` is its
   `test_service`). In the plan, name the lint and test stages that run, or
   say that the project configures none; nerv adds no stage of its own.
   - A refusal, a failure, `nothing-to-release` or anything but an explicit
     yes: stop there, as `/release:build` says. Nothing is deployed.
   - Keep the result document: the projects whose `status` is `packaged`,
     each with its `version`, `tag` and `artifact` directory.
   - None packaged: report the result as `/release:build` step 6 does and
     stop.
   - No deploy (step 0): report the result as `/release:build` step 6 does,
     say the artifacts stay local (nothing was committed, tagged, pushed or
     published), and stop.

2. **Deploy each packaged artifact, in the order of the result document.**
   For each one, follow the `/release:deploy` command, steps 2 to 7, with that `artifact` directory as the selector (no artifact
   picking, no question about which one): `nerv deploy --artifact <dir>
   --dry-run --json <targets>`, the plan, an explicit yes naming project,
   version, tag and targets, then the same command with `--yes`. The yes to
   package in step 1 never counts as a yes to deploy.
   - A refusal, a failure or anything but an explicit yes for one artifact:
     stop there; do not deploy the next ones. Say which artifacts are
     packaged and not deployed, and that `/release:deploy <project>@<version>`
     deploys each one later. Never add `--rebase` unless the user explicitly
     asks for it, as `/release:deploy` step 3 says.

3. **Report.** In the user's language, one row per project: version, tag,
   packaged (yes or no), deployed (done, failed, pending or not asked), with
   the pending steps of a failed deploy and what to fix. Say that running
   `/release:deploy <project>@<version>` again is safe and resumes after the
   steps that were done; running `/release:ship` again builds again.
