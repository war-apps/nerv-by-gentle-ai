---
description: Release this repository (or each monorepo project) from origin with nerv ship — always a dry run first, the real release only after the user's explicit yes
argument-hint: [--branch <name>] [--project <name>] (nothing else is accepted)
---

# /nerv:ship

Release what is on origin from this machine, as the CI release would:
pipeline, semantic-release (release commit, CHANGELOG, tag, GitHub release),
publish (docker, NuGet, npm) and hooks, per project in dependency order, in
a throwaway clone. The user's repository is never modified.

`nerv ship` releases **what is on origin**: the local branch must equal
origin's (it never pushes the user's commits). Its `release:` section is
the local `.nerv/nerv.yaml`, committed or not; a path it names that is not
on origin's branch is refused. `--branch <name>` picks the
branch (default: the current one); `--project <name>` releases only that
project of the `release:` section.

**Accepted arguments.** Check `$ARGUMENTS` before running anything: it may
hold only `--branch <name>` and `--project <name>`, each at most once, each
followed by a non-empty value that does not start with `-`. Anything else
(including `--dry-run`, `--yes`, `--expect`, `--cwd`, `--json`, extra
words) is not accepted: tell the user which part is not accepted, that
`/nerv:ship` takes only `--branch` and `--project`, and stop without
running `nerv`. Below, `<args>` means exactly those accepted arguments.
Never add flags other than the ones written here.

1. **Rehearse first, always.** Run:

   ```
   nerv ship --dry-run --json <args>
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
   not on origin, no origin remote, a path of the local `release:` section
   not on origin's branch, an invalid `release:` section (every
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

5. **Release, only after that yes, bound to what was approved.** Run:

   ```
   nerv ship --yes --json <args> --expect <project>=<version> ...
   ```

   with one `--expect <project>=<version>` for each project of the dry-run
   document whose `status` is `dry-run`, using its `project` and `version`
   exactly as printed. The real run reads origin and computes again; if
   origin moved since the dry run and the computed releases are not
   exactly the approved ones, it refuses (exit 1, `status` `refused`,
   `error` naming expected and computed versions) before running anything.
   Tell the user this before running it: they approved these versions, and
   a moved origin makes the release refuse instead of releasing something
   they did not approve. On that refusal, say that nothing was released
   and that a new `/nerv:ship` will rehearse and show the new plan; do not
   rerun on your own.

6. **Report the result document.** In the user's language: `status`
   (`released`, `nothing-to-release`, `declined`, `refused`, `failed`,
   `interrupted`) and `error`, then one row per project with `status`
   (`released`, `no-release`, `failed`, `not-started`), `version`, `tag`,
   `tagged` and `published` (the publish steps that completed). For a
   failed project quote its `error` and its `action` (the recovery advice:
   what is on origin and what to do) without rewording the instructions.
   Never run `nerv ship` again on your own after a failure: when a tag is on
   origin a rerun would release the next version.
