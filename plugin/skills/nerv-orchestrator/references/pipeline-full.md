## FULL pipeline (Phase 3)

FULL adds MAGI vote, governance veto, waves, quality-gated implementation,
and a full audit-and-closure stage on top of the shared primitives
(RED/GREEN/REFACTOR, user-validated commits, the RDD hook, usage
collection, the deliberation log). FULL keeps its own work-unit cycle:
`nerv:kaworu` RED, `nerv:aoba` commit, pilot GREEN, `nerv:aoba` commit.
Once every wave is closed and Maya's full gate is green, the run proceeds
into the audit stage (steps 13-19 below): a frozen patch, four blind audit
passes, Kaji's compilation and the refuter batch, Hyuga's ranking behind a
user HARD issue gate, fix routing with a bounded re-audit loop, then
documentation, the run summary and the close on the ODD document
(`references/odd-document.md`). FULL is the only tier that creates the
feature document `odd/tasks/{feature}.md`, its Engram copy
`odd/{feature}/tasks`, the run folder `odd/{feature}/nerv/` and the close
commit; Ikari creates the first three when Classification resolves FULL
(or a run ratchets to FULL), before step 1. Never silently skip a step in
the table below or downgrade FULL to LIGHT or DIRECT mid-run.

| Step | Actor | Launch prompt carries | Expected envelope | Gate |
|---|---|---|---|---|
| 0. Preflight | Ikari, `task:dispatcher` (only when the task plugin is installed), Aoba | — | `run.started` envelopes; worktree/branch state | user HARD if asked |
| 1. Intel | `nerv:ritsuko` (MODE: intel) | feature scope, the ODD document, skills | `nerv/exploration.md` (Ikari writes it to its locator, plus a `## Log` pointer) | gatekeeper |
| 2. Test plan | `nerv:ritsuko` (MODE: test-plan) | `nerv/exploration.md`, the seeded `## Specs`, skills | the test-plan `## Log` entry (cases per `S#`, `### Corner-case questions`), appended verbatim by Ikari | **user HARD** — Ikari relays the questions as one grouped blocking prompt; the answers are written into `### Answers` and gate the plan |
| 3. Plan | `nerv:misato` | the ODD document (with the answers), `nerv/exploration.md`, skills | `## Specs` completed, the design `## Log` entry (must contain `### New skills, scripts and commands`), `## Tasks` (ids `T1`, `T2`, ... with `Pilot` and `Depends on`) | gatekeeper — verifies the `### New skills, scripts and commands` heading exists in the design entry |
| 4. Criticality | `nerv:hyuga` (dispatch a) | `## Tasks`, `critical_paths` | `nerv/criticality.md`; Ikari copies it into the `Criticality` column | none |
| 5. MAGI vote round | `nerv:balthasar`, `nerv:melchior`, `nerv:casper` (MODE: vote), one parallel batch, blind | each member's reading list (MAGI vote mechanics below) and the tasks in scope | one JSON object each per the contract in `nerv-artifacts.md`, merged by Ikari into `nerv/votes.md` and the `MAGI` column | critical task = unanimous approve; standard = 2-of-3; rejected tasks → step 6 |
| 6. Revise loop | `nerv:misato` (`misato-revise`) | rejected tasks + their findings | revised tasks only, re-voted at step 5 (revised tasks only; approved tasks stay `frozen`) | cap 2 re-votes per task; at cap, user: override-approve / kill task / Misato ruling |
| 7. Governance veto | `nerv:fuyutsuki` | the design entry's `### New skills, scripts and commands` | `nerv/veto-ruling.md` | a veto reopens only the owning task (frozen siblings stay frozen); cap 2 revision rounds; at cap, user: drop the item or abandon the task |
| 8. Plan approval | Ikari relays | tasks with criticality, vote results, veto rulings, test-plan summary | user decision logged, and recorded as a `## Log` entry | **user HARD** — no implementation before this gate |
| 9. Waves | `nerv:hyuga` (dispatch b) | frozen `## Tasks`, `votes.md` | `nerv/waves.md` (the ODD document is never edited by Hyuga) | gatekeeper |
| 10. Baseline | `nerv:maya` (MODE: full, phase 0) | feature scope | `nerv/maya-report.md` baseline | gatekeeper |
| 11. Implementation wave N | `nerv:kaworu` (RED) → `nerv:aoba` (commit, user validates) → assigned pilot (GREEN/TRIANGULATE/REFACTOR) → `nerv:aoba` (commit, user validates) → RDD hook | wave task, skills, TDD mode+runner, `commit_ref` | code + TDD evidence rows; Aoba records each commit in the task's `Commit` column, Ikari the task's `Status` | repeat for every wave in `waves.md` in dependency order; a wave starts only when every wave it depends on is closed with a `wave_report`; the loop terminates when the last wave is closed; pilots in a wave may run in one parallel batch when their tasks are independent; deviations → `nerv:hyuga` deviation → `nerv:misato` ruling |
| 12. Maya full gate a→b→c→d | `nerv:maya` (MODE: full) | full feature diff (base..HEAD), gathered once after the last wave closes — never per wave | `nerv/maya-report.md` phases a-d, each green before the next starts | `impl-wrong` → owning pilot; `spec-wrong` → Misato as a binding ruling (`MODE: ruling`, source `maya`); `ambiguous` → one Misato ruling → user only if a product decision is needed |
| 13. Freeze patch | `nerv:aoba` | base (branch point for round 1, previous round's HEAD for re-audits), round number, run folder locator | `nerv/audit/diff-round-N.patch`, `nerv/audit/round-N.yaml` (`{round, base, head, created_at}`), `nerv/audit/commits-round-N.txt` | gatekeeper |
| 14. Audit passes, 4 in parallel, blind | `nerv:melchior`, `nerv:balthasar`, `nerv:casper` (MODE: audit), `nerv:gendo` | `diff-round-N.patch`, `nerv/audit/commits-round-N.txt` + the ODD document locator, skills; `RDD scope` for melchior/balthasar (`cross-commit` only with native assess evidence, else `full`; see RDD narrowing) | one JSON pass object each per the contract in `nerv-artifacts.md`; Ikari writes each to `nerv/audit/pass-<name>-round-N.json` | gatekeeper — JSON validation, one retry |
| 15. Compile + refute | `nerv:kaji` (compile), `nerv:fuyutsuki` (MODE: refute, read-only; one batch over that round's inferential BLOCKER/CRITICAL) | the four pass objects/files | `nerv/audit-report.md` with refuter outcomes merged (`refuted` → dropped to a `## Refuted` appendix, `inconclusive` → WARNING, kept) | none |
| 16. Ranking + issue gate | `nerv:hyuga` (dispatch c) | `audit-report.md` (post-refuter) | `nerv/issue-ranking.md` | **user HARD** — Ikari relays the ranked NOW/DEFER list as one blocking prompt: approve the NOW set / edit it / accept residual and close |
| 17. Fix routing + re-audit loop | owning pilot per approved issue (FULL work-unit cycle: Kaworu RED when behavioral, Aoba commit, pilot fix, Aoba commit, RDD hook), `nerv:aoba` (fix-delta patch), audit passes, `nerv:kaji` | fixes committed; `nerv/audit/diff-round-N+1.patch` scoped to the fix delta only; updated `audit-report.md` carrying forward unresolved items | cap 2 re-audits (loop back to step 14 over the fix-delta patch); at the cap the user accepts the residual (`residual_accepted` in `issue-ranking.md`) or declines the remainder; deviations → Misato ruling |
| 18. Docs + curate | `nerv:ritsuko` (MODE: docs), `nerv:fuyutsuki` (MODE: curate) | `issue-ranking.md`, fix commits, `## Tasks`, docs updates | `nerv/issue-resolutions.md`, `nerv/agent-config.md`, repo doc updates (Ikari writes them at Ritsuko-named locators), `## Summary` appended to `nerv/deliberation-log.md` | gatekeeper |
| 19. Run summary + close | `nerv:aoba`; `task:dispatcher` `run.closed` (only when the task plugin is installed) | usage table, task envelopes and any KB mirrors still unflushed after Ikari's Engram flush at close (`usage-and-log.md`) | `nerv/run-summary.md`, the close `## Log` entry, then Aoba's close commit (`docs: close nerv run for {feature}`, committing `odd/tasks/{feature}.md` and `odd/{feature}/nerv/` per `artifacts.commit`); no archive (see `### Task events` below) | **user validates** the close commit |

### Task events

When the task plugin is installed, Ikari emits the core's `## Task events`
through `task:dispatcher` at these points in FULL (in DIRECT and LIGHT:
Preflight, the work unit's verification start, and Close). Without
`task:dispatcher`, none of them runs.

- **Preflight (step 0).** `run.started`, after classification. A
  `needs_input` envelope is relayed verbatim and dispatched again with the
  answer in `data`.
- **Implementation wave start (step 11).** `wave.started` once per wave,
  with the wave in `data`.
- **Maya's full gate start (before step 12).** `verification.started`.
- **Issue gate (step 16).** `audit.ranked` with the ranked audit summary in
  `data`, then `issue.deferred` once per accepted `DEFER` issue, so
  deferred findings become follow-up work.
- **Close (step 19).** `run.closed`, before Aoba writes the run summary,
  so its envelope appears in it. It never completes the task: completing
  it stays an explicit `/task:close`. Ikari also sets `Status` `done` on
  every completed `## Tasks` row (each already carries its commits, from
  Aoba), appends the close `## Log` entry, records `closed_at` in
  `state.yaml`, and deletes `.nerv/locks/{run-id}.lock`.

A halted run emits `run.halted` instead — Ikari asks the user for the
mandatory cause first and passes it in `data`. A `blocked` or `failed`
envelope never halts or retries the run. Ikari appends every envelope to
`nerv/task-events.yaml` and logs one `task_event` entry
(`{event, provider, result, task_ref}`) in `nerv/deliberation-log.md`.

### Plan gatekeeper

Step 3's gatekeeper check is mechanical and specific: before criticality
runs, Ikari re-reads the ODD document and confirms the design `## Log`
entry carries the `### New skills, scripts and commands` heading,
verbatim, with content under it — either a list of items or the single
word `none` — and that every `## Tasks` row links at least one `S#`. A
design entry missing the heading fails the same retry-once-then-stop rule as any other gatekeeper
check (see `## Gatekeeper` below); Misato does not proceed to criticality
without it, because Fuyutsuki's veto step has nothing to rule on
otherwise.

### Pilot selection differs from LIGHT

LIGHT auto-selects a pilot from the touched-file domain (Ritsuko's
suggestion, user-overridable at commit validation). FULL never
auto-selects: Misato assigns the `Pilot` (`rei|shinji|asuka|toji`)
explicitly per task in `## Tasks`, informed by the same domain map but as a plan
decision MAGI can vote on and Hyuga can re-confirm in `waves.md`'s
`pilot_assignments`. A disagreement between a `## Tasks` row's `Pilot`
and `waves.md`'s `pilot_assignments` for the same task is a gatekeeper
failure at step 9 — `waves.md` must match `## Tasks` exactly, it never
overrides it.

### Audit stage mechanics

**Freeze rule.** Aoba freezes the patch the audit passes read, never
hand-edited: `git diff <base>..HEAD` written to
`nerv/audit/diff-round-N.patch`, plus `nerv/audit/round-N.yaml`
(`{round, base, head, created_at}`) recording the exact base and HEAD
hashes, and `nerv/audit/commits-round-N.txt` (`git log --format='%h %s'
--stat <base>..HEAD`), which every audit launch receives. `base` is the feature's branch point for round 1 and the previous
round's HEAD for every re-audit (step 17) — a re-audit patch scopes only
the fix delta, never the cumulative diff.

**Pass batch and JSON gatekeeping.** Ikari launches all four audit passes
— the three MAGI in MODE: audit, `nerv:melchior` (structure and security),
`nerv:balthasar` (software principles and readability) and `nerv:casper`
(process), plus `nerv:gendo` (coverage, reliability, correctness,
resilience and performance) — in one parallel batch, blind
to each other, each reading only the frozen patch,
`nerv/audit/round-N.yaml`, `nerv/audit/commits-round-N.txt` and the ODD
document's `## Specs`, `## Tasks` (without its `MAGI` column), and the
design and test-plan `## Log` entries — never the `MAGI` column,
`nerv/votes.md`, an earlier round's audit report or pass files, or the
gate and ruling `## Log` entries (plan gate, issue gate, plan approval,
Misato rulings).
Each pass returns exactly one JSON object as its final text — the same
gatekeeping MAGI JSON gets: parseable, `pass` and `round` present, every
BLOCKER/CRITICAL finding carrying `location`, `severity`, `claim`,
`evidence_class`, `causal_disposition`, and `proof_refs`. A malformed
object is retried once with the parse failure quoted; a second failure
stops the audit round and reports. Ikari writes each validated object to
`nerv/audit/pass-<name>-round-N.json`.

**Kaji dedupe and compile.** `nerv:kaji` reads the four pass objects (from
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

**Refuter batch.** `nerv:fuyutsuki` (MODE: refute) runs once per audit
round over that round's inferential BLOCKER/CRITICAL items, reading the
frozen patch and repo history read-only — in this mode he writes
nothing and calls no `mem_save`, although his other modes may — returning `{"round": N, "results": [{finding_id,
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
fixes it through FULL's own work-unit cycle: `nerv:kaworu` writes a RED
regression test when the issue is behavioral, `nerv:aoba` commits it (user
validates), the pilot fixes it, `nerv:aoba` commits the fix (user
validates), then the RDD hook runs as usual. Any deviation from the
approved fix routes to `nerv:misato` for a binding ruling, same mechanics
as a wave deviation.

**Re-audit loop.** Once every approved fix lands, Aoba freezes
`diff-round-N+1.patch` scoped to the fix delta only (base = the previous
round's HEAD), the same four passes run over it, Kaji compiles round N+1
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
fix-delta range). Ikari launches `melchior` and `balthasar`
with `RDD scope: cross-commit` (cross-commit and integration concerns
only; melchior's security lens and balthasar's readability lens inherit
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
`gendo` always keep full NERV scope (plan conformance, commit
hygiene and TDD commit order for Casper; test-plan coverage, reliability,
correctness, resilience and performance for gendo), regardless of
the RDD switch. Native evidence cannot replace gendo: an
`already_reviewed` range only proves that some native review covered it,
and the native plan picks its lenses by risk (a medium candidate gets a
single consolidated lens), so the native reliability and resilience
lenses may never have run on it; `gendo.md` carries the full
rationale.

### Ratchet handling

The diff already produced while a run was still DIRECT or LIGHT becomes the
wave-1 candidate once Ikari reclassifies to FULL. Misato's `## Tasks` MUST
include that diff as its own task row, with `Status`
`implemented-pre-plan` in addition to its `ID`/`Pilot`/`Depends on`, and MAGI votes on it
exactly like any other task — there is no free pass for pre-plan work.
Its `Commit` holds the work commit the DIRECT or LIGHT run already made
(Aoba copies it in), and the run files that run kept in
`.nerv/runs/{run-id}/` move into `odd/{feature}/nerv/` when the document
is created (`odd-document.md`, Ratchet to FULL).

### MAGI vote mechanics

`nerv:balthasar`, `nerv:melchior`, `nerv:casper` (MODE: vote) launch
together in exactly one parallel batch, never sequentially and never with
visibility into each other's output — a blind vote loses its meaning the
moment one member sees another's findings first. Each reads exactly the
sections its own lens needs, and the launch brief names them:

- Balthasar: the design `## Log` entry, `## Tasks` and `nerv/criticality.md`;
- Melchior: the design `## Log` entry, `## Tasks`, `## Specs` and `nerv/criticality.md`;
- Casper: `## Specs`, the test-plan `## Log` entry, `## Tasks` and `nerv/criticality.md`.

`## Tasks` is read without its `MAGI` column. Every round is blind to the
earlier ones too: a member reads only its list, never the `MAGI` column,
`nerv/votes.md`, an earlier round's audit report or pass files, or the
gate and ruling `## Log` entries (plan gate, issue gate, plan approval,
Misato rulings) before casting its vote. A revised task's own text
carries what the revision changed. Each member returns exactly the JSON
contract from `nerv-artifacts.md` as its final text — one object per
launch, never a tool call as the last action.

Ikari merges the three objects into `nerv/votes.md` and computes `result`
per task from `nerv/criticality.md`: a task marked `critical` needs all
three members to `approve`; a `standard` task needs 2 of 3. Any member's
`escalation` to `critical` on a task applies for the rest of that round
even if the standard rule would otherwise have passed it — escalation
always tightens the requirement, never loosens it, and it never moves a
task back down to `standard`. Tasks that pass their rule are marked
`frozen: true` (Ikari copies result, rule, round and frozen into the task's
`MAGI` column); Misato may not edit a frozen task row again in this feature,
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

Fuyutsuki reads only the design entry's `### New skills, scripts and
commands` heading and rules once per declared item — never on anything outside
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
above) apply identically inside FULL waves as they do in DIRECT and LIGHT — there is
no separate FULL-only commit or budget mechanism.

Aoba writes the `Commit` column: each Aoba launch that commits records the
new hash in its task's `## Tasks` row before returning (RED and GREEN
hashes of one task, comma-separated; a fix commit from step 17 appended to
its task's row). Ikari sets `Status`, and nobody but Aoba edits `Commit`.

