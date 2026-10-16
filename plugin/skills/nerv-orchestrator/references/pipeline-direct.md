## DIRECT pipeline (direct inline)

gentle-ai's direct-inline route: Ikari does the work itself. No NERV agent
is launched except `nerv:hyuga` `DISPATCH: tracker` when the user chose a
tracker task, and `nerv:maya` only when the verification row below asks for
an independent verifier.

| Step | Actor | Does | Gate |
|---|---|---|---|
| 1. Preflight | Ikari; Hyuga (d) only with a tracker task | answers collected; tracker `start` and the worktree/branch run after step 2 | user HARD if asked |
| 2. Classify | Ikari | `classification` event: DIRECT plus the route declaration (calls and evidence used, the one file, why it is mechanical, which triggers did not fire); worktree/branch created by Ikari when requested | none |
| 3. RED (when applicable) | Ikari | strict TDD and a behavior change with a relevant runnable deterministic test: write or adjust the one pinning assertion (the allowed mechanical second-file edit), run it, observe it fail for the expected reason | none |
| 4. Edit | Ikari | the one mechanical file, nothing else | none |
| 5. Focused check | Ikari; Hyuga (d) `moveStage(testing)` with a tracker task | run the focused test/lint/build for the touched file (GREEN when step 3 ran) and log `<command>: <observed result>` as an `envelope` entry with actor `ikari` (step 3's RED likewise); passive docs get a structural readback; see Verification below | a failure is fixed inline once within the budget, otherwise ratchet |
| 6. Work-unit commit | Ikari | shows the diff and the conventional commit message with `commit_ref`, then commits | **user validates before commit** |
| 7. RDD hook | native engine (via Ikari) | `references/rdd-relay.md` | per RDD relay |
| 8. Run summary | Ikari | `nerv/run-summary.md` per its schema in `nerv-artifacts.md` (usage rows only for agents actually launched) | none |
| 9. Close | Ikari, Hyuga (d) with a tracker task | `closed_at` in `state.yaml`, lock deleted, tracker `close`/`done` | none |

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
uncommitted inline diff is the pilot's starting state. A FULL criterion
ratchets straight to FULL.

The orchestrator lock, the deliberation log, the user-validated commit,
the attribution readback, the RDD relay and the delivery budget apply
unchanged.
