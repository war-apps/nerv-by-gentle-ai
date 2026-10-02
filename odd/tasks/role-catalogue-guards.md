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

- [x] T1 Guard tests: docs roles table (purpose, gentle-ai equivalent, group, default
  model/effort from agent frontmatter) and the `/nerv:configure` group legend match the
  catalogue; `GroupDescriptions` member names match `Groups` (route: delegated direct). Commit `268771c`.
- [x] T2 Wizard: make the models-table comment accurate about column widths (route: delegated
  direct, same writer). Commit `2ba0a77`.
- [x] T3 Wizard test: reach the models section by anchoring on its prompt instead of a fixed
  count of blank answers (route: delegated direct, same writer). Commit `4f5e36f`.

## Acceptance criteria

- Changing a purpose, equivalent, group member or default in Go/agents without updating the
  prose makes a test fail with a clear message.
- `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

## Progress

- 2026-10-02: branch created, document written.
- 2026-10-02: T1-T3 done by one delegated writer. Guards live in `role_catalogue_prose_test.go`
  (root package). Bite evidence: breaking a group in the docs table, an equivalent, a default
  model, adding a ghost row, changing a purpose, dropping a row, trimming a member from the
  `configure.md` legend, and trimming one from `GroupDescriptions` each failed with a message
  naming the role/group and field; all restored. T3: `modelsInput` answers blank until the
  models offer is printed; an extra early prompt injected temporarily left all five
  ModelsSection tests green. Existing prose was already consistent.

- 2026-10-02: native review (medium, reliability lens) approved and acknowledged. Accepted
  R3-models-input-unbounded-blanks: `modelsInput` now stops after `maxBlankAnswers` (200) with an
  error naming the missing offer, proven by `TestModelsInput_FailsWhenTheOfferNeverAppears`.

- 2026-10-02: second native review approved and acknowledged. Accepted
  R3-purpose-truncated-in-wizard: RED `test(wizard): require every role purpose in full…`
  (observed: fuyutsuki, hyuga and ritsuko purposes cut off at 42 columns), GREEN `fix(wizard):
  size the purpose column to the longest role purpose` (the fixed 42-column cap and `truncate`
  are gone). R3-print-test-unchecked-assertion fixed in the same test commit.

## Next step

PR to `develop`.
