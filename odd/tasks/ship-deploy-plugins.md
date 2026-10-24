# Ship and deploy plugins (versioning, build, artifact, deploy, git host)

Locator: `odd/tasks/ship-deploy-plugins.md`. Engram mirror topic: `odd/ship-deploy-plugins/tasks` (project `nerv-gentle-ai`).
Branch: `feature/ship-deploy-plugins`, cut from `feature/tasks-plugin-split` at 89b10ce, because it depends on the extension-plugin installer. Delivery: `ask-on-risk` with `feature-branch-chain` (cached from the previous feature). Test runner: `go test ./...` plus the root prose tests.

## Specs

- **S1. Plugins of their own.** The user asked: "algo mas que quiero separar es la parte de shipping, versionado y deploy. esto tambien me gustaria que sea un plugin de nerv y q me lo sugiera instalar (como task). ambos plugins instalan sus propios agentes."
  - Shipping and versioning ship as the extension plugin `ship` (`ship@nerv`); deploy ships as `deploy` (`deploy@nerv`).
  - `nerv install` suggests them the way it suggests `task`: first `ship`, then `deploy`, which requires `ship`.
  - Each plugin ships its own agents. Core `plugin/` keeps no ship, version, release-setup, pipeline or deploy command.
- **S2. Native version engine.** The user asked: "el plugin de shipping, debe tener como plugin el agente o script para saber la version, sin usar semantic-release pero se debe poder comportar igual y debe poder configurarse tambien."
  - The next version is computed natively in Go, without semantic-release, node or npm.
  - It must give the same result semantic-release gives for the behaviours nerv relies on today:
    - Conventional Commits with the `conventionalcommits` preset and the note keywords `BREAKING CHANGE`, `BREAKING CHANGES` and `BREAKING`;
    - branches, with prerelease ids and channels (`main`, `develop` as alpha, `release/*` as rc on beta, `hotfix/*` as beta);
    - `tag_format`;
    - per-project `path`, `shared_paths` and `scopes` filters;
    - `depends_on` with a minimum bump;
    - the release notes and CHANGELOG section.
  - Every one of these is configurable.
- **S3. Configuration source** (user decision: "nerv.yaml + importar la existente (Recommended)").
  - The engine's configuration lives in `.nerv/nerv.yaml`, under the ship plugin's section.
  - A setup command imports, once, an existing JSON or YAML semantic-release config (`package.json` `release`, `.releaserc.json`, `.releaserc.yaml`/`.yml`). It reports what it cannot translate, such as the `npm` and `exec` plugins.
  - A `.js` config is not imported; it is reported so the user migrates it by hand.
- **S4. Build mode per project.** The user asked: "otro que sea el modo de build por default si es por dockercompose o script (ambos deben ser seteados por proyecto)."
  - Each project sets `build.mode: compose | script`.
  - User decision: "Solo compose | script, sin presets". The dotnet, node, docker, nuget and npm presets are removed. In `script` mode every stage command is written by the project.
- **S5. Ship produces a local artifact.** The user asked: "debe poder recibir un nuemero de version, buildear, testear, hacer todo lo que hace actualmente "/nerv:ship" y dejar un articfact final local."
  - `ship` accepts an explicit version, or computes it (S2).
  - It builds, tests and packages in a throwaway clone of origin, the way `/nerv:ship` does today, with the dry run first, hooks, the `--expect` check, monorepo order and coverage.
  - It leaves a final local artifact that survives the clone.
- **S6. Deploy consumes the artifact.** The user asked: "otro plugin seria el de deploy que utiliza ese artifact y deploya en donde debe ademas de generar un tag y release en github o en el entorno git que sea."
  - `deploy` takes a ship artifact and publishes and deploys it to its targets.
  - It creates the tag and the release on GitHub or whichever git host the repository uses.
- **S7. Git writes belong to deploy** (user decision: "Todo en deploy (Recommended)").
  - `ship` never writes to git: no commit, no tag, no push.
  - `deploy` creates the release commit (CHANGELOG plus version files), the tag and the push, and the git host release.
  - An artifact that is never deployed leaves no trace in the repository.
