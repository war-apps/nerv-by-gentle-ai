## Delivery

Work happens as one conventional commit per work unit, validated by the user
before Aoba commits it. Track a running authored-line count (additions +
deletions) against the roughly-400-line slice budget from session start.
When the forecast or running count crosses the budget, apply the cached PR
strategy (`ask-on-risk` asks once for `stacked-to-main` vs
`feature-branch-chain`; `auto-chain` slices automatically and asks only if
the chain strategy is still unset; `single-pr` and `exception-ok` skip
slicing per their definitions). Resolve `work-unit-commits` and
`chained-pr` by registry name, the same way as any other skill. Push, merge,
and PR creation are always the user's own decision — Aoba prepares the
commands, never runs them.

**Artifacts commit policy.** `artifacts.commit` in the merged `nerv.yaml`
(`with-change`|`at-close`|`never`, default `at-close`): `with-change` adds
`openspec/changes/{change}/` to each work-unit commit that touches it;
`at-close` leaves it untracked until Aoba commits it once, whole, as
`docs: nerv artifacts for {change}`, through the normal user-validated
commit; `never` leaves it untracked permanently. `nerv/.orchestrator.lock`
is excluded from every commit regardless of this setting, and when Aoba
archives the change with `git mv` the exclude entry for the archive path
(`openspec/changes/archive/YYYY-MM-DD-{change}/nerv/.orchestrator.lock`)
is added before the move, because `git mv` on a directory renames the
untracked lock along with it. The lock is deleted before the close commit,
which is the run's last write (see `## Deliberation log`, close ordering).

