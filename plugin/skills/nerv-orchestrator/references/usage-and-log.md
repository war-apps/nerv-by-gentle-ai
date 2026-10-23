## Usage collection

Every Agent tool result carries the launch's usage (tokens, tool uses,
duration). After each launch Ikari records one row
`{agent, model, tokens_total, duration_s}` from that result —
`tokens_total` because the Agent tool reports one combined usage figure per
launch, not separate input/output counts. The accumulated table feeds the run
summary (written by Ikari in DIRECT and LIGHT, handed to Aoba in FULL's
run-summary launch); figures are never estimated, and Ikari
never omits a launch, including retries and failed ones (mark them in the
row). `model` in this row is the REPORTED model from the Agent result, kept
as-is — it is not the resolved model from the Mandatory model gate, though
the two are compared at envelope readback (see Delegation triggers).

## Deliberation log

Ikari appends one entry per event to `nerv/deliberation-log.md` (append-only,
Ikari's own mechanical write, never delegated) as
`{ts, phase, actor, event_type, payload_ref}`. Event types used in Phase 1:
`preflight_answer`, `classification`, `ratchet`, `launch`, `envelope`,
`gate_relayed`, `gate_decision`, `commit_recorded`, `rdd_assess`,
`rdd_receipt`, `stop`. The `classification` payload carries the route
declaration (tier plus trigger evidence, see SKILL.md Classification). FULL adds the Phase 2 event types listed in
`nerv-artifacts.md` (`corner_case_relayed`, `corner_case_answer`,
`vote_cast`, `escalation_criticality`, `vote_result`, `veto_evaluated`,
`plan_gate_relayed`, `plan_gate_decision`, `wave_plan`, `wave_report`,
`deviation`, `ruling_issued`), plus the Phase 3 audit-stage event types,
also defined in `nerv-artifacts.md`: `patch_frozen`, `audit_pass`,
`dedupe_merge`, `refuter_result`, `ranking_issued`, `issue_gate_relayed`,
`issue_gate_decision`, `fix_routed`, `reaudit`, `residual_accepted`,
`docs_written`, `archived`, `log_curated`. The task plugin adds
`task_event` (payload `{event, provider, result, task_ref}`, defined in
`nerv-artifacts.md`), logged once per envelope `task:dispatcher` returns —
`run.started`, `verification.started` (Maya's full-gate start in FULL, the
work unit's verification start in DIRECT and LIGHT), `wave.started`,
`audit.ranked`, `issue.deferred`, `run.closed` and `run.halted` — and only
when the task plugin is installed. Phase 5 adds `resume` (`{from_step, took_over_from}`) and
`lock_refused`, also defined in `nerv-artifacts.md`, logged by the
Orchestrator lock and Resume protocols. A `launch` entry's payload opens with
the launch's `({model}) {Display name}: {action}` label, the same text as its Agent
tool `description`.

**Completeness rule.** The log is the run's only chronological record, so
it never skips a step that happened: every launch gets its `launch` line
and its `envelope` line (Ritsuko's docs launch and Hyuga's ranking
envelope included), every artifact Hyuga ranks gets `ranking_issued`, and
the issue gate always produces `issue_gate_relayed` and
`issue_gate_decision`, even when the NOW set is empty and the answer was
pre-granted in the launch context (payload: `approved-as-ranked`, the
empty NOW set, the DEFER ids). Recording a decision only in
`issue-ranking.md` or `issue-resolutions.md` is not a substitute: those
files are the artifact, the log line is the event. With the task plugin
installed, a `task_event` is logged for every `issue.deferred` envelope as
well, including `result=skipped` ones.

**Close ordering.** The close commit (`docs: close nerv run for {change}`)
is the run's last write. Ikari appends the `stop` event before launching
it, so the committed log is complete; the close commit's own hash is
reported in the run's final message, never appended to the log
afterwards. After close, `git status` must show nothing under the change
(J4/J6 check): a `commit_recorded` or `stop` line appended after the close
commit is a protocol violation, not a known limitation.

