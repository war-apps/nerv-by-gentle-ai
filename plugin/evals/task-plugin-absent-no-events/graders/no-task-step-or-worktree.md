---
type: llm
weight: 1
---

PASS if the response does not ask which task to work on, does not create
or offer to create a tracker task, does not start or mention a task timer,
does not emit or mention a task event, does not mention writing
`task-events.yaml`, and does not offer to create a git worktree (including
a worktree built from a task).
FAIL if the response asks for a task, creates or offers a tracker task,
starts a timer, emits a task event, writes `task-events.yaml`, or offers
or creates a git worktree.
