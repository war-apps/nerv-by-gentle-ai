# NERV Artifact Schemas

One definition per NERV-owned artifact, referenced by every agent instead of
repeating the shape inline. gentle-ai-owned filenames (`state.yaml`,
`exploration.md`, `proposal.md`, `specs/{domain}/spec.md`, `design.md`,
`tasks.md`) keep gentle-ai's own shape and stay where `gentle-ai sdd-status`
expects them; only NERV-owned files are defined here. All NERV files live
under `openspec/changes/{change}/nerv/`, except `state.yaml` which lives at
`openspec/changes/{change}/state.yaml` (gentle-ai's own file, extended with
one NERV-owned key). Engram topic key for every artifact:
`nerv/{change}/{artifact}` (artifact = file name without extension).

## state.yaml (extended)

- Purpose: native SDD change state, extended with the NERV path decision so
  a resumed session knows which pipeline governed the change without
  re-classifying.
- Author: Ikari (the `nerv:` block only — the rest of the file is
  gentle-ai's own and is never hand-edited by NERV agents).
- Location: `openspec/changes/{change}/state.yaml`.
- Engram key: `nerv/{change}/state` (mirror of the `nerv:` block only).
- `task_ref` holds `{PREFIX}-{id}` — the resolved provider's
  `task_ref_prefix` uppercased plus the task id, e.g. `TW-49132010` —
  when a task is active, or `null` when the change has no task
  (`tasks.provider: none`, or the user chose to work without one at
  Preflight).
- `closed_at` is `null` until the change closes, then set once by Ikari at
  Close (LIGHT's Close step or FULL's step 19), the same write that deletes
  `nerv/.orchestrator.lock`.

```yaml
dependsOn: []
nerv:
  path: light                     # light | full
  classification_reason: "single domain, no critical path, under budget"
  task_ref: "TW-49132010"         # or null when no task
  created_at: "2026-09-24T14:03:00Z"
  closed_at: null                 # set once, at Close
```

## .orchestrator.lock (Ikari)

- Purpose: concurrency guard — detects a second NERV session resuming the
  same change while the first is still alive, so Resume never races an
  active run.
- Author: Ikari only; no other agent reads or writes this file. Never
  persisted to Engram or committed to git — local, ephemeral, machine-scoped
  state (Ikari adds it to `.git/info/exclude` on creation).
- Location: `openspec/changes/{change}/nerv/.orchestrator.lock`.
- Engram key: none.

```yaml
session_id: "26b0c7bf-8fa7-4b75"   # UUID segment of the scratchpad path
host: "WALTER-PC"                  # machine name
started_at: "2026-09-24T14:03:00Z"
heartbeat_at: "2026-09-24T14:11:00Z"
phase: "full"
step: "11-wave-2"
pid: null
```

Fresh when `heartbeat_at` is under 15 minutes old (see `## Orchestrator
lock` in `nerv-orchestrator/SKILL.md`); refreshed before every launch and
after every envelope; deleted at close or on an explicit stop.

## exploration-light.md (Ritsuko)

- Purpose: LIGHT-path micro-intel — enough to pick a pilot and scope Maya's
  reduced gate, not a full exploration document.
- Author: Ritsuko (LIGHT micro-intel dispatch).
- Location: `nerv/exploration-light.md`.
- Engram key: `nerv/{change}/exploration-light`.

```markdown
# Exploration (light) — {change}

## Touched files
- {path} — {why it is touched}

## Entry points
- {function/route/component the change enters through}

## Existing tests
- {test file(s) already covering this area, or "none found"}

## Risks
- {short risk note, or "None"}

## Suggested pilot
{rei|shinji|asuka|toji} — {one-line domain justification}
```

## maya-report.md (Maya)

- Purpose: baseline + quality-gate results, reduced-mode aware, with TDD
  evidence reproduced independently (never trusted from a self-report).
- Author: Maya.
- Location: `nerv/maya-report.md`.
- Engram key: `nerv/{change}/maya-report`.

```markdown
# Maya report — {change}

## Mode
{full|reduced} — reduced scope: {touched tests + lint/build only, list them}

## Phases
| Phase | Project | Detected | Lint configured | Build configured | Result |
|---|---|---|---|---|---|
| a (unit/touched tests) | {project} | yes/no | - | - | pass\|fail\|no-lint-build-configured |
| b (lint) | {project} | - | yes/no | - | pass\|fail\|no-lint-build-configured |
| c (build) | {project} | - | - | yes/no | pass\|fail\|no-lint-build-configured |
| d (full suite) | {project} | yes/no | - | - | pass\|fail\|skipped-reduced-mode |

## Failures
| test_id | verdict | routed_to |
|---|---|---|
| {test id} | spec-wrong\|impl-wrong\|ambiguous | {pilot\|Ikari-user-escalation} |

## TDD evidence
| step | test | command | observed result | commit |
|---|---|---|---|---|
| RED | {test name} | {exact command} | {exact observed output/exit} | {commit hash} |
| GREEN | {test name} | {exact command} | {exact observed output/exit} | {commit hash} |
| TRIANGULATE | {test name} | {exact command} | {exact observed output/exit} | {commit hash} |
| REFACTOR | {test name} | {exact command} | {exact observed output/exit} | {commit hash} |

RED is reproduced by read-only inspection of the RED commit in git history
(`git show`/`git log -p` output read, never checked out or extracted into
the tree) plus a live run of the suite at the current tree for GREEN; the
working tree is never mutated by the gate. maya.md governs on any
remaining difference.
```

### MODE full additions

The schema above is mode-agnostic; in `full` mode (FULL pipeline, step 12
of `nerv-orchestrator/SKILL.md`) phases gate each other strictly:
`b` does not start until `a` is green, `c` until `b` is green, `d` until
`c` is green. A failure at any phase stops the gate there — it never
continues to the next phase on a red result. The full gate (mode `full`)
runs once, after the last wave closes, over the whole change diff
(base..HEAD) — never per wave. Each failure in the `## Failures` table
routes `impl-wrong` back to the owning pilot (code, not plan — no
unfreeze needed) or `spec-wrong` to Misato as a binding ruling (`MODE:
ruling`, source `maya`), which applies the unfreeze design rule: unfreeze
the owning task (`unfrozen_by_ruling`), a scoped revision of its
tests/spec via Kaworu, a re-vote of that task alone, then re-freeze; a
failure Maya cannot classify is reported `ambiguous` and resolved by
exactly one Misato ruling (also `MODE: ruling`, no unfreeze unless the
ruling names one) before any user escalation, per the Bounded loops
table.

## run-summary.md (Aoba)

- Purpose: close-out narrative, usage accounting, RDD receipts, commit and
  PR-slice record for the change.
- Author: Aoba.
- Location: `nerv/run-summary.md`.
- Engram key: `nerv/{change}/run-summary`.

```markdown
# Run summary — {change}

## Work done
{short narrative}

## Agents
| agent | model | tokens_total | duration_s |
|---|---|---|---|
| nerv:kaworu | sonnet | {n} | {n} |

`tokens_total` is the single combined usage figure the Agent tool result
reports per launch (it does not split input/output tokens). Figures come
only from usage data Ikari collected from each Agent tool result — never
estimated.

## RDD receipts
| commit | review_due | outcome | boundary_advanced_to |
|---|---|---|---|

## Commits
| hash | subject |
|---|---|

## PR slices
{none, or the chained/stacked slice list with boundaries}
```

## deliberation-log.md (Ikari, append-only)

- Purpose: full event trail of the run — the only append-only NERV
  artifact; never rewritten, only appended to.
- Author: Ikari, appending inline after each event (a mechanical
  single-file write, not delegated).
- Location: `nerv/deliberation-log.md`.
- Engram key: `nerv/{change}/deliberation-log` (re-saved in full on each
  append, since Engram upserts overwrite rather than append).

```markdown
# Deliberation log — {change}

- {ts} | {phase} | {actor} | preflight_answer | {payload_ref}
- {ts} | {phase} | {actor} | classification | {payload_ref}
- {ts} | {phase} | {actor} | ratchet | {payload_ref}
- {ts} | {phase} | {actor} | launch | {payload_ref}
- {ts} | {phase} | {actor} | envelope | {payload_ref}
- {ts} | {phase} | {actor} | gate_relayed | {payload_ref}
- {ts} | {phase} | {actor} | gate_decision | {payload_ref}
- {ts} | {phase} | {actor} | commit_recorded | {payload_ref}
- {ts} | {phase} | {actor} | rdd_assess | {payload_ref}
- {ts} | {phase} | {actor} | rdd_receipt | {payload_ref}
- {ts} | {phase} | {actor} | stop | {payload_ref}
```

`payload_ref` is a short pointer (commit hash, envelope field, file path) —
never the full payload inline; keep the log scannable.

### Phase 2 event types

FULL adds these event types to the same append-only log, same shape
(`{ts, phase, actor, event_type, payload_ref}`):

- `corner_case_relayed` — Ikari relayed Ritsuko's corner-case questions to
  the user as one grouped blocking prompt.
- `corner_case_answer` — the user's answer was recorded into
  `test-plan.md`'s `## Answers`.
- `vote_cast` — one MAGI member's vote for one task in one round (one
  entry per member per task).
