# Known limitations and troubleshooting

## Known limitations

- **Audit coverage tracks the test plan.** Kaji's passes flag what the
  test plan and code disagree on; a scenario the user explicitly declined
  during the corner-case interview is a recorded decision, not a missing
  case, and will not surface as a finding.
- **Teamwork is the only task provider shipped today.** Task tracking is the
  optional task extension (`task@nerv`), and `task-teamwork@nerv` is its only
  provider. Another provider is a new `task-<name>` provider with a valid `provider.yaml`; see
  "Adding a provider" in [integration.md](integration.md#task-extension).
- **No task question, no timer, no task event.** The task extension is not
  installed: the orchestrator emits task events only when the agent type
  `task:dispatcher` is available. Install it with `nerv install --with task`
  (or `--with task-teamwork`) and restart Claude Code. A `tasks:` block in
  the configuration alone does nothing; core ignores it and never deletes it.
- **"Task events pending" in the run summary.** A provider answered
  `blocked` or `failed` (for example, missing MCP tools); the run went on
  without it. Fix the cause named in the envelope's `error`, then run
  `/task:replay {feature}` with the name the hint printed (the run id for a
  DIRECT or LIGHT run).
- **Interactive gates can't be driven by `claude -p`.** Non-interactive
  harness runs (`bench/journeys.md`'s default variant) pre-answer every
  blocking prompt in the launch context; the interactive variants exist
  precisely because a batch session cannot answer an
  `AskUserQuestion`-shaped prompt itself.
- **RDD advisory findings aren't a separate backlog.** Non-blocking
  findings from an acknowledged review receipt are recorded inline in the
  feature document (`odd/tasks/{feature}.md`) of a FULL run, or in the run
  summary of a DIRECT or LIGHT run, not tracked in a dedicated issue list.
- **Reviewed-boundary bookkeeping is per branch.** The "last reviewed
  commit" that RDD assessment walks forward from is tracked per branch,
  not per feature or per worktree; rebasing or cherry-picking across
  branches can make that boundary stale.

## Troubleshooting

Start with `nerv doctor`: it checks the machine and the repository and
lists the steps to take ([commands.md](commands.md#nerv-doctor)).

- **Plugin cache still shows old files after an edit.** `nerv install`
  materializes the plugin tree from the binary's embedded snapshot, taken
  at `go build` time, into `~/.nerv/marketplace`, then refreshes the
  Claude Code cache from that directory — a rebuild picks up uncommitted
  edits, but the *running* Claude Code session still holds the plugin
  cache from before the refresh. Rebuild (`go build ./cmd/nerv`), run
  `nerv install --no-configure --no-skills` (or just `nerv install`), and
  restart Claude Code.
- **`nerv` not on PATH after `install.sh`.** The script installs to
  `$NERV_INSTALL_DIR` (default `$HOME/.local/bin`); add that directory to
  `PATH`, or re-run with `NERV_INSTALL_DIR` pointed at a directory already
  on `PATH`.
- **Plugin cache version does not match.** Run
  `nerv install --no-configure --no-skills` to re-materialize the plugin,
  re-register it, and refresh the Claude Code cache without touching
  skills or running the wizard, then restart Claude Code.
- **The wizard says "input ended" / exits immediately.** `nerv configure`
  (with no mode flag) only prompts when stdin is a real terminal; reading
  from a redirected or empty stdin (for example `< /dev/null`) is treated
  as immediate EOF and aborts with exit 1 and no side effects. Run it in
  an interactive terminal, or drive it non-interactively with
  `--answers <file>` (one answer per line) instead.
- **`gentle-ai review status` times out around 25 s on a bound lineage.**
  Observed `operation_timeout` on the pre-native STATUS budget
  (`reviewFacadeOperationTimeout`, no env override). Retry once; if it
  keeps failing, continue without that review (ordinary policy) or reduce
  the reviewed scope and try again.
- **`lens_context_budget_exceeded` on a review.** The accumulated range is
  too large for the reviewer context. Review commit by commit from a
  detached review worktree (`git worktree add --detach`; lineages share
  the same `.git`), and keep individual commits near the ~400-line
  delivery-budget heuristic so each fits.
- **Looking for an archive step or `openspec/changes/`.** There is none any
  more: Nerv by Gentle-AI tracks each FULL run on gentle-ai's ODD feature
  document (`odd/tasks/{feature}.md`) and keeps its governance files in
  `odd/{feature}/nerv/`. A feature closes in place: its `## Tasks` rows
  carry their commits and its `## Log` records the close.
- **A DIRECT or LIGHT run left no `odd/` file and no close commit.** That
  is by design: only FULL runs create the ODD document and the close
  commit. DIRECT and LIGHT runs keep their state, deliberation log, task
  events and run summary in the runtime folder `.nerv/runs/{run-id}/`,
  which is never committed, and their only commit is the work commit. An `openspec/`
  folder left from an older release is no longer read; `nerv migrate`
  converts the changes still in progress.
- **An agent reports `partial` because `mem_save` was rejected** ("session
  registration could not be confirmed"). The feature is not blocked: the
  agent hands the knowledge-base mirror back, Ikari saves it at close, and
  the run summary lists any mirror still unflushed under `## KB mirrors
  unflushed`.
- **A lock says a run is active, but nobody is working.** A lock whose
  heartbeat is 15 minutes old or more (and not parked on a user gate) is
  stale, not active; `/nerv:status` says so and a resume may proceed.
- **A launch dies under memory pressure.** The protocol retries the launch
  once. If the session itself is interrupted, resuming is safe: the
  orchestrator lock and its heartbeat let a resume session detect and take
  over a genuinely dead run without re-voting frozen tasks or re-running
  closed waves.
- **Commits carry a stray `Co-Authored-By` trailer.** The harness
  attribution reminder some environments inject is ignored by Aoba on
  purpose — Nerv by Gentle-AI commits never carry AI attribution trailers; a
  gatekeeper
  check greps the commit message for this before it lands.
- **`issue.deferred` or `/task:start` blocks with "tasklist not found."** A
  stale `tasks.providers.teamwork.tasklist_id` (or `project_id`) in
  the configuration. Fix the config (project or user scope), then replay the
  pending envelope with `/task:replay {feature}`; the task extension never
  silently creates the task elsewhere.
- **Old `/task:*` copies in `~/.claude/commands/task/`.** An older release
  copied the commands there. The `task` extension now serves them itself;
  `nerv install` reports the old copies and removes them only after you
  confirm.
- **Extra reviewer sessions appear on every tool use.** That's the
  `security-guidance` plugin's own hook (installed independently), not
  Nerv by Gentle-AI. Nerv by Gentle-AI's own review relay only runs after an
  Aoba work-unit commit.
