---
description: Set up the release section of .ai-workflow.yml interactively — imports an existing semantic-release config with nerv release setup --import, detects the projects, compose files and scripts, asks each project's build provider (docker-compose or bash) and its stage commands or compose file, writes the section and validates it with nerv version next
argument-hint: [--project <name>]
---

# /release:setup

Build the `release:` section of `<repo>/.ai-workflow.yml` for `nerv version
next` and `nerv release`: import what an existing semantic-release config
says, detect what the repository has, propose a complete section, ask only
what cannot be detected (one question at a time), write it, and prove it
with `nerv version next --offline`. It never commits, pushes, ships or
deploys. Load the `release` skill first: its §2 is the source of truth for
every key, default and rule. Never invent a key it does not list: unknown
keys are errors.

**Arguments.** `$ARGUMENTS` may be empty or `--project <name>` (a non-empty
value not starting with `-`). With `--project`, only that project of an
existing section is added or edited; every other project stays as it is.
Anything else: say what is not accepted and stop.

Talk to the user in their language; everything written to files (YAML,
comments) is in English.

## 1. Preconditions

1. Run `nerv version`. If `nerv` is not on PATH, give the install line of
   the skill's §1 and stop.
2. Run `git rev-parse --show-toplevel`. On failure, say the release commands
   need a git repository and stop. Every path below is relative to that
   top level.