- `escalation_criticality` — a MAGI member escalated a task's criticality
  to `critical` (escalation is one-way, never down).
- `vote_result` — a task's round concluded: `approved` or `rejected`,
  with the applied rule (`unanimous` or `majority-2:1`).
- `veto_evaluated` — Fuyutsuki ruled on one item from `design.md`'s
  `## New skills, scripts and commands`.
- `plan_gate_relayed` — Ikari presented the plan-approval summary (tasks,
  criticality, votes, veto rulings, test-plan summary) to the user.
- `plan_gate_decision` — the user's plan-approval decision.
- `wave_plan` — Hyuga's `waves.md` was produced or updated.
- `wave_report` — a wave's implementation step completed (or a deviation
  was raised for it — see `deviation` below).
- `deviation` — a pilot or Hyuga signaled a wave deviation
  (`scope|dependency|blocked`) requiring a Misato ruling.
- `ruling_issued` — Misato recorded a binding ruling (from a Hyuga
  deviation, a Maya `spec-wrong` failure, or a Maya `ambiguous` failure).
  Payload carries `unfreezes: task_ids[]` — the frozen task(s), if any,
  the ruling reopened via `unfrozen_by_ruling`.

## gentle-ai-owned artifacts authored by NERV roles

These keep gentle-ai's own filenames, location, and shape — `gentle-ai
sdd-status` depends on it — but are authored by NERV roles, never by the
`sdd-propose`/`spec`/`design`/`tasks` agents:

- `exploration.md` — Ritsuko (MODE: intel). Full FULL-path exploration;
  written before `tasks.md` exists, so it never references a task id.
- `specs/{domain}/spec.md` — Ritsuko (MODE: test-plan). WHAT the change
  must do, as scenarios, in gentle-ai's own spec shape; written alongside
  `nerv/test-plan.md`, also before `tasks.md` exists.
- `proposal.md`, `design.md`, `tasks.md` — Misato.
  - `design.md` MUST contain a `## New skills, scripts and commands`
    section listing every new skill, script, or command the plan
    introduces, or the single word `none`. This is Fuyutsuki's input —
    the plan gatekeeper (FULL pipeline step 3) verifies the section
    exists before the change proceeds to criticality.
  - `tasks.md` tasks carry stable ids (`T1`, `T2`, ...), a `pilot:
    rei|shinji|asuka|toji` field, and a `depends_on: []` field, each per
    task. A task ratcheted in from a LIGHT-path diff additionally carries
    `status: implemented-pre-plan` alongside its `id`/`pilot`/
    `depends_on` fields, and MAGI votes on it exactly like any other task
    — there is no free pass for pre-plan work.

