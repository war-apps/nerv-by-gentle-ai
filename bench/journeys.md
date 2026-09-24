# NERV bench journeys

Behavioral test suite of the overlay. Each journey runs in a scratch repo
(`D:\projects\nerv-bench-repo` on this machine) and lists the observable
outcomes that must hold. Run them after every phase and before any push.

## Common setup

- gentle-ai 3.7.0 installed; plugin `nerv@nerv` installed from the committed
  HEAD under test (`claude plugin uninstall nerv@nerv && claude plugin install nerv@nerv`).
- Scratch repo with a real test runner. Reference layout: .NET 10 class
  library `src/Calc` + xUnit project `tests/Calc.Tests`, solution `Bench.slnx`,
  one passing test (`Calculator.Add`).
- `.nerv/nerv.yaml` with `enabled: true`, `git.worktree: never`,
  `git.base_branch: main`, `tasks.provider: none`, `tasks.ask_when_missing: false`,
  skills `testing: [tdd]`, `code: [dotnet-best-practices]`.
- `openspec/config.yaml` with `strict_tdd: true` and
  `rules.apply.test_command: dotnet test`; `.atl/skill-registry.md` refreshed
  with `gentle-ai skill-registry refresh --cwd <repo>`.
- Non-interactive runs: `gentle-ai review mode disable --scope clone --cwd <repo>`
  so RDD consent envelopes do not block a session without a terminal. The
  global switch stays as the user set it. Interactive runs keep RDD as is.

## J0: overlay activation (Phase 0)

1. Fresh session in the scratch repo (marker present), prompt: "Answer YES or
   NO: does your context contain a section whose first line is
   `# NERV orchestrator protocol (active: .nerv/nerv.yaml enabled)`? Then list
   agent types starting with `nerv:`."
   Expected: `YES` and `nerv:aoba` (plus every agent shipped in the build).
2. Same prompt in a repo without `.nerv/nerv.yaml`. Expected: `NO`; agents may
   still be listed (they are global once installed).
3. `nerv ping` in the scratch repo. Expected: `nerv:aoba` spawned once,
   envelope with `status: success`, `executive_summary: aoba online`, the
   model from its frontmatter, repo root and branch.
4. `bash tests/hook-session-start.test.sh` in the plugin repo: all PASS/SKIP.

## J1: LIGHT path, non-interactive harness (Phase 1)

Setup: branch `feature/subtract` in the scratch repo, `Calculator` has only
`Add`. Prompt (piped to `claude -p --max-turns 60 --allowedTools
'Agent,Bash(git *),Bash(dotnet *),Bash(gentle-ai *),Read,Write,Edit,Glob,Grep'`):

> NERV LIGHT journey (non-interactive harness run). Context the user gives
> you up front so preflight stays silent: task ref: none; work in place on
> the current branch feature/subtract (no worktree); pace: fast-forward;
> artifact store: openspec; PR strategy: single-pr; the user pre-validates
> every commit Aoba shows in this run (commit without asking); change name:
> subtract-method.
> Request: add a `Subtract(int a, int b)` method to `Calculator` in
> src/Calc/Calculator.cs that returns a - b, with a unit test in
> tests/Calc.Tests. Follow the NERV orchestrator protocol injected in this
> session end to end.

Expected, in order:

1. Classification `LIGHT` recorded in `nerv/deliberation-log.md`; no gate
   asked (everything was pre-answered).
2. `openspec/changes/subtract-method/state.yaml` with `dependsOn: []` and the
   `nerv` block (`path: light`).
3. `nerv:ritsuko` launched in `MODE: intel-light`; `nerv/exploration-light.md`
   names `src/Calc/Calculator.cs`, the xUnit project, runner `dotnet test`,
   suggested pilot `shinji`.
