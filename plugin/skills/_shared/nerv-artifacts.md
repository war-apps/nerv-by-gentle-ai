# NERV Artifact Schemas

One definition per NERV-owned artifact, referenced by every agent instead of
repeating the shape inline. Every run is tracked on gentle-ai's ODD feature
document, `odd/tasks/{feature}.md` (header, `## Specs`, `## Tasks`,
`## Log`; Engram mirror `odd/{feature}/tasks`); which role writes which
section is defined in `nerv-orchestrator/references/odd-document.md` and
summarized in `## ODD document sections authored by NERV roles` below. The
NERV governance files below live in the run folder `odd/{feature}/nerv/`
and are committed with the feature per `artifacts.commit`; every `nerv/...`
locator is relative to `odd/{feature}/`. The orchestrator lock is the one
exception: it lives in the runtime folder, `.nerv/locks/{feature}.lock`,
and is never committed. Engram topic key for every run-folder artifact:
`nerv/{feature}/{artifact}` (artifact = file name without extension).

## state.yaml

- Purpose: the run's state, with the NERV path decision so a resumed
  session knows which pipeline governed the feature without
  re-classifying.
- Author: Ikari only; never hand-edited by NERV agents.
- Location: `nerv/state.yaml` (`odd/{feature}/nerv/state.yaml`).
- Engram key: `nerv/{feature}/state` (mirror of the `nerv:` block only).
- `feature` is the feature name, the `{feature}` of every locator.
- `task_ref` holds `{PREFIX}-{id}`, e.g. `TW-49132010`, taken from the
  `run.started` envelope the task plugin returns at Preflight, or `null`
  when the change has no task (the task plugin is not installed, or its
  envelopes carry no `task_ref`).
- `closed_at` is `null` until the feature closes, then set once by Ikari at
  Close (DIRECT's or LIGHT's Close step, or FULL's step 19), the same
  write that deletes `.nerv/locks/{feature}.lock`.
- The merged configuration's `models` section (per-role `model`/`effort`,
  resolved as described in `nerv-orchestrator/SKILL.md`'s Configuration
  resolution) is not mirrored into `state.yaml`; it is resolved fresh each
  session.

```yaml
nerv:
  feature: "tw-49132010-short-slug"
  path: light                     # direct | light | full
  classification_reason: "single domain, no critical path, under budget"
  task_ref: "TW-49132010"         # or null when no task
  created_at: "2026-09-24T14:03:00Z"
  closed_at: null                 # set once, at Close
```

## Orchestrator lock (Ikari)

- Purpose: concurrency guard — detects a second NERV session resuming the
  same feature while the first is still alive, so Resume never races an
  active run.
- Author: Ikari only; no other agent reads or writes this file. Never
  persisted to Engram or committed to git — local, ephemeral, machine-scoped
  state in the runtime folder `.nerv/`, which ignores itself.
- Location: `.nerv/locks/{feature}.lock`.
- Engram key: none.

```yaml
session_id: "26b0c7bf-8fa7-4b75"   # UUID segment of the scratchpad path
host: "WALTER-PC"                  # machine name
started_at: "2026-09-24T14:03:00Z"
heartbeat_at: "2026-09-24T14:11:00Z"
phase: "full"
step: "11-wave-2"
waiting_on: "agent"                # agent | user | none
pid: null
```

`waiting_on: user` marks a lock parked on a blocking prompt; such a lock is
held at any heartbeat age (a human gate has no upper bound) and a resume
against it must ask the user before taking over. Ikari sets it right before
relaying the prompt and clears it (`none`) right after the answer.

Fresh when `heartbeat_at` is under 15 minutes old (see `## Orchestrator
lock` in `nerv-orchestrator/SKILL.md`); refreshed before every launch and
after every envelope; deleted at close or on an explicit stop.

## exploration-light.md (Ritsuko)

- Purpose: LIGHT-path micro-intel — enough to pick a pilot and scope the
  verification, not a full exploration document; written only when the
  mapping trigger fired.
- Author: Ritsuko (LIGHT micro-intel dispatch).
- Location: `nerv/exploration-light.md`.
- Engram key: `nerv/{feature}/exploration-light`.

```markdown
# Exploration (light) — {feature}

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
- Engram key: `nerv/{feature}/maya-report`.

```markdown
# Maya report — {feature}

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

