# NERV

NERV is a Claude Code plugin implementing an Evangelion-named multi-agent
governance workflow: Ikari orchestrates, Fuyutsuki holds governance veto,
Misato authors the plan, MAGI (Balthasar / Melchor / Casper) vote per task,
pilots (Rei / Shinji / Asuka / Toji / Kaworu) implement, Kaji compiles a
ranked, multi-pass audit, Maya gates quality, Hyuga owns criticality/waves/
task-tracking, and Aoba handles git operations and the run summary.

NERV brings governance that a plain implementation loop lacks: a per-task
MAGI vote gated by criticality, a governance veto on new skills/scripts/
commands, a mandatory quality gate before any audit, a multi-pass audit
compiler with a ranked user issue gate, an append-only deliberation log, and
a run summary reporting tokens, time, and model per agent.

## Requirements

- **gentle-ai 3.x** — major version 3 is required; minor and patch are free.
  Tested against 3.7.0. 4.x is untested and not supported until NERV's
  contracts (the reuse map below) are re-verified against it.
- Claude Code with plugin marketplaces support (2.1+).
- git.
- bash available to hooks (Git Bash on Windows).
- PowerShell 7, for the installer (`tools/install.ps1`).

Tested against: gentle-ai 3.7.0

## Relation to gentle-ai

NERV is an **overlay**, not a fork. It reuses gentle-ai's native engine and
contracts unchanged — the SDD artifact pipeline, RDD (receipt-driven review),
the skill registry and resolver, strict TDD, delivery budgeting with chained
PRs, and the lossless blocking-prompt contract — and expresses NERV's own
governance on top. NERV never modifies any gentle-ai file: nothing is ever
written under `~/.claude/agents` or `~/.claude/skills`, so `gentle-ai sync`
cannot see or touch it.

## How NERV integrates with gentle-ai

### Layering

```mermaid
flowchart TB
    CC[Claude Code session]

    subgraph GA["gentle-ai"]
        GACfg["CLAUDE.md sections"]
        GAHooks["hooks"]
        GABin["gentle-ai binary"]
        GAAgents["~/.claude/agents + skills"]
    end

    subgraph NERVP["NERV plugin (~/.claude/plugins/cache/nerv/...)"]
        NHook["SessionStart hook"]
        NAgents["agents nerv:*"]
        NSkills["skills"]
        NCommands["commands"]
    end

    subgraph Repo["repo"]
        NConfig[".nerv/nerv.yaml"]
        NChange["openspec/changes/{change}/nerv/"]
    end

    Sync["gentle-ai sync"]

    CC --> GA
    GA --> NERVP
    NERVP --> Repo
    Sync -.writes only here.-> GA
    NERVP -.reads only.-> GA
```

NERV sits strictly above gentle-ai and below the repo it governs: Claude Code
loads gentle-ai first (`CLAUDE.md`, its hooks, the `gentle-ai` binary, and
the agents/skills it writes under `~/.claude`), NERV's plugin cache layers
its own hook, agents, skills, and commands on top, and the repo carries only
NERV's own state (`.nerv/nerv.yaml`, the `nerv/` subfolder of each change).
`gentle-ai sync` never sees NERV's files because NERV never writes into the
gentle-ai box, and NERV never writes into it either — it only reads from it.

### Activation

```mermaid
sequenceDiagram
    participant CC as Claude Code
    participant Hook as SessionStart hook
    participant GA as gentle-ai review stop-hook
    participant NS as nerv-session-start.sh
    participant Cfg as .nerv/nerv.yaml
    participant Skill as nerv-orchestrator/SKILL.md
    participant Sess as Session

    CC->>Hook: SessionStart event
    Hook->>GA: run gentle-ai review stop-hook
    Hook->>NS: run nerv-session-start.sh
    NS->>Cfg: check enabled: true
    alt marker present
        NS->>Skill: read SKILL.md
        NS->>Sess: inject protocol verbatim
        Sess->>Sess: session becomes Ikari
    else marker absent
        NS->>Sess: inject nothing
        Sess->>Sess: gentle-ai ODD routing stays in charge
    end
```

