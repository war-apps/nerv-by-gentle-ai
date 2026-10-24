---
description: Build, test and package this repository (or each monorepo project) from origin into a local artifact with nerv release — always a dry run first, the real run only after the user's explicit yes; never writes to git
argument-hint: [--branch <name>] [--project <name>] [--version <semver>] (nothing else is accepted)
---

# /release:build

Build, test and package what is on origin from this machine, as the CI
build would, per project in dependency order, in a throwaway clone, with each
project's build provider (`docker-compose` or `bash`), and leave a local artifact
per project in `.nerv/artifacts/<project>/<version>/` for `/deploy:run`.
`nerv release` never writes to git (no commit, no tag, no push) and never
publishes. Load the `release` skill first: it is the contract the steps below
refer to (§ numbers).

**Accepted arguments.** Check `$ARGUMENTS` before running anything: it may
hold only `--branch <name>`, `--project <name>` and `--version <semver>`,
each at most once, each followed by a non-empty value that does not start
with `-`. Anything else (including `--dry-run`, `--yes`, `--force`,
`--expect`, `--cwd`, `--json`, extra words) is not accepted: tell the user
which part is not accepted, that `/release:build` takes only `--branch`,
`--project` and `--version`, and stop without running `nerv`. Below,
`<args>` means exactly those accepted arguments.

1. **Rehearse first, always.** Run:

   ```
   nerv release --dry-run --json <args>
   ```

   If `nerv` is not on PATH, give the install line of §1 and stop. The dry
   run is real work in the clone (every stage and the `prepare` hooks; with
   `docker-compose`, `docker compose build`, `docker compose run --rm
   <test_service>` and `docker save`), so it can take minutes; it writes no
   artifact.

2. **On a refusal or failure, stop.** Show `error` and the relevant stderr
   lines as they are, in a short message, with the meaning of the exit code
   (§5). For a project without a build provider, a moved or removed key or a 4.2.0
   `release:` section, suggest `nerv migrate` (or `/release:setup`). `status`
   `nothing-to-release` is a normal result: say so and stop.

3. **Present the plan.** In the user's language, one short table, one row
   per project: project (`branch`), version (blank for `no-release`),
   channel, tag (the tag `/deploy:run` will create) and last version. Under
   it, for each releasing project, its build mode and its stages in order
   with their steps, condensed from stderr (e.g. `build: compose:build`,
   `test: compose:test`, `package: compose:save ghcr.io/org/api:1.4.0`), and
   its hooks. Say that the pipeline passed in the clone, that the real run
   builds again and then writes `.nerv/artifacts/<project>/<version>/`, and
   that nothing is committed, tagged, pushed or published.

4. **Ask, and wait.** Ask the user explicitly whether to package exactly
   this, naming the versions, then stop and wait. Only an explicit yes
   (e.g. "yes", "package it") counts; anything else, silence or a question
   means do not run it.

5. **Package, only after that yes, bound to what was approved.** Run:

   ```
   nerv release --yes --json <args> --expect <project>=<version> ...
   ```

   with one `--expect <project>=<version>` for each project of the dry-run
   document whose `status` is `dry-run`, exactly as printed. Tell the user
   that if origin moved and the versions differ, the run refuses before any
   stage. On that refusal, say nothing was packaged and that a new
   `/release:build` will rehearse again; do not rerun on your own. When the
   refusal says `pass --force to replace it`, say that version is already
   packaged; never add `--force` yourself.

   The `release:builder` agent can run steps 1 and 5 in the background
   (`MODE: rehearse`, then `MODE: package` with `APPROVED` set to the
   versions the user said yes to); the question in step 4 always stays in
   this session.

6. **Report the result document.** In the user's language: `status` and
   `error`, then one row per project with `status`, `version`, `tag` and
   `artifact`. For a failed project quote its `error` and its `action`
   without rewording them. Nothing is ever on origin after `nerv release`;
   the next step for a packaged artifact is `/deploy:run`.