## run-summary.md (Aoba or Ikari)

- Purpose: close-out narrative, usage accounting, RDD receipts, commit and
  PR-slice record for the change.
- Author: Aoba in FULL; Ikari itself in DIRECT and LIGHT (the Agents table
  then lists only the agents actually launched, possibly none).
- Location: `nerv/run-summary.md`.
- Engram key: `nerv/{feature}/run-summary`.

```markdown
# Run summary — {feature}

## Work done
{short narrative}

## Agents
| agent | model | tokens_in | tokens_out | tokens_total | duration_s |
|---|---|---|---|---|---|
| nerv:kaworu | sonnet | {n} | {n} | {n} | {n} |

Each figure has the source named in the Usage collection section of
`nerv-orchestrator/references/usage-and-log.md`: `tokens_total` and
`duration_s` from the Agent tool's usage block, `tokens_in` and
`tokens_out` from the launch's subagent transcript, `model` as that
transcript reports it. Figures are never estimated; the only allowed gap
is `n/a (no transcript)`.

## RDD receipts
| commit | review_due | outcome | boundary_advanced_to |
|---|---|---|---|

## Commits
| hash | subject |
|---|---|

## PR slices
{none, or the chained/stacked slice list with boundaries}

## KB mirrors unflushed
{only when the Engram flush at close left some knowledge-base mirror
unsaved: one line per mirror (topic key, error); else omit}

## Task events pending
{only when the task plugin is installed and some envelope in
nerv/task-events.yaml is pending: one line per envelope (event, provider,
error), then: replay them with `/task:replay {feature}`}
```

## task-events.yaml (Ikari, append-only)

- Purpose: every envelope the task plugin's `task:dispatcher` returned for
  this change, so undelivered events can be replayed.
- Author: Ikari, after every dispatch. Never created when the task plugin
  is not installed (`task:dispatcher` unavailable).
- Location: `nerv/task-events.yaml`.
- Shape: one list item per envelope, verbatim from the dispatcher
  (`{event, provider, result, task_ref, data, error, replay}`), plus the
  `payload` it was dispatched with. An envelope with `result` `blocked` or
  `failed` and `replay: true` is **pending** until `/task:replay {feature}`
  marks it `replayed: <ISO time>` with its new result.

```yaml
- event: run.started
  provider: <provider name>
  result: ok                      # ok | skipped | blocked | failed | needs_input
  task_ref: "TW-49132010"
  data: {stage: "<provider stage>", timer: started}
  payload: {task_ref: "", repo: "<repo>", feature: "{feature}", branch: "<branch>", worktree: "", session_id: "<uuid>", data: {}, disabled_events: []}
```

## deliberation-log.md (Ikari, append-only)

- Purpose: full event trail of the run — the only append-only NERV
  artifact; never rewritten, only appended to.
- Author: Ikari, appending inline after each event (a mechanical
  single-file write, not delegated).
- Location: `nerv/deliberation-log.md`.
- Engram key: `nerv/{feature}/deliberation-log` (re-saved in full on each
  append, since Engram upserts overwrite rather than append).

```markdown
# Deliberation log — {feature}

- {ts} | {phase} | {actor} | preflight_answer | {payload_ref}
- {ts} | {phase} | {actor} | classification | {payload_ref}
- {ts} | {phase} | {actor} | ratchet | {payload_ref}
- {ts} | {phase} | {actor} | launch | ({model}) {Display name}: {action}; {payload_ref}
- {ts} | {phase} | {actor} | effort_drift | {payload_ref}
- {ts} | {phase} | {actor} | envelope | {payload_ref}
- {ts} | {phase} | {actor} | model_mismatch | {payload_ref}
- {ts} | {phase} | {actor} | gate_relayed | {payload_ref}
- {ts} | {phase} | {actor} | gate_decision | {payload_ref}
- {ts} | {phase} | {actor} | commit_recorded | {payload_ref}
- {ts} | {phase} | {actor} | rdd_assess | {payload_ref}
- {ts} | {phase} | {actor} | rdd_receipt | {payload_ref}
- {ts} | {phase} | {actor} | stop | {payload_ref}
```

