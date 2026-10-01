## LIGHT pipeline (Phase 1 — implemented)

| Step | Actor | Launch prompt carries | Expected envelope | Gate |
|---|---|---|---|---|
| 1. Preflight | Ikari, Hyuga (d), Aoba | — | tracker start via Hyuga `DISPATCH: tracker` (`createTask` if requested, then `start`); worktree/branch state | user HARD if asked |
| 2. Classify | Ikari | — | LIGHT decision recorded in log | none |
| 3. Micro-intel (optional) | Ritsuko | touched-file locators, skills | envelope carrying exploration-light.md; Ikari writes it to its locator; downstream steps read it when present | gatekeeper |
| 4. RED | Kaworu | change/task locators, skills, TDD mode+runner, commit_ref | failing test(s), commit | user validates commit |
| 5. GREEN/REFACTOR | one pilot (domain-matched) | same + RED commit ref | passing code, TDD evidence rows | gatekeeper |
| 6. Reduced quality gate | Maya | touched test/lint/build scope only | `maya-report.md` (reduced-mode note) | obvious fail → back to pilot; ambiguous → Ikari asks user |
| 7. Work-unit commit | Aoba | diff, commit_ref | commit shown, hash | **user validates before commit** |
| 8. RDD hook | native engine (via Ikari) | see RDD section | receipt or `review_due: false` | per RDD section |
| — repeat 4-8 per work unit — | | | | |
| 9. Run summary | Aoba | usage table from Ikari | `nerv/run-summary.md` | none |
| 10. Close | Ikari, Hyuga (d) | — | change closed, tracker `close`/`done`/`block` run | none |

When micro-intel is skipped, steps 4-6 receive the request text in
`## Change` instead, and pilots must not report the missing
exploration-light.md as a blocker.

Pilot selection for step 5 is automatic from the touched-file domain; the
user may override it when validating the step-7 commit. Maya's reduced mode
(step 6) runs only the tests and lint/build touching the changed files —
never the full suite — per the `b`/`c` phases of her report schema.

At step 10, Ikari sets `status: done` on every completed task in `tasks.md`
(that field only, a mechanical write), records `closed_at` in `state.yaml`,
and deletes `nerv/.orchestrator.lock`.

