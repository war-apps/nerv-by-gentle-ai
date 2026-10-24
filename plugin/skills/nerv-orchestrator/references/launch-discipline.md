## Launch discipline

Rules Ikari keeps on every launch, in every tier. Each one closes a process
error a real run made.

- **One gate per worktree.** Ikari never launches two agents at once in one
  worktree when one of them is a gate: Maya, or a writer (Kaworu or a pilot)
  that would run alongside Maya. A gate reads the tree, so a concurrent
  writer turns its calibration or baseline into a dirty-tree run. Parallel
  batches stay allowed between writers with disjoint surfaces and between
  read-only passes (MAGI votes, audit passes).
- **Owners freeze and write verbatim.** Freezes and verbatim writes stay
  with their owning agent: Aoba freezes every audit round (`diff-round-N.patch`,
  `round-N.yaml`, `commits-round-N.txt`), and each artifact is written by
  the agent the contract in `nerv-artifacts.md` names. Ikari never freezes a
  round itself, never with a manual `git diff`, and never rewrites,
  summarizes or reorders another agent's artifact.
- **Text instead of a write.** When an agent returns its artifact as text
  instead of writing it (Ritsuko always; any agent whose write tool cannot
  append, such as Fuyutsuki's curate `## Summary`), Ikari appends it
  verbatim to the locator the brief named and records that it did: the
  `envelope` log entry carries `appended_by: ikari` and the locator. A
  file the agent wrote somewhere else (the worktree root, another feature)
  is moved by Ikari to its locator and recorded the same way.
- **Absolute, feature-scoped paths.** Every artifact path in a launch brief
  is absolute and scoped to the feature: `<worktree>/odd/{feature}/nerv/...`
  for the run folder and `<worktree>/odd/tasks/{feature}.md` for the ODD
  document in FULL, `<worktree>/.nerv/runs/{run-id}/...` in DIRECT and
  LIGHT, never a bare filename or a path relative to an unstated
  directory.
- **Audit inputs.** Audit launches always include `nerv/audit/commits-round-N.txt`
  next to `diff-round-N.patch` and `round-N.yaml`; a round whose freeze did
  not produce it goes back to Aoba before any pass is launched.
- **Briefs carry only what was approved.** Ikari never adds wording the user
  did not approve to an agent brief: scope, constraints, issue text and
  rulings are passed as the user, the plan or the binding ruling stated
  them, quoted, never extended with Ikari's own requirements.
