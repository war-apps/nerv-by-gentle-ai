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

- [ ] T1 - Prose contract tests (RED) for the three tiers, pilot solo mode,
  FULL step 17, state schema and tracker trigger. Route: inline (one test
  file, already understood).
- [ ] T2 - Orchestrator core + references: classification, delegation
  triggers, Identity, Supersession, `pipeline-direct.md`, `pipeline-light.md`,
  `pipeline-full.md` step 17, usage-and-log, resume. Route: inline (the parent
  is the delegated writer for this feature; no further delegation available
  to a subagent).
- [ ] T3 - Roles and shared contracts: pilots' `MODE: solo`, Ritsuko
  intel-light as the optional explorer, Maya routing in solo mode, Aoba and
  Kaworu scope notes, `nerv-artifacts.md`, `nerv-tasks/SKILL.md`. Route:
  inline (same reason as T2).
- [ ] T4 - `plugin/evals/classification-direct/` eval case; docs
  (`docs/integration.md`, `bench/journeys.md`, `bench/README.md`,
  `CHANGELOG.md`). Route: inline.

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

- RED: (filled after observation)
- GREEN: (filled after observation)
- Commits: (filled after commit)

## Next step

User review of the branch `feat/orchestrator-routing-tiers`; push/PR are the
user's decision.
