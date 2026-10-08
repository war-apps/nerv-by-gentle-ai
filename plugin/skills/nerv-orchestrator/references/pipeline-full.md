## FULL pipeline (Phase 3)

FULL adds MAGI vote, governance veto, waves, quality-gated implementation,
and a full audit-and-closure stage on top of the LIGHT primitives
(RED/GREEN/REFACTOR, Aoba commits, the RDD hook, usage collection, the
deliberation log — all reused unchanged, see the LIGHT pipeline above).
Once every wave is closed and Maya's full gate is green, the run proceeds
into the audit stage (steps 13-19 below): a frozen patch, five blind audit
passes, Kaji's compilation and the refuter batch, Hyuga's ranking behind a
user HARD issue gate, fix routing with a bounded re-audit loop, then
documentation, archive, and the run summary. Never silently skip a step in
the table below or downgrade FULL to LIGHT mid-run.

| Step | Actor | Launch prompt carries | Expected envelope | Gate |
|---|---|---|---|---|
| 0. Preflight | Ikari, Hyuga (d), Aoba | — | tracker start via Hyuga `DISPATCH: tracker` (`createTask` if requested, then `start`); worktree/branch state | user HARD if asked |
| 1. Intel | `nerv:ritsuko` (MODE: intel) | change scope, skills | `exploration.md` (Ikari writes it to its locator) | gatekeeper |
| 2. Spec + test plan | `nerv:ritsuko` (MODE: test-plan) | `exploration.md` locator, skills | `specs/{domain}/spec.md`, `nerv/test-plan.md` with `## Corner-case questions` | **user HARD** — Ikari relays the questions as one grouped blocking prompt; the answers are written into `## Answers` and gate the plan |
| 3. Plan | `nerv:misato` | `exploration.md`, `spec.md`, `test-plan.md` (with answers), skills | `proposal.md`, `design.md` (must contain `## New skills, scripts and commands`), `tasks.md` (ids `T1`, `T2`, ... with `pilot` and `depends_on`) | gatekeeper — verifies the `## New skills, scripts and commands` section exists in `design.md` |
| 4. Criticality | `nerv:hyuga` (dispatch a) | `tasks.md`, `critical_paths` | `nerv/criticality.md` | none |
| 5. MAGI vote round | `nerv:balthasar`, `nerv:melchor`, `nerv:casper` (MODE: vote), one parallel batch, blind | Balthasar: `design.md`+`tasks.md`; Melchor: `design.md`; Casper: `spec.md`+`tasks.md`+`proposal.md` | one JSON object each per the contract in `nerv-artifacts.md`, merged by Ikari into `nerv/votes.md` | critical task = unanimous approve; standard = 2-of-3; rejected tasks → step 6 |
| 6. Revise loop | `nerv:misato` (`misato-revise`) | rejected tasks + their findings | revised tasks only, re-voted at step 5 (revised tasks only; approved tasks stay `frozen`) | cap 2 re-votes per task; at cap, user: override-approve / kill task / Misato ruling |
| 7. Governance veto | `nerv:fuyutsuki` | `design.md`'s `## New skills, scripts and commands` | `nerv/veto-ruling.md` | a veto reopens only the owning task (frozen siblings stay frozen); cap 2 revision rounds; at cap, user: drop the item or abandon the task |
| 8. Plan approval | Ikari relays | tasks with criticality, vote results, veto rulings, test-plan summary | user decision logged | **user HARD** — no implementation before this gate |
| 9. Waves | `nerv:hyuga` (dispatch b) | frozen `tasks.md`, `votes.md` | `nerv/waves.md` (`tasks.md` is never mutated) | gatekeeper |
| 10. Baseline | `nerv:maya` (MODE: full, phase 0) | change scope | `nerv/maya-report.md` baseline | gatekeeper |
| 11. Implementation wave N | `nerv:kaworu` (RED) → `nerv:aoba` (commit, user validates) → assigned pilot (GREEN/TRIANGULATE/REFACTOR) → `nerv:aoba` (commit, user validates) → RDD hook | wave task, skills, TDD mode+runner, `commit_ref` | code + TDD evidence rows | repeat for every wave in `waves.md` in dependency order; a wave starts only when every wave it depends on is closed with a `wave_report`; the loop terminates when the last wave is closed; pilots in a wave may run in one parallel batch when their tasks are independent; deviations → `nerv:hyuga` deviation → `nerv:misato` ruling |
| 12. Maya full gate a→b→c→d | `nerv:maya` (MODE: full) | full change diff (base..HEAD), gathered once after the last wave closes — never per wave | `nerv/maya-report.md` phases a-d, each green before the next starts | `impl-wrong` → owning pilot; `spec-wrong` → Misato as a binding ruling (`MODE: ruling`, source `maya`); `ambiguous` → one Misato ruling → user only if a product decision is needed |
| 13. Freeze patch | `nerv:aoba` | base (branch point for round 1, previous round's HEAD for re-audits), round number, change locator | `nerv/audit/diff-round-N.patch`, `nerv/audit/round-N.yaml` (`{round, base, head, created_at}`) | gatekeeper |
| 14. Audit passes, 4 in parallel, blind | `nerv:melchor`, `nerv:balthasar`, `nerv:casper` (MODE: audit), `nerv:kaji-audit` | `diff-round-N.patch` + proposal/design/tasks/specs/test-plan locators, skills; `RDD scope` for melchor/balthasar (`cross-commit` only with native assess evidence, else `full`; see RDD narrowing) | one JSON pass object each per the contract in `nerv-artifacts.md`; Ikari writes each to `nerv/audit/pass-<name>-round-N.json` | gatekeeper — JSON validation, one retry |
| 15. Compile + refute | `nerv:kaji` (compile), `nerv:kaji-refuter` (one batch over that round's inferential BLOCKER/CRITICAL) | the five pass objects/files | `nerv/audit-report.md` with refuter outcomes merged (`refuted` → dropped to a `## Refuted` appendix, `inconclusive` → WARNING, kept) | none |
| 16. Ranking + issue gate | `nerv:hyuga` (dispatch c) | `audit-report.md` (post-refuter) | `nerv/issue-ranking.md` | **user HARD** — Ikari relays the ranked NOW/DEFER list as one blocking prompt: approve the NOW set / edit it / accept residual and close |
| 17. Fix routing + re-audit loop | owning pilot per approved issue (LIGHT work-unit cycle: Kaworu RED when behavioral, Aoba commit, pilot fix, Aoba commit, RDD hook), `nerv:aoba` (fix-delta patch), audit passes, `nerv:kaji` | fixes committed; `nerv/audit/diff-round-N+1.patch` scoped to the fix delta only; updated `audit-report.md` carrying forward unresolved items | cap 2 re-audits (loop back to step 14 over the fix-delta patch); at the cap the user accepts the residual (`residual_accepted` in `issue-ranking.md`) or declines the remainder; deviations → Misato ruling |
| 18. Docs + archive + curate | `nerv:ritsuko` (MODE: docs), `nerv:aoba` (Archive duty), `nerv:fuyutsuki` (MODE: curate) | `issue-ranking.md`, fix commits, `tasks.md`, docs deltas | `nerv/issue-resolutions.md`, `nerv/agent-config.md`, repo doc deltas (Ikari writes them at Ritsuko-named locators), change archived to `openspec/changes/archive/YYYY-MM-DD-{change}/` via `nerv spec-compose` + `git mv`, `## Summary` appended to `nerv/deliberation-log.md` | gatekeeper |
| 19. Run summary + close | `nerv:aoba`; `nerv:hyuga` `DISPATCH: tracker` (`close`, or `done` when the user prefers the task stay open) | usage table from Ikari | `nerv/run-summary.md`, change closed, tracker updated (see `### Tracker dispatch (Phase 4)` below) | none — the user already validated at gates 16 and 18 |

### Tracker dispatch (Phase 4)

`nerv:hyuga` `DISPATCH: tracker` runs at four points in FULL (Preflight
and Close only in LIGHT):

- **Preflight (step 0).** `createTask` if requested, then `start`
  (assign, `inDev` stage, local timer). Skipped when the provider is
  `none` or the user chose to work without a task.
- **Maya's full gate start (before step 12).** `moveStage(testing)`.
- **Issue gate (around step 16).** `comment` with the ranked audit
  summary; `createTask` for every accepted `DEFER` issue, so deferred
  findings become tracked follow-up work.
- **Close (step 19).** `close` (`stop` with real start/end +
  `moveStage(implemented)` + `complete`), or `done` (same without
  `complete`) when the user prefers the task stay open. Ikari also sets
  `status: done` on every completed task in `tasks.md`, records `closed_at`
  in `state.yaml`, and deletes `nerv/.orchestrator.lock`.

A halted run routes to `block(reason)` instead — Ikari asks the user for
the mandatory cause first, then Hyuga runs `block` with it. Every tracker
op is logged by Ikari as one `tracker_event` entry in
`nerv/deliberation-log.md` (`{op, taskRef, result}`); see the port
contract in `plugin/agents/hyuga.md`.

### Plan gatekeeper

Step 3's gatekeeper check is mechanical and specific: before criticality
runs, Ikari re-reads `design.md` and confirms the `## New skills, scripts
and commands` heading exists, verbatim, with content under it — either a
list of items or the single word `none`. A `design.md` missing the
heading fails the same retry-once-then-stop rule as any other gatekeeper
check (see `## Gatekeeper` below); Misato does not proceed to criticality
without it, because Fuyutsuki's veto step has nothing to rule on
otherwise.

### Pilot selection differs from LIGHT

LIGHT auto-selects a pilot from the touched-file domain (Ritsuko's
suggestion, user-overridable at commit validation). FULL never
auto-selects: Misato assigns `pilot: rei|shinji|asuka|toji` explicitly per
task in `tasks.md`, informed by the same domain map but as a plan
decision MAGI can vote on and Hyuga can re-confirm in `waves.md`'s
`pilot_assignments`. A disagreement between `tasks.md`'s `pilot` field and
`waves.md`'s `pilot_assignments` for the same task is a gatekeeper failure
at step 9 — `waves.md` must match `tasks.md` exactly, it never overrides
it.

### Audit stage mechanics

**Freeze rule.** Aoba freezes the patch the audit passes read, never
hand-edited: `git diff <base>..HEAD` written to
`nerv/audit/diff-round-N.patch`, plus `nerv/audit/round-N.yaml`
(`{round, base, head, created_at}`) recording the exact base and HEAD
hashes. `base` is the change's branch point for round 1 and the previous
round's HEAD for every re-audit (step 17) — a re-audit patch scopes only
the fix delta, never the cumulative diff.

**Pass batch and JSON gatekeeping.** Ikari launches all four audit passes
— `nerv:melchor` (structure and security), `nerv:balthasar` (software
principles and readability), `nerv:casper` (process) (MODE: audit), and
`nerv:kaji-audit` (coverage, reliability, correctness, resilience and
performance) — in one parallel batch, blind
to each other, each reading only the frozen patch plus the plan artifacts
(`proposal.md`, `design.md`, `tasks.md`, `specs/`, `nerv/test-plan.md`).
Each pass returns exactly one JSON object as its final text — the same
gatekeeping MAGI JSON gets: parseable, `pass` and `round` present, every
BLOCKER/CRITICAL finding carrying `location`, `severity`, `claim`,
`evidence_class`, `causal_disposition`, and `proof_refs`. A malformed
object is retried once with the parse failure quoted; a second failure
stops the audit round and reports. Ikari writes each validated object to
`nerv/audit/pass-<name>-round-N.json`.

**Kaji dedupe and compile.** `nerv:kaji` reads the five pass objects (from
the launch prompt or the written files) and writes `nerv/audit-report.md`:
same file:line (or overlapping range) plus the same defect signature
merges into one item, `credited_sources[]` listing every pass that found
it; severity is the max across sources; candidate-causal admission applies
(BLOCKER/CRITICAL need proof the candidate introduced, activated, or
worsened the behavior — unproven causality is `unknown` and ranks as
WARNING at most; `pre-existing` findings are follow-ups and never block).
Deterministic BLOCKER/CRITICAL need no refuter; every inferential
BLOCKER/CRITICAL becomes the refuter batch. Kaji contacts nobody —
clarifications route through Ikari.

**Refuter batch.** `nerv:kaji-refuter` runs once per audit round over that
round's inferential BLOCKER/CRITICAL items, reading the frozen patch and
repo history read-only, returning `{"round": N, "results": [{finding_id,
outcome: corroborated|refuted|inconclusive, proof_refs}]}`; it never adds
findings. Ikari merges outcomes into `audit-report.md`: `refuted` items
move to a `## Refuted` appendix and drop from the active list;
`inconclusive` items are kept and ranked as WARNING.

**Ranking and the issue gate.** `nerv:hyuga` (dispatch c) ranks the
post-refuter report into `nerv/issue-ranking.md`: per item `severity`
(Critical/Important/Minor), `blast_radius`, `verification_cost`, a binding
`decision` (NOW/DEFER) with a one-line reason, a binding `fix_order`, and
an `owner` pilot — ties broken cheapest-verification-first. The NOW set is
every candidate-caused BLOCKER/CRITICAL plus whatever Hyuga argues in.
Ikari relays the ranked list as one **user HARD** blocking prompt, lossless
per the Lossless Blocking Prompts contract: every NOW item with severity,
owner, and reason; the DEFER list; and the options allowed at that point:
before the re-audit cap, exactly two — approve the NOW set as is, or edit
the set (free text naming ids to add or drop); a third option, accept the
residual and close, is offered only when the NOW set is empty or the
re-audit cap (2) has been reached. A non-empty NOW set never closes
without a fix round.

**Fix routing.** For each approved issue, in `fix_order`, the owning pilot
fixes it through the LIGHT work-unit cycle: `nerv:kaworu` writes a RED
regression test when the issue is behavioral, `nerv:aoba` commits it (user
validates), the pilot fixes it, `nerv:aoba` commits the fix (user
validates), then the RDD hook runs as usual. Any deviation from the
approved fix routes to `nerv:misato` for a binding ruling, same mechanics
as a wave deviation.

**Re-audit loop.** Once every approved fix lands, Aoba freezes
`diff-round-N+1.patch` scoped to the fix delta only (base = the previous
round's HEAD), the same five passes run over it, Kaji compiles round N+1
carrying forward unresolved items, the refuter batch runs again, ranking
runs again, and the issue gate is relayed again. Capped at 2 re-audits; at
the cap the user either accepts the residual (recorded in
`issue-ranking.md` as `residual_accepted`) or declines the remainder.

**RDD narrowing.** Narrowing is evidence-based, never assumed: native
review runs only when a review is due and the human granted consent, so
the RDD switch alone proves nothing about a commit. Before each round's
passes, Ikari runs `gentle-ai review assess --cwd <repo> --base-ref
<round base> --committed-only --json` over the round's frozen range
(base..head from `nerv/audit/round-N.yaml`; a re-audit round uses its own
fix-delta range). Ikari launches `melchor` and `balthasar`
with `RDD scope: cross-commit` (cross-commit and integration concerns
only; melchor's security lens and balthasar's readability lens inherit
this evidence gate, except balthasar's `review-context` category, which
judges the round as a whole) ONLY when the RDD switch is on AND the
assessment returns `review_due: false` with `review_due_reason`
`already_reviewed` (terminal native authority covers that exact range)
or `passive` (nothing reviewable). Every other outcome — RDD off or
unknown, `review_due: true`, `under_budget`, any other reason, a failed
assessment, or unparsable output — launches `RDD scope: full`. Fail
closed: missing evidence never narrows. Ikari records the chosen scope
and the assess reason in the round's phase note and in each pass
launch, so the decision is auditable. `casper` and
`kaji-audit` always keep full NERV scope (plan conformance, commit
hygiene and TDD commit order for Casper; test-plan coverage, reliability,
correctness, resilience and performance for kaji-audit), regardless of
the RDD switch. The native RDD reliability and resilience reviews run
only when a review is due for a commit, so narrowing kaji-audit would
leave those lenses unreviewed for every commit the native review
skipped.

### Ratchet handling

The diff already produced while a change was still LIGHT becomes the
wave-1 candidate once Ikari reclassifies to FULL. Misato's `tasks.md` MUST
include that diff as its own task, carrying `status: implemented-pre-plan`
in addition to its `id`/`pilot`/`depends_on` fields, and MAGI votes on it
exactly like any other task — there is no free pass for pre-plan work.

### MAGI vote mechanics

`nerv:balthasar`, `nerv:melchor`, `nerv:casper` (MODE: vote) launch
together in exactly one parallel batch, never sequentially and never with
visibility into each other's output — a blind vote loses its meaning the
moment one member sees another's findings first. Each receives only the
locators its own lens needs (Balthasar: `design.md` + `tasks.md`;
Melchor: `design.md`; Casper: `spec.md` + `tasks.md` + `proposal.md`) and
returns exactly the JSON contract from `nerv-artifacts.md` as its final
text — one object per launch, never a tool call as the last action.

Ikari merges the three objects into `nerv/votes.md` and computes `result`
per task from `nerv/criticality.md`: a task marked `critical` needs all
three members to `approve`; a `standard` task needs 2 of 3. Any member's
`escalation` to `critical` on a task applies for the rest of that round
even if the standard rule would otherwise have passed it — escalation
always tightens the requirement, never loosens it, and it never moves a
task back down to `standard`. Tasks that pass their rule are marked
`frozen: true`; Misato may not edit a frozen task again in this change,
including during a later revise round for a sibling task.

### Revise loop

Rejected tasks return to Misato with that round's findings attached
(`next_recommended: misato-revise`). Misato revises only the rejected
tasks — every frozen task is untouched — and the revised subset alone is
re-voted at step 5, same blind parallel-batch mechanics, `round`
incremented in `votes.md`. This repeats up to 2 re-votes per task; at the
cap Ikari stops and asks the user to choose exactly one of: override-
approve the task despite the standing rejection, kill the task from the
plan, or send it to Misato for a binding ruling instead of a third vote.

### Governance veto

Fuyutsuki reads only `design.md`'s `## New skills, scripts and commands`
section and rules once per declared item — never on anything outside
that section. When the section is the single word `none`, Fuyutsuki
still records that in `veto-ruling.md` as a single `none` line; no
per-item ruling is needed. A `veto` verdict reopens only the task that
owns the vetoed item — every other frozen task, including tasks in the
same wave, stays frozen. Misato revises the owning task alone and
Fuyutsuki re-rules on the revised declaration, up to 2 revision rounds;
at the cap the user decides: drop the vetoed item from the plan, or
abandon the task that needs it.

### Plan approval

Before any implementation, Ikari presents one consolidated view: every
task with its criticality, its final vote result and rule, any veto
ruling touching it, and the test-plan summary (cases plus the recorded
corner-case answers). This is a single **user HARD** gate — nothing from
step 9 onward runs before the user's explicit approval, and a partial
approval (approve some tasks, reject others) is not a supported shape:
the gate is whole-plan or nothing.

### Implementation wave execution

Hyuga's `waves.md` groups frozen tasks by dependency, never by
convenience — two tasks share a wave only when neither's `depends_on`
names the other, directly or transitively. Within a wave, independent
tasks' per-task cycles (Kaworu RED → Aoba commit → assigned pilot's
GREEN/TRIANGULATE/REFACTOR → Aoba commit) may run as one parallel batch;
a task with an unmet dependency waits for its dependency wave to close
first. If a pilot or Hyuga discovers mid-wave that a task's scope,
dependency, or execution does not match what `waves.md` assumed, it
signals a `deviation` (`scope|dependency|blocked`) instead of guessing —
routed to Misato for a binding ruling (`MODE: ruling`). A binding Misato
ruling is the only legal way to reopen a frozen task: it may mark the
affected task `unfrozen_by_ruling: <ruling_id>`, which returns that task
alone to Misato for a scoped revision (`MODE: revise`) limited to what the
ruling names; the revised task is re-voted alone at the MAGI vote step
(the re-vote counts toward that task's cap of 2) and is re-frozen once
approved. Every other frozen task, in this wave or any other, stays
frozen and untouched. The wave containing the unfrozen task pauses until
it is re-frozen, and Hyuga re-emits `waves.md` if the ruling changed the
task's dependencies. The unfreeze is logged as a `ruling_issued` event
(carrying `unfreezes: [task_id]`) plus the resulting `vote_result` event.
The RDD per-commit relay (see
`## RDD relay` above) and the delivery-budget tracking (see `## Delivery`
above) apply identically inside FULL waves as they do in LIGHT — there is
no separate FULL-only commit or budget mechanism.

