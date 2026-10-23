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
- **Assumptions to confirm during the work:**
  - The Go engines stay in the `nerv` binary, under new subcommands, and the plugins are their only callers.
  - `nerv pipeline` is folded into ship (validate, test) and deploy (release, docker push, sync back-merge, deploy).
  - Artifact layout: `.nerv/artifacts/<project>/<version>/` holding `manifest.json` (project, version, channel, base commit, the files changed by the release, the outputs), the notes, the CHANGELOG section, and the packages or image references.
  - nerv's own maintainer tooling (`nerv release preview|apply|guard`, used by `.github/workflows/release.yml`) stays in core.

## Tasks

| ID | Specs | Route | Status | Commit |
|---|---|---|---|---|
| T1 | S2, S3 | inline | pending | — |
| T2 | S4, S5 | inline | pending | — |
| T3 | S6, S7, S8 | inline | pending | — |
| T4 | S1, S9 | inline | pending | — |

- **T1:** the native version engine (`internal/versioning`), its config schema, and the semantic-release config importer. Parity tests against the behaviours listed in S2, using the dim configs as fixtures.
- **T2:** the build modes, compose and script, with presets removed. The artifact writer, and ship reworked to produce the artifact with no git writes.
- **T3:** the deploy contract (manifest and operations), the deploy runner (release commit, tag, push, publish, deploy), and the `deploy-github` and `deploy-ssh-compose` providers.
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
- **Next step:** T1.
