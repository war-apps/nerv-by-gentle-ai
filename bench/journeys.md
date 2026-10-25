# NERV bench journeys

Behavioral test suite of the overlay. Each journey runs in a scratch repo
(`D:\projects\nerv-bench-repo` on this machine) and lists the observable
outcomes that must hold. Run them after every phase and before any push.

## Common setup

- gentle-ai 4.x installed (tested against 4.0.0); plugin `nerv@nerv` installed from the committed
  HEAD under test (`claude plugin uninstall nerv@nerv && claude plugin install nerv@nerv`).
  The task extension (`task@nerv`) is **not** installed, except where J5 and its variants say so, so no
  task event runs.
- Scratch repo with a real test runner. Reference layout: .NET 10 class
  library `src/Calc` + xUnit project `tests/Calc.Tests`, solution `Bench.slnx`,
  one passing test (`Calculator.Add`).
- `.ai-workflow/config.yaml` with `enabled: true`, `git.worktree: never`,
  `git.base_branch: main`, skills `testing: [tdd]`, `code: [dotnet-best-practices]`.
- `.atl/skill-registry.md` refreshed with `gentle-ai skill-registry refresh
  --cwd <repo>`. The ODD document's header then records strict TDD with the
  runner `dotnet test`, which the orchestrator resolves from the solution
  (one workspace-level command covers every project).
- Non-interactive runs: `gentle-ai review mode disable --scope clone --cwd <repo>`
  so RDD consent envelopes do not block a session without a terminal. The
  global switch stays as the user set it. Interactive runs keep RDD as is.

## J0: overlay activation (Phase 0)

1. Fresh session in the scratch repo (marker present), prompt: "Answer YES or
   NO: does your context contain a section whose first line is
   `# NERV orchestrator protocol (active: .ai-workflow/config.yaml enabled)`? Then list
   agent types starting with `nerv:`."
   Expected: `YES` and `nerv:aoba` (plus every agent shipped in the build).
2. Same prompt in a repo without `.ai-workflow/config.yaml`. Expected: `NO`; agents may
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
> artifact store: odd; PR strategy: single-pr; the user pre-validates
> every commit shown in this run (commit without asking); change name:
> subtract-method.
> Request: add a `Subtract(int a, int b)` method to `Calculator` in
> src/Calc/Calculator.cs that returns a - b, with a unit test in
> tests/Calc.Tests. Follow the NERV orchestrator protocol injected in this
> session end to end.

Expected, in order:

1. Classification `LIGHT` recorded in `nerv/deliberation-log.md`; no gate
   asked (everything was pre-answered). The `classification` event
   carries the route declaration: LIGHT because the change is new
   behavior with its own test (two non-trivial files), with the
   evidence-budget and mapping triggers stated.
2. The ODD document `odd/tasks/subtract-method.md` (header with strict TDD
   and `dotnet test`, `S1` quoting the request, one `T1` row with route
   `delegated (LIGHT)`, `L1` verbatim) and
   `odd/subtract-method/nerv/state.yaml` with the `nerv` block
   (`feature: subtract-method`, `path: light`).
3. `nerv:ritsuko` (`MODE: intel-light`) is launched only when the mapping
   trigger fired; in this two-file scratch repo it normally is not, and the
   route declaration says so. When it runs, `nerv/exploration-light.md`
   names `src/Calc/Calculator.cs`, the xUnit project, runner `dotnet test`,
   suggested pilot `shinji`.
