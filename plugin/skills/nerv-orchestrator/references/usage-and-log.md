## Usage collection

After each launch Ikari records one row `{agent, model, tokens_in,
tokens_out, tokens_total, duration_s}`. Figures are never estimated, and
Ikari never omits a launch, including retries and failed ones (mark them in
the row). Each value has one source:

- `tokens_total` and `duration_s`: the usage block of the Agent tool result,
  or of its task notification for a background launch — `total_tokens` (the
  subagent's tokens) and `duration_ms` divided by 1000.
- `tokens_in` and `tokens_out`: that usage block carries one combined
  figure, so Ikari reads the split from the launch's own transcript,
  `~/.claude/projects/<project dir>/<session_id>/subagents/agent-<agentId>.jsonl`
  (`agentId` from the same Agent result, `session_id` as in the
  orchestrator lock). Over its assistant records, counted once per
  `message.id`, `tokens_in` is the sum of `input_tokens`,
  `cache_creation_input_tokens` and `cache_read_input_tokens`, and
  `tokens_out` the sum of `output_tokens`. When that transcript is missing
  or unreadable, both read `n/a (no transcript)` and the row still carries
  `tokens_total`; this is the only allowed gap.
- `model`: the `message.model` that transcript reports — the REPORTED
  model, compared with the resolved one at envelope readback (see
  Delegation triggers). Without a transcript, it is the resolved model of
  the Mandatory model gate, else the agent file's frontmatter `model:`,
  and the row says which (`model source: resolved|frontmatter`).

The accumulated table feeds the run summary (written by Ikari in DIRECT and
LIGHT, handed to Aoba in FULL's run-summary launch).

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
`docs_written`, `log_curated`. The task plugin adds
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

**Engram flush at close.** Right before the run summary (LIGHT step 8,
FULL step 19), Ikari collects every `Pending KB mirror` block an envelope
of this run carried — an agent whose knowledge-base `mem_save` the host
rejected reported `partial`, with the topic key and the exact content — and
any of its own votes or gate-decision mirrors that failed. Ikari saves it
itself: `mem_save` to project `nerv` under that same topic key, `type:
"decision"`, `capture_prompt: false`, content verbatim, then reads it back,
and logs one `kb_mirror_flushed` event per mirror (payload: topic key,
result). A mirror that still fails never stops the run: the run summary
lists it under `## KB mirrors unflushed` (topic key and error).

**Close ordering.** In FULL, the close commit (`docs: close nerv run for
{feature}`) is the run's last write. Ikari appends the `stop` event before launching
it, so the committed log is complete; the close commit's own hash is
reported in the run's final message, never appended to the log
afterwards. After close, `git status` must show nothing under
`odd/tasks/{feature}.md` or `odd/{feature}/` (J4/J6 check; with
`artifacts.commit: never` they stay untracked by design): a `commit_recorded` or `stop` line appended after the close
commit is a protocol violation, not a known limitation. DIRECT and LIGHT
have no close commit: their log lives in `.nerv/runs/{run-id}/`, which is
never committed, so the `stop` event is simply the log's last line.