## test-plan.md (Ritsuko)

- Purpose: FULL-path scenario plan — cases to create or modify, plus the
  corner-case interview Ritsuko cannot resolve alone. The corner-case
  answers gate Misato's plan (FULL pipeline step 2).
- Author: Ritsuko (MODE: test-plan). Written before `tasks.md` exists, so
  cases are not linked to task ids; Misato's own `tasks.md` maps tasks to
  the relevant cases when the plan is written.
- Location: `nerv/test-plan.md`.
- Engram key: `nerv/{change}/test-plan`.

```markdown
# Test plan — {change}

## Cases
| file | case | layer | expected behavior |
|---|---|---|---|
| {path} | {case name} | unit\|integration\|e2e | {expected behavior} |

## Corner-case questions
1. {question} — options: {option a} / {option b} / ...
2. {question} — options: {option a} / {option b} / ...

## Answers
1. {the user's answer, recorded verbatim by Ikari after relaying the
   question as part of one grouped blocking prompt}
2. {...}
```

`## Answers` starts empty when Ritsuko writes the file; Ikari fills it
after the user responds, never before, never on the user's behalf.

## criticality.md (Hyuga)

- Purpose: per-task criticality classification, driving the MAGI vote rule
  (unanimous for `critical`, 2-of-3 for `standard`).
