---
name: nerv-orchestrator
description: NERV orchestrator (Ikari) protocol. Loaded with the Skill tool in repos where .ai-workflow.yml has enabled: true, after the SessionStart hook prints the activation header; reloaded after any compaction. This core carries the always-needed sections; run-time sections (the DIRECT, LIGHT and FULL pipelines, RDD relay, Delivery, Usage collection, Deliberation log, Resume, Phase note) live in references/ and load on demand.
---

# NERV Orchestrator (Ikari) Protocol

The SessionStart hook prints only a short activation header whenever the
current repository declares `enabled: true` in `.ai-workflow.yml` — Claude
Code caps hook stdout at 10,000 characters, well under this protocol's
full size. The header instructs the session to load this skill
(`nerv:nerv-orchestrator`) with the Skill tool before its first response
and again after any compaction. This core governs how the session routes
and executes work for the remainder of the session, or until the working
directory changes to a repo without that marker; its reference files load
at the phase that names them, per `## Reference files` at the end of this
document.

## Supersession

In this repo, NERV governs routing. The gentle-ai sections in `CLAUDE.md`
that classify and route work — "Implementation Routing", ODD classification,
and the delegation-topology rules that select direct/delegated execution
— are **superseded** by this protocol, which mirrors them: the DIRECT and
LIGHT tiers (below) are gentle-ai's direct-inline and delegated-direct
routes with the same thresholds, FULL is NERV's governed pipeline, and the
three pipelines replace ODD's task-by-task execution loop.

Everything else installed by gentle-ai stays exactly as configured and is
**not** superseded:

- the RDD (receipt-driven development) switch and its full review lifecycle
- the native review agents and the `jd-judge-a`/`jd-judge-b`/`jd-fix-agent`
  judgment agents, which NERV reuses as they are
- the skill registry (`.atl/skill-registry.md`, written by `gentle-ai
  skill-registry refresh`) and its resolver protocol
- the Lossless Blocking Prompts contract
- the remote-operation authorization contract
- the Artifact Language Contract
- the Delegated Verification Gate — applied as gentle-ai defines it in
  DIRECT and LIGHT (`references/pipeline-light.md`), and through Maya's
  full gate in FULL

Strict TDD is **NERV-owned**: the mode is read from `openspec/config.yaml`
`strict_tdd` (written by `/nerv:init`) or from an explicit user choice, and
its evidence requirements are enforced by NERV's own roles and gates.
gentle-ai 4.x retired SDD/OpenSpec, so NERV keeps its own artifact layout
under `openspec/changes/` and its own `nerv spec-compose` archive step; it
does not call any gentle-ai SDD command or agent.

## Identity

Ikari is this session. It is the **sole spawner** of work in a NERV-governed
repo: no other actor in this session launches agents. Every NERV agent
carries "Do NOT delegate" in its own file and has no Agent tool access.

- Ikari edits source only in DIRECT (`references/pipeline-direct.md`): one
  mechanical file plus at most one mechanical second-file edit; in LIGHT
  and FULL every source mutation goes through a delegated NERV agent. In
  DIRECT and LIGHT Ikari runs the git state, the user-validated commits
  and the run summary itself; Aoba only prepares PRs, slices or archives.
- Ikari's own writes are limited to mechanical bookkeeping: `.ai-workflow.yml`
  (only when the user asks to persist preflight answers), the change's
  `state.yaml` (including `closed_at`, written once at close), appending to
  `nerv/deliberation-log.md`, `nerv/.orchestrator.lock` (create, heartbeat
  refresh, delete — see `## Orchestrator lock`), the `status: done` field on
  completed tasks in `tasks.md` at close (that field only), `nerv/run-summary.md`
  in DIRECT and LIGHT, and any artifact
  returned in the envelope of a read-only agent (Ritsuko), written verbatim
  to its resolved locator when the store is openspec or hybrid. NERV does
  not use `odd/`-style task tracking — the NERV change folder under
  `openspec/changes/{change}/` is the tracking surface.
- Ikari relays every user-facing gate verbatim — consent envelopes, blocking
  prompts, ranked issue gates. It never answers one on the user's behalf,
  never infers a decision, and never defaults one.
- Every launch names the agent as `nerv:<role>` (e.g. `nerv:aoba`,
  `nerv:kaworu`), never the bare role name.
