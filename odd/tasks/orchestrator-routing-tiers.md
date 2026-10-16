# Orchestrator routing tiers (DIRECT / LIGHT / FULL)

## Objective

Make Ikari (`plugin/skills/nerv-orchestrator/`) scale the number of agents it
launches with the size of the change, using gentle-ai's own routing criteria
(Work Routing Ladder, Delegation Rules, Mandatory Delegation Triggers,
Delegated Verification Gate, Implementation Routing/ODD) as closely as
possible.

## Problem

Classification had only LIGHT and FULL. Even LIGHT launched ~5-6 agents for a
typo (Hyuga + Aoba preflight, Kaworu RED, pilot GREEN, Maya gate, Aoba commit
and run summary), and Ikari never wrote source, so gentle-ai's "direct
inline" tier had no NERV equivalent.

## Why

gentle-ai routes small, understood work inline and delegates only when a
mandatory trigger fires. NERV superseded that routing but kept the heavier
shape everywhere, which costs time and tokens on trivial changes.

## Scope

- Three tiers: DIRECT (gentle-ai direct inline), LIGHT (gentle-ai delegated
  direct, minimal agents), FULL (unchanged NERV pipeline).
- One-way ratchet DIRECT -> LIGHT -> FULL.
- Delegation triggers rewritten with gentle-ai's current thresholds.
- Verification in DIRECT/LIGHT follows the Delegated Verification Gate.
- Route declaration (tier + trigger evidence) in the deliberation log.
- Pilot solo mode (own RED/GREEN) for LIGHT; Kaworu stays in FULL.
- FULL fix routing (step 17) spells out its own cycle.
- `state.yaml` `path: direct`; Ikari writes the run summary in DIRECT/LIGHT;
  tracker testing move "when the work unit's verification starts".

Out of scope: any change to the FULL pipeline's steps, gates or roles; Go CLI
behavior.

## Constraints

- Keep "user validates before commit" in DIRECT and LIGHT (kept on purpose:
  gentle-ai's tiers do not require it, but it is not clearly contrary).
- Never weaken RDD consent relay, lossless prompts, or attribution checks.
- `SKILL.md` stays under 500 lines (`skill_split_test.go`).
- ~400 authored lines per task is a planning heuristic only.

## Tasks

- [x] T1 - Prose contract tests (RED) for the three tiers, pilot solo mode,
  FULL step 17, state schema and tracker trigger. Route: inline (one test
  file, already understood).
- [x] T2 - Orchestrator core + references: classification, delegation
  triggers, Identity, Supersession, `pipeline-direct.md`, `pipeline-light.md`,
  `pipeline-full.md` step 17, usage-and-log, resume. Route: inline (the parent
  is the delegated writer for this feature; no further delegation available
  to a subagent).
- [x] T3 - Roles and shared contracts: pilots' `MODE: solo`, Ritsuko
  intel-light as the optional explorer, Maya routing in solo mode, Aoba and
  Kaworu scope notes, `nerv-artifacts.md`, `nerv-tasks/SKILL.md`. Route:
  inline (same reason as T2).
- [x] T4 - `plugin/evals/classification-direct/` eval case; docs
  (`docs/integration.md`, `bench/journeys.md`, `bench/README.md`,
  `CHANGELOG.md`). Route: inline.

- [x] T5 - Native 4-lens review findings: resume rebuilds DIRECT/LIGHT
  state (T2 had listed resume but not changed it — corrected here), Maya's
  `ikari-commit` and DIRECT envelope, Toji solo/commit wording, Kaworu's
  stale input, docs "off or unknown", explicit `Classification: <TIER>`
  graders and prompts, solo-block drift test (option b: identical-block
  test, since agents read only their own file), stronger DIRECT test.
  Route: inline.

## Acceptance criteria

- `go test -count=1 ./...`, `go vet ./...`, `gofmt -l .` clean.
- New prose tests observed RED before the change, GREEN after.
- FULL pipeline table unchanged except step 17's wording of its own cycle.

## Checks

- `go test -count=1 ./...`
- `go vet ./...`
- `gofmt -l .`
- plugin evals need a model (`claude plugin eval`): skipped, stated.

## Progress

See task checkboxes and the evidence below.

## Evidence

- RED (observed before any source write, `go test -count=1 -run '...' .`,
  exit 1): six new tests in `skill_split_test.go` failed —
  `TestOrchestrator_ClassifiesThreeRoutingTiers` (missing three-tier
  heading, `references/pipeline-direct.md`, gentle-ai thresholds; retired
  4-file/20-call triggers present), `TestLightPipeline_LaunchesOneBoundedWriter`
  (Kaworu step row, no `MODE: solo`), `TestDirectPipeline_...` (file absent),
  `TestPilots_DocumentSoloMode` (4 pilots), `TestFullPipeline_FixRoutingSpellsOutItsOwnCycle`,
  `TestSharedContracts_KnowTheDirectTier`.
- GREEN: `go test -count=1 ./...` exit 0 after T2-T3 (SKILL.md at 500 lines,
  the split test's cap); `go vet ./...` exit 0; `gofmt -l .` empty.
- Evals: `claude plugin eval` needs a model; not run. The new
  `classification-direct/scaffold.sh` was executed once in a scratch dir
  (exit 0, README with the typo committed).
- Commits:
  - T1+T2 `4c8a179` feat(orchestrator): route work through DIRECT, LIGHT and FULL tiers
  - T3 `cc078cf` feat(agents): let a LIGHT pilot run its own test-first cycle
  - T4 `082c646` test(evals): add a DIRECT classification case
  - T4 `79fcdd4` docs: describe the DIRECT, LIGHT and FULL routing tiers
- T5 RED (`go test -count=1 .`, exit 1): 7 new tests failed — solo blocks
  drift (Toji), never-commits wording (Asuka/Rei/Toji), Maya `aoba-commit`
  and no DIRECT envelope, Kaworu "When neither resolves", resume without
  `path: direct`/`path: light`/never-discard, docs "RDD is off", graders
  matching option lists and "not TIER". The strengthened DIRECT test passed
  at once (the rules were already written); it pins them, it was not RED.
- T5 GREEN: `go test -count=1 ./...` exit 0, `go vet ./...` exit 0,
  `gofmt -l .` empty; grader patterns checked with PyYAML + Python `re`.
- T5 commit: `7bbdaf8` fix: address the routing-tiers review findings.
- Incident: before T5, four committed eval files were found altered in the
  working tree by something outside this writer (light `case.yaml` and
  `scaffold.sh` all NUL bytes, full reverted to pre-branch text, mtime
  12:33:37); restored from HEAD with `git checkout --` on those exact paths.
- RDD: not run by this writer (no review assess/consent relay from a
  subagent); the parent owns the native review decision per commit.

## Rationale

- DIRECT keeps "user validates before commit": gentle-ai's direct-inline
  tier does not require it, but removing a user gate was not clearly
  authorized.
- DIRECT with RDD off/unknown and a `high`/unassessable assess tier
  launches Maya: gentle-ai's gate only covers delegated writers, so this is
  a conservative NERV derivation, not a gentle-ai rule.
- LIGHT is one coherent work unit (more than one is already a FULL
  criterion), so its table no longer repeats per work unit; test and code
  land in one work-unit commit (gentle-ai: tests alongside the behavior).

## Next step

User review of the branch `feat/orchestrator-routing-tiers`; push/PR are the
user's decision.
