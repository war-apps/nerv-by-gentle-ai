## LIGHT pipeline (delegated direct)

gentle-ai's delegated-direct route with the fewest agents: an explorer only
when the mapping trigger fires, one bounded writer (the domain pilot in
`MODE: solo`, doing its own test-first cycle), Maya only when the
verification gate asks for an independent verifier, and `task:dispatcher`
only when the task plugin is installed. Ikari runs the git state, the commit and the run summary
itself; Aoba is launched only for PR preparation, slicing or archiving.

| Step | Actor | Launch prompt carries | Expected envelope | Gate |
|---|---|---|---|---|
| 1. Preflight | Ikari; `task:dispatcher` only when the task plugin is installed | — | answers collected; `run.started` and the worktree/branch run after step 2 | user HARD if asked |
| 2. Classify | Ikari | — | `classification` event: LIGHT plus the route declaration (which triggers fired, the domain signal); worktree/branch created by Ikari when requested | none |
| 3. Explorer (only when the mapping trigger fires) | `nerv:ritsuko` (MODE: intel-light) | touched-file locators, skills | envelope of at most ~2k tokens carrying `exploration-light.md`; Ikari writes it to its locator and runs one spot check | gatekeeper |
| 4. Work unit | one domain pilot (`MODE: solo`) | request or `exploration-light.md` locator, skills, TDD mode+runner, `## Verification` commands, known environmental failures, `commit_ref` | own RED observed, GREEN/TRIANGULATE/REFACTOR, TDD evidence rows, one `<command>: <observed result>` per verification command; no commit | gatekeeper; `partial`/`blocked` → Ikari decides (one retry, Maya, or ratchet) |
| 5. Verification gate | Ikari; `nerv:maya` (MODE: reduced) only when the gate below requires it; `task:dispatcher` `verification.started` when the task plugin is installed | touched test/lint/build scope only | spot-check result; `maya-report.md` when Maya ran | obvious fail → back to the pilot; ambiguous → Ikari asks the user |
| 6. Work-unit commit | Ikari | diff, `commit_ref` | commit shown, hash | **user validates before commit** |
| 7. RDD hook | native engine (via Ikari) | see `rdd-relay.md` | receipt or `review_due: false` | per RDD relay |
| 8. Run summary | Ikari; `task:dispatcher` when the task plugin is installed | usage table | `run.closed` emitted first, so its envelope is in the summary; then `nerv/run-summary.md` per its schema in `nerv-artifacts.md` | none |
| 9. Close | Ikari | — | change closed; a run that halts instead emits `run.halted` with the cause | none |

LIGHT is one coherent work unit (more than one is a FULL criterion); a fix
after a failing gate re-enters step 4 with the failure quoted. When the
explorer is skipped, step 4 receives the request text in `## Change`, and
the pilot must not report the missing `exploration-light.md` as a blocker.
Pilot selection for step 4 is automatic from the touched-file domain; the
user may override it when validating the step-6 commit.

### Verification gate

gentle-ai's Delegated Verification Gate, decided by two inputs Ikari reads
deterministically after the pilot returns: the RDD state (`on`, `off`,
`unknown` — from the status line, else `gentle-ai review mode status`; a
failure is `unknown`) and the risk tier of `gentle-ai review assess --cwd
<repo> --json` over the pilot's uncommitted diff (`passive`, `medium`,
`high`; a failure or unrecognized verb is `high`).

- **RDD on**: the pilot's `## Verification` report is the verification of
  record, and the native review at step 7 is the independent check. Maya
  stays on demand: the pilot reported `partial` or `blocked`, or an
  expensive check is better run on a cheaper profile. A `passive`
  candidate needs only Ikari's structural readback.
- **RDD off or unknown**: `passive` → structural readback only; `medium` →
  pilot self-verification, plus Maya when the pilot ran on a small-model
  profile (resolved effort `low` or a mini model such as `haiku`); `high`
  or unassessable → pilot self-verification plus Maya as the independent
  verifier. `unknown` never lowers a tier, and the small-model bias raises
  it by one.
- **Spot check**: in every tier Ikari re-runs one reported verification
  command before step 6 and compares it with the report; a mismatch goes
  back to the pilot.
- When the native review does not reach a terminal outcome for this
  candidate (consent declined, RDD disabled after the status read, START
  or STATUS refused), Ikari applies the RDD-off rule to the committed work
  unit before close; a resulting fix re-enters step 4 as a new commit.

Maya's reduced mode runs only the tests and lint/build touching the
changed files — never the full suite — per the `b`/`c` phases of her
report schema.

At step 9, Ikari sets `status: done` on every completed task in `tasks.md`
when the change has one (that field only, a mechanical write), records
`closed_at` in `state.yaml`, and deletes `nerv/.orchestrator.lock`.