Every session start runs gentle-ai's own `review stop-hook` first, then
NERV's `nerv-session-start.sh`. The NERV hook checks `.nerv/nerv.yaml` for
`enabled: true`; only then does it read and inject
`nerv-orchestrator/SKILL.md` verbatim, which is the moment the session
becomes Ikari. Without the marker the hook injects nothing at all, and
gentle-ai's own ODD Implementation Routing stays in charge of the session.

### Reuse map

```mermaid
flowchart LR
    subgraph Reused["Reused unchanged"]
        R1["sdd-init"]
        R2["skill-registry refresh"]
        R3["sdd-status / sdd-continue"]
        R4["sdd-archive-compose"]
        R5["review assess/status/start/mode + lenses + refuter"]
        R6["work-unit-commits"]
        R7["chained-pr"]
        R8["branch-pr"]
        R9["skill-resolver"]
        R10["persistence contract"]
        R11["lossless blocking prompts"]
    end

    subgraph Superseded["Superseded in NERV repos"]
        S1["ODD Implementation Routing / delegation topology"]
    end

    subgraph NotUsed["Not used"]
        N1["sdd-propose"]
        N2["sdd-spec"]
        N3["sdd-design"]
        N4["sdd-tasks"]
        N5["sdd-apply"]
        N6["sdd-verify"]
    end

    NotUsed -.NERV roles author those artifacts instead.-> Superseded
```

NERV calls a large slice of gentle-ai's native engine completely unchanged
(SDD's read-only/mechanical steps, the whole RDD lifecycle, the delivery and
skill-resolution skills, the persistence and lossless-prompt contracts). It
supersedes exactly one thing — gentle-ai's ODD Implementation Routing — with
its own LIGHT/FULL classification and pipelines, only in repos carrying the
`.nerv/nerv.yaml` marker. It never touches the authoring half of SDD
(`sdd-propose` through `sdd-verify`); NERV's own roles (Misato, Ritsuko, the
MAGI, the pilots) author the equivalent artifacts instead.

### LIGHT pipeline

```mermaid
flowchart LR
    A["Preflight"] --> B["Classify: LIGHT"]
    B --> C["nerv:ritsuko intel-light"]
    C --> D["nerv:kaworu RED"]
    D --> E["nerv:aoba commit — user validates"]
    E --> F["pilot GREEN/TRIANGULATE/REFACTOR"]
    F --> G["nerv:maya reduced gate"]
    G --> H["nerv:aoba commit — user validates"]
    H --> I["RDD hook"]
    I --> J["Run summary"]
    J --> K["Close"]
```

LIGHT is the small-change path: after preflight and classification, an
optional Ritsuko micro-intel primes the work, Kaworu writes the failing RED
test, a domain-matched pilot makes it pass, Maya runs a reduced gate scoped
to the touched files only, and Aoba commits twice — once for RED, once
after the gate — each commit validated by the user before it happens. The
RDD hook, run summary, and close steps are shared with FULL.

### FULL pipeline

```mermaid
flowchart TB
    A["Intel (ritsuko)"] --> B["Spec + test plan (ritsuko)"]
    B --> C["Corner-case questions [user HARD gate]"]
    C --> D["Plan (misato)"]
    D --> E["Criticality (hyuga)"]
    E --> F["MAGI vote: balthasar + melchor + casper — parallel, blind"]
    F --> G{"Rejected tasks?"}
    G -- yes --> H["Misato revise (cap 2)"]
    H --> F
    G -- no/resolved --> I["Governance veto (fuyutsuki, cap 2)"]
    I --> J["Plan approval [user HARD gate]"]
    J --> K["Waves (hyuga)"]
    K --> L["Baseline (maya)"]
    L --> M["Wave: kaworu RED"]
    M --> N["aoba commit — user validates"]
    N --> O["pilot GREEN/TRIANGULATE/REFACTOR"]
    O --> P["aoba commit — user validates"]
    P --> Q["RDD hook"]
    Q --> M
    Q --> R["Maya full gate a-d"]
    R --> S["Audit placeholder — Phase 3, not shipped"]
    S --> T["Run summary (aoba)"]
    T --> U["Close"]
```

