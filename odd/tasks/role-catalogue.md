# Role catalogue: purpose and gentle-ai equivalent per role

Branch `feature/role-catalogue` from `develop` (`0ca3de0`). Started 2026-10-02.
Delivery strategy: `ask-on-risk`; chain strategy cached: stacked to `develop` (user choice
2026-10-02). Forecast ~450 authored changed lines; slice if the running count exceeds ~400.
TDD: strict (global setting). Runner: `go test ./...` (plus `gofmt -l .` and `go vet ./...`).
Engram mirror: topic `odd/role-catalogue/tasks`.

## Objective

Every surface that shows or configures a NERV role's model/effort also says what the role does
and which gentle-ai phase it is equivalent to, from one Go source of truth.

## Problem / why

- User report 2026-10-02: in the per-role model configuration it is not clear what each role
  does nor its equivalent among gentle-ai's agents.
- Exploration 2026-10-02: the wizard table/prompts, `nerv configure --print`/`--set-model`,
  `/nerv:configure` and `/nerv:status` show bare role names; groups `magi`, `pilots`,
  `kaji-passes` are unexplained; no role→phase equivalence exists anywhere; the only role
  descriptions live in `docs/integration.md` (roles table) and agent frontmatter.
  `config.Roles()` (`internal/config/models.go`) carries names and groups only.

## Scope and constraints

- Role catalogue (user-approved 2026-10-02), purpose in English, one line each:

  | Role | Purpose | gentle-ai equivalent |
  |---|---|---|
  | misato | authors the plan (proposal, design, tasks) | sdd-design |
  | ritsuko | intelligence, test planning, end-of-run docs | sdd-explore |
  | hyuga | task criticality, dependency waves, tracking | sdd-tasks |
  | melchor | MAGI vote: structure and security | jd-judge-b |
  | balthasar | MAGI vote: software principles | jd-judge-a |
  | casper | MAGI vote: process and documentation | jd-judge-a |
  | fuyutsuki | governance veto on new skills/scripts/commands | — (none) |
  | kaworu | writes the failing tests first | sdd-apply |
  | shinji | backend pilot | sdd-apply |
  | asuka | frontend pilot | sdd-apply |
  | rei | data pilot (persistence, observability) | sdd-apply |
  | toji | infrastructure pilot (CI/CD, containers) | sdd-apply |
  | maya | quality gate (tests, lint, build) | sdd-verify |
  | kaji | audit compiler | sdd-verify |
  | kaji-security | audit pass: security | jd-judge-a |
  | kaji-coverage | audit pass: tests vs test plan | sdd-verify |
  | kaji-refuter | refutes severe audit findings | jd-judge-b |
  | aoba | commits, PRs and run telemetry | sdd-archive |

- Group legend: `magi` = the three voters, `pilots` = the implementers (kaworu, shinji, asuka,
  rei, toji — confirm against the existing group definition), `kaji-passes` = the audit passes.
- The equivalence is informational: it never changes resolution; `from:<phase>` stays explicit.
- `--print` JSON gains additive fields only (existing consumers keep working).
- A test guards that every role in the catalogue exists in `plugin/agents/` and vice versa.

## Tasks

- [ ] T1 Catalogue in `internal/config`: per-role purpose and gentle-ai equivalent, group
  descriptions, plus the agents-vs-catalogue guard test (route: delegated direct).
- [ ] T2 Surfaces in Go: wizard table columns and group legend, phase picker suggesting the
  role's equivalent first, `--print` fields, unknown-role errors listing purposes where cheap
  (route: delegated direct).
- [ ] T3 Plugin and docs: `/nerv:configure`, `/nerv:status`, `docs/configuration.md` roles table,
  `docs/integration.md` link (route: delegated direct).

## Acceptance criteria

- Tests prove the catalogue covers exactly the plugin agents, the wizard shows purpose and
  equivalent, the phase picker lists the equivalent first, and `--print` carries the new fields.
- `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

## Progress

- 2026-10-02: branch created, document written.

## Next step

T1.