`payload_ref` is a short pointer (commit hash, envelope field, file path) —
never the full payload inline; keep the log scannable. Two exceptions carry
their payload shape here rather than by pointer, since it is already
short: `model_mismatch` (payload: `{role, resolved, reported}`) — logged
by the Mandatory model gate when an envelope readback's reported model
differs from the resolved one; a warning, never a stop. `effort_drift`
(payload: `{role, resolved, cached}`) — logged once per role per session
when the resolved effort differs from that role's cached agent frontmatter
effort; the pipeline continues either way. A `launch` line's payload opens
with the launch's `({model}) {Display name}: {action}` label — the same text as its
Agent tool `description` — before the pointer.

### Phase 2 event types

FULL adds these event types to the same append-only log, same shape
(`{ts, phase, actor, event_type, payload_ref}`):

- `corner_case_relayed` — Ikari relayed Ritsuko's corner-case questions to
  the user as one grouped blocking prompt.
- `corner_case_answer` — the user's answer was recorded verbatim into
  the test plan's `### Answers` in the ODD document's `## Log`.
- `vote_cast` — one MAGI member's vote for one task in one round (one
  entry per member per task).
- `escalation_criticality` — a MAGI member escalated a task's criticality
  to `critical` (escalation is one-way, never down).
- `vote_result` — a task's round concluded: `approved` or `rejected`,
  with the applied rule (`unanimous` or `majority-2:1`).
- `veto_evaluated` — Fuyutsuki ruled on one item from the design Log
  entry's `### New skills, scripts and commands`.
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

## ODD document sections authored by NERV roles

The ODD document `odd/tasks/{feature}.md` replaces the former per-change
plan files; its sections are authored by NERV roles (full table in
`nerv-orchestrator/references/odd-document.md`):

- Header, `L1` and the seeded `S#` — Ikari, from the user's request
  verbatim, plus the TDD mode and runner.
- `nerv/exploration.md` — Ritsuko (MODE: intel). Full FULL-path
  exploration, written by Ikari verbatim to the run folder with a one-line
  `## Log` pointer; written before `## Tasks` exists, so it never
  references a task id.
- The test plan — Ritsuko (MODE: test-plan), one `## Log` entry (shape
  below), written by Ikari verbatim.
- `## Specs` (scope, acceptance criteria, checks, marked assumptions),
  `## Tasks`, and the design `## Log` entry — Misato.
  - The design entry MUST contain a `### New skills, scripts and
    commands` heading listing every new skill, script, or command the
    plan introduces, or the single word `none`. This is Fuyutsuki's input
    — the plan gatekeeper (FULL pipeline step 3) verifies it exists
    before the feature proceeds to criticality.
  - `## Tasks` rows carry stable ids (`T1`, `T2`, ...), the linked `S#`,
    a `Pilot` (`rei|shinji|asuka|toji`) and `Depends on`, each per task.
    A task ratcheted in from a DIRECT- or LIGHT-path diff carries
    `Status` `implemented-pre-plan`, and MAGI votes on it exactly like any
    other task — there is no free pass for pre-plan work.
- `Criticality`, `MAGI`, `Status` and `Commit` columns — Ikari's
  bookkeeping, copied from `nerv/criticality.md`, `nerv/votes.md` and the
  commits.

### The test plan (Ritsuko, in the `## Log`)

- Purpose: FULL-path scenario plan — cases to create or modify per `S#`,
  plus the corner-case interview Ritsuko cannot resolve alone. The
  corner-case answers gate Misato's plan (FULL pipeline step 2).
- Author: Ritsuko (MODE: test-plan), returned as text; Ikari appends it
  verbatim as one `## Log` entry. Written before `## Tasks` exists, so
  cases link to `S#`, not to task ids; Misato maps tasks to cases when the
  plan is written.
- Location: the ODD document's `## Log`, entry `L{n} test plan`.

```markdown
- **L{n}** ({date}): test plan (Ritsuko).
  | S# | file | case | layer | expected behavior |
  |---|---|---|---|---|
  | S1 | {path} | {case name} | unit\|integration\|e2e | {expected behavior} |

  ### Corner-case questions
  1. {question} — options: {option a} / {option b} / ...

  ### Answers
  1. {the user's answer, recorded verbatim by Ikari after relaying the
     question as part of one grouped blocking prompt}
```

