# Known limitations and troubleshooting

## Known limitations

- **Audit coverage tracks the test plan.** Kaji's passes flag what the
  test plan and code disagree on; a scenario the user explicitly declined
  during the corner-case interview is a recorded decision, not a missing
  case, and will not surface as a finding.
- **Teamwork is the only task-tracker adapter.** `tasks.provider` accepts
  `teamwork` or `none`; adding another provider is documented under "Adding a
  provider" in `plugin/skills/nerv-tasks/SKILL.md`.
- **Interactive gates can't be driven by `claude -p`.** Non-interactive
  harness runs (`bench/journeys.md`'s default variant) pre-answer every
  blocking prompt in the launch context; the interactive variants exist
  precisely because a batch session cannot answer an
  `AskUserQuestion`-shaped prompt itself.
- **RDD advisory findings aren't a separate backlog.** Non-blocking
  findings from an acknowledged review receipt are recorded inline in the
  feature document (`odd/tasks/<feature>.md`), not tracked in a dedicated
  issue list.
- **Reviewed-boundary bookkeeping is per branch.** The "last reviewed
  commit" that RDD assessment walks forward from is tracked per branch,
  not per change or per worktree; rebasing or cherry-picking across
  branches can make that boundary stale.

## Troubleshooting

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
- **The `sdd-archive` agent refuses to launch.** gentle-ai's SDD dispatcher
  refuses `sdd-archive` outside a native SDD session (it wants an
  interactive AskUserQuestion preflight Nerv by Gentle-AI doesn't run). Nerv by
  Gentle-AI archives
  mechanically instead — Aoba runs `git mv` plus
  `gentle-ai sdd-archive-compose` per delta spec.
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
- **`createTask` blocks with "tasklist not found."** A stale
  `tasks.providers.teamwork.tasklist_id` (or `project_id`) in `nerv.yaml`.
  Fix the config (project or user scope) and re-run; the preflight is
  designed to stop and re-ask rather than silently create the task
  elsewhere.
- **Extra reviewer sessions appear on every tool use.** That's the
  `security-guidance` plugin's own hook (installed independently), not
  Nerv by Gentle-AI. Nerv by Gentle-AI's own review relay only runs after an
  Aoba work-unit commit.
