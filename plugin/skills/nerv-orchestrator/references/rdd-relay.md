## RDD relay

Ikari never enables or disables RDD. After each work-unit commit (Ikari's in DIRECT/LIGHT, Aoba's in FULL), run:

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

**NERV runtime folders.** gentle-ai refuses to assess while any path is
untracked (`unassessable`, even with `--committed-only`), and the untracked
NERV runtime folders — `.nerv/` and `openspec/` content the run has not
committed (see `artifacts.commit` in `delivery.md`) — trip it on every run.
Ikari excludes them itself and never asks the user to edit
`.git/info/exclude`. gentle-ai 4.2 has no per-path exclude flag: the scope
is all-or-declared, so Ikari declares it:

1. Run the read-only status command the refusal names, `gentle-ai review
   status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent
   claude-code --next-transition`, and take `eligible_untracked_inventory`
   (a `sha256:` digest).
2. List the untracked paths with `git ls-files --others --exclude-standard`
   and drop every path under `.nerv/` or `openspec/`.
3. Nothing left → rerun assess (same flags) plus
   `--untracked-scope=exclude --expected-untracked-inventory=<digest>`.
   Paths left → plus `--untracked-scope=select` with one
   `--intended-untracked=<path>` per remaining path and the same
   `--expected-untracked-inventory=<digest>`: the NERV folders are the
   undeclared rest, out of the candidate, and real new source files still
   count.

A digest mismatch means the tree changed between the two calls: repeat from
step 1 once. Log both attempts as `rdd_assess`, with the declared scope in
the payload. A START or STATUS that asks for the same declaration gets the
same flags.
