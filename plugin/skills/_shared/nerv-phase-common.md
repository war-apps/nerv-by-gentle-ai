# NERV Agent — Common Protocol

Boilerplate identical across all `nerv:*` agents. Every NERV agent MUST load
this file alongside its own role file (agents reference this file instead of
repeating its contents).

<!-- nerv:executor-boundary -->
Executor boundary: every NERV agent is an EXECUTOR, not an orchestrator. Do
the role's work yourself, with your own tools, in this same invocation. Do
NOT call the Agent tool, do NOT launch a sub-agent, and do NOT bounce work
back to Ikari unless your role file explicitly says to stop and report a
blocker. Subagents cannot spawn subagents in this system.
<!-- /nerv:executor-boundary -->

## A. Skill loading

1. Check whether Ikari injected a `## Skills to load before work` block in
   your launch prompt. If present, read those exact `SKILL.md` files before
   any task-specific work.
2. If no skills block was provided, check for `SKILL: Load` instructions. If
   present, load those exact skill files.
3. If neither was provided, fall back to the skill registry:
   a. `mem_search(query: "skill-registry", project: "{project}")` — if
      found, `mem_get_observation(id)` for the full content.
   b. Fallback: read `.atl/skill-registry.md` from the project root.
   c. From the registry's index, match triggers to your task and read the
      exact listed `SKILL.md` paths.
4. If no registry exists, proceed on your role file alone.

The preferred path is (1) — exact paths Ikari already resolved. Paths (2)
and (3) are fallbacks. Searching the registry is skill loading, not
delegation. If a skills block is present, ignore redundant `SKILL: Load`
instructions.

## B. Artifact retrieval

Ikari injects the artifact store and the exact locators it already
resolved. Read what you are given — do **not** detect the artifact store
and do not branch on it; re-deriving the store disagrees with the authority
that launched you.

| reported store | locator shape | how to read it |
|---|---|---|
| `openspec` | repo path, e.g. `openspec/changes/{change}/nerv/maya-report.md` | read the file |
| `engram` | topic key, e.g. `nerv/{change}/maya-report` | `mem_search(query: "<locator>", project: "{project}")` → `mem_get_observation(id)` |
| `hybrid` | either shape | read the file when the locator is a path, the observation when it is a topic key |

`mem_search` returns 300-character previews only. Always call
`mem_get_observation(id)` for the full content of any topic-key locator —
never work from a preview. Run parallel retrievals when more than one
artifact is needed. A locator reported as `<unresolved>` means the artifact
does not exist; report it as a blocker rather than substituting another
store's copy or going looking for one yourself.

## C. Artifact persistence

Persist every artifact you produce to the store Ikari reported, using that
artifact's locator, `nerv/{change}/{artifact}` as the topic key when the
store is `engram` or `hybrid` (artifact name = the file name without its
extension, e.g. `maya-report`).

### Engram mode

```
mem_save(
  title: "nerv/{change}/{artifact}",
  topic_key: "nerv/{change}/{artifact}",
  type: "architecture",
  project: "{project}",
  capture_prompt: false,
  content: "{full artifact markdown}"
)
```

`topic_key` enables upserts — saving again updates rather than duplicates.
`capture_prompt: false` is mandatory: these are automated pipeline outputs,
not human/proactive saves. Set it when the tool schema supports it; omit it
rather than failing if an older schema rejects it.

### OpenSpec mode

The file was already written during your main step (via `Write`). No
additional action needed.

### Hybrid mode

Attempt both writes and read back each one. Hybrid writes are not atomic:
preserve successful writes and report partial persistence with the
outstanding locator. Never claim a mirror that did not happen and never roll
back a write that did succeed.

### None mode

Return the result inline only. Do not write any files and do not call
`mem_save`.

## D. Return envelope

> **CRITICAL — response ordering**: your FINAL output MUST be text, not a
> tool call. If `mem_save` is needed, call it BEFORE your final text
> response — a tool call as the last action loses your analysis, because
> Ikari only receives the tool result. Do not call `mem_session_summary`;
> that is reserved for top-level sessions.

Return exactly these fields as your final text:

- `status`: `success`, `partial`, or `blocked`
- `executive_summary`: 1-3 sentences
- `detailed_report`: full output, or omit if already inline
- `artifacts`: list of artifact keys/paths written
- `next_recommended`: one of `none`, `next-pilot`, `maya-gate`,
  `aoba-commit`, `tracker-close`, `magi-vote`, `ikari-decision`,
  `misato-revise`, `plan-gate`, `waves`
- `risks`: risks discovered, or "None"
- `skill_resolution`: `paths-injected`, `fallback-registry`,
  `fallback-path`, or `none`

If you report anything other than `paths-injected` for `skill_resolution`,
Ikari re-reads the registry before your next launch.

## E. Key Learnings

Close your final report with a `## Key Learnings` section: 1-5 numbered,
standalone, ≥20-character factual sentences, so Engram can passively
capture them. This applies to the final text response only, never to
intermediate tool output or artifact content.

## F. Shared contract blocks

Every NERV agent's role file also carries, unchanged in wording:

- `<!-- nerv:agent-language-contract -->` — the Artifact Language Contract
  (generated artifacts default to English; neutral/professional Spanish only
  on explicit request; never regional slang regardless of conversation
  language).
- `<!-- nerv:remote-authorization -->` — the remote-operation authorization
  contract (no ambient SSH/credential reuse without explicit per-destination
  authorization; delegation cannot expand scope).

Both blocks are reproduced verbatim in each agent's own file, not only here,
so an agent reading only its own file still carries the full contract.
