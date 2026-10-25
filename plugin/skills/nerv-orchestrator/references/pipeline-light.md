## LIGHT pipeline (delegated direct)

gentle-ai's delegated-direct route with the fewest agents: an explorer only
when the mapping trigger fires, one bounded writer (the domain pilot in
`MODE: solo`, doing its own test-first cycle), Maya only when the
verification gate asks for an independent verifier, and `task:dispatcher`
only when the task extension is installed. Ikari runs the git state, the commit and the run summary
itself; Aoba is launched only for PR preparation or slicing.

LIGHT creates no ODD feature document, no Engram copy of one and no
commit at close: like gentle-ai's ODD for small work, it works from the
request and the diff. Its run files (`state.yaml`, `deliberation-log.md`,
`task-events.yaml`, `run-summary.md`, `exploration-light.md`,
`maya-report.md`) stay in the runtime folder `.nerv/runs/{run-id}/`, never
committed, and the lock in `.nerv/locks/{run-id}.lock`; every `nerv/...`
locator below resolves there. The run asks the user to approve only its
work commit.

| Step | Actor | Launch prompt carries | Expected envelope | Gate |
|---|---|---|---|---|
| 1. Preflight | Ikari; `task:dispatcher` only when the task extension is installed | — | answers collected; `run.started` and the worktree/branch run after step 2 | user HARD if asked |
| 2. Classify | Ikari | — | `classification` event: LIGHT plus the route declaration (which triggers fired, the domain signal); worktree/branch created by Ikari when requested | none |
| 3. Explorer (only when the mapping trigger fires) | `nerv:ritsuko` (MODE: intel-light) | touched-file locators, skills | envelope of at most ~2k tokens carrying `exploration-light.md`; Ikari writes it to its locator and runs one spot check | gatekeeper |
| 4. Work unit | one domain pilot (`MODE: solo`) | request or `exploration-light.md` locator, skills, TDD mode+runner, `## Verification` commands, known environmental failures, `commit_ref` | own RED observed, GREEN/TRIANGULATE/REFACTOR, TDD evidence rows, one `<command>: <observed result>` per verification command; no commit | gatekeeper; `partial`/`blocked` → Ikari decides (one retry, Maya, or ratchet) |
| 5. Verification gate | Ikari; `nerv:maya` (MODE: reduced) only when the gate below requires it; `task:dispatcher` `verification.started` when the task extension is installed | touched test/lint/build scope only | spot-check result; `maya-report.md` when Maya ran | obvious fail → back to the pilot; ambiguous → Ikari asks the user |
| 6. Work-unit commit | Ikari | diff, `commit_ref` | commit shown, hash | **user validates before commit** |
| 7. RDD hook | native engine (via Ikari) | see `rdd-relay.md` | receipt or `review_due: false` | per RDD relay |
| 8. Run summary | Ikari; `task:dispatcher` when the task extension is installed | usage table | Engram flush at close (`usage-and-log.md`) and `run.closed` first, so both are in the summary; then `nerv/run-summary.md` per its schema in `nerv-artifacts.md` | none |
| 9. Close | Ikari | `.nerv/runs/{run-id}/state.yaml` | `closed_at`, the `stop` event, lock deleted (see below); a run that halts instead emits `run.halted` with the cause and logs it in the deliberation log | none |

LIGHT is one coherent work unit (more than one is a FULL criterion); a fix
after a failing gate re-enters step 4 with the failure quoted. When the
explorer is skipped, step 4 receives the request text in `## Feature`, and
the pilot must not report the missing `exploration-light.md` as a blocker.
Pilot selection for step 4 is automatic from the touched-file domain; the
user may override it when validating the step-6 commit.

### Verification gate

gentle-ai's Delegated Verification Gate, the same whether RDD is on or
off. After the pilot returns, Ikari runs `gentle-ai review assess --cwd
<repo> --agent claude-code --json` over the pilot's uncommitted diff
(`passive`, `medium`, `high`).

- **Tier**: `passive` → Ikari's structural readback only; `medium` → the
  pilot's `## Verification` report is the verification of record, whatever
  model or effort the pilot ran on.
- **Independent verifier**: Maya runs as the independent verifier when,
  and only when, the change touches one of gentle-ai's Task Size high-risk
  items — (1) data or irreversible effects, (2) security, (3) contracts
  others consume, (4) concurrency, (5) delivery or environment, (6) no test
  would catch a regression — and Ikari names it by adding `--escalate-item
  <1-6> --escalate-reason "<one line>"` to that assessment. A native `high`
  without a named item, a failed or unknown assessment, an unrecognized
  verb, and the pilot's model or effort never launch her by themselves:
  the pilot's self-verification stands, and Ikari names the item when one
  applies. Maya also runs on demand when the pilot reported `partial` or
  `blocked`, or an expensive check is better run on a cheaper profile.
- **RDD on**: the native review at step 7 is an additional outside view of
  the committed work unit. It never replaces or skips this gate, and Maya,
  when the gate calls her, runs before step 6, so before the review. A
  declined consent, a disabled clone, or a refused START or STATUS changes
  nothing about verification.
- **Spot check**: in every tier Ikari re-runs one reported verification
  command before step 6 and compares it with the report; a mismatch goes
  back to the pilot.

Maya's reduced mode runs only the tests and lint/build touching the
changed files — never the full suite — per the `b`/`c` phases of her
report schema.

At step 9, Ikari records the outcome, any failed, skipped or pending
check and the next step in the run summary, records `closed_at` in
`.nerv/runs/{run-id}/state.yaml`, appends the `stop` event and deletes
`.nerv/locks/{run-id}.lock`. Nothing is committed at close: the step-6
work commit is the run's only commit, and the runtime folder is never
staged. There is no archive. A LIGHT run is never left delivered but
unclosed. A FULL criterion found mid-run ratchets to FULL, which creates
the feature document from the run (`odd-document.md`, Ratchet to FULL).
