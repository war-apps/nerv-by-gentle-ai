## ODD document and run folder

NERV tracks every run on gentle-ai's ODD (Organic Driven Development)
feature document, and keeps its own governance files in a run folder next
to it. OpenSpec is no longer used: there is no `openspec/` folder, no
proposal, design or spec-delta file, and no archive step.
Loaded with the core at the feature's creation, and again whenever an
actor writes the document.

### Layout

| Path | Holds | Committed |
|---|---|---|
| `odd/tasks/{feature}.md` | the ODD feature document: header, `## Specs`, `## Tasks`, `## Log` | yes, per `artifacts.commit` |
| `odd/{feature}/nerv/` | the run folder: `state.yaml`, `deliberation-log.md`, `task-events.yaml`, `run-summary.md`, `audit/` (`diff-round-N.patch`, `round-N.yaml`, `commits-round-N.txt`, `pass-<name>-round-N.json`) and the other governance files of `nerv-artifacts.md` | yes, with the feature, per `artifacts.commit` |
| `.nerv/locks/{feature}.lock` | the orchestrator lock (`## Orchestrator lock` in the core) | never: `.nerv/` is runtime only and ignores itself |

Every `nerv/...` locator in this skill, the shared contracts and the agent
files is relative to `odd/{feature}/`: `nerv/maya-report.md` is
`odd/{feature}/nerv/maya-report.md`. Launch briefs still carry it absolute
(`references/launch-discipline.md`).

**Engram.** The document's mirror is the ODD topic `odd/{feature}/tasks`
in the repo's own Engram project, saved by whoever wrote the document
(Ikari or Misato) with the full current document and its repository
path, then read back; the two writes are not atomic, and a failed mirror
is reported as pending, never claimed. The run folder's artifacts keep
their `nerv/{feature}/{artifact}` topics (`nerv-artifacts.md`).

### The document

Shape, in this fixed order (gentle-ai's ODD protocol):

1. **Header**, two or three lines: the objective, the branch, the delivery
   strategy (the cached PR strategy) and the test runner with the TDD mode
   (`### TDD mode and runner` below).
2. **`## Specs`**: numbered `S1`..`Sn`. Each quotes the user's exact
   strings, error messages, commands and examples verbatim, never
   summarized, and adds no requirement the user did not ask for; an
   assumption is marked as one. Then its authorized scope, acceptance
   criteria and applicable checks.
3. **`## Tasks`**: one table row per task, then one `- **T#:** ...` line
   per task with its description.

   ```markdown
   | ID | Specs | Route | Pilot | Depends on | Criticality | MAGI | Status | Commit |
   |---|---|---|---|---|---|---|---|---|
   | T1 | S1, S2 | delegated (FULL) | shinji | — | standard | approved, majority-2:1, round 1, frozen | done | a1b2c3d |
   ```

   `Route` is `inline` (DIRECT) or `delegated` with the tier and the
   trigger evidence. `Status` is `pending`, `implemented-pre-plan` (a
   ratcheted DIRECT or LIGHT diff), `in progress`, `done` or `killed`.
   DIRECT and LIGHT leave `Criticality` and `MAGI` as `—`.
4. **`## Log`**, last: `L1` is the user's original request verbatim; later
   user corrections are appended verbatim with their date, and analysis,
   decisions, evidence, progress and the next step are appended as
   `L2`..`Ln`. A requirement change appends its verbatim entry, rewrites
   only the affected `S#` and reopens only its linked task.

### Who writes what

Ikari's writes here are mechanical bookkeeping; content comes from the
role that owns it.

| Former artifact or role output | Now | Written by |
|---|---|---|
| the per-change folder | `odd/tasks/{feature}.md` plus `odd/{feature}/nerv/` | Ikari creates both |
| user request | header and `L1` verbatim; the request's rules seeded as `S1`..`Sn`, quotes only | Ikari, at the feature's creation |
| `proposal.md` (why, scope) | the `## Specs` scope and acceptance criteria | Misato (FULL); Ikari in DIRECT and LIGHT, from the request alone |
| `specs/{domain}/spec.md` (scenarios) | acceptance criteria under each `S#`, and the test plan's cases | Misato, from Ritsuko's test plan |
| `design.md` | one `## Log` entry, `design`, with `### Decisions` and `### New skills, scripts and commands` (a list, or `none`) | Misato |
| `tasks.md` | `## Tasks` rows and lines (`ID`, `Specs`, `Route`, `Pilot`, `Depends on`, `Status`) | Misato |
| `criticality.md` | `Criticality` column (detail stays in `nerv/criticality.md`) | Hyuga writes the file; Ikari copies the column |
| `votes.md` | `MAGI` column: result, rule, round, frozen (detail stays in `nerv/votes.md`) | Ikari |
| `exploration.md`, `exploration-light.md` | `nerv/` files, plus a one-line `## Log` pointer | Ikari writes Ritsuko's text verbatim |
| `nerv/test-plan.md` | one `## Log` entry, `test plan`: cases per `S#`, `### Corner-case questions`, `### Answers` | Ikari, verbatim from Ritsuko; Ikari fills `### Answers` with the user's words |
| plan gate, issue gate, rulings | one `## Log` entry each (rulings also in their `nerv/` file) | Ikari; Misato for rulings |
| Maya's verdict, the RDD receipts | one `## Log` evidence line each (detail in `nerv/maya-report.md` and the deliberation log) | Ikari |
| `status: done` in `tasks.md`, the archive | `Status` and `Commit` per task row, and a close entry in `## Log` | Ikari (Aoba records commits in FULL) |
| `state.yaml`, deliberation log, audit rounds, run summary, task events | `odd/{feature}/nerv/` | unchanged owners (`nerv-artifacts.md`) |

A MAGI-frozen task's row is never edited again by Misato; Ikari's
bookkeeping columns (`Criticality`, `MAGI`, `Status`, `Commit`) stay
writable. Hyuga never edits the document.

### TDD mode and runner

Resolved once per feature, before the first launch, and written into the
header (`Test runner: <command> (strict TDD)`); an explicit user choice
wins. Discover every project root from the repo root (the explicit
workspace membership when declared, otherwise the root and at most two
levels below it; skip `.git`, `node_modules`, `vendor`, `dist`, `build`,
`out`, `target`, `.cache`, `__pycache__`, `.venv`, `venv` and nested
repositories). The mode is `strict` only when the project set is non-empty
and one explicit workspace-level test command (an existing script or
target such as `go test ./...`, `npm test` or `dotnet test`, run from the
repo root) covers every in-scope project; otherwise it is `standard`, with
that command when there is one or `none`, and Ikari tells the user why.
Never guess a runner. This is gentle-ai's test-first policy: strict means
RED is observed before GREEN for every behavior change.

### Close

There is no archive. A feature closes when every task row carries its
`Status` and `Commit`, the `## Log` records the close (outcome, failed,
skipped or pending checks, next step), `state.yaml` has `closed_at`, and
the lock is deleted. The document and the run folder then reach git per
`artifacts.commit` (`delivery.md`); they stay in `odd/` for later runs and
reviews to read.

### Resume

gentle-ai's ODD resume order: `mem_context`, then `mem_search` scoped to
this project and feature (`odd/{feature}/tasks`, `nerv/{feature}`), then
`mem_get_observation` for the full document, then the file
`odd/tasks/{feature}.md` itself. Reconcile the two before continuing the
next unfinished task (`references/resume.md`).
