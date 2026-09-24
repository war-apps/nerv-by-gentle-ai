# NERV Artifact Schemas

One definition per NERV-owned artifact, referenced by every agent instead of
repeating the shape inline. gentle-ai-owned filenames (`state.yaml`,
`exploration.md`, `proposal.md`, `specs/{domain}/spec.md`, `design.md`,
`tasks.md`) keep gentle-ai's own shape and stay where `gentle-ai sdd-status`
expects them; only NERV-owned files are defined here. All NERV files live
under `openspec/changes/{change}/nerv/`, except `state.yaml` which lives at
`openspec/changes/{change}/state.yaml` (gentle-ai's own file, extended with
one NERV-owned key). Engram topic key for every artifact:
`nerv/{change}/{artifact}` (artifact = file name without extension).

## state.yaml (extended)

- Purpose: native SDD change state, extended with the NERV path decision so
  a resumed session knows which pipeline governed the change without
  re-classifying.
- Author: Ikari (the `nerv:` block only — the rest of the file is
  gentle-ai's own and is never hand-edited by NERV agents).
- Location: `openspec/changes/{change}/state.yaml`.
- Engram key: `nerv/{change}/state` (mirror of the `nerv:` block only).

```yaml
dependsOn: []
nerv:
  path: light                     # light | full
  classification_reason: "single domain, no critical path, under budget"
  task_ref: "TW-49132010"         # or null when no task
  created_at: "2026-09-24T14:03:00Z"
```

## exploration-light.md (Ritsuko)

- Purpose: LIGHT-path micro-intel — enough to pick a pilot and scope Maya's
  reduced gate, not a full exploration document.
- Author: Ritsuko (LIGHT micro-intel dispatch).
- Location: `nerv/exploration-light.md`.
- Engram key: `nerv/{change}/exploration-light`.

```markdown
# Exploration (light) — {change}

## Touched files
- {path} — {why it is touched}

## Entry points
- {function/route/component the change enters through}

## Existing tests
- {test file(s) already covering this area, or "none found"}

## Risks
- {short risk note, or "None"}

## Suggested pilot
{rei|shinji|asuka|toji} — {one-line domain justification}
```

## maya-report.md (Maya)

- Purpose: baseline + quality-gate results, reduced-mode aware, with TDD
  evidence reproduced independently (never trusted from a self-report).
- Author: Maya.
- Location: `nerv/maya-report.md`.
- Engram key: `nerv/{change}/maya-report`.

```markdown
# Maya report — {change}

## Mode
{full|reduced} — reduced scope: {touched tests + lint/build only, list them}

## Phases
| Phase | Project | Detected | Lint configured | Build configured | Result |
|---|---|---|---|---|---|
| a (unit/touched tests) | {project} | yes/no | - | - | pass\|fail\|no-lint-build-configured |
| b (lint) | {project} | - | yes/no | - | pass\|fail\|no-lint-build-configured |
| c (build) | {project} | - | - | yes/no | pass\|fail\|no-lint-build-configured |
| d (full suite) | {project} | yes/no | - | - | pass\|fail\|skipped-reduced-mode |

## Failures
| test_id | verdict | routed_to |
|---|---|---|
| {test id} | spec-wrong\|impl-wrong\|ambiguous | {pilot\|Ikari-user-escalation} |

## TDD evidence
| step | test | command | observed result | commit |
|---|---|---|---|---|
| RED | {test name} | {exact command} | {exact observed output/exit} | {commit hash} |
| GREEN | {test name} | {exact command} | {exact observed output/exit} | {commit hash} |
| TRIANGULATE | {test name} | {exact command} | {exact observed output/exit} | {commit hash} |
| REFACTOR | {test name} | {exact command} | {exact observed output/exit} | {commit hash} |

RED is reproduced by read-only inspection of the RED commit in git history
(`git show`/`git log -p` output read, never checked out or extracted into
the tree) plus a live run of the suite at the current tree for GREEN; the
working tree is never mutated by the gate. maya.md governs on any
remaining difference.
```

## run-summary.md (Aoba)

- Purpose: close-out narrative, usage accounting, RDD receipts, commit and
  PR-slice record for the change.
- Author: Aoba.
- Location: `nerv/run-summary.md`.
- Engram key: `nerv/{change}/run-summary`.

```markdown
# Run summary — {change}

## Work done
{short narrative}

## Agents
| agent | model | tokens_in | tokens_out | duration_s |
|---|---|---|---|---|
| nerv:kaworu | sonnet | {n} | {n} | {n} |

Figures come only from usage data Ikari collected from each Agent tool
result — never estimated.

## RDD receipts
| commit | review_due | outcome | boundary_advanced_to |
|---|---|---|---|

## Commits
| hash | subject |
|---|---|

## PR slices
{none, or the chained/stacked slice list with boundaries}
```

## deliberation-log.md (Ikari, append-only)

- Purpose: full event trail of the run — the only append-only NERV
  artifact; never rewritten, only appended to.
- Author: Ikari, appending inline after each event (a mechanical
  single-file write, not delegated).
- Location: `nerv/deliberation-log.md`.
- Engram key: `nerv/{change}/deliberation-log` (re-saved in full on each
  append, since Engram upserts overwrite rather than append).

```markdown
# Deliberation log — {change}

- {ts} | {phase} | {actor} | preflight_answer | {payload_ref}
- {ts} | {phase} | {actor} | classification | {payload_ref}
- {ts} | {phase} | {actor} | ratchet | {payload_ref}
- {ts} | {phase} | {actor} | launch | {payload_ref}
- {ts} | {phase} | {actor} | envelope | {payload_ref}
- {ts} | {phase} | {actor} | gate_relayed | {payload_ref}
- {ts} | {phase} | {actor} | gate_decision | {payload_ref}
- {ts} | {phase} | {actor} | commit_recorded | {payload_ref}
- {ts} | {phase} | {actor} | rdd_assess | {payload_ref}
- {ts} | {phase} | {actor} | rdd_receipt | {payload_ref}
- {ts} | {phase} | {actor} | stop | {payload_ref}
```

`payload_ref` is a short pointer (commit hash, envelope field, file path) —
never the full payload inline; keep the log scannable.

## FULL-path artifacts (Phase 2+)

Named here only — schemas ship with the phase that introduces them:

- `nerv/test-plan.md` (Ritsuko)
- `nerv/criticality.md` (Hyuga)
- `nerv/votes.md` (Ikari, merged from MAGI)
- `nerv/veto-ruling.md` (Fuyutsuki)
- `nerv/waves.md` (Hyuga)
- `nerv/audit/diff-round-N.patch` (Aoba)
- `nerv/audit-report.md` (Kaji)
- `nerv/issue-ranking.md` (Hyuga)
- `nerv/issue-resolutions.md` (Ritsuko)
- `nerv/agent-config.md` (Ritsuko)