4. `nerv:shinji` in `MODE: solo` writes the `Subtract` test first;
   `dotnet test` fails with a compile error on the missing symbol (honest
   RED in C#), then adds `Subtract`, `dotnet test` green, TRIANGULATE
   second case; evidence table rows RED ("✅ Written (Shinji)")/GREEN/
   TRIANGULATE/REFACTOR and one `<command>: <observed result>` per
   verification command in its envelope; `next_recommended: ikari-commit`.
5. Verification gate: Ikari re-runs one reported command (spot check).
   `nerv:maya` (`MODE: reduced`) runs only when the gate requires an
   independent verifier (RDD off/unknown with a `high` or unassessable
   tier, or a small-model pilot on `medium`); with RDD on, the native
   review after the commit is the independent check.
6. Ikari commits the work unit itself (`feat:`, test and implementation
   together) after the pre-validation; no AI attribution trailer.
7. Ikari writes `nerv/run-summary.md` with an agents table filled from the
   usage collected (never estimated; Shinji, plus Ritsuko or Maya only if
   launched), the commit and no PR slices. No `nerv:kaworu` or `nerv:aoba`
   launch appears in the log.
8. `T1`'s row carries `Status` `done` and the work-unit commit, the
   `## Log` records the close, the close commit `docs: close nerv run for
   subtract-method` holds the document and `odd/subtract-method/nerv/`
   (`artifacts.commit: at-close`), and no archive or `openspec/` folder
   exists.

DIRECT variant: the request "fix the typo `Retuns` in the XML doc comment
of `Add`" classifies DIRECT (`path: direct`): Ikari edits the one file,
runs the focused build/test itself, shows the commit for validation and
commits it, runs the RDD hook and writes the run summary — no agent
launched (the `classification-direct` eval case covers the
classification slice).

Interactive variant: same request without the pre-answered context. Expected
extra gates: the grouped preflight question (worktree, branch, base),
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
set it), `.ai-workflow/config.yaml` unchanged from the Common setup above
(no task extension installed).

Prompt (piped to `claude -p --max-turns 200 --allowedTools 'Agent,Bash(git
*),Bash(dotnet *),Bash(gentle-ai *),Read,Write,Edit,Glob,Grep'`):

> NERV FULL journey (non-interactive harness run). Context the user gives
> you up front so every gate stays silent: task ref: none; work in place on
> the current branch feature/multiply-and-ci (no worktree); pace:
> fast-forward; artifact store: odd; PR strategy: single-pr; change
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
2. `odd/tasks/multiply-and-ci.md` with `S1`/`S2` quoting the two asks and
   `L1`, and `odd/multiply-and-ci/nerv/state.yaml` with the `nerv` block
   (`path: full`).
3. `nerv:ritsuko` (MODE: intel) returns `nerv/exploration.md` covering both
   `src/Calc/Calculator.cs` and the new `.github/workflows/` surface;
   Ikari writes it and a `## Log` pointer.
4. `nerv:ritsuko` (MODE: test-plan) returns the test-plan `## Log` entry
   with cases for `Multiply` (`S1`) and for the workflow file (`S2`),
   plus a `### Corner-case questions` section (e.g. "should
   `Multiply` overflow-check?", "should CI run on pull_request too or
   push only?"). Ikari relays the questions as one grouped blocking
   prompt; per the pre-answered context, the most conservative option is
   chosen for each (no overflow-check scope creep beyond `int * int`; CI
   on `push` only, as literally asked) and recorded verbatim in
   `### Answers`.
5. `nerv:misato` completes `## Specs`, writes the design `## Log` entry
   (with `### New skills, scripts and commands` — either `none`, or naming
   `.github/workflows/ci.yml` as a new script if the design treats it as
   one, in which case `nerv:fuyutsuki` evaluates it at step 9), and
   `## Tasks` with `T1` (`Multiply`, `S1`, pilot `shinji`, no
   dependencies) and `T2` (`ci.yml`, `S2`, pilot `toji`, no
   dependencies).
6. `nerv:hyuga` (dispatch a) writes `nerv/criticality.md` for T1 and T2
   (both `standard` unless a `critical_paths` entry matches).
7. MAGI vote round 1: `nerv:balthasar`, `nerv:melchior`, `nerv:casper`
   launched in one parallel batch, blind, each returning the JSON
   contract from `nerv-artifacts.md`; Ikari merges into `nerv/votes.md`
   with three member entries per task (T1, T2), `rule` applied per each
   task's criticality, `result: approved`, `frozen: true` for both, copied
   into each row's `MAGI` column (a
   rejection here would exercise the revise loop instead — not expected
   on this simple change).
