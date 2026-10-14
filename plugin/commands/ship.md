---
description: Release this repository (or each monorepo project) from origin with nerv ship — always a dry run first, the real release only after the user's explicit yes
argument-hint: [--branch <name>] [--project <name>]
---

# /nerv:ship

Release what is on origin from this machine, as the CI release would:
pipeline, semantic-release (release commit, CHANGELOG, tag, GitHub release),
publish (docker, NuGet, npm) and hooks, per project in dependency order, in
a throwaway clone. The user's repository is never modified.

`nerv ship` releases **what is on origin**: the local branch must equal
origin's (it never pushes the user's commits). `--branch <name>` picks the
branch (default: the current one); `--project <name>` releases only that
project of the `release:` section. Pass `$ARGUMENTS` through unchanged in
both runs below, and never add `--yes` or `--dry-run` to them yourself
except as written here.

1. **Rehearse first, always.** Run:

   ```
   nerv ship --dry-run --json $ARGUMENTS
   ```

   If `nerv` is not on PATH, tell the user to install it — `curl -fsSL
   https://raw.githubusercontent.com/war-apps/nerv-by-gentle-ai/main/scripts/install.sh
   | bash`, or `go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest`
   — and stop. Never run semantic-release, docker push, `dotnet nuget push`
   or `npm publish` yourself.

   The dry run is real work in the clone (restore, build, test, pack, docker
   build, `prepare` hooks), so it can take minutes; it never tags, pushes or
   publishes. stdout is one JSON document `{status, error, projects}`;
   stderr carries the progress and each releasing project's plan (stage
   headers such as `restore`, `test`, `publish (not run: dry run)`, each
   with its steps; secrets shown as `$NAME`).

2. **On a refusal or failure, stop.** Exit 1 (`status` `refused`) is a
   refusal: the local branch is not origin's (push or pull first), branch
   not on origin, no origin remote, an invalid `release:` section (every
   problem listed) or an unknown project. Exit 2 (`failed`) is a failed step
   or an environment problem (origin unreachable, git, node or npm missing).
   130, 143 or 129 means it was interrupted. Show `error` and the relevant
   stderr lines as they are, in a short message, and stop. `status`
   `nothing-to-release` is a normal result: say so and stop.

3. **Present the plan.** In the user's language, one short table, one row
   per project of `projects`:

   | Column | From |
   |---|---|
   | Project | `project` (`branch`) |
   | Version | `version` (blank when `status` is `no-release`) |
   | Channel | `channel` (absent means the default channel) |
   | Tag | `tag` |
   | Last version | `last_version`, or none |

   Under it, for each releasing project, list its stages in order with
   their steps, condensed from the plan on stderr (e.g. `test: dotnet:test`,
   `publish: docker:push ghcr.io/org/api:1.4.0, nuget:push`) and its hooks.
   Say that the pipeline passed in the clone and that the real run will
   build again, then release (commit, tag, push to origin, GitHub release)
   and publish.

4. **Ask, and wait.** Ask the user explicitly whether to release exactly
   this, naming the versions and tags, then stop and wait for the answer.
   The CLI's own `[y/N]` prompt cannot be answered from this shell, which
   is why the real run needs `--yes`. Only an explicit yes (e.g. "yes",
   "release it") counts; anything else, silence or a question means do not
   release: answer the question or confirm that nothing was released.

5. **Release, only after that yes.** Run:

   ```
   nerv ship --yes --json $ARGUMENTS
   ```

   It reads origin and computes again. If the versions or tags it reports
   differ from the ones the user approved (origin moved in between), say
   so plainly in the report.

6. **Report the result document.** In the user's language: `status`
   (`released`, `nothing-to-release`, `declined`, `refused`, `failed`,
   `interrupted`) and `error`, then one row per project with `status`
   (`released`, `no-release`, `failed`, `not-started`), `version`, `tag`,
   `tagged` and `published` (the publish steps that completed). For a
   failed project quote its `error` and its `action` (the recovery advice:
   what is on origin and what to do) without rewording the instructions.
   Never run `nerv ship` again on your own after a failure: when a tag is on
   origin a rerun would release the next version.