4. `nerv:kaworu` writes `Subtract` test only; `dotnet test` fails with a
   compile error on the missing symbol (honest RED in C#); envelope
   `next_recommended: aoba-commit`.
5. `nerv:aoba` commits the RED (`test:` conventional commit, one file).
6. `nerv:shinji` adds `Subtract`, `dotnet test` green, TRIANGULATE second
   case, evidence table rows RED/GREEN/TRIANGULATE/REFACTOR in its envelope;
   never edits the RED test.
7. `nerv:maya` in `MODE: reduced` writes `nerv/maya-report.md`: phase a
   (touched tests) pass, phase d build pass, lint
   `no-lint-build-configured` stated explicitly, TDD evidence reproduced
   (RED commit inspected, GREEN re-run).
8. `nerv:aoba` commits the implementation (`feat:`), then writes
   `nerv/run-summary.md` with an agents table filled from the usage the
   orchestrator collected (never estimated), the two commits and no PR
   slices.
9. `git log --oneline` shows the RED commit before the GREEN commit.
10. `gentle-ai sdd-status subtract-method --cwd <repo> --json` succeeds with
    the `nerv/` folder present.

Interactive variant: same request without the pre-answered context. Expected
extra gates: the grouped preflight question (task, worktree, branch, base),
one commit-validation stop per commit, and the RDD consent envelope after the
first commit when the clone-local switch is not disabled.

## J2: classification FULL (Phase 2)

Setup: same scratch repo as J1, fresh branch `feature/multiply-and-ci` off
`main`, `Calculator` has only `Add`/`Subtract` (from J1). Prompt: "Add a
`Multiply(int a, int b)` method to `Calculator` in src/Calc/Calculator.cs,
and add a GitHub Actions workflow at `.github/workflows/ci.yml` that runs
`dotnet test` on push. Follow the NERV orchestrator protocol injected in
this session."

Expected:

1. Ikari's classification finds two pilot domains touched — backend
   (`shinji`, the `Calculator.Multiply` change) and ci-cd/infra (`toji`,
   the new workflow file) — and resolves `FULL`, recorded in
   `nerv/deliberation-log.md` as a `classification` event naming both
   domain signals.
2. Because FULL ships in this Phase 2 build, the run proceeds exactly as
   J3 describes below — Ikari does **not** offer the LIGHT-with-
   acknowledgement-or-stop prompt that a Phase 1-only build would show.
3. Regression check against the removed Phase 1 behavior: a Phase 1-only
   build (FULL pipeline absent) would instead stop here with one blocking
   prompt offering exactly two choices — proceed as LIGHT with explicit
   acknowledgement that MAGI vote, governance veto, and audit are skipped,
   or stop. This journey exists to confirm that path is gone now that
   FULL ships; if the two-choice prompt appears in this Phase 2 build,
   that is a regression.

## J3: FULL path with MAGI (Phase 2)

Non-interactive harness continuing directly from J2's request, with every
gate pre-answered in the prompt so the run completes unattended. Change
name: `multiply-and-ci`.

Setup: same scratch repo, `gentle-ai review mode disable --scope clone --cwd
<repo>` (RDD off in the bench clone — the global switch stays as the user
set it), `.nerv/nerv.yaml` unchanged from the Common setup above
(`tasks.provider: none`, `tasks.ask_when_missing: false`).

Prompt (piped to `claude -p --max-turns 200 --allowedTools 'Agent,Bash(git
*),Bash(dotnet *),Bash(gentle-ai *),Read,Write,Edit,Glob,Grep'`):

> NERV FULL journey (non-interactive harness run). Context the user gives
> you up front so every gate stays silent: task ref: none; work in place on
> the current branch feature/multiply-and-ci (no worktree); pace:
> fast-forward; artifact store: openspec; PR strategy: single-pr; change
> name: multiply-and-ci; for any corner-case question, choose the most
> conservative option and record it as the answer; plan approval is
> pre-granted — proceed past the plan-approval gate once tasks, criticality,
> votes, and veto rulings are all recorded; the user pre-validates every
> commit Aoba shows in this run (commit without asking).
> Request: add a `Multiply(int a, int b)` method to `Calculator` in
> src/Calc/Calculator.cs, and add a GitHub Actions workflow at
> `.github/workflows/ci.yml` that runs `dotnet test` on push. Follow the
> NERV orchestrator protocol injected in this session end to end.

Expected, in order:

1. Classification `FULL` (per J2), recorded in `nerv/deliberation-log.md`.
2. `openspec/changes/multiply-and-ci/state.yaml` with the `nerv` block
   (`path: full`).
3. `nerv:ritsuko` (MODE: intel) writes `exploration.md` covering both
   `src/Calc/Calculator.cs` and the new `.github/workflows/` surface.
4. `nerv:ritsuko` (MODE: test-plan) writes `specs/{domain}/spec.md` and
   `nerv/test-plan.md` with cases for `Multiply` and for the workflow
   file, plus a `## Corner-case questions` section (e.g. "should
   `Multiply` overflow-check?", "should CI run on pull_request too or
   push only?"). Ikari relays the questions as one grouped blocking
   prompt; per the pre-answered context, the most conservative option is
   chosen for each (no overflow-check scope creep beyond `int * int`; CI
   on `push` only, as literally asked) and recorded verbatim in
   `## Answers`.
5. `nerv:misato` writes `proposal.md`, `design.md` (with `## New skills,
   scripts and commands` — either `none`, or naming `.github/workflows/
   ci.yml` as a new script if the design treats it as one, in which case
   `nerv:fuyutsuki` evaluates it at step 9), and `tasks.md` with `T1`
   (`Multiply`, `pilot: shinji`, `depends_on: []`) and `T2` (`ci.yml`,
   `pilot: toji`, `depends_on: []`).
6. `nerv:hyuga` (dispatch a) writes `nerv/criticality.md` for T1 and T2
   (both `standard` unless a `critical_paths` entry matches).
7. MAGI vote round 1: `nerv:balthasar`, `nerv:melchor`, `nerv:casper`
   launched in one parallel batch, blind, each returning the JSON
   contract from `nerv-artifacts.md`; Ikari merges into `nerv/votes.md`
   with three member entries per task (T1, T2), `rule` applied per each
   task's criticality, `result: approved`, `frozen: true` for both (a
   rejection here would exercise the revise loop instead — not expected
   on this simple change).
8. `nerv:fuyutsuki` writes `nerv/veto-ruling.md` — one row per item from
   `design.md`'s `## New skills, scripts and commands` (or a single
   `none` line if that section was `none`).