- Author: Hyuga (dispatch a).
- Location: `nerv/criticality.md`.
- Engram key: `nerv/{change}/criticality`.

```markdown
# Criticality — {change}

| task_id | criticality | rationale | path_signals |
|---|---|---|---|
| T1 | critical\|standard | {why} | [{path1}, {path2}, ...] |
```

A task is auto-`critical` when it touches a `critical_paths` entry from
the merged `nerv.yaml`; any MAGI member may also escalate a task to
`critical` during voting (never down), which supersedes this table's
value for that round.

## votes.md (Ikari, merged from MAGI)

- Purpose: per-task, per-round record of the MAGI vote — each member's
  vote, findings, and escalations, plus the applied rule and result.
  Never authored directly by a MAGI member; Ikari merges the three blind
  JSON outputs it receives from one parallel batch.
- Author: Ikari.
- Location: `nerv/votes.md`.
- Engram key: `nerv/{change}/votes`.

```markdown
# Votes — {change}

## Round {n}

### {task_id}
| member | vote | findings | escalation |
|---|---|---|---|
| balthasar | approve\|reject | [{claim, category, evidence_class, proof_refs}] | null\|{to: critical, reason} |
| melchor | approve\|reject | [{claim, category, evidence_class, proof_refs}] | null\|{to: critical, reason} |
| casper | approve\|reject | [{claim, category, evidence_class, proof_refs}] | null\|{to: critical, reason} |

escalations: [{member, task_id, to: critical, reason}]
result: approved\|rejected
rule: unanimous\|majority-2:1
round: {n}
frozen: true\|false
unfrozen_by_ruling: {ruling_id}   # optional, per task — present only when a
                                  # binding Misato ruling reopened this task
```

Repeat the `### {task_id}` block per task in scope for the round; repeat
`## Round {n}` per re-vote (cap 2 re-votes per task, revised tasks only —
approved tasks stay `frozen: true` and are never re-voted).

### MAGI member JSON output contract

Each of `nerv:balthasar`, `nerv:melchor`, `nerv:casper` (MODE: vote) is
launched blind in the same parallel batch, with identical task locators
and no visibility into the other members' output. Its final text is
exactly one JSON object:

```json
{
  "round": 1,
  "votes": [
    {
      "task_id": "T1",
      "vote": "approve",
      "findings": [
        {
          "claim": "...",
          "category": "...",
          "evidence_class": "deterministic",
          "proof_refs": ["design.md:42"]
        }
      ],
      "escalation": null
    }
  ],
  "evidence": ["design.md read in full", "tasks.md T1 read"]
}
```

Lens assignment: Balthasar reviews design.md and tasks.md for SOLID/KISS/
YAGNI/DRY and pattern fit; Melchor reviews design.md for architecture,
design, dead code, duplication, and security; Casper reviews spec.md,
tasks.md, and proposal.md for docs, comments, scope, commit hygiene, and
plan consistency. Every member votes every task in scope from its own
lens; Ikari merges the three objects into the table above.

## veto-ruling.md (Fuyutsuki)

- Purpose: governance verdict on every item `design.md` declared under
  `## New skills, scripts and commands`.
- Author: Fuyutsuki.
- Location: `nerv/veto-ruling.md`.
- Engram key: `nerv/{change}/veto-ruling`.

```markdown
# Veto ruling — {change}

| item | verdict | reason |
|---|---|---|
| {skill\|script\|command name} | approve\|veto | {reason} |
```

When `design.md` declares `none`, this file records a single line stating
`none` instead of an empty table. Any `veto` reopens only the owning task
(frozen siblings stay frozen); cap 2 revision rounds, then the user
decides: drop the item, or abandon the task.

## waves.md (Hyuga)

- Purpose: implementation ordering — groups frozen tasks into waves with
  their dependencies and pilot assignment. Never mutates `tasks.md`.
- Author: Hyuga (dispatch b).
- Location: `nerv/waves.md`.
- Engram key: `nerv/{change}/waves`.

```markdown
# Waves — {change}

| wave_id | task_ids | depends_on | pilot_assignments |
|---|---|---|---|
| W1 | [T1, T2] | [] | {T1: shinji, T2: toji} |
| W2 | [T3] | [W1] | {T3: rei} |
```

