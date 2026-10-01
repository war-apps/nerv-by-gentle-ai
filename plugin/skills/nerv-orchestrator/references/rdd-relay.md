## RDD relay

Ikari never enables or disables RDD. After each Aoba work-unit commit, run:

```
gentle-ai review assess --cwd <repo> --agent claude-code --base-ref <last reviewed boundary> --committed-only --json
```

Read `review_due` and `review_due_reason`. When `review_due` is true, run
the returned `next_transition.command` verbatim and follow its transitions
exactly as gentle-ai's native review lifecycle prescribes — relay any
`gentle-ai.review-integration.consent/v3` envelope to the user losslessly,
never on their behalf. When `review_due` is false, record the reason
(`passive`, `under_budget`, `already_reviewed`) and continue; the reviewed
boundary advances to this commit only once its review is acknowledged, or
immediately for `passive`. A failed or unavailable assessment is always
treated as due — never inferred as low risk. Log every assessment and every
receipt as `rdd_assess` / `rdd_receipt` events in `deliberation-log.md`. The
first boundary of a change is its branch point.

**Untracked-path refusal.** When `review assess` returns `unassessable` for
untracked paths (the NERV change folder is untracked by default — see
`artifacts.commit` below), run the read-only status command it names:
`gentle-ai review status --cwd <repo> --contract
gentle-ai.review-integration/v2 --agent claude-code --next-transition`, take
`eligible_untracked_inventory` from it, and rerun assess with
`--untracked-scope=exclude --expected-untracked-inventory=<that digest>`.
With `artifacts.commit: at-close` or `never`, the NERV folder is exactly
that expected untracked content. Log both attempts as `rdd_assess`.