FULL adds MAGI governance on top of the same LIGHT primitives (RED/GREEN/
REFACTOR, Aoba commits, RDD hook). Ritsuko produces intel and a spec/test
plan gated by a user corner-case interview; Misato's plan goes through
criticality, a blind parallel MAGI vote (with a capped revise loop on
rejection), and Fuyutsuki's governance veto before a single whole-plan user
approval gate. Hyuga's waves then drive per-task implementation cycles
identical to LIGHT's, Maya runs the full a-d gate, and — since Kaji's audit
is not shipped yet — the run goes straight from the audit placeholder
statement to the run summary and close.

### RDD per commit

```mermaid
sequenceDiagram
    participant Aoba as nerv:aoba
    participant Ikari as Ikari
    participant GA as gentle-ai review
    participant User as User
    participant Lenses as Native lenses (risk/resilience/readability/reliability)
    participant Pilot as NERV pilot

    Aoba->>Ikari: work-unit commit
    Ikari->>GA: review assess --committed-only
    GA-->>Ikari: review_due?
    alt review_due = true
        Ikari->>GA: STATUS
        GA-->>Ikari: START
        Ikari->>User: relay consent envelope verbatim
        alt granted
            User-->>Ikari: granted
            Ikari->>Lenses: run native lenses
            Lenses-->>Ikari: correction_required?
            opt correction_required
                Ikari->>Pilot: bounded correction
                Pilot-->>Ikari: corrected candidate
            end
            Ikari->>GA: validation
            GA-->>Ikari: acknowledge
            Ikari->>Ikari: log rdd_receipt
        else declined
            User-->>Ikari: declined
            Ikari->>Ikari: continue under ordinary policy
        end
    else review_due = false
        Ikari->>Ikari: record reason, continue
    end
    Note over Ikari: Ikari never enables/disables the RDD switch
```

After every Aoba work-unit commit, Ikari assesses that commit against the
last reviewed boundary. When review is due, Ikari relays gentle-ai's native
consent envelope to the user losslessly and verbatim — never deciding on
their behalf. On a grant, the native lenses run, a bounded NERV-pilot
correction applies only if one is required, and an acknowledged receipt is
logged as `rdd_receipt`; on a decline, the run continues under ordinary
repository policy. Ikari itself never enables or disables the RDD switch.

## Install

1. `git clone https://github.com/war-apps/nerv-gentle-ai.git` (the folder path is registered as a local plugin marketplace, so keep the clone where it will stay).
2. `pwsh tools/install.ps1`
3. Restart Claude Code.
4. In any repo where you want NERV active, create `.nerv/nerv.yaml` with:

   ```yaml
   enabled: true
   ```

Run `pwsh tools/install.ps1 -Uninstall` to remove the marketplace and
plugin registration again.

### Updating after local changes

Claude Code snapshots a directory marketplace from the repository's
**committed HEAD** into `~/.claude/plugins/cache/nerv/nerv/<version>/`
(it records the git commit SHA in `installed_plugins.json`). Uncommitted
edits are invisible to sessions. After changing plugin files:

1. Commit the change.
2. Refresh the cache. `claude plugin update nerv@nerv` only re-snapshots
   when `version` in `plugin/.claude-plugin/plugin.json` changed; with the
   same version run `claude plugin uninstall nerv@nerv` followed by
   `claude plugin install nerv@nerv` (settings.json keeps `nerv@nerv`
   enabled, so nothing else changes).
3. Restart Claude Code.

## Phase 0 status

This is the Phase 0 slice: the smallest proof that the overlay mechanism
works. It ships one agent (`aoba`), one command (`/nerv:status`), one
SessionStart hook that injects the NERV orchestrator protocol only in repos
where `.nerv/nerv.yaml` has `enabled: true`, an installer, and this README.
The full Ikari protocol (classification, MAGI vote, audit, waves) is not
implemented yet — a NERV-enabled repo currently falls back to gentle-ai's
ordinary organic flow for actual work.

## Roadmap

- **Phase 1 (done)** — LIGHT path end to end: the Ikari protocol
  (classification, delegation triggers, RDD relay, usage collection),
  `ritsuko`, `shinji`, `kaworu`, `maya`, and `/nerv:init`.
