## Phase note

This is the Phase 4 build: NERV's governance surface (Phase 3) is complete,
plus the single config file. LIGHT keeps the
same pipeline shape as Phase 1 (Ritsuko micro-intel, Kaworu, one
domain-matched pilot, Maya reduced gate, Aoba). FULL ships Misato's plan
authorship and rulings, MAGI (Balthasar, Melchior, Casper), Fuyutsuki's
governance veto, Hyuga's criticality, waves and ranking dispatches, all
five pilots (`rei`, `shinji`, `asuka`, `toji`, `kaworu`),
and the full audit-and-closure stage (Kaji, `gendo`, Fuyutsuki's
read-only refute mode, the ranked issue gate, fix routing, the
bounded re-audit loop, and Aoba's mechanical Archive duty). The configuration
(`config.yml` and `.ai-workflow.yml`: two scopes, one schema, project overrides user)
is resolved at Preflight. Task tracking later moved out of core into the
optional task plugin (`task@nerv`): core only emits the events of
SKILL.md `## Task events` through `task:dispatcher`, and only when that
agent is available.
Phase 5 hardening ships the orchestrator lock (concurrency guard, heartbeat
staleness), safe resume, close-time `tasks.md`/`state.yaml` bookkeeping, the
`artifacts.commit` policy, and RDD's untracked-path recovery path.
Routing tiers (after Phase 5) mirror gentle-ai's Work Routing Ladder:
DIRECT (Ikari inline, one mechanical file), LIGHT (an explorer only when
the mapping trigger fires, one pilot in `MODE: solo`, Maya only when the
Delegated Verification Gate asks for her) and FULL (unchanged), with the
one-way ratchet DIRECT → LIGHT → FULL; the LIGHT shape described above is
the pre-tier history.
