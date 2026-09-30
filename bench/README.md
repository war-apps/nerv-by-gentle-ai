# NERV bench

Two complementary test suites live here:

- **`journeys.md`** — the manual behavioral suite. Its journeys (J0-J6)
  exercise activation, LIGHT/FULL classification, the full MAGI/audit
  pipeline, the task-tracker integration, and the resume/lock protocol,
  end to end, in a real scratch repo with a real test runner. Run it by
  hand, per phase, for everything a mechanical grader cannot judge: RDD
  consent envelopes, real git worktrees, the MAGI vote, the governance
  veto, the audit-and-closure stage, and live Teamwork calls.
- **`plugin/evals/`** — a `claude plugin eval` corpus: six cheap,
  deterministic cases derived from J0, J1 (preflight/classification only)
  and J5 (tracker `none`), each scaffolding its own throwaway git repo and
  `.nerv/nerv.yaml`. It checks only what a grader can judge mechanically
  or with a small model: the SessionStart activation gate, LIGHT/FULL
  classification, and the tracker-`none` path never touching Teamwork.

## Running the eval corpus

From the repository root:

```bash
claude plugin eval ./plugin --scaffold --runs 1 --ablation with-without --max-cost-usd 5 --json
```

- `--scaffold` is required: every case seeds its own empty git repo and
  `.nerv/nerv.yaml` through a `context.scaffold_script`, and
  `claude plugin eval` skips that script unless the flag is passed.
- `--ablation with-without` (already the default once the `nerv` plugin
  resolves from `./plugin`) also runs each case with no plugin loaded, so
  the report shows the delta the plugin actually contributes, not just a
  raw pass rate.
- The classification cases' `llm` grader calls a judge model and adds to
  the run's cost; `--max-cost-usd 5` is a sane ceiling for this six-case
  corpus at `--runs 1`.
- Results land under `plugin/evals/results/<timestamp>/report.html` and
  `aggregate-result.json` (both git-ignored — see `.gitignore`).

This corpus is **not** run in CI. It exists to catch a regression in the
cheap, mechanical parts of the protocol (does the SessionStart hook still
gate on `enabled: true`? does classification still say LIGHT/FULL where
it should? does `tasks.provider: none` still keep Teamwork untouched?)
between runs of the full manual suite above. `journeys.md` remains the
suite of record for everything the eval graders cannot judge.