`### Answers` starts empty when the entry is written; Ikari fills it after
the user responds, never before, never on the user's behalf. An answer
that changes a requirement also gets its own verbatim `## Log` entry, and
Misato rewrites only the affected `S#`.

## criticality.md (Hyuga)

- Purpose: per-task criticality classification, driving the MAGI vote rule
  (unanimous for `critical`, 2-of-3 for `standard`).
- Author: Hyuga (dispatch a).
- Location: `nerv/criticality.md`.
- Engram key: `nerv/{feature}/criticality`.

```markdown
# Criticality — {feature}

| task_id | criticality | rationale | path_signals |
|---|---|---|---|
| T1 | critical\|standard | {why} | [{path1}, {path2}, ...] |
```

A task is auto-`critical` when it touches a `critical_paths` entry from
the merged configuration; any MAGI member may also escalate a task to
`critical` during voting (never down), which supersedes this table's
value for that round.

## votes.md (Ikari, merged from MAGI)

- Purpose: per-task, per-round record of the MAGI vote — each member's
  vote, findings, and escalations, plus the applied rule and result.
  Never authored directly by a MAGI member; Ikari merges the three blind
  JSON outputs it receives from one parallel batch.
- Author: Ikari.
- Location: `nerv/votes.md`.
- Engram key: `nerv/{feature}/votes`.

```markdown
# Votes — {feature}

## Round {n}

### {task_id}
| member | vote | findings | escalation |
|---|---|---|---|
| balthasar | approve\|reject | [{claim, category, evidence_class, proof_refs}] | null\|{to: critical, reason} |
| melchior | approve\|reject | [{claim, category, evidence_class, proof_refs}] | null\|{to: critical, reason} |
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

Each of `nerv:balthasar`, `nerv:melchior`, `nerv:casper` (MODE: vote) is
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
          "proof_refs": ["odd/tasks/{feature}.md:42"]
        }
      ],
      "escalation": null
    }
  ],
  "evidence": ["ODD document design entry read in full", "## Tasks T1 read"]
}
```

Lens assignment: Balthasar reviews the design entry and `## Tasks` for
SOLID/KISS/YAGNI/DRY and pattern fit; Melchior reviews the design entry
for architecture, design, dead code, duplication, and security; Casper
reviews `## Specs`, `## Tasks`, and the test plan for docs, comments,
scope, commit hygiene, and plan consistency. Every member votes every task in scope from its own
lens; Ikari merges the three objects into the table above.

## veto-ruling.md (Fuyutsuki)

- Purpose: governance verdict on every item the design Log entry declared
  under `### New skills, scripts and commands`.
- Author: Fuyutsuki.
- Location: `nerv/veto-ruling.md`.
- Engram key: `nerv/{feature}/veto-ruling`.

```markdown
# Veto ruling — {feature}

| item | verdict | reason |
|---|---|---|
| {skill\|script\|command name} | approve\|veto | {reason} |
```

When the design entry declares `none`, this file records a single line stating
`none` instead of an empty table. Any `veto` reopens only the owning task
(frozen siblings stay frozen); cap 2 revision rounds, then the user
decides: drop the item, or abandon the task.

## waves.md (Hyuga)

- Purpose: implementation ordering — groups frozen tasks into waves with
  their dependencies and pilot assignment. Never edits the ODD document.
- Author: Hyuga (dispatch b).
- Location: `nerv/waves.md`.
- Engram key: `nerv/{feature}/waves`.

```markdown
# Waves — {feature}

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
- Engram key: `nerv/{feature}/audit-diff-round-N`,
  `nerv/{feature}/audit-round-N`.

```
git diff <base>..HEAD > odd/{feature}/nerv/audit/diff-round-N.patch
```

```yaml
round: 1
base: "a1b2c3d"          # the feature's branch point for round 1,
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
  `melchior`, `balthasar`, `casper`, `gendo`.
- Engram key: `nerv/{feature}/audit-pass-<name>-round-N`.

#### Legacy pass names

A round started before the `melchior`/`gendo` rename and resumed after it
may hold `pass-melchor-round-N.json` and `pass-kaji-audit-round-N.json`
(Engram `nerv/{feature}/audit-pass-melchor-round-N` and
`nerv/{feature}/audit-pass-kaji-audit-round-N`). Whoever reads a round's
passes (Kaji, or Ikari on resume) reads the legacy name only when the new
one is absent and counts it as the `melchior` or `gendo` pass
respectively, whatever its own `pass` field says; nothing is renamed or
rewritten.

