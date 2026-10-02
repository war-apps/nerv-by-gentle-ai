# Drop the GitHub Projects and Jira task-provider stubs

Branch `feature/drop-stub-task-providers` from `develop` (`c39b309`). Started 2026-10-02.
Delivery strategy: `ask-on-risk`; chain strategy cached: stacked to `develop`. Forecast ~250
authored changed lines, one PR. Ships as a breaking change.
TDD: strict (global setting). Runner: `go test ./...` (plus `gofmt -l .` and `go vet ./...`).
Engram mirror: topic `odd/drop-stub-task-providers/tasks`.

## Objective

Remove the `github-projects` and `jira` task-provider configuration (both were stubs whose every
operation is `not_implemented`), keeping the provider port open so a real adapter can be added
later.

## Problem / why

- User request 2026-10-02: "saca la configuraciones de gh project y de jira. dejar la posibilidad
  de agregarlos en el futuro. ponelo como un breaking change".
- Exploration 2026-10-02: the only executable source is `config.AllowedValues("tasks.provider")`
  (`internal/config/catalogue.go`), consumed by managed-value validation, `--repo-provider`, and
  the wizard choosers. Byte-compared YAML fixtures carry the stub lines. Prose: the two stub
  files under `plugin/skills/nerv-tasks/providers/`, `nerv-tasks/SKILL.md`, `agents/hyuga.md`,
  `commands/configure.md`, `commands/init.md`, `nerv-orchestrator/SKILL.md`,
  `references/phase-note.md`, `docs/configuration.md`, `docs/commands.md`,
  `docs/integration.md`, `docs/troubleshooting.md`.

## Scope and constraints

- `tasks.provider` allowed values become `teamwork | none`; `--repo-provider` help and the wizard
  follow from the list. Configs holding `github-projects`/`jira` are rejected on set/validate.
- Delete `providers/github-projects.md` and `providers/jira.md`; remove the stub sub-key lines
  from docs and fixtures.
- Keep the door open: `nerv-tasks/SKILL.md` gains a short "Adding a provider" section (adapter
  file `providers/<name>.md` honoring the §7 adapter contract, the value added to
  `AllowedValues("tasks.provider")`, its `tasks.providers.<name>.*` sub-keys, docs) and an explicit
  rule: an unknown or missing provider adapter leaves the tracker `blocked` with a clear message.
  `docs/integration.md` points to it.
- Breaking change: the code commit uses `feat!:` with a `BREAKING CHANGE:` footer; the
  changelog entry is generated at release time from it.
- Dated history in `odd/tasks/` and `CHANGELOG.md` stays as written.

## Tasks

- [x] T1 Go: allowed values `teamwork | none`, tests and fixtures, help text (route: delegated
  direct). RED then GREEN, the GREEN commit is `feat!:` with the footer.
  Commits `a275482` (RED), `cdf8cc7` (GREEN).
- [x] T2 Plugin and docs: delete the stubs, update every mention, add "Adding a provider" and
  the unknown-provider rule (route: delegated direct). Commit `7adab9b`.

## Acceptance criteria

- Setting or validating `tasks.provider: github-projects|jira` is rejected; `teamwork` and `none`
  still work; the wizard offers only those.
- No `github-projects`/`jira` mention remains outside dated history.
- `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

## Progress

- 2026-10-02: branch created, document written.
- 2026-10-02: T1 RED observed (`go test ./...`): `TestAllowedValues/tasks.provider` got
  `[teamwork github-projects jira none]`, want `[teamwork none]`;
  `TestValidateManagedValue_RemovedTaskProviders` and
  `TestInitRepo_RemovedStubProviders_Refused` failed (no error for the removed values). GREEN:
  all packages pass. T2 done; remaining `github-projects`/`jira` mentions are only the two
  regression tests. `go test ./...`, `go vet ./...`, `gofmt -l .` clean;
  `tests/hook-session-start.test.sh` 12 passed, 0 failed.

- 2026-10-02: native review of `c39b309..HEAD` (medium, reliability lens) approved and
  acknowledged. Accepted R3-hyuga-unknown-provider-rule: `agents/hyuga.md` now lists the
  unknown-provider case among its `blocked` statuses.

## Next step

PR to `develop`; after the merge, refresh the plugin cache (also carries the role catalogue
commands from #49).