- **Phase 2 (in progress)** — FULL path with MAGI: `misato`, `hyuga`,
  `balthasar`, `melchor`, `casper`, `fuyutsuki`, the vote/veto/waves
  artifacts, and the plan-approval HARD gate.
- **Phase 3** — Audit and closure: `kaji`, `kaji-security`, `kaji-coverage`,
  the frozen-patch audit passes, ranked issue gate, fix routing, and
  archive.
- **Phase 4** — Task-tracking layer (Hyuga) and single config file: the
  provider-agnostic task port, the Teamwork adapter, and the preflight
  question for task/worktree/branch/base.
- **Phase 5** — Hardening: the full `bench/journeys.md` suite, README
  parity, and the resume protocol.

## Roles

| Role | One-line responsibility |
|---|---|
| `fuyutsuki` | Governance veto ruling on new skills/scripts/commands; curates the deliberation log. |
| `ritsuko` | Intel, spec + test plan, and docs — three spawns per run. |
| `misato` | Authors the proposal, design, and tasks; issues binding rulings. |
| `balthasar` | MAGI vote member (per-task vote and audit passes). |
| `melchor` | MAGI vote member — the strong-model side of the MAGI asymmetry. |
| `casper` | MAGI vote member; also runs in AUDIT mode. |
| `rei` | Pilot — implements assigned tasks within a wave. |
| `shinji` | Pilot — implements assigned tasks within a wave. |
| `asuka` | Pilot — implements assigned tasks within a wave. |
| `toji` | Pilot — implements assigned tasks within a wave. |
| `kaworu` | Pilot — commits the failing (RED) test before the pilot's GREEN implementation. |
| `kaji` | Compiles and dedupes the multi-pass audit into `audit-report.md`. |
| `kaji-security` | Audit pass focused on security. |
| `kaji-coverage` | Audit pass focused on test coverage. |
| `maya` | Quality gate — runs tests, lint, and build across phases a-d. |
| `hyuga` | Criticality, waves, issue ranking, and task-tracker dispatch. |
| `aoba` | Git operations and run telemetry — commits, delivery prep, and the run summary. |

Ikari (the orchestrator) is the session itself, not a spawnable agent — see
`plugin/skills/nerv-orchestrator/SKILL.md`.

## Configuration schema

Every NERV setting lives in `nerv.yaml`. There are exactly two copies, same
schema: user scope `~/.claude/nerv/nerv.yaml` (personal defaults, never
committed) and project scope `<repo>/.nerv/nerv.yaml` (committed). Project
overrides user key by key; a missing key falls back to the user file, then
to the built-in default.

```yaml
# ~/.claude/nerv/nerv.yaml (user) and <repo>/.nerv/nerv.yaml (project): same schema
enabled: true                       # project scope only: activates NERV in this repo
skills:                             # stacks per consuming role; names must exist in .atl/skill-registry.md
  testing: [tdd, playwright-best-practices]                        # ritsuko, kaworu, maya
  code: [dotnet-best-practices, typescript-best-practices]         # pilots
  best-practices: [best-practices, solid-principles, clean-code-guard]  # balthasar
  architecture: [hexagonal-architecture, c4-architecture]          # melchor
  audit: [security-review, clean-code-guard]                       # kaji passes
critical_paths: [auth/, payments/, migrations/, infra/]            # Hyuga auto-critical
git:
  base_branch: develop              # default base for the worktree offer
  worktree: ask                     # ask | always | never
  branch_pattern: "feature/{prefix}-{id}-{slug}"   # prefix comes from the provider (tw, gh, jira)
  commit_ref_pattern: "({PREFIX}-{id})"
tasks:
  provider: teamwork                # teamwork | github-projects | jira | none ; "ask" when absent
  ask_when_missing: true            # preflight asks task + worktree + branch if no active task
  subtasks_per_wave: false
  providers:                        # one block per provider, only the enabled one is required
    teamwork:
      assignee_id: 686035           # user scope
      project_id: 1271726           # project scope
      tasklist_id: 3951970          # project scope
      stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }
    github-projects: { owner: "", project_number: 0 }    # later
    jira: { site: "", project_key: "" }                  # later
  sources:                          # extra work sources for listings (replaces ~/.claude/work/sources.md)
    - { name: erp-proveedores, type: google-sheets, ... }
```
