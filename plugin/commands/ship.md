---
description: Build, test and package this repository (or each monorepo project) from origin into a local artifact with nerv ship — always a dry run first, the real run only after the user's explicit yes; never writes to git
argument-hint: [--branch <name>] [--project <name>] [--version <semver>] (nothing else is accepted)
---

# /nerv:ship

Build, test and package what is on origin from this machine, as the CI
build would, per project in dependency order, in a throwaway clone, in each
project's build mode (`compose` or `script`), and leave a local artifact
per project in `.nerv/artifacts/<project>/<version>/` for `nerv deploy`.
`nerv ship` never writes to git (no commit, no tag, no push) and never
publishes: deploying, tagging and the git host release belong to `nerv
deploy`. The user's repository only gets the artifacts, which
`.nerv/artifacts/` ignores by itself.

`nerv ship` builds **what is on origin**: the local branch must equal
origin's (it never pushes the user's commits). Its `release:` section is
the local `.ai-workflow.yml`, committed or not, and every project needs
`build.mode`; a path it names that is not on origin's branch is refused.
`--branch <name>` picks the branch (default: the current one); `--project
<name>` packages only that project of the `release:` section; `--version
<semver>` uses that version instead of computing it (with several projects
it needs `--project`, and it must be newer than the project's last tag).

**Accepted arguments.** Check `$ARGUMENTS` before running anything: it may
hold only `--branch <name>`, `--project <name>` and `--version <semver>`,
each at most once, each followed by a non-empty value that does not start
with `-`. Anything else (including `--dry-run`, `--yes`, `--force`,
`--expect`, `--cwd`, `--json`, extra words) is not accepted: tell the user
which part is not accepted, that `/nerv:ship` takes only `--branch`,
`--project` and `--version`, and stop without running `nerv`. Below,
`<args>` means exactly those accepted arguments. Never add flags other than
the ones written here.

1. **Rehearse first, always.** Run:

   ```
   nerv ship --dry-run --json <args>
   ```

   If `nerv` is not on PATH, tell the user to install it — `curl -fsSL
   https://raw.githubusercontent.com/war-apps/nerv-by-gentle-ai/main/scripts/install.sh
   | bash`, or `go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest`
   — and stop. Never run semantic-release, `git tag`, `git push`, docker
   push, `dotnet nuget push` or `npm publish` yourself.

   The dry run is real work in the clone (restore, lint, build, test,
   coverage, package, `prepare` hooks; in compose mode `docker compose
   build`, `docker compose run --rm <test_service>` and `docker save`), so
   it can take minutes; it writes no artifact. stdout is one JSON document
   `{status, error, semantic_release_config, projects}`; stderr carries the
   progress and each releasing project's plan (stage headers such as
   `restore`, `test`, `package`, each with its steps).

2. **On a refusal or failure, stop.** Exit 1 (`status` `refused`) is a
   refusal: no `release:` section, a project without `build.mode` or with a
   removed preset key (the error says exactly what to write instead), the
   local branch is not origin's (push or pull first), branch not on origin,
   no origin remote, a path of the local `release:` section not on origin's
   branch, an invalid `release:` section (every problem listed), an unknown
   project, or a `--version` that is not newer than the project's last tag
   or lacks `--project` in a monorepo. Exit 2 (`failed`) is a failed step or
   an environment problem (origin unreachable, git, node, npm or docker
   missing). 130, 143 or 129 means it was interrupted. Show `error` and the
   relevant stderr lines as they are, in a short message, and stop. For a
   section that needs `build.mode` or still uses presets, suggest
   `/nerv:release-setup`. `status` `nothing-to-release` is a normal
   result: say so and stop.

3. **Present the plan.** In the user's language, one short table, one row
   per project of `projects`:

   | Column | From |
   |---|---|
   | Project | `project` (`branch`) |
   | Version | `version` (blank when `status` is `no-release`) |
   | Channel | `channel` (absent means the default channel) |
   | Tag | `tag` (the tag `nerv deploy` will create) |
   | Last version | `last_version`, or none |

   Under it, for each releasing project, its build mode and its stages in
   order with their steps, condensed from the plan on stderr (e.g. `build:
   compose:build`, `test: compose:test`, `package: compose:save
   ghcr.io/org/api:1.4.0`) and its hooks. Say that the pipeline passed in
   the clone, that the real run will build again and then write the
   artifact `.nerv/artifacts/<project>/<version>/`, and that nothing is
   committed, tagged, pushed or published.

4. **Ask, and wait.** Ask the user explicitly whether to package exactly
   this, naming the versions, then stop and wait for the answer. The CLI's
   own `[y/N]` prompt cannot be answered from this shell, which is why the
   real run needs `--yes`. Only an explicit yes (e.g. "yes", "package it")
   counts; anything else, silence or a question means do not run it:
   answer the question or confirm that nothing was packaged.

5. **Package, only after that yes, bound to what was approved.** Run:

   ```
   nerv ship --yes --json <args> --expect <project>=<version> ...
   ```

   with one `--expect <project>=<version>` for each project of the dry-run
   document whose `status` is `dry-run`, using its `project` and `version`
   exactly as printed. The real run reads origin and gets the versions
   again; if origin moved since the dry run and the versions are not
   exactly the approved ones, it refuses (exit 1, `status` `refused`,
   `error` naming expected and computed versions) before running anything.
   Tell the user this before running it. On that refusal, say that nothing
   was packaged and that a new `/nerv:ship` will rehearse and show the new
   plan; do not rerun on your own. When the refusal says the artifact
   already exists (`pass --force to replace it`), tell the user that
   version is already packaged; never add `--force` yourself.

6. **Report the result document.** In the user's language: `status`
   (`packaged`, `nothing-to-release`, `declined`, `refused`, `failed`,
   `interrupted`) and `error`, then one row per project with `status`
   (`packaged`, `no-release`, `failed`, `not-started`), `version`, `tag`
   and `artifact` (the artifact directory). For a failed project quote its
   `error` and its `action` without rewording the instructions. Nothing is
   ever on origin after `nerv ship`; the next step for a packaged artifact
   is `nerv deploy`.