Tasks in the same wave with no dependency between them may be launched in
one parallel batch during implementation (FULL pipeline step 11).

## Audit and closure artifacts (Phase 3)

### audit/diff-round-N.patch, audit/round-N.yaml (Aoba)

- Purpose: the frozen `BASE..HEAD` diff every audit pass reads, plus the
  exact base/HEAD hashes so a resumed session or a re-audit never
  re-derives the boundary by guesswork.
- Author: Aoba. Never hand-edited after generation.
- Location: `nerv/audit/diff-round-N.patch`, `nerv/audit/round-N.yaml`.
- Engram key: `nerv/{change}/audit-diff-round-N`,
  `nerv/{change}/audit-round-N`.

```
git diff <base>..HEAD > openspec/changes/{change}/nerv/audit/diff-round-N.patch
```

```yaml
round: 1
base: "a1b2c3d"          # the change's branch point for round 1,
                          # the previous round's HEAD for round N>1
head: "e4f5g6h"
created_at: "2026-09-24T15:10:00Z"
```

### audit/pass-<name>-round-N.json (Ikari, from each pass envelope)

- Purpose: the validated JSON object returned by one audit pass, persisted
  so Kaji's compile step and a resumed session can read it without
  re-running the pass.
- Author: Ikari (mechanical write of a pass's own final-text JSON, never
  edited).
- Location: `nerv/audit/pass-<name>-round-N.json`, `<name>` one of
  `melchor`, `balthasar`, `casper`, `kaji-security`, `kaji-coverage`.
- Engram key: `nerv/{change}/audit-pass-<name>-round-N`.

#### Audit pass JSON output contract

Each of `nerv:melchor`, `nerv:balthasar`, `nerv:casper` (MODE: audit),
`nerv:kaji-security`, `nerv:kaji-coverage` is launched in one parallel
batch, blind to the other four, over the frozen `diff-round-N.patch` plus
the plan artifacts (`proposal.md`, `design.md`, `tasks.md`, `specs/`,
`nerv/test-plan.md`). Its final text is exactly one JSON object, no prose
before or after it (a `## Key Learnings` block may follow):

```json
{
  "pass": "kaji-coverage",
  "round": 1,
  "findings": [
    {
      "id": "kaji-coverage-divide-by-zero",
      "location": "src/Calc/Calculator.cs:42",
      "severity": "CRITICAL",
      "claim": "Divide has no test for a zero divisor",
      "evidence_class": "deterministic",
      "causal_disposition": "introduced",
      "proof_refs": ["diff-round-1.patch:Calculator.cs hunk", "nerv/test-plan.md"]
    }
  ],
  "evidence": ["diff-round-1.patch read in full", "nerv/test-plan.md read"]
}
```