#### Audit pass JSON output contract

Each of `nerv:melchior`, `nerv:balthasar`, `nerv:casper` (MODE: audit),
and `nerv:gendo` is
launched in one parallel batch, blind to the other three, over the frozen
`diff-round-N.patch` plus the ODD document (`## Specs`, `## Tasks`, and the
design and test-plan `## Log` entries). Its final text is exactly one JSON object, no prose
before or after it (a `## Key Learnings` block may follow):

```json
{
  "pass": "gendo",
  "round": 1,
  "findings": [
    {
      "id": "gendo-divide-by-zero",
      "location": "src/Calc/Calculator.cs:42",
      "severity": "CRITICAL",
      "claim": "Divide has no test for a zero divisor",
      "evidence_class": "deterministic",
      "causal_disposition": "introduced",
      "proof_refs": ["diff-round-1.patch:Calculator.cs hunk", "odd/tasks/{feature}.md test plan"]
    }
  ],
  "evidence": ["diff-round-1.patch read in full", "test plan read"]
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
frozen patch instead of the plan: Melchior — architecture, design, dead
code, duplication, plus security across all layers (injection, authz,
secrets, data exposure, unsafe defaults, dependency risk, crypto, infra
hardening); Balthasar — SOLID, KISS, YAGNI, DRY, pattern fit, plus
readability (misleading names, unexplained constants, complexity,
intention, review size and context; `WARNING` at most unless the obscured
behavior is a proven correctness failure); Casper — plan conformance
(every task in `## Tasks` delivered as specified and nothing extra,
BASE..HEAD), commit hygiene (atomic, conventional, correct scopes), and
TDD commit order (the RED commit precedes the GREEN commit for every
task, checked in git history); `gendo` — implemented tests vs
the test plan (missing cases, weakened assertions), reliability
beyond the plan (invalid inputs, failure paths, contracts, boundaries,
regressions, determinism), correctness and edge cases in the changed
implementation, and resilience and performance (fallbacks and graceful
degradation, retry/backoff safety, timeouts and cancellation, rollback
or fix-forward safety, latency, load, resource use and SLO risk,
performance regressions such as N+1, unbounded loops or allocations,
blocking I/O on hot paths, missing pagination, accidental quadratic
work, and observability of failures); findings outside the test plan
follow the same candidate-causal admission. Passes never edit files and never persist their own output —
Ikari writes the validated object to its locator.

## audit-report.md (Kaji)

- Purpose: the merged, deduplicated audit findings for one round, ranked
  candidate-causal admission applied, refuter outcomes folded in.
- Author: `nerv:kaji` (compile), refuter outcomes merged in by Ikari after
  `nerv:fuyutsuki` runs in `MODE: refute`.
