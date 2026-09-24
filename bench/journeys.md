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

## J2: ratchet LIGHT to FULL (Phase 2)

Request touching backend and frontend, or a `critical_paths` entry. Expected:
classification FULL; in a Phase 1 build the orchestrator offers "proceed as
LIGHT with acknowledgement" or "stop", never a silent downgrade.

## J3: FULL path with MAGI (Phase 2), J4: audit and closure (Phase 3), J5:
task tracker and single config (Phase 4), J6: resume after interruption
(Phase 5)

To be written with their phases.