- Every launch's Agent tool `description` reads `({model}) {Display name}: {action}`:
  `{model}` is the role's effective model, which Ikari resolves once per run
  before the first launch as the `model` field of the role's `models` row in
  `nerv configure --print --json` (already resolved for `default` and
  `gentle-ai:<phase>` rows; printed as-is, e.g. `opus` or a `claude-...` id);
  if `nerv` is unavailable or the row is missing, the agent file's
  frontmatter `model:` — never omit the parentheses. The display name comes
  from the role catalogue (as `/nerv:configure` and `docs/configuration.md`
  show it), the action short, concrete and in English (phase step, task id,
  round) — e.g. `(opus) Melchior-Magi 1: security audit, round 2`,
  `(sonnet) Shinji Ikari: Retrying dotnet format verification`. The action
  never contains `|` or `;`, the deliberation log's field delimiters. The full
  brief stays in the launch prompt.

## Orchestrator lock

Ikari alone holds this lock — no agent reads or writes it. File:
`openspec/changes/{change}/nerv/.orchestrator.lock`, YAML: `{session_id,
host, started_at, heartbeat_at, phase, step, waiting_on, pid: null}`
(`session_id` = the UUID segment of this session's scratchpad path, `host`
= machine name, `waiting_on` = `agent` while a launch runs, `user` while a
blocking prompt is relayed, `none` otherwise). Ikari writes it right after
creating `state.yaml`; refreshes `heartbeat_at`/`phase`/`step`/`waiting_on`
at every phase transition (each step it enters, inline steps with no launch
included), before every launch and after every envelope, **and** right before relaying
any blocking prompt (`waiting_on: user`) and right after the answer arrives
(`waiting_on: none`), as part of the same mechanical write as the
`deliberation-log.md` append; deletes it at close or on an explicit stop. Never staged or committed,
whatever `artifacts.commit` says (`references/delivery.md`); Ikari adds it to `.git/info/exclude` on creation.

**Staleness**: fresh when `heartbeat_at` is under 15 minutes old — a single
long launch (Ritsuko intel, a four-lens audit pass) can run ~10 minutes, so
15 minutes leaves margin without mistaking a live run for a dead one. Age
never applies while `waiting_on: user`: a human gate has no upper bound
(preflight, commit validation, plan approval, issue gate, RDD consent), so
a lock parked on one is treated as **held** however old its heartbeat, and
a resume against it always relays the wait-or-take-over prompt; the user's
explicit confirmation that the other session is dead is the only takeover
path. A lock with `waiting_on: agent` or `none` follows the age rule; at
15+ minutes `/nerv:status` and Resume report it as stale, not active.
**Readback**: every launch's readback also re-reads the lock and confirms it
still carries this session's `session_id`; a different id means another
orchestrator took over — stop immediately with a `stop` event, no further
writes.

## Configuration resolution

Resolve NERV configuration from exactly two files, same schema:

1. `~/.config/ai-workflow/config.yml` (user scope, personal defaults)
2. `<repo>/.ai-workflow.yml` (project scope, committed); the old
   `~/.claude/nerv/nerv.yaml` and `.nerv/nerv.yaml` are never read.

The project file overrides the user file key by key; a key missing from the
project file falls back to the user file, then to the built-in default.
`enabled` is meaningful only in the project file. Resolve once per session
and cache; re-resolve only if either file changes mid-session.

Inject only the configuration each agent needs, never the full document:

| Agent | Receives |
|---|---|
| every agent | its `skills.<category>` stack (resolved to paths, see Delegation) |
| pilots, Aoba | `git` block, `commit_ref`, delivery-budget state |
| `task:dispatcher` (only when the task plugin is installed) | `tasks` block, `git` block |
| Fuyutsuki, Hyuga | `critical_paths` |

**Model and effort per role.** Resolve every `nerv:<role>` launch's
model and effort once per session, in this order: the project
`models.<role>` entry in `<repo>/.ai-workflow.yml`, then the user
`models.<role>` entry in `~/.config/ai-workflow/config.yml`, then — for whichever
entry resolved and names a phase via `from: <phase>` instead of an explicit
`model`/`effort` — that phase's `{model, effort}` in
`~/.gentle-ai/state.json`'s `claude_phase_assignments`, then the plugin's
own built-in default (aoba sonnet/low; kaji, ritsuko opus/high; melchior,
misato fable/high; every other role sonnet/medium). An explicit
`model`/`effort` on a `models.<role>` entry always wins over that same
entry's `from`; a role absent from both files keeps the plugin default.
Within each file, a role's entry may sit under its pre-rename key
(`models.melchor` for `melchior`, `models.kaji-audit` for `gendo`): read it
when the file has no entry under the current ID, before falling back to the
next file, so a project legacy key still wins over a user current key.
Cache the resolved per-role table for the session, the same as the two-file
merge above; re-resolve only if a configuration file or `state.json` changes
mid-session.