Severity scale and candidate-causal admission are the same rule used
everywhere in NERV (mirrors gentle-ai's native review lenses):

- `BLOCKER` — catastrophic impact or no viable recovery.
- `CRITICAL` — material user, security, data, or correctness failure.
- `WARNING` — non-blocking follow-up.
- `SUGGESTION` — optional improvement.
- **Candidate-causal admission**: `BLOCKER`/`CRITICAL` require proof
  (changed hunk, created path, before/after) that the candidate
  introduced, activated, or worsened the behavior. Unproven causality is
  `unknown` and ranks as `WARNING` at most. `pre-existing` findings are
  follow-ups and never block closure. Only candidate-caused
  `BLOCKER`/`CRITICAL` block closure.

Lens assignment for AUDIT mode mirrors VOTE mode's lenses, applied to the
frozen patch instead of the plan: Melchor — architecture, design, dead
code, duplication; Balthasar — SOLID, KISS, YAGNI, DRY, pattern fit;
Casper — plan conformance (every task in `tasks.md` delivered as
specified and nothing extra, BASE..HEAD), commit hygiene (atomic,
conventional, correct scopes), and TDD commit order (the RED commit
precedes the GREEN commit for every task, checked in git history);
`kaji-security` — security across all layers; `kaji-coverage` —
implemented tests vs `nerv/test-plan.md` (missing cases, weakened
assertions). Passes never edit files and never persist their own output —
Ikari writes the validated object to its locator.

## audit-report.md (Kaji)

- Purpose: the merged, deduplicated audit findings for one round, ranked
  candidate-causal admission applied, refuter outcomes folded in.
- Author: `nerv:kaji` (compile), refuter outcomes merged in by Ikari after
  `nerv:kaji-refuter` runs.
- Location: `nerv/audit-report.md` (one file, reused and extended each
  round — round N's compile carries forward unresolved items from N-1).
- Engram key: `nerv/{change}/audit-report`.

```markdown
# Audit report — {change}

## Round {n}

| id | location | severity | claim | evidence_class | causal_disposition | credited_sources | refuter |
|---|---|---|---|---|---|---|---|
| kaji-coverage-divide-by-zero | src/Calc/Calculator.cs:42 | CRITICAL | Divide has no test for a zero divisor | deterministic | introduced | [kaji-coverage] | n/a |
| balthasar-unchecked-divisor | src/Calc/Calculator.cs:42 | WARNING | Divisor not validated before use | inferential | introduced | [balthasar, kaji-security] | corroborated |

## Refuted

| id | location | severity | claim | outcome | proof_refs |
|---|---|---|---|---|---|
```

Dedupe rule: same `file:line` (or overlapping range) plus the same defect
signature is one item; `credited_sources[]` lists every pass that found
it. Severity is the max across sources. Candidate-causal admission (above)
applies before ranking. Deterministic `BLOCKER`/`CRITICAL` need no
refuter (`refuter: n/a`); every inferential `BLOCKER`/`CRITICAL` is listed
in that round's refuter batch (`refuter: pending` until the batch
returns).

#### Refuter batch contract

`nerv:kaji-refuter` runs once per audit round, reading only the frozen
patch and repo history (read-only), over exactly that round's inferential
`BLOCKER`/`CRITICAL` items. Its final text is exactly one JSON object:

```json
{
  "round": 1,
  "results": [
    {
      "finding_id": "balthasar-unchecked-divisor",
      "outcome": "corroborated",
      "proof_refs": ["src/Calc/Calculator.cs:42", "git log -p src/Calc/Calculator.cs"]
    }
  ]
}
```

`outcome` is one of `corroborated`, `refuted`, `inconclusive`. The refuter
never adds findings — only judges the ones it is given. Ikari merges
outcomes into `audit-report.md`: `refuted` → the item moves to the
`## Refuted` appendix and drops from the active ranking; `inconclusive` →
kept in the active table, ranked as `WARNING` for Hyuga's purposes
regardless of its original severity.

## issue-ranking.md (Hyuga)

- Purpose: the ranked, binding fix order Ikari relays as the user HARD
  issue gate, and the record of that gate's decision including any
  residual accepted at the re-audit cap.
- Author: `nerv:hyuga` (dispatch c).
- Location: `nerv/issue-ranking.md`.
- Engram key: `nerv/{change}/issue-ranking`.

```markdown
# Issue ranking — {change}

## Round {n}

| issue_id | severity | blast_radius | verification_cost | decision | reason | fix_order | owner |
|---|---|---|---|---|---|---|---|
| kaji-coverage-divide-by-zero | Critical | local | cheap | NOW | candidate-caused, unguarded divide | 1 | shinji |
| balthasar-unchecked-divisor | Important | local | cheap | DEFER | same root cause as #1, covered by the guard fix | - | shinji |

## Gate decision

decision: approved-as-ranked | edited | residual-accepted
edited_ids: []             # ids the user added or dropped, when decision is "edited"

## residual_accepted

{present only at the re-audit cap, when the user accepts the residual
instead of a further re-audit round}
round_at_cap: {n}
accepted_findings: [{issue_id}, ...]
reason: "{user's stated reason, recorded verbatim}"
```

`severity` is `Critical | Important | Minor`; `blast_radius` is
`local | module | cross-module | system`; `verification_cost` is
`cheap | moderate | expensive`, ties broken cheapest-verification-first.
The NOW set is every candidate-caused `BLOCKER`/`CRITICAL` plus whatever
Hyuga argues in; every `DEFER` item carries a one-line `reason`. The gate
Ikari relays offers exactly three options: approve the NOW set as ranked,
edit it (free text naming ids to add or drop), or accept the residual and
close — the last option is available only when the round is at the
re-audit cap (2).

## issue-resolutions.md (Ritsuko)

- Purpose: closes the loop between every audit-report item and its fate —
  the fix commit that resolved it, or the residual decision that left it
  open.
- Author: `nerv:ritsuko` (MODE: docs).
- Location: `nerv/issue-resolutions.md`.
- Engram key: `nerv/{change}/issue-resolutions`.

```markdown
# Issue resolutions — {change}

| issue_id | severity | resolution | commit | notes |
|---|---|---|---|---|
| kaji-coverage-divide-by-zero | Critical | fixed | {hash} | DivideByZero guard added, regression test by kaworu |
| balthasar-unchecked-divisor | Important | deferred | - | residual_accepted at re-audit cap round 2 |
```

Ritsuko reads in authority order — persisted `tasks.md` > Ikari's launch
facts > `nerv/maya-report.md` — to resolve which commit closed which
issue; a mismatch between sources is reported, never silently resolved by
guessing.

## agent-config.md (Ritsuko)

- Purpose: the per-launch record of who ran the change — pilots used,
  models, and skill resolution — for audit and reproducibility.
- Author: `nerv:ritsuko` (MODE: docs).
- Location: `nerv/agent-config.md`.
- Engram key: `nerv/{change}/agent-config`.

```markdown
# Agent config — {change}

| phase | agent | model | effort | skill_resolution |
|---|---|---|---|---|
| implementation W1/T1 | nerv:shinji | sonnet | medium | paths-injected |
| audit round 1 | nerv:kaji-coverage | sonnet | medium | paths-injected |
```

One row per launch across the whole change, pilots and audit passes
included; `skill_resolution` copies the value each agent's own return
envelope reported (`paths-injected`, `fallback-registry`,
`fallback-path`, or `none`).

### Phase 3 event types

The audit-and-closure stage adds these event types to
`nerv/deliberation-log.md`, same shape as every other entry
(`{ts, phase, actor, event_type, payload_ref}`):

- `patch_frozen` — Aoba froze `diff-round-N.patch` (payload: round, base,
  head).
- `audit_pass` — one audit pass's validated JSON object was written to
  `nerv/audit/pass-<name>-round-N.json` (one entry per pass per round).
- `dedupe_merge` — Kaji merged two or more pass findings into one
  `audit-report.md` item (payload: merged `id`, `credited_sources[]`).
- `refuter_result` — one refuter verdict was merged into `audit-report.md`
  (payload: `finding_id`, `outcome`).
- `ranking_issued` — Hyuga produced or updated `issue-ranking.md`.
- `issue_gate_relayed` — Ikari presented the ranked NOW/DEFER list to the
  user as one blocking prompt.
- `issue_gate_decision` — the user's issue-gate decision (approved /
  edited / residual-accepted).
