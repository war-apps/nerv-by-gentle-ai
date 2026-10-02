# Role catalogue guards and wizard test hardening

Branch `feature/role-catalogue-guards` from `develop` (`ab94931`). Started 2026-10-02.
Delivery strategy: `ask-on-risk`; chain strategy cached: stacked to `develop`. Forecast ~200
authored changed lines, one PR.
TDD: strict (global setting). Runner: `go test ./...` (plus `gofmt -l .` and `go vet ./...`).
Engram mirror: topic `odd/role-catalogue-guards/tasks`.

## Objective

Close the advisory findings left by the native reviews of the role catalogue (lineages of
2026-10-02 on `feature/role-catalogue` and `feature/drop-stub-task-providers`).

## Problem / why

User request 2026-10-02: "ejecuta las revisiones sugeridas". Open advisories:
- R3-legend-duplicated-in-prose / R3-docs-defaults-untested / R3-001: the group legend in
  `plugin/commands/configure.md`, the roles table in `docs/configuration.md`, and the member
  names written in `config.Roles().GroupDescriptions` copy catalogue data with no test tying
  them to `config.Roles()` (Info, Groups) or to the agents' default model/effort.
- R3-row-width-unbounded: `internal/wizard/models.go` comment says rows stay within ~120 columns,
  but model/source columns now size to their longest value.
- R3-scripted-answer-count: `internal/wizard/wizard_test.go` (`modelsLines`) hardcodes 28 blank
  answers to reach the models section.

## Scope and constraints

- Guard tests only read the prose; they never generate it. Failures name the role and the
  mismatching field.
- Prefer making the comment true over adding truncation that hides model ids; a long custom
  model id or source must stay fully visible.
- No behavior change for users beyond what the comment fix implies.

## Tasks

- [ ] T1 Guard tests: docs roles table (purpose, gentle-ai equivalent, group, default
  model/effort from agent frontmatter) and the `/nerv:configure` group legend match the
  catalogue; `GroupDescriptions` member names match `Groups` (route: delegated direct).
- [ ] T2 Wizard: make the models-table comment accurate about column widths (route: delegated
  direct, same writer).
- [ ] T3 Wizard test: reach the models section by anchoring on its prompt instead of a fixed
  count of blank answers (route: delegated direct, same writer).

## Acceptance criteria

- Changing a purpose, equivalent, group member or default in Go/agents without updating the
  prose makes a test fail with a clear message.
- `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

## Progress

- 2026-10-02: branch created, document written.

## Next step

T1–T3.