- **S8. Providers with a contract** (user decision: "Proveedores con contrato (Recommended)").
  - Deploy targets and git hosts are provider plugins, with a manifest declaring their config, the tools they need and their operations, as with task providers.
  - The first providers are `deploy-github` (tag, push and release) and `deploy-ssh-compose` (today's SSH, Infisical and compose deploy).
  - Adding another host or target, such as GitLab or Gitea, is another plugin, with no change to `deploy`.
- **S9. Migration.** A repository using today's `release:` and `pipeline:` sections must be told exactly what to change, by the setup command. Assumption: the old sections stop being read, and a run with them refuses with a message pointing to the setup command.
- **S10. Remove SDD and use the gentle-ai mode.** The user asked: "sacar sdd y usemos el modo de gentle-ai". User decision: "ODD + agentes NERV encima (Recommended)".
  - Remove NERV's OpenSpec-style artifact flow: `openspec/` changes, proposal, design, spec deltas, archive and `nerv spec-compose`.
  - Track work with gentle-ai's ODD feature document (`odd/tasks/<feature>.md`) and native RDD review.
  - The NERV agents and their governance (Misato, MAGI, Maya, Kaji, Aoba and the rest) remain, reading and writing the ODD document instead of openspec.
- **S11. Configuration file.** The user asked: "cambiemos la configuracion dejemosla en un archivo .ai-workflow.yml en el root del proyecto, se mantiene la configuracion por proyecto". User decision: "Se mantiene, renombrado (Recommended)".
  - Project config is `<repo>/.ai-workflow.yml`.
  - User config is `~/.config/ai-workflow/config.yml`, holding personal settings such as models and provider user ids.
  - The same key-by-key merge applies, with the project winning.
  - Core, `task`, `ship`, `deploy` and their providers all read from these two files.
- **S12. Migration.** The user asked: "crear scripts de migracion entre la version anterior y esta". User decision: "Subcomando nerv migrate (Recommended)".
  - `nerv migrate` in Go, cross-platform, with `--dry-run` showing every change before writing, backups and tests.
  - Run once per repo, plus once for the user config.
  - It covers:
    - moving `.nerv/nerv.yaml` and `~/.claude/nerv/nerv.yaml` to the S11 files;
    - translating `release:` and `pipeline:` into the ship and deploy config;
    - importing semantic-release (S3);
    - converting `openspec/` (S10);
    - installing the plugins the repo used (`task`, `ship`, `deploy` and their providers).
- **Assumptions to confirm during the work:**
  - Runtime files (the artifacts, the orchestrator lock, the deliberation logs) stay in a git-ignored runtime folder. Its name (`.nerv/` or `.ai-workflow/`) is decided in T6.
  - `openspec/changes` still in progress become ODD feature documents, and archived ones stay where they are, read-only.
  - The Go engines stay in the `nerv` binary, under new subcommands, and the plugins are their only callers.
  - `nerv pipeline` is folded into ship (validate, test) and deploy (release, docker push, sync back-merge, deploy).
  - Artifact layout: `.nerv/artifacts/<project>/<version>/` holding `manifest.json` (project, version, channel, base commit, the files changed by the release, the outputs), the notes, the CHANGELOG section, and the packages or image references.
  - nerv's own maintainer tooling (`nerv release preview|apply|guard`, used by `.github/workflows/release.yml`) stays in core.

## Tasks

| ID | Specs | Route | Status | Commit |
|---|---|---|---|---|
| T1 | S2, S3 | delegated writer, in parallel with T2 (Writer trigger (a): independent units with disjoint surfaces, each over 100 lines; also the context backstop) | committed (RED: 14 Compute and Import tests plus the changelog test; GREEN: `go test ./internal/versioning` ok; parity 16/16 against semantic-release 24.2.9) | see `git log -- internal/versioning` |
| T2 | S4, S5 | delegated writer, in parallel with T1 (same trigger) | committed and verified (no blockers) | f0254d2 |
| T3 | S6, S7, S8 | delegated writer (context backstop) | committed and verified; fixes in ed99e5d | 322edc0, 367d066, ed99e5d |
| T4 | S1, S9 | inline | pending | — |
| T5 | S10 | delegated writer (context backstop; about 38 files of prose) | in progress | — |
| T6 | S11 | delegated writer (context backstop; the edit surface spans the whole tree) | committed and verified; fixes in c5c0bf4 | 9c6032c, c5c0bf4 |
| T7 | S12 | — | pending | — |

- **T1:** the native version engine (`internal/versioning`), its config schema, and the semantic-release config importer. Parity tests against the behaviours listed in S2, using the dim configs as fixtures.
- **T2:** the build modes, compose and script, with presets removed. The artifact writer, and ship reworked to produce the artifact with no git writes.
- **T3:** the deploy contract (manifest and operations), the deploy runner (release commit, tag, push, publish, deploy), and the `deploy-github` and `deploy-ssh-compose` providers.
- **T5:** remove SDD/openspec from the orchestrator, the agents, `internal/specs` and the docs; the agents work on the ODD document.
- **T6:** central config loader for `.ai-workflow.yml` plus `~/.config/ai-workflow/config.yml`, used by every package and plugin.
- **T7:** `nerv migrate`.
- **Order:** T1/T2 → T6 → T3 → T5 → T4 → T7.
- **T4:** the plugins `ship`, `deploy`, `deploy-github` and `deploy-ssh-compose`, with their agents and commands; installer suggestions and requirements; removal from core; migration messages; docs.

Forecast: well over 400 lines (about 11.5k lines of Go move or are rewritten), so it will be delivered as chained slices.

## Log

- **L1** (2026-10-23, user, verbatim):
  > algo mas que quiero separar es la parte de shipping, versionado y deploy. esto tambien me gustaria que sea un plugin de nerv y q me lo sugiera instalar (como task). ambos plugins instalan sus propios agentes.
  > el plugin de shipping, debe tener como plugin el agente o script para saber la version, sin usar semantic-release pero se debe poder comportar igual y debe poder configurarse tambien. otro que sea el modo de build por default si es por dockercompose o script (ambos deben ser seteados por proyecto). debe poder recibir un nuemero de version, buildear, testear, hacer todo lo que hace actualmente "/nerv:ship" y dejar un articfact final local. otro plugin seria el de deploy que utiliza ese artifact y deploya en donde debe ademas de generar un tag y release en github o en el entorno git que sea
- **L2** (2026-10-23): the user's decisions: "Todo en deploy (Recommended)", "nerv.yaml + importar la existente (Recommended)", "Solo compose | script, sin presets", "Proveedores con contrato (Recommended)".
- **L3** (2026-10-23): mapping evidence.
  - `nerv ship` leaves no local artifact: `Workspace.Close` removes `<ws>/artifacts`.
  - semantic-release, through the embedded node runner in `internal/semrel`, makes the release commit, the CHANGELOG, the tag and the GitHub release.
  - `internal/release` already has native semver and Conventional Commit parsing, but only for nerv itself.
  - There is no compose or script build mode today.
  - Ship, pipeline and deploy have no coupling to the orchestrator, the agents or the config catalogue.
  - The dim repos keep their semantic-release config as JSON under `package.json` `release`.
- **L4** (2026-10-23): seam between T1 and T2.
  - T1 exposes `versioning.Compute(ctx, versioning.Request) ([]versioning.Result, error)`.
    - `Result` carries the same fields as `semrel.ProjectResult`'s 12 action outputs, plus `ChangelogSection` and `Bump`.
    - `Request` holds `RepoDir`, `Branch`, `Projects`, `Config` and `Offline`.
  - T2 consumes the version through an interface (`VersionSource`) that can be fed an explicit version or a computed one. It keeps the existing semrel-backed source until T4 wires in the engine.
- **L5** (2026-10-23, user, verbatim):
  > otros puntos que quiero cambiar para tenerlos en cuenta:
  > * sacar sdd y usemos el modo de gentle-ai
  > * cambiemos la configuracion dejemosla en un archivo .ai-workflow.yml en el root del proyecto, se mantiene la configuracion por proyecto
  > * crear scripts de migracion entre la version anterior y esta
- **L6** (2026-10-23): T1 outcome and decisions.
  - **Parity:** 16/16 scenarios match semantic-release 24.2.9 field by field, including the notes.
  - **Not supported:** maintenance branches (`1.x`, range), addChannel, `extends`, and presets other than conventionalcommits. The importer warns about each.
  - **Decision (parent):** ship fetches origin's tags and `refs/notes/semantic-release*` inside its throwaway clone and calls `Compute` with `Offline: true` there, so the user's repo is never written (S7).
  - **Decision (parent):** deploy must write the `refs/notes/semantic-release-<tag>` channel note when it creates a prerelease tag, otherwise prerelease numbering resets. This goes into T3.
  - **Risk: item 6.** Notes parity relies on a hand-written ICU collation approximation; re-run the parity harness when the preset changes.
- **L7** (2026-10-23): T2 outcome.
  - **Checks:** `go test ./...` all ok; the `semrel_integration` ship tests ran for real.
  - **Differential check:** only the expected differences. `nerv version next` also refuses preset keys, which matches S9 (the parent accepts this).
  - **Git incident:** the T2 writer staged its deletions with `git rm`, and the parent's doc commit 756d248 swept them in, so HEAD did not build. The parent rebuilt its three local commits (805d3f8, d30bbe5, 2d35cf4) without the deletions and committed T2 as f0254d2. The backup branch is `backup/ship-deploy-pre-fix`. Every commit builds again.
  - **Open:** stale preset docs in `docs/commands.md`, `docs/configuration.md` and `docs/pipeline.md`, to be fixed in T4. `--version` derives the channel from the prerelease id, not from the branch map.
- **L8** (2026-10-23): T6 decision (parent).
  - The runtime folder stays `.nerv/`, which becomes runtime only (artifacts, lock, logs). It is never committed.
  - The user config path is `os.UserConfigDir()/ai-workflow/config.yml`: `~/.config/ai-workflow/config.yml` on Linux, `%AppData%\ai-workflow\config.yml` on Windows.
- **L9** (2026-10-23): the T1 verifier ran more than 30 probe groups. Everything matched except the notes ordering of typographic punctuation, symbols and emoji, which was a blocker because dim.comprobantes.api has 40 such subjects.
  - **Fix:** 26d33ef. `localeCompare` now uses `golang.org/x/text/collate` CLDR root, a new dependency (risk item 5), with the verifier's regression tests. RED observed, then GREEN.
  - **Advisories:** `presetConfig.types` (custom section titles) is not translated, and the importer warns. The order follows ICU root, not the es_ES locale.
  - **Importer on the dim repos:** it warns that `hotfix/*` with `prerelease: true` is invalid and needs a fixed id such as `beta`, that `npm` and `exec` (`bump-csproj.sh`, `set-release-outputs.sh`) are not translatable, and that `git`/`github` go to deploy.
- **L10** (2026-10-23): the T2 verifier found no blockers. S4, S5, S7 (ship side) and S9 (T2 part) pass. After a real ship, the user's `.git` and origin hash identically.
  - **Advisories assigned to T3:**
    - `artifact.Verify` must also hash `notes.md` and `CHANGELOG.section.md` and reject extra files in `outputs/`;
    - `--force` should rename the new artifact before deleting the old one;
    - `--version` gives empty notes, so deploy should regenerate them or ship should compute them for the given version.
  - **Advisories assigned to T4:**
    - `hooks.before_publish`/`after_publish` should point to deploy;
    - `.nerv/artifacts` symlinks should be refused before the pipeline runs;
    - the `--expect` wording with `--version`;
    - a possible tar name clash.
- **L11** (2026-10-23): T6 committed in 9c6032c. RED/GREEN reported per rule. The differential check found differences only in file names, the hint line and the new `.nerv/.gitignore`. Checks: `go test ./...` ok; hook tests ok. The two versioning tests the writer saw fail had run between the parent adding the regression test and applying the fix in 26d33ef; with `-count=5` they pass every time.
- **L12** (2026-10-23): the T6 verifier found one blocker.
  - **B1:** the prose hard-coded the Linux user-config path, so agents on Windows would miss `%AppData%`. Fixed in c5c0bf4, with a RED-then-GREEN prose test.
  - **Advisories:**
    - Fixed in c5c0bf4: the fake-home guard is no longer bypassed by `XDG_CONFIG_HOME` (the real home now comes from the user database, compared case-insensitively on Windows), and the damaged comments are corrected.
    - Assigned to T4: a nested `enabled: true` activates the hook (a baseline defect); `.nerv/.gitignore` is written only by init and the wizard; the hint line is not indented in pipeline text mode.
    - Assigned to T7: `workflowconfig.Load` and `Scalar` have no production callers; the merge lives in prose.
  - **Interference:** while T3 was in progress, `plugin.Extensions` treated `deploy-*` (they have a `provider.yaml`) as task providers that require `task`, so two `cmd/nerv` install tests failed. Fix in T4: provider kinds and requirements per manifest kind.
- **L13** (2026-10-23): T3 committed.
  - **Deploy:** the contract is `deploy-provider.yaml`, and the runner does the release commit (`--cleanup=whitespace`), the annotated tag, the channel note on the release commit, an atomic push, the targets and the host release.
  - **Parity:** after a deploy, `Compute` gives the next alpha.N.
  - **Installer fix (322edc0):** extensions get a provider kind (task or deploy), and a requirement this nerv does not ship is refused. This closes slice 01 advisory R1-001.
  - **Open, safe defaults chosen by the writer:** empty notes after `ship --version` only warn; there is no automatic back-merge (`deploy.sync` later); providers come from the embedded extensions only.
- **L14** (2026-10-23): T5 layout decision (parent).
  - The ODD document `odd/tasks/<feature>.md` replaces proposal, design, tasks and spec deltas.
  - The NERV per-run governance files (state, deliberation log, audit rounds, run summary) live in `odd/<feature>/nerv/` and are committed with the feature. The orchestrator lock moves to `.nerv/locks/<feature>.lock` (runtime).
  - `openspec/` and `nerv spec-compose` are removed. In-flight openspec changes are converted by `nerv migrate` (T7).
- **L15** (2026-10-23): the T3 verifier found two blockers, both fixed in ed99e5d with its regression tests (RED, then GREEN).
  - **B1 (security):** a crafted artifact could declare `.git/config` as a release file and run `core.fsmonitor` in the deploy clone, even on a dry run. Release paths with a `.git` component, or written through a symlink, are now refused.
  - **B2:** `deploy-ssh-compose` deployed an environment with no `DOCKER_IMAGE_VERSION`, which the base refused. Target selection is applied again.
  - **Passed:** git writes and parity (alpha.N, rc.N, stable), atomic pushes with recovery after a rejection, idempotent re-runs, tamper checks, and injection through the notes or argv (none possible).
  - **Advisories assigned to T4:**
    - `ship --version` must take the channel from the branch config, otherwise rc collides with `beta`;
    - preflight every target's tools and data (`sshpass`, `infisical`) before the push;
    - warn on `--rebase` that the tag points at code that was never built;
    - clean up the push error and gh's "release not found" noise.
  - **Advisories assigned to T7:** `selection: branches` has no equivalent and needs a migration message; `manifest.json` itself is not hashed (it is a local artifact; record as a known limit).
- **Next step:** T5 (running). The user's L5 decisions are "ODD + agentes NERV encima (Recommended)", "Se mantiene, renombrado (Recommended)" and "Subcomando nerv migrate (Recommended)". Then T6.
