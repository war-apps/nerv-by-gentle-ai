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

```yaml
dependsOn: []
nerv:
  path: light                     # light | full
  classification_reason: "single domain, no critical path, under budget"
  task_ref: "TW-49132010"         # or null when no task
  created_at: "2026-09-24T14:03:00Z"
```

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

## FULL-path artifacts (Phase 3)

Named here only — schemas ship with Phase 3, when Kaji and its audit
passes are installed:

- `nerv/audit-report.md` (Kaji)
- `nerv/issue-ranking.md` (Hyuga)
- `nerv/audit/diff-round-N.patch` (Aoba)
- `nerv/issue-resolutions.md` (Ritsuko)
- `nerv/agent-config.md` (Ritsuko)