8. `nerv:fuyutsuki` writes `nerv/veto-ruling.md` — one row per item from
   the design entry's `### New skills, scripts and commands` (or a single
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
14. Ikari states plainly that the audit stage (Kaji, 4 passes, ranking,
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
17. `/nerv:status` lists `multiply-and-ci` with all gentle-ai-owned files
    present and the `nerv/` folder alongside them.

Interactive variant: same request without the pre-answered context.
Expected extra gates beyond J1's interactive variant: the corner-case
questions relayed as one grouped blocking prompt after step 4 (real user
answers required before Misato's plan step), the plan-approval gate after
step 9 (real HARD stop — tasks/criticality/votes/veto summary shown before
any implementation), and a commit-validation stop per commit in step 12 —
RDD consent envelopes also apply per commit when the clone-local switch is
not disabled, same as J1.

## J4: audit and closure (Phase 3)

Setup: fresh branch `feature/divide-audit` off the scratch repo's `main`,
where `feature/multiply-and-ci` (J3's FULL change, `multiply-and-ci`) is
merged first — so J4 starts from a real `BASE..HEAD` with `Add`,
`Subtract`, and `Multiply` already present and CI already wired. The
request seeds a defect deliberately: it asks for a
`Divide(int a, int b)` method whose task is specified in `## Tasks` as
"returns `a / b`" with a test plan that omits the division-by-zero case,
so the audit stage has a real, candidate-caused finding to catch rather
than a synthetic one.

`gentle-ai review mode disable --scope clone --cwd <repo>` (RDD off in the
bench clone, as in J1/J3). `.ai-workflow/config.yaml` as in Common setup plus
`critical_paths: [src/Calc/]`: the audit stage lives only in the FULL
pipeline, and a single-domain `Divide` change would classify LIGHT, so the
critical-path rule is what forces FULL here.

Prompt (piped to `claude -p --max-turns 250 --allowedTools 'Agent,Bash(git
*),Bash(dotnet *),Bash(gentle-ai *),Read,Write,Edit,Glob,Grep'`):

> NERV FULL journey — audit and closure (non-interactive harness run).
> Context the user gives you up front so every gate stays silent: task
> ref: none; work in place on the current branch feature/divide-audit (no
> worktree); pace: fast-forward; artifact store: odd; PR strategy:
> single-pr; change name: divide-audit; for any corner-case question,
> choose the most conservative option and record it as the answer; plan
> approval is pre-granted — proceed past the plan-approval gate once
> tasks, criticality, votes, and veto rulings are all recorded; the user
> pre-validates every commit Aoba shows in this run (commit without
> asking); at the audit issue gate, approve the NOW set exactly as ranked;
> if a re-audit round reaches the cap of 2, accept the residual and close.
> Request: add a `Divide(int a, int b)` method to `Calculator` in
> src/Calc/Calculator.cs that returns `a / b`, with a unit test in
> tests/Calc.Tests. Follow the NERV orchestrator protocol injected in this
> session end to end, including the audit and closure stage.

Expected, in order:

1. Classification `FULL`, recorded in `nerv/deliberation-log.md` with the
   critical-path signal (`src/Calc/` from `critical_paths`); a LIGHT
   classification here is a regression because the audit stage would never
   run.
2. `odd/divide-audit/nerv/state.yaml`, the test-plan `## Log` entry of
   `odd/tasks/divide-audit.md` with the `Divide` case present and **no**
   division-by-zero case (the seeded gap), and `## Tasks` with the
   `Divide` task specified as "returns `a / b`".
3. Implementation lands `Divide` exactly as specified — no defensive
   divisor check, since nothing in the plan asked for one — via
   Kaworu RED → Aoba commit → pilot GREEN → Aoba commit.
4. `nerv:maya` full gate a→b→c→d green.
5. Audit stage, round 1:
   - `nerv:aoba` freezes `nerv/audit/diff-round-1.patch` and
     `nerv/audit/round-1.yaml` (base = branch point, head = current HEAD).
   - Four passes launch in one parallel batch, blind:
     `nerv:melchior` (MODE: audit, structure and security),
     `nerv:balthasar` (MODE: audit, software principles and readability),
     `nerv:casper` (MODE: audit, process), `nerv:gendo`. Each
     writes its validated JSON to
     `nerv/audit/pass-<name>-round-1.json`.
   - Expected finding: `nerv:gendo` flags the missing
     division-by-zero case as `CRITICAL`, `evidence_class: deterministic`,
     `causal_disposition: introduced` (the task specified `a / b` with no
     guard and the test plan never asked for the zero case — both
     candidate-caused). `nerv:melchior` or `nerv:balthasar` may
     additionally flag the unchecked divisor (accept either or both;
     record whichever actually fired).
   - `nerv:kaji` compiles `nerv/audit-report.md`: the coverage finding
     listed with `refuter: n/a` (deterministic, no refuter needed); if a
     second pass also flagged the divisor as an inferential finding, it is
     listed with `refuter: pending` and included in the round's refuter
     batch.
   - If a refuter batch ran, `nerv:fuyutsuki` (MODE: refute, read-only)
     returns its verdict and Ikari merges the outcome into
     `audit-report.md`.
6. `nerv:hyuga` (dispatch c) writes `nerv/issue-ranking.md`: the
   division-by-zero finding ranked `NOW` (`severity: Critical`,
   `blast_radius: local`, `verification_cost: cheap`, `owner: shinji` or
   whichever pilot owns `Calculator`); any WARNING-level companion finding
   may be `NOW` or `DEFER` per Hyuga's judgment, with a one-line reason
   either way.
7. Issue gate: Ikari relays the ranked list as one blocking prompt; the
   harness pre-approves the NOW set exactly as ranked (`decision:
   approved-as-ranked` in `issue-ranking.md`'s `## Gate decision`).
8. Fix routing: the owning pilot fixes the NOW issue through FULL's
   own work-unit cycle — `nerv:kaworu` writes a RED test named `DivideByZero`
   (asserting the documented guard behavior, e.g. a thrown
   `DivideByZeroException` or a caller-visible error), `nerv:aoba` commits
   the RED test, the pilot adds the guard, `nerv:aoba` commits the fix,
   RDD hook runs (no-op, RDD disabled in this bench clone).
9. Re-audit round 2: `nerv:aoba` freezes
   `nerv/audit/diff-round-2.patch` scoped to the fix delta only (base =
   round 1's HEAD); the same four passes run again over that delta; Kaji
   compiles round 2, carrying forward any round-1 item still unresolved.
   Expected: no candidate-caused `BLOCKER`/`CRITICAL` remains in round 2
   — the guard is in place and tested — so the loop ends here, under the
   cap of 2, without needing `residual_accepted`.
10. `nerv:ritsuko` (MODE: docs) writes `nerv/issue-resolutions.md` (the
    division-by-zero item marked `fixed` with the guard commit hash) and
    `nerv/agent-config.md` (every launch across the whole run, audit
    passes included).
11. Every `## Tasks` row carries `Status` `done` and its commit (the fix
    commit recorded too); no archive step runs and no `openspec/` folder
    exists.
12. `nerv:fuyutsuki` (MODE: curate) appends a `## Summary` block to the
    top of `nerv/deliberation-log.md`.
13. `nerv:aoba` writes `nerv/run-summary.md` (agents table including every
    audit-pass and refuter launch, commits including the RED/fix pair,
    no PR slices since the strategy is `single-pr`).
14. Close: the `## Log` close entry, then Aoba's close commit `docs: close
    nerv run for divide-audit`.

Expected artifacts, in creation order: `odd/tasks/divide-audit.md` (header,
seeded `## Specs`, `L1`), `nerv/state.yaml`, `nerv/exploration*.md`, the
test-plan `## Log` entry, the completed `## Specs`, the design entry and
`## Tasks`,
(FULL-only: `nerv/criticality.md`, `nerv/votes.md`, `nerv/veto-ruling.md`,
`nerv/waves.md`), `nerv/maya-report.md`, `nerv/audit/diff-round-1.patch`,
`nerv/audit/round-1.yaml`, `nerv/audit/pass-<name>-round-1.json` ×4,
`nerv/audit-report.md`, `nerv/issue-ranking.md`,
`nerv/audit/diff-round-2.patch`, `nerv/audit/round-2.yaml`,
`nerv/audit/pass-<name>-round-2.json` ×4 (updated `audit-report.md`),
`nerv/issue-resolutions.md`, `nerv/agent-config.md`,
`nerv/run-summary.md`.

Expected log events, in order (Phase 1/2 event types included where the
run's own classification exercises them, plus every Phase 3 event type):
`classification`, `launch`, `envelope`, ..., `patch_frozen` (round 1),
`audit_pass` ×4 (round 1), `dedupe_merge` (if any), `refuter_result` (if
any), `ranking_issued`, `issue_gate_relayed`, `issue_gate_decision`,
`fix_routed`, `commit_recorded` (RED and fix), `reaudit` (round 2),
`patch_frozen` (round 2), `audit_pass` ×4 (round 2), `ranking_issued`
(round 2, empty NOW set) or its equivalent closure signal, `docs_written`,
`log_curated`, `stop`/close. No `residual_accepted` entry is
expected in this journey since round 2 resolves clean under the cap.

Non-interactive prompt gates pre-answered, matching J3's list plus two
audit-stage additions: worktree/branch/base (J1), corner-case
questions answered conservatively (J3), plan approval pre-granted (J3),
every Aoba commit pre-validated (J1/J3), **issue gate: approve the NOW set
exactly as ranked**, and **at the re-audit cap: accept the residual and
close** (not exercised in the expected happy path above, but must be
present in the harness prompt so the run does not stall if round 2 finds
something new).

## J5: task extension events and single config

Interactive-first journey: the task question comes back from the task
extension as `needs_input`, and the grouped preflight question
(worktree/branch/base) needs a human answer, so this journey runs
interactively by default; a non-interactive variant follows with both
pre-answered in the prompt, matching J1/J3's harness-context style.

Setup:

- `task@nerv` and `task-teamwork@nerv` installed (`nerv install --with
  task-teamwork`), and Claude Code restarted, so the agent type
  `task:dispatcher` is available. The Teamwork MCP server that
  `task-teamwork`'s `provider.yaml` declares is configured.
- Bench repo `.ai-workflow/config.yaml`:
  ```yaml
  enabled: true
  tasks:
    provider: teamwork
    ask_when_missing: true
    providers:
      teamwork:
        project_id: <TEST_PROJECT_ID>     # a TEST project the user names —
        tasklist_id: <TEST_TASKLIST_ID>   # NEVER the production ERP project
                                           # (1271726 / tasklist 3951970)
  git:
    worktree: never
    base_branch: main
  ```
- User-scope `~/.config/ai-workflow/config.yaml`:
  ```yaml
  tasks:
    providers:
      teamwork:
        assignee_id: 686035
        stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }
  ```
- No local timer for this session in `~/.claude/work/timers.json` at start.

Prompt (interactive): "Add a `Modulo(int a, int b)` method to `Calculator`
in src/Calc/Calculator.cs that returns `a % b`, with a unit test in
tests/Calc.Tests. Follow the NERV orchestrator protocol injected in this
session."

Expected, in order:

1. After classification, Ikari launches `task:dispatcher` with `EVENT:
   run.started`. With no task ref, the envelope is `needs_input` and Ikari
   relays its task question verbatim. Answer: create a task titled "Add
   Calculator.Modulo" in the configured TEST tasklist, priority `medium`.
   Ikari dispatches `run.started` again with the answer in `data`.
2. The task exists in Teamwork, assigned to the user (`assignee_id` from
   the user-scope file), in the `inDev` stage (`DESARROLLO`); a local timer
   keyed by `{taskRef, sessionId}` exists in `~/.claude/work/timers.json`.
   Then the grouped preflight question asks Branch name (carrying `tw-{id}`
   from `git.branch_pattern` with `{prefix}` = `tw`) and Base branch
   (`main`); Worktree is skipped (`git.worktree: never`). Commits carry
   `(TW-{id})` (`commit_ref` from `git.commit_ref_pattern`).
3. `odd/{feature}/nerv/state.yaml`'s `nerv` block has
   `task_ref: "TW-{id}"`, taken from the `run.started` envelope.
4. Maya's full gate start (FULL), or the work unit's verification start
   (DIRECT and LIGHT), emits `verification.started` — the task moves to
   `TESTING` in Teamwork.
5. Close emits `run.closed`: exactly one Teamwork timelog with real start
   and end (from the local timer's `startedAt`), rounded per the
   provider's `rounding_minutes`; the task lands in `IMPLEMENTA`
   (Implementado) and stays **open** — `run.closed` never completes it.
   `/task:close` run by hand afterward completes it.
6. `nerv/task-events.yaml` holds every envelope of the run, and
   `nerv/deliberation-log.md` carries one `task_event` entry
   (`{event, provider, result, task_ref}`) per envelope.
7. `/task:me` run by hand afterward renders the unified table — same
   columns, same stage/priority emoji — with sheet sources from
   `tasks.sources` included alongside the Teamwork rows.
8. Per-event switch variant: with `tasks.events.verification.started: off`
   in the bench repo file, step 4's envelope is `skipped` with `reason:
   switched off` and the task stays in `DESARROLLO`.
9. Missing-tools variant: with the Teamwork MCP server removed (or only the
   claude.ai Teamwork connector available), `run.started` answers `blocked`
   naming the missing tools; the run goes on without halting or retrying,
   and the run summary lists the envelope under "Task events pending" with
   `/task:replay <feature>`. After restoring the server, `/task:replay
   <feature>` re-dispatches it and marks it `replayed`.
10. Halted-run variant: a pilot signals a block mid-run (e.g. an
    unresolvable dependency). Ikari asks the user for the mandatory cause
    first — the run never blocks silently or invents one — then emits
    `run.halted` with the cause in `data`; the resulting comment on the
    task reads `BLOQUEADA: <cause>`, matching `/task:blocked`'s own comment
    format, the task moves to the blocked stage and the local timer is
    stopped and logged.

Non-interactive variant: same setup, with the task and the grouped
question pre-answered in the prompt (task: create with title/list/priority
given up front; worktree: skipped since `git.worktree: never`; branch and
base pre-stated), piped like J1
(`claude -p --max-turns 60 --allowedTools 'Agent,Bash(git *),Bash(dotnet
*),Bash(gentle-ai *),Read,Write,Edit,Glob,Grep,mcp__teamwork__*'`).
Expected outcomes 1-10 above hold identically, except no interactive prompt
is shown.

## J6: Resume and concurrent orchestrator (Phase 5)

Verifies the orchestrator lock (`.nerv/locks/{feature}.lock`)
and its 15-minute heartbeat rule: a resume while the lock is fresh and
foreign must refuse rather than run alongside a live orchestrator; a resume
after the heartbeat goes stale must take over cleanly, without re-voting
frozen tasks or re-running closed waves.

Setup: same scratch repo as J3, fresh branch `feature/power-resume` off
`main` (`Calculator` has `Add`/`Subtract`/`Multiply`/`Divide` from
J1/J3/J4), `.ai-workflow/config.yaml` as in Common setup (no task extension),
`gentle-ai review mode disable --scope clone --cwd <repo>`. Feature name:
`power-resume`. Request: add a `Power(int base, int exponent)` method to
`Calculator` in src/Calc/Calculator.cs, using the same J3-style FULL
prompt (task ref: none; work in place on `feature/power-resume`; pace:
fast-forward; artifact store: odd; PR strategy: single-pr; corner
cases answered conservatively; plan approval pre-granted; every Aoba
commit pre-validated).

1. **Start and kill.** Launch the FULL run non-interactively
   (`claude -p --max-turns 200 ...`, same allowedTools as J3), tailing
   `nerv/deliberation-log.md`. As soon as the `vote_result` events for
   both/all tasks appear in the log (MAGI vote round 1 concluded, tasks
   frozen), kill the session process directly (not a graceful stop).
   Expected before the kill: `.nerv/locks/power-resume.lock` exists, holder
   `session_id`, `host`, `phase`/`step` and `heartbeat_at` populated
   (`pid` is always `null`: the protocol keys liveness on the heartbeat,
   not on a process id), refreshed at least once since creation.
2. **Refused resume (fresh lock).** Within 15 minutes of the kill, launch
   a resume session with the J3 resume prompt ("Resume the NERV run for
   feature `power-resume`. Follow the NERV orchestrator protocol injected
   in this session, including its Resume section."). Expected: Ikari
   reads the lock, finds `heartbeat_at` fresh (under 15 minutes old) and
   the holder id foreign to this session, and refuses to take over —
   `nerv/deliberation-log.md` gets a `lock_refused` event naming the
   foreign holder and the lock's age; the session states plainly that a
   live orchestrator appears to hold this change and stops. Apart from
   that single log append, no file under `odd/power-resume/` or
   `odd/tasks/power-resume.md` is written, modified or deleted by this
   session, and the lock keeps the foreign holder and its `heartbeat_at`
   untouched (verify by hashing every file there and the lock, except
   `deliberation-log.md`, before and after — identical; `git status
   --porcelain` is not enough while the feature's files are still
   untracked). In a non-interactive bench
   run the harness prompt pre-answers the protocol's wait-or-take-over
   prompt with "wait"; a real interactive session relays that prompt and
   may take over only after the user confirms the holder is dead — not
   exercised here since 15 minutes have not elapsed.
3. **Legitimate resume (stale lock).** Make the heartbeat stale: either
   wait past the 15-minute rule, or, for a faster bench run, edit
   `.nerv/locks/power-resume.lock`'s `heartbeat_at` field directly to a
   timestamp older than 15 minutes (test-only shortcut — a real resume
   never edits the lock by hand). Launch the resume session again with
   the same prompt. Expected: the resume first reads `mem_context`,
   `mem_search` for the feature, `mem_get_observation` and
   `odd/tasks/power-resume.md` (ODD's order); `nerv/deliberation-log.md` gets a `resume`
   event with `from_step` (the step after the last completed one — here,
   after the vote) and `took_over_from` (the dead holder's id from the
   stale lock); the lock is rewritten with this session's holder id and a
   fresh heartbeat. The MAGI-voted, frozen tasks from step 1 are not
   re-voted (no new `vote_cast`/`vote_result` events for the same task
   ids); any wave already closed before the kill is not re-run (no
   duplicate `wave_report` for a closed wave id). The run continues from
   Fuyutsuki's veto ruling onward through plan approval, waves,
   implementation, Maya's full gate, and close exactly as J3 describes.
4. **`/nerv:status` during and after.** Run `/nerv:status` while the
   resumed run is still in progress: it reports the lock state (holder
   id, wave in progress, heartbeat age). Run it again after the run
   closes: the lock line is absent (the lock file is removed at close).
5. **`## Tasks` status at close.** After close, every `## Tasks` row the
   run completed (T1 and any others from `waves.md`) shows `Status` `done`
   and its commit — not `pending`, the defect the J4 close process left
   behind before Phase 5 (see `odd/tasks/nerv-overlay.md` T3.2) — and the
   `## Log` records the close.
6. **Artifacts commit policy: `at-close`.** With `.ai-workflow/config.yaml`'s
   `artifacts.commit: at-close` (the default, unchanged from Common
   setup), inspect the closing commit: `docs: close nerv run for
   power-resume` exists in `git log`, and `git show --stat` on it does
   **not** list anything under `.nerv/` (the lock is never committed at
   any point — it is deleted before close, and `.nerv/` ignores itself).
   Confirm the ODD document and the run folder's content (deliberation
   log, votes, veto ruling, waves, maya reports, run summary) are fully
   present in that commit, never left uncommitted after close: `git
   status --porcelain odd/` prints nothing once the session exits. The
   close commit is the run's last write, so the `stop` event precedes it
   in the committed log and no `commit_recorded` line for the close
   commit itself exists (its hash is in the session's final message).

### Variant: task extension absent (J5 companion)

Run standalone (not chained off J3/J6's change) to confirm a run without
the task extension stays free of task steps. Setup: fresh branch, `task@nerv`
not installed (no `task:dispatcher` agent type), `.ai-workflow/config.yaml` still
carrying the J5 `tasks:` block, no active timer. Interactive session,
prompt: "Add a `Modulo` method..." (as J5). Expected: the grouped
question asks only Worktree (per `git.worktree`), Branch name and Base
branch — no task question; no agent is launched for task events, no
`task_event` entry appears in `nerv/deliberation-log.md`, no
`nerv/task-events.yaml` is created, and the `tasks:` block is left in
`.ai-workflow/config.yaml` untouched. The `task-plugin-absent-no-events` eval case
covers the preflight slice of this variant.

### Variant: missing-config preflight

Interactive only (the grouped question cannot be exercised
non-interactively). Setup: fresh branch in the scratch repo, the task
extension installed as in J5, `.ai-workflow/config.yaml` present with `enabled: true`
but **no `tasks:` block at all**, and no local timer referenced in
`~/.claude/work/timers.json` for this session. Prompt: "Add a
`Sqrt(double x)` method to `Calculator`... Follow the NERV orchestrator
protocol injected in this session."

Expected: `run.started` is still emitted; with `ask_when_missing`
defaulting to `true` and only one provider installed, the dispatcher
returns `needs_input` with the task question, which Ikari relays verbatim.
Choosing "work without a task" leaves `task_ref` empty, so the branch
proposal omits `{prefix}-{id}` and commits carry no task suffix.
