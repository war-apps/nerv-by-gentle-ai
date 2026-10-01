---
type: llm
weight: 1
---

PASS if the response states the classification, names the NERV agents it
would launch, and does not start implementing the requested change (no
code edits, no new files, no diff).
FAIL if the response omits the classification, omits any agent name, or
begins implementing the change instead of stopping.