3. Read `.ai-workflow.yml` if it exists.
   - It has a top-level `release:` key with no 4.2.0 key (`semantic_release:`
     or a project preset: `presets`, `dotnet`, `node`, `docker`, `nuget`,
     `npm`): show that block as it is and ask one question: **edit it** (the
     default) or **start over**. Stop and wait. Never overwrite it silently.
   - It has a 4.2.0 `release:` section (one of those keys) or a `pipeline:`
     key: they are no longer read. Say so, and that `nerv migrate --dry-run`
     shows the whole migration of the file (config, sections, plugins).
     Offer to rebuild only the release part here, from the 4.2.0 `release:`
     block (§2 of this command, the skill's §6), and leave `pipeline:` for
     `nerv migrate` or for the user to delete. One question; stop and wait.

Only the `release:` block is added (at the end of the file, after one blank
line) or replaced (from the `release:` line up to the next line that starts at
column 0 with a non-space, non-`#` character, or the end of the file). Keep
every other byte as it is: the rest of the file is managed by `nerv
configure`'s line-based editor. No `.ai-workflow.yml` yet: create it
holding only the section, and mention that `/nerv:init` adds the rest of
NERV's configuration.

## 2. Import an existing semantic-release config

Look for `package.json` `release`, `.releaserc`, `.releaserc.json`,
`.releaserc.yaml`, `.releaserc.yml`, and the JavaScript forms
(`.releaserc.js`, `.cjs`, `.mjs`, `release.config.*`).

- **A JSON or YAML config:** run `nerv release setup --import --dry-run`. It
  prints the translated engine keys under `release.providers.conventional-commits` (`branches`, `release_rules`,
  `note_keywords`, `tag_format`, `changelog`) and one warning per part it
  cannot translate (the `npm` and `exec` plugins, `presetConfig.types`,
  `extends`, maintenance branches, an invalid prerelease branch). Show the
  translation and every warning as they are, and say where each warned part
  goes: `exec` commands become stages or hooks (step 4), `@semantic-release/git`
  and `@semantic-release/github` go to the `deploy:` section (`/deploy:setup`),
  `npm` publishing goes to a deploy target. Ask once whether to import it;
  on an explicit yes, run `nerv release setup --import` and continue from the
  section it wrote.
- **A JavaScript config:** it is never imported. Read it, say it must be
  migrated by hand, and propose the equivalent engine keys yourself (the
  questions of step 5 when it is unclear).
- **None:** propose the default `branches` (skill §2.1), adjusted to the
  repository's branch names (`master`, no `release/*`).

The semantic-release config is no longer used after the import; say it can
be deleted once `/release:version` gives the expected versions. Never run
semantic-release, node or npm.

## 3. Detect (read-only)

Use your file tools and read-only git commands only. Nothing is installed,
built or run.

| Look at | To learn |
|---|---|
| `.github/workflows/*.yml` | The build, test, coverage and package commands CI runs (the script stages), with their flags (e.g. `-warnaserror`); docker build steps (image names, Dockerfiles, context, build args). |
| `docker-compose*.yml`, `compose*.yaml` | Whether compose builds the project: its services with a `build:` key and their `image:` names (the project's `images`), and a service that runs the tests (`providers.docker-compose.test_service`). |
| `*.csproj`, `*.sln`, `*.slnx` | dotnet projects: the deployable one, packable libraries, test projects (not release projects). |
| `Dockerfile`, `*.Dockerfile`, `docker/` | Images and their `ARG`s (e.g. `APP_VERSION`). |
| `package.json` (each project) | Scripts `lint` / `build` / `test`; the package manager (`packageManager`, else the lockfile) for the restore command. |
| Release scripts (`scripts/*`, exec commands, CI steps) | Steps release covers (a bump-csproj script is `version_files`), steps that become stages, and the rest (hooks). Publishing steps (docker push, nuget push, npm publish) belong to deploy. |
| Layout and `git log --format=%s -200` | Monorepo or not, and the conventional-commit scopes used per folder. |

## 4. Propose the complete section

Show the whole `release:` section as one YAML block, then one line of reason
per non-obvious choice. Rules:

- **Projects.** One project with `path: .` for a single deployable repo;
  one per deployable unit in a monorepo, each with its `path`, its `scopes`
  when the log uses scopes other than the project name, and `shared_paths`
  for shared build files. Keep the default `tag_format` unless the existing
  tags use another one.
- **`build`**, the build provider, for every project (or once as
  `release.build` when all share it); it is required and has no default,
  and there are no presets.
  - `docker-compose`: `providers.docker-compose.file` and `test_service`
    (under `release.providers` when shared, else the project's
    `providers`) and the project's `images` it builds, with `${version}`
    where the tag is the version. Only `restore`, `lint` and `coverage`
    stages may be written.
  - `bash`: every stage command the project uses, copied from CI with its
    flags (`restore`, `lint`, `build`, `test`, `coverage`, `package`). The
    `package` stage writes what the artifact keeps into
    `$NERV_RELEASE_OUTPUTS_DIR` (`dotnet pack -o "$NERV_RELEASE_OUTPUTS_DIR"`,
    `docker save -o "$NERV_RELEASE_OUTPUTS_DIR/<name>.tar" <image>` after a
    `docker build`). Never a publish step.
- **`version_files`**, **`coverage`**, **`providers.bash.shared_host`**,
  **`submodules`**, **hooks** and **`depends_on`** as the skill's §2 says.
  A pinned .NET SDK becomes a `before_restore` hook writing `global.json`.
- **A 4.2.0 `release:` block**: carry its projects, paths, scopes,
  `tag_format`, `changelog`, `depends_on`, `build`, `stages` and `hooks`
  over; drop `semantic_release:`; rewrite presets as stage commands; list
  what was moved where, and what goes to `deploy:`.

## 5. Ask only what cannot be detected

Ask ONE question at a time, each with a recommended default, then STOP and
wait for the answer before the next one. Never assume an answer. Always ask,
for each project, unless the user already said it:

- its **build provider**, `docker-compose` or `bash` (recommend the one
  detection points to);
- with `docker-compose`, the **compose file**, the **test service** and the
  **image names**, when detection does not show them;
- with `bash`, the **command of each stage** the project uses, when CI
  does not show it, and what the `package` stage must leave in the artifact.

Other typical questions: coverage floors, `depends_on` and its `bump`,
whether a detected script stays a hook, becomes a stage or is dropped, and
branch names or prerelease ids the import warned about.

After the answers, show the final section in full and ask once whether to
write it. Write only on an explicit yes.

## 6. Write and validate

1. Write the section into `.ai-workflow.yml` as described in step 1.
2. Run `nerv version next --offline --json`, and in a monorepo also with
   `--project <name>` per project.
3. **Exit 1 with a configuration error**: show the path-qualified lines as
   they are, fix the section, write it again and rerun. At most two fix
   rounds; if it still fails, stop and show the remaining problems. Any
   exit 2: show stderr and stop.
4. On success, show one row per project: project, next version (or "no new
   release"), channel, tag, last release. Say the result used local tags
   only (`--offline`).

## 7. Next steps

Tell the user, without doing any of it:

1. Review `.ai-workflow.yml`: `nerv release` uses this local file as it is,
   committed or not. Committing it is recommended so the team shares it.
2. Run `/release:build`: it always rehearses first and packages the artifact
   only after an explicit yes; it never commits, tags or pushes.
3. To release the artifact, install the deploy plugin (`nerv install --with
   deploy,deploy-github`) and run `/deploy:setup`.

Never commit, push, run `nerv release` or edit any file other than
`.ai-workflow.yml`.