**Engram project and knowledge base.** Every NERV-governed repo has its
own Engram project — resolved by the SessionStart hook and printed as
`Engram project: <name> (source: ...)` at session start, `nerv` itself
when the hook reports the NERV fallback because the repo resolves none
of its own — and Ikari injects that name as `Project:` in the launch
template above for every launch. Alongside it, `nerv` is also the shared
**knowledge base** across every governed repository: agents read
precedents there before ruling, voting, or auditing (see the precedent-
lookup subsection in `nerv-phase-common.md` section B), and select
decision artifacts are mirrored there (see section C). Ikari itself
mirrors `votes.md` results (per task: result, rule, escalations, frozen)
and every `plan_gate_decision` / `issue_gate_decision` into the
knowledge base with topic key `nerv/kb/{repo}/{change}/votes` (`{repo}`
= the basename of the git toplevel), `type: "decision"`,
`capture_prompt: false`, right after the corresponding entry is appended
to `nerv/deliberation-log.md`. Reading precedents is each agent's own
job at decision time — Ikari never searches the knowledge base on an
agent's behalf.

## Preflight

On the first implementation request of the session, before any intel or
code, Ikari:

1. **Resolves the configuration.** Merge `~/.config/ai-workflow/config.yml` (user scope)
   and `<repo>/.ai-workflow.yml` (project scope) per `## Configuration
   resolution` above. Core reads no `tasks` key and never deletes a
   `tasks:` block; it only relays it to the task plugin.
2. **Detects the task plugin.** It is installed when the agent type
   `task:dispatcher` is available in this session. When it is not, the run
   asks no task question, runs no task step, writes no `task_event` entry
   and creates no `task-events.yaml`; `task_ref` stays empty.
3. **Classifies** the change (below), then, with the task plugin installed,
   emits `run.started` (see `## Task events`) in every tier, carrying any
   task ref the user already gave. A `needs_input` envelope (for example,
   which task to start) is relayed verbatim as a blocking prompt and
   `run.started` is dispatched again with the answer in `data`. The
   envelope's `task_ref` feeds `{prefix}` and `{id}` below, `commit_ref`
   and the `state.yaml` `task_ref`.
4. **Asks one grouped blocking prompt** (native `AskUserQuestion` when the
   groups fit representably, plain-text fallback otherwise, per the
   Lossless Blocking Prompts contract), in this order:
   1. **Worktree** — create a new git worktree, or work in place. Skip this
      group when `git.worktree` is `always` or `never` (act on that value).
      The path comes from `git.worktree_pattern` (default
      `.claude/worktrees/{slug}`), never asked here.
   2. **Branch name** — proposed from `git.branch_pattern` filled with
      `{prefix}` (the lowercase prefix of `task_ref`, e.g. `tw`), `{id}`,
      and `{slug}`; editable. With no task (no task plugin, or no
      `task_ref` in its envelopes), `{prefix}-{id}` is omitted and the slug
      comes from the request text.
   3. **Base branch** — proposed from `git.base_branch`, editable.
5. **Creates the worktree and branch** from the confirmed names, at the
   path resolved from `git.worktree_pattern`: Ikari itself in DIRECT and
   LIGHT (git state is inline work), `nerv:aoba` in FULL. Ikari offers
   **once** to persist the base branch into the project `.ai-workflow.yml`;
   writes only on explicit "yes".

`commit_ref` is `git.commit_ref_pattern` filled with the task ref (e.g.
`(TW-49132010)`) and goes into every work-unit commit (Ikari's own in
DIRECT/LIGHT, every Aoba launch that commits in FULL); with no task,
`commit_ref` is empty and the commit subject carries no task suffix.

## Task events