9. Plan approval gate: recorded as pre-granted per the harness context —
   `nerv/deliberation-log.md` carries `plan_gate_relayed` followed
   immediately by `plan_gate_decision` (pre-granted); no prompt actually
   blocks execution.
10. `nerv:hyuga` (dispatch b) writes `nerv/waves.md` — T1 and T2 in the
    same wave (`W1`), `depends_on: []` for both since they are
    independent; `pilot_assignments: {T1: shinji, T2: toji}`.
11. `nerv:maya` (MODE: full, phase 0) writes the baseline into
    `nerv/maya-report.md`.
12. Per task in the wave (both may run in one parallel batch since they
    are independent): `nerv:kaworu` RED commit, then `nerv:aoba` commit
    shown and validated (pre-validated per harness context); then the
    assigned pilot's GREEN/TRIANGULATE/REFACTOR, then `nerv:aoba` commit
    shown and validated. `git log --oneline` shows, for each task, its RED
    commit before its GREEN commit.
13. `nerv:maya` (MODE: full) writes phases a→b→c→d into
    `nerv/maya-report.md`, each green before the next starts.
14. Ikari states plainly that the audit stage (Kaji, 5 passes, ranking,
    issue gate) is not shipped in this build — no audit artifacts are
    produced.
15. `nerv:aoba` writes `nerv/run-summary.md` (agents table with
    `tokens_total` per launch, commits, no PR slices since the strategy
    is `single-pr` and the run stayed under the delivery budget).
16. `nerv/deliberation-log.md` carries the Phase 2 event types exercised
    by this run: `corner_case_relayed`, `corner_case_answer`, `vote_cast`
    (per member per task), `vote_result` (per task), `veto_evaluated`,
    `plan_gate_relayed`, `plan_gate_decision`, `wave_plan`, `wave_report`
    — plus the Phase 1 types (`classification`, `launch`, `envelope`,
    `commit_recorded`, `rdd_assess`, ...). No `escalation_criticality`,
    `deviation`, or `ruling_issued` entries are expected on this
    straightforward change (nothing escalates, nothing deviates, no
    ruling is needed).
17. `gentle-ai sdd-status multiply-and-ci --cwd <repo> --json` succeeds,
    reporting the change with all gentle-ai-owned files present and the
    `nerv/` folder alongside them.

Interactive variant: same request without the pre-answered context.
Expected extra gates beyond J1's interactive variant: the corner-case
questions relayed as one grouped blocking prompt after step 4 (real user
answers required before Misato's plan step), the plan-approval gate after
step 9 (real HARD stop — tasks/criticality/votes/veto summary shown before
any implementation), and a commit-validation stop per commit in step 12 —
RDD consent envelopes also apply per commit when the clone-local switch is
not disabled, same as J1.

## J4: audit and closure (Phase 3), J5: task tracker and single config
(Phase 4), J6: resume after interruption (Phase 5)

To be written with their phases.