- `fix_routed` — an approved issue was routed to its owning pilot's
  LIGHT work-unit cycle (payload: `issue_id`, `owner`).
- `reaudit` — a new audit round started over the fix-delta patch (payload:
  round number).
- `residual_accepted` — the user accepted the residual at the re-audit
  cap instead of a further round (payload: round, accepted findings).
- `docs_written` — Ritsuko wrote `issue-resolutions.md`, `agent-config.md`,
  and any repo doc deltas.
- `archived` — Aoba's Archive duty moved the change to
  `openspec/changes/archive/YYYY-MM-DD-{change}/` (payload:
  `{archive_path, composed_specs[]}`).
- `log_curated` — Fuyutsuki appended the `## Summary` block to
  `nerv/deliberation-log.md` (MODE: curate).

### Phase 4 event types

The task-tracking layer adds one event type to the same append-only log,
same shape (`{ts, phase, actor, event_type, payload_ref}`):

- `tracker_event` — one task-tracking port operation completed (payload:
  `{op, taskRef, result}`, taken verbatim from `nerv:hyuga`'s
  `DISPATCH: tracker` envelope). Logged by Ikari after every tracker
  dispatch: Preflight's `createTask`/`start`, Maya's full-gate
  `moveStage(testing)`, the issue gate's `comment`/`createTask` for
  accepted `DEFER` issues, and Close's `close`/`done`/`block`.

### Phase 5 event types

The orchestrator-lock and safe-resume hardening adds these event types to
the same append-only log, same shape (`{ts, phase, actor, event_type,
payload_ref}`):

- `resume` — a NERV session resumed an interrupted change (payload:
  `{from_step, took_over_from}` — `took_over_from` carries the stale
  session's `session_id` on a takeover, or is absent on a normal resume).
- `lock_refused` — a resume attempt found a fresh `.orchestrator.lock` held
  by another session and the user chose to wait rather than take over
  (payload: the other session's `session_id` and `heartbeat_at`).