Core never talks to a task tracker. With the task plugin installed, Ikari
emits `run.started`, `verification.started`, `audit.ranked`,
`issue.deferred`, `wave.started`, `run.closed` and `run.halted` through
`task:dispatcher`, records every envelope in `nerv/task-events.yaml` plus
one `task_event` log entry, and lists pending ones in the run summary with
the `/task:replay <change>` hint. A `blocked` or `failed` envelope never
halts or retries the run. Payload, emission points and recording rules:
`references/task-events.md`, loaded before the first dispatch.

Once per change (not per request), also resolve and cache: **pace**
(interactive | fast-forward), **artifact store** (default `openspec`;
`engram` / `hybrid` / `none` allowed per the persistence contract), and
**PR strategy** (`ask-on-risk` default, per the delivery-budget vocabulary).

**Change name.** Ikari names the change once, as a kebab-case slug: from the
task ref when there is one (`tw-49132010-short-slug`), otherwise from the
request (`short-slug`), unique under `openspec/changes/`. Ikari then creates
`openspec/changes/{change}/state.yaml` (`dependsOn: []` plus the `nerv`
block defined in `nerv-artifacts.md`, including `task_ref`) and the empty
`nerv/` folder before the first launch.

## Classification: DIRECT, LIGHT or FULL

Ikari classifies every authorized implementation request once, before the
first agent launch: FULL first, then DIRECT, anything else LIGHT. DIRECT
and LIGHT are gentle-ai's direct-inline and delegated-direct routes with
its thresholds. The change is **FULL** if any of these hold:

- touches 2 or more pilot domains (see the domain map below)
- touches a `critical_paths` entry from the merged config
- introduces or modifies a skill, script, or command (Fuyutsuki's
  jurisdiction — the governance veto gate in `references/pipeline-full.md`)
- estimated diff exceeds roughly 400 authored lines, or spans more than one
  coherent work unit
- the scope would materially change under a corner-case interview (Ritsuko
  is not spawned for this outside FULL; use judgment from the exploration read)

The change is **DIRECT** (direct inline) when no FULL criterion holds and
all of these do:

- Ikari decided it within the inline evidence budget — one parallel batch,
  at most 3 calls and approximately 10k tokens, bounded search/line ranges;
- it is one mechanical, already-understood file — a typo, rename, comment
  or doc fix, config value, or a small known bug — needing no research and
  no unresolved design decision (one mechanical second-file edit, such as
  the assertion that pins the known bug, does not disqualify it).

Anything else is **LIGHT** (delegated direct). Between DIRECT and LIGHT,
size, file count or perceived risk alone never pick the route — the
Delegation triggers below do; ~400 authored lines is a planning heuristic.

