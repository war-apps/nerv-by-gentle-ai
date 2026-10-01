## Usage collection

Every Agent tool result carries the launch's usage (tokens, tool uses,
duration). After each launch Ikari records one row
`{agent, model, tokens_total, duration_s}` from that result —
`tokens_total` because the Agent tool reports one combined usage figure per
launch, not separate input/output counts. The accumulated table is handed
to Aoba in the run-summary launch; Aoba never estimates figures, and Ikari
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
`rdd_receipt`, `stop`. FULL adds the Phase 2 event types listed in
`nerv-artifacts.md` (`corner_case_relayed`, `corner_case_answer`,
`vote_cast`, `escalation_criticality`, `vote_result`, `veto_evaluated`,
`plan_gate_relayed`, `plan_gate_decision`, `wave_plan`, `wave_report`,
`deviation`, `ruling_issued`), plus the Phase 3 audit-stage event types,
also defined in `nerv-artifacts.md`: `patch_frozen`, `audit_pass`,
`dedupe_merge`, `refuter_result`, `ranking_issued`, `issue_gate_relayed`,
`issue_gate_decision`, `fix_routed`, `reaudit`, `residual_accepted`,
`docs_written`, `archived`, `log_curated`. Phase 4 adds `tracker_event`
(payload `{op, taskRef, result}`, defined in `nerv-artifacts.md`), logged
once per Hyuga tracker op — Preflight, Maya's full-gate start, the issue
gate, and Close. Phase 5 adds `resume` (`{from_step, took_over_from}`) and
`lock_refused`, also defined in `nerv-artifacts.md`, logged by the
Orchestrator lock and Resume protocols.

**Completeness rule.** The log is the run's only chronological record, so
it never skips a step that happened: every launch gets its `launch` line
and its `envelope` line (Ritsuko's docs launch and Hyuga's ranking
envelope included), every artifact Hyuga ranks gets `ranking_issued`, and
the issue gate always produces `issue_gate_relayed` and
`issue_gate_decision`, even when the NOW set is empty and the answer was
pre-granted in the launch context (payload: `approved-as-ranked`, the
empty NOW set, the DEFER ids). Recording a decision only in
`issue-ranking.md` or `issue-resolutions.md` is not a substitute: those
files are the artifact, the log line is the event. A `tracker_event` is
logged for the DEFER `createTask` ops as well (`result=skipped` when the
provider is `none`).

**Close ordering.** The close commit (`docs: close nerv run for {change}`)
is the run's last write. Ikari appends the `stop` event before launching
it, so the committed log is complete; the close commit's own hash is
reported in the run's final message, never appended to the log
afterwards. After close, `git status` must show nothing under the change
(J4/J6 check): a `commit_recorded` or `stop` line appended after the close
commit is a protocol violation, not a known limitation.

