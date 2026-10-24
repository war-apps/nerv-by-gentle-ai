## Delivery

Work happens as one conventional commit per work unit, validated by the user
before it is committed — by Ikari itself in DIRECT and LIGHT, by Aoba in
FULL. Track a running authored-line count (additions +
deletions) against the roughly-400-line slice budget from session start.
When the forecast or running count crosses the budget, apply the cached PR
strategy (`ask-on-risk` asks once for `stacked-to-main` vs
`feature-branch-chain`; `auto-chain` slices automatically and asks only if
the chain strategy is still unset; `single-pr` and `exception-ok` skip
slicing per their definitions). Resolve `work-unit-commits` and
`chained-pr` by registry name, the same way as any other skill. Push, merge,
and PR creation are always the user's own decision — Aoba prepares the
commands, never runs them.

**Artifacts commit policy.** `artifacts.commit` in the merged configuration
(`with-change`|`at-close`|`never`, default `at-close`) governs the ODD
document `odd/tasks/{feature}.md` and the run folder `odd/{feature}/nerv/`:
`with-change` adds the document and run-folder files a work-unit commit
touched to that commit; `at-close` leaves them untracked until the close
commit (`docs: close nerv run for {feature}`, by the tier's committer)
commits them once, whole, through the normal user-validated commit;
`never` leaves them untracked permanently. Each task's commit is recorded
in its `## Tasks` row either way. There is no archive and no archive
commit.

**Repos that git-ignore the artifacts.** Once per feature, before the
first commit, Ikari runs `git check-ignore -q odd/{feature}/` and `git
check-ignore -q odd/tasks/{feature}.md` (`.nerv/` is runtime only and
always ignores itself, so it is not checked). When either is ignored, the
repository's policy wins over `artifacts.commit`: the artifacts stay
local, nothing under them is committed or force-added, there is no close
commit, and the run says so — in the first commit-validation prompt and
in the run summary (`artifacts: local, ignored by the repo`).

The orchestrator lock `.nerv/locks/{feature}.lock` is never staged and is
excluded from every commit regardless of this setting: it lives in the
runtime folder, which ignores itself. The lock is deleted before the close
commit, which is the run's last write (see `## Deliberation log`, close
ordering).