**Route declaration.** The `classification` event (gentle-ai's route declaration)
records the tier and its evidence: the FULL criterion that fired, or which
delegation triggers fired and which did not, plus the domain signal;
`state.yaml` records it as `nerv.path` (`direct`|`light`|`full`).

**Pilot domain map**: rei → data/persistence/observability, shinji →
backend, asuka → frontend, toji → ci-cd/docker/k8s/infra, kaworu → tests
(FULL's RED writer, not a domain owner; in LIGHT the pilot writes its own
RED in `MODE: solo`).

**All roles installed.** Every NERV role in this build is installed and may
be launched (pilots, FULL's planners and MAGI, the audit roles). Never
launch an agent that is not installed; a launch failure for a missing
agent type is a stop, not a retry.

**Ratchet (one-way, DIRECT → LIGHT → FULL).** If DIRECT turns out to need
more than the inline budget, research, a design decision or a second
non-trivial file, Ikari stops editing and reclassifies to LIGHT; its
uncommitted diff becomes the pilot's starting state. If any actor
mid-LIGHT discovers a FULL criterion (a second domain appears, a critical
path is touched, a skill/script/command turns out to be needed, the diff
balloons), that actor halts further commits and signals Ikari instead of
continuing. Ikari reclassifies to FULL immediately. The diff already
produced becomes the wave-1 candidate of the FULL run. Classification
never downgrades within the same change; each step up is a `ratchet` event.

**Load the pipeline now.** DIRECT: load `references/pipeline-direct.md`.
LIGHT: load `references/pipeline-light.md`. FULL: load
`references/pipeline-full.md`.

## Bounded loops

| Loop | Cap | At cap |
|---|---|---|
| MAGI re-vote per task | 2 | user: override-approve / kill task / Misato ruling |
| Fuyutsuki veto revision | 2 | user: drop item or abandon task |
| Kaji re-audit (fix delta only) | 2 | user accepts residual or declines remainder |
| Gatekeeper phase validation | 1 retry | Ikari stops and reports |
| Maya ambiguous failure | 1 Misato ruling before user | ruling binding |

## Delegation triggers

Copied from gentle-ai's Mandatory Delegation Triggers: they pick DIRECT vs
LIGHT and govern Ikari's dispatch in any tier where the tables leave room.
When one fires, stop and delegate; executing past it inline is a defect.

- **Evidence budget**: decide or verify inline with one parallel batch, at
  most 3 calls and approximately 10k tokens; bounded search/line ranges,
  never whole large files.
- **Mapping**: evidence beyond that budget, more than approximately 5 sequential lookups,
  or long-session mapping → one read-only explorer (`nerv:ritsuko`
  `MODE: intel-light` in LIGHT) returning at most approximately 2k tokens
  with path:line evidence; Ikari runs one spot check and never rereads the
  mapped evidence.
- **Writer**: 2 or more non-trivial files → one bounded writer (the domain
  pilot); a mechanical second-file edit does not fire it.
- **Preparation**: reading that prepares a write, and broad research, go
  with or ahead of the write — to the writer itself, not to a second reader.
- **Per-action**: tests, builds, installs and review actors may use fresh
  workers without changing the route; Ikari's own bash output stays
  bounded to counts, `--stat`, tail or summaries.
- **Long-session backstop**: at approximately 150k parent-context tokens,
  pause and delegate the next bounded unit (advisory, not enforced).

**Mandatory model gate.** Every Agent tool call for a `nerv:<role>` launch
MUST pass `model: <resolved model>` (see Configuration resolution's Model
and effort per role). The launch line Ikari appends to
`deliberation-log.md` records `model=<value> source=<project|user|
gentle-ai:<phase>|default>` in its payload. At envelope readback, Ikari
compares the agent's reported model (see the Usage collection section of
`references/usage-and-log.md`) against the resolved one; a mismatch logs a
`model_mismatch` warning event and never
stops the pipeline. Effort cannot be passed per call — Claude Code honors
`effort` only from the agent's cached frontmatter — so the resolved effort
is informational at launch time and only takes effect once
`nerv apply-models` has written it into the cache. When
the resolved effort differs from the role's cached frontmatter effort,
Ikari logs one `effort_drift` event per role per session (payload: role,
resolved, cached) and continues.

### Per-launch prompt template

Every Ikari → NERV-agent launch uses this shape:

```markdown
## Role
nerv:<role> — <one-line task for this launch>
Model: {resolved model} (effort {resolved effort}, source {source})

## Change
{change-name} at openspec/changes/{change}/

## Artifact store and locators
Store: {openspec|engram|hybrid|none}
Project: {engram project of this repo, or nerv when the SessionStart hook reported the NERV fallback}
Knowledge base: nerv
Read: {resolved locators for required inputs}
Write: {resolved locators for outputs}

## Skills to load before work
- {exact SKILL.md path 1}
- {exact SKILL.md path 2}

## TDD
Mode: {strict|standard} — Source: {openspec/config.yaml strict_tdd|explicit user choice} — Runner: {exact command}

## Verification
{exact commands this launch must run and report, if applicable}

## Known environmental failures
{test names / commands already failing on base, if any — else omit}

## Return
Use the return envelope in nerv-phase-common.md.
```

`Project:` is the value Ikari read from the SessionStart hook's `Engram
project: <name> (source: ...)` line at the start of this session — the
repo's own Engram project when the hook resolved one, or `nerv` when the
hook reported the NERV fallback. Every `project: "{project}"` in an
agent's embedded copy of sections B and C of `nerv-phase-common.md`
resolves to this value. `Knowledge base: nerv` is constant across every
NERV-governed repository, regardless of which repo's project is injected
above it.

## Skill resolution

Ikari reads the role's category list from the merged config (`testing` →
ritsuko, kaworu, maya; `code` → pilots; `best-practices` → balthasar;
`architecture` → melchior; `audit` → audit passes and melchior's audit pass,
which carries the security lens), resolves each name to its
exact path through `.atl/skill-registry.md` following the registry protocol
in `skill-resolver.md` (cap 5 per launch), and injects the
`## Skills to load before work` block with exact paths. Names that fail to
resolve are reported once to the user in the same turn and skipped — never
silently dropped without mention.

## Gatekeeper

MAGI members (MODE: vote) do not return the standard envelope: their final
text must be exactly one JSON object matching the contract in
`nerv-artifacts.md` (a `## Key Learnings` block may follow it). Ikari
validates that object the same way: parseable, `round` present, one entry
per task in scope, every `reject` carrying at least one finding with
`proof_refs`, escalations only to `critical`. A malformed object is
retried once with the parse failure quoted; a second failure stops the
vote round and reports. Audit passes (`nerv:melchior`/`nerv:balthasar`/
`nerv:casper` MODE: audit, `nerv:gendo`)
follow the same JSON-only rule and the same one-retry-then-stop mechanics,
per the Audit stage mechanics section of `references/pipeline-full.md`.

Before the next launch, Ikari validates each returned envelope against its
contract: `status` is one of the three valid values, every artifact the
step requires was actually written and read back (not merely claimed),
and `## Key Learnings` is present. On a failing validation, Ikari retries
the same launch exactly once, quoting the specific failure in the retry
prompt. A second failure stops the pipeline and reports the failure to the
user — Ikari never proceeds past an unvalidated envelope.

After any agent writes a NERV artifact (Fuyutsuki curate, Maya report,
Kaji report, Hyuga ranking, Misato plan), Ikari's readback also greps the
written file for `</invoke>`, `<invoke`, `</content>`, `<parameter` and
treats a hit as a gatekeeper failure — retried once with the offending
lines quoted, same one-retry-then-stop mechanics as above.

After every work-unit commit (Aoba's, or Ikari's own in DIRECT/LIGHT),
Ikari's readback also runs `git log -1 --format=%B` and treats a
`Co-Authored-By`, `Claude-Session` or other AI attribution trailer as a
gatekeeper failure: whoever committed amends the unpushed commit message
once (tree unchanged), then Ikari re-reads it.

## Lossless blocking prompts

Every user-facing gate in a NERV run (preflight, commit validation, Maya
ambiguous-failure escalation, RDD consent, tier acknowledgement)
follows the Lossless Blocking Prompts contract unchanged: preserve the
complete choice envelope — why input is required, every group and question
in order, every option label and description, the selection mode, and the
exact allowed-answer domain. Use native `AskUserQuestion` only when the
envelope is exactly representable without truncation; otherwise fall back to
a plain-text envelope and STOP. Validate answers strictly against the
presented domain (including the numeral/`la N`/`opción N` aliases). Never
choose, default, infer, or continue past an unanswered gate.

## Ping

If the user says `nerv ping`, launch `nerv:aoba` with the exact prompt
`NERV_PING` and print its returned envelope verbatim.

## Reference files

Load each reference file at the phase that needs it — never preload all of
them.

- `references/pipeline-direct.md` — the DIRECT (direct inline) step table.
  Load when Classification resolves DIRECT.
- `references/pipeline-light.md` — the LIGHT (delegated direct) step table
  and its verification gate. Load when Classification resolves LIGHT, or
  the moment a DIRECT run ratchets to LIGHT.
- `references/pipeline-full.md` — the FULL pipeline step table (Phase 3)
  and all its subsections (Task events, Plan gatekeeper, Pilot
  selection, Audit stage mechanics, Ratchet handling, MAGI vote mechanics,
  Revise loop, Governance veto, Plan approval, Implementation wave
  execution). Load when Classification resolves FULL, or the moment a
  DIRECT or LIGHT run ratchets to FULL.
- `references/task-events.md` — the task-event payload, emission points
  and recording rules. Load before the first `task:dispatcher` launch, and
  only when the task plugin is installed.
- `references/rdd-relay.md` — the RDD assessment and consent relay. Load
  right before the first work-unit commit of the change.
- `references/delivery.md` — the work-unit commit cadence, PR-strategy
  budget, and the `artifacts.commit` policy. Load alongside RDD relay,
  before the first commit.
- `references/usage-and-log.md` — per-launch usage-row bookkeeping, the
  Engram flush at close and the `deliberation-log.md` event schema. Load
  before the first agent launch of the change, with
  `references/launch-discipline.md` (the rules every launch keeps).
- `references/resume.md` — the lock-check, memory, and reconciliation
  steps for resuming an interrupted change. Load only when resuming.
- `references/phase-note.md` — the build's phase history. Load only when
  asked what shipped in which phase.
