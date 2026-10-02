## Phase note

This is the Phase 4 build: NERV's governance surface (Phase 3) is complete,
plus the task-tracking layer and the single config file. LIGHT keeps the
same pipeline shape as Phase 1 (Ritsuko micro-intel, Kaworu, one
domain-matched pilot, Maya reduced gate, Aoba), with Preflight and Close
now also running Hyuga's tracker dispatch. FULL ships Misato's plan
authorship and rulings, MAGI (Balthasar, Melchor, Casper), Fuyutsuki's
governance veto, Hyuga's criticality, waves, ranking, and tracker
dispatches, all five pilots (`rei`, `shinji`, `asuka`, `toji`, `kaworu`),
and the full audit-and-closure stage (Kaji, `kaji-security`,
`kaji-coverage`, `kaji-refuter`, the ranked issue gate, fix routing, the
bounded re-audit loop, and Aoba's mechanical Archive duty). The task
tracker (`nerv-tasks/SKILL.md`, the Teamwork adapter delegating to
`~/.claude/commands/task/*.md`, and
Hyuga's `DISPATCH: tracker`) and the single `nerv.yaml` config file
(two scopes, one schema, project overrides user) are wired into Preflight,
Maya's full-gate start, the issue gate, and Close in both pipelines.
Phase 5 hardening ships the orchestrator lock (concurrency guard, heartbeat
staleness), safe resume, close-time `tasks.md`/`state.yaml` bookkeeping, the
`artifacts.commit` policy, and RDD's untracked-path recovery path.