- Location: `nerv/audit-report.md` (one file, reused and extended each
  round — round N's compile carries forward unresolved items from N-1).
- Engram key: `nerv/{feature}/audit-report`.

```markdown
# Audit report — {feature}

## Round {n}

| id | location | severity | claim | evidence_class | causal_disposition | credited_sources | refuter |
|---|---|---|---|---|---|---|---|
| gendo-divide-by-zero | src/Calc/Calculator.cs:42 | CRITICAL | Divide has no test for a zero divisor | deterministic | introduced | [gendo] | n/a |
| melchior-unchecked-divisor | src/Calc/Calculator.cs:42 | WARNING | Divisor not validated before use | inferential | introduced | [melchior, balthasar] | corroborated |

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

`nerv:fuyutsuki` (MODE: refute) runs once per audit round, reading only
the frozen patch and repo history (read-only; in this mode he writes
nothing and calls no `mem_save`), over exactly that round's inferential
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
- Engram key: `nerv/{feature}/issue-ranking`.

```markdown
# Issue ranking — {feature}

## Round {n}

| issue_id | severity | blast_radius | verification_cost | decision | reason | fix_order | owner |
|---|---|---|---|---|---|---|---|
| gendo-divide-by-zero | Critical | local | cheap | NOW | candidate-caused, unguarded divide | 1 | shinji |
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
- Engram key: `nerv/{feature}/issue-resolutions`.

```markdown
# Issue resolutions — {feature}

| issue_id | severity | resolution | commit | notes |
|---|---|---|---|---|
| gendo-divide-by-zero | Critical | fixed | {hash} | DivideByZero guard added, regression test by kaworu |
| balthasar-unchecked-divisor | Important | deferred | - | residual_accepted at re-audit cap round 2 |
```

Ritsuko reads in authority order — the persisted `## Tasks` > Ikari's launch
facts > `nerv/maya-report.md` — to resolve which commit closed which
issue; a mismatch between sources is reported, never silently resolved by
guessing.

## agent-config.md (Ritsuko)

- Purpose: the per-launch record of who ran the change — pilots used,
  models, and skill resolution — for audit and reproducibility.
- Author: `nerv:ritsuko` (MODE: docs).
- Location: `nerv/agent-config.md`.
- Engram key: `nerv/{feature}/agent-config`.

```markdown
# Agent config — {feature}

| phase | agent | model | effort | skill_resolution |
|---|---|---|---|---|
| implementation W1/T1 | nerv:shinji | sonnet | medium | paths-injected |
| audit round 1 | nerv:gendo | sonnet | medium | paths-injected |
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
  FULL work-unit cycle (payload: `issue_id`, `owner`).
- `reaudit` — a new audit round started over the fix-delta patch (payload:
  round number).
- `residual_accepted` — the user accepted the residual at the re-audit
  cap instead of a further round (payload: round, accepted findings).
- `docs_written` — Ritsuko wrote `issue-resolutions.md`, `agent-config.md`,
  and any repo doc deltas.
- `log_curated` — Fuyutsuki appended the `## Summary` block to
  `nerv/deliberation-log.md` (MODE: curate).

### Task plugin event types

When the task plugin is installed, it adds one event type to the same
append-only log, same shape (`{ts, phase, actor, event_type, payload_ref}`):

- `task_event` — one envelope returned by `task:dispatcher` (payload:
  `{event, provider, result, task_ref}`, taken verbatim from the envelope,
  which Ikari also appends to `nerv/task-events.yaml`). Logged by Ikari
  after every dispatch: `run.started` at Preflight,
  `verification.started`, `wave.started`, `audit.ranked` and
  `issue.deferred` at the issue gate, and `run.closed` or `run.halted` at
  Close. Never logged when the task plugin is not installed.

### Phase 5 event types

The orchestrator-lock and safe-resume hardening adds these event types to
the same append-only log, same shape (`{ts, phase, actor, event_type,
payload_ref}`):

- `resume` — a NERV session resumed an interrupted change (payload:
  `{from_step, took_over_from}` — `took_over_from` carries the stale
  session's `session_id` on a takeover, or is absent on a normal resume).
- `lock_refused` — a resume attempt found a fresh orchestrator lock (`.nerv/locks/{feature}.lock`) held
  by another session and the user chose to wait rather than take over
  (payload: the other session's `session_id` and `heartbeat_at`).

### Run hygiene event types

- `kb_mirror_flushed` — Ikari saved a knowledge-base mirror an agent
  handed back as `Pending KB mirror` (payload: topic key, result), at the
  Engram flush at close.

## Knowledge base (Engram project nerv)

Decision artifacts from every NERV-governed repository are mirrored to
the shared Engram project `nerv` — the knowledge base every NERV run
reads for precedent, regardless of which repo produced it (see `##
Configuration resolution` in `nerv-orchestrator/SKILL.md` and sections B
and C of `nerv-phase-common.md`). Topic-key scheme:
`nerv/kb/{repo}/{feature}/{artifact}`, where `{repo}` is the basename of
the git toplevel.

| artifact | mirrored by |
|---|---|
| votes (per-task result, rule, escalations, frozen) plus `plan_gate_decision` / `issue_gate_decision` | Ikari |
| veto-ruling | Fuyutsuki |
| ruling-* and the `## Decisions` extract from a plan/design decision | Misato |
| audit-report | Kaji |

Per-feature artifacts (`nerv/{feature}/{artifact}`, or their
`odd/{feature}/nerv/...` file) keep going to the repo's own
Engram project exactly as documented per artifact above; the knowledge-
base mirror is an additional write, never a substitute for it.
