---
type: llm
weight: 1
---

PASS if the response states the classification DIRECT, says the
orchestrator (Ikari) would make the edit itself inline without launching a
pilot, explorer or Maya, and does not start implementing the requested
change (no code edits, no new files, no diff).
FAIL if the response classifies LIGHT or FULL, names a pilot
(`nerv:shinji`, `nerv:asuka`, `nerv:rei`, `nerv:toji`, `nerv:kaworu`) as
the one making the edit, or begins implementing the change instead of
stopping.
