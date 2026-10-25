## DIRECT pipeline (direct inline)

gentle-ai's direct-inline route: Ikari does the work itself. No NERV agent
is launched except `nerv:maya`, and `nerv:maya` only when the verification
row below asks for an independent verifier. When the task extension is installed, Ikari also emits
the task events of the core's `## Task events` through `task:dispatcher`.

DIRECT creates no ODD feature document, no Engram copy of one and no
commit at close: like gentle-ai's ODD for small work, it works from the
request and the diff. Its run files (`state.yaml`, `deliberation-log.md`,
`task-events.yaml`, `run-summary.md`, `maya-report.md` when Maya ran) stay
in the runtime folder `.nerv/runs/{run-id}/`, never committed, and the
lock in `.nerv/locks/{run-id}.lock`. The run asks the user to approve
only its work commit.

| Step | Actor | Does | Gate |
|---|---|---|---|
| 1. Preflight | Ikari; `task:dispatcher` only when the task extension is installed | answers collected; `run.started` and the worktree/branch run after step 2 | user HARD if asked |
| 2. Classify | Ikari | `classification` event: DIRECT plus the route declaration (calls and evidence used, the one file, why it is mechanical, which triggers did not fire); worktree/branch created by Ikari when requested | none |
| 3. RED (when applicable) | Ikari | strict TDD and a behavior change with a relevant runnable deterministic test: write or adjust the one pinning assertion (the allowed mechanical second-file edit), run it, observe it fail for the expected reason | none |
| 4. Edit | Ikari | the one mechanical file, nothing else | none |
| 5. Focused check | Ikari; `task:dispatcher` `verification.started` when the task extension is installed | run the focused test/lint/build for the touched file (GREEN when step 3 ran) and log `<command>: <observed result>` as an `envelope` entry with actor `ikari` (step 3's RED likewise); passive docs get a structural readback; see Verification below | a failure is fixed inline once within the budget, otherwise ratchet |
| 6. Work-unit commit | Ikari | shows the diff and the conventional commit message with `commit_ref`, then commits | **user validates before commit** |
| 7. RDD hook | native engine (via Ikari) | `references/rdd-relay.md` | per RDD relay |
| 8. Run summary | Ikari; `task:dispatcher` when the task extension is installed | `run.closed` emitted first, so its envelope is in the summary; then `.nerv/runs/{run-id}/run-summary.md` per its schema in `nerv-artifacts.md` (usage rows only for agents actually launched) | none |
| 9. Close | Ikari | `closed_at` in `.nerv/runs/{run-id}/state.yaml`, the `stop` event, lock deleted; nothing is committed, so the step-6 commit stays the run's only commit; a run that halts instead emits `run.halted` with the cause | none |

**Test-first exception.** When there is no relevant runnable deterministic
test (passive docs, a comment, no runner), step 3 is skipped and the
reason is logged in the route declaration; never invent RED evidence.

**Verification.** DIRECT is inline, so Ikari's own step-5 report is the
verification of record. RDD on: the native review at step 7 is the
independent check. RDD off or unknown: Ikari runs `gentle-ai review assess
--cwd <repo> --json` over the uncommitted diff — `passive` and `medium`
keep the focused check; `high`, a failed assessment or an unrecognized
verb launches `nerv:maya` (`MODE: reduced`) as the independent verifier
before step 6. A failed assessment never lowers the tier.

**Ratchet to LIGHT.** The moment DIRECT needs more than the inline evidence
budget, research, a design decision or a second non-trivial file — or the
focused check still fails after one inline fix — Ikari stops editing, logs
a `ratchet` event, loads `pipeline-light.md` and continues there; the
uncommitted inline diff is the pilot's starting state and the run folder
stays `.nerv/runs/{run-id}/`. A FULL criterion ratchets straight to FULL,
which creates the feature document from the run (`odd-document.md`,
Ratchet to FULL).

The orchestrator lock, the deliberation log, the user-validated work
commit, the attribution readback, the RDD relay and the delivery budget
apply unchanged.
