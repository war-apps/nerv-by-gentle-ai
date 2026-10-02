# Strip the removed task providers from existing configs

Branch `feature/legacy-provider-cleanup` from `develop` (`355c163`). Started 2026-10-02.
Delivery strategy: `ask-on-risk`; chain strategy cached: stacked to `develop`. Forecast ~300
authored changed lines, one PR.
TDD: strict (global setting). Runner: `go test ./...` (plus `gofmt -l .` and `go vet ./...`).
Engram mirror: topic `odd/legacy-provider-cleanup/tasks`.

## Objective

When a `nerv.yaml` (user or project scope) still carries the task providers removed in #50,
`nerv` removes them from the file itself instead of preserving or warning about them.

## Problem / why

- Native review finding R3-legacy-provider-no-upgrade-diagnostic (2026-10-02): a config holding
  `tasks.provider: jira|github-projects` gets no diagnostic; it is kept on every write and the
  tracker only fails later (`blocked`).
- User decision 2026-10-02: "no agregues el aviso, directamente removelos"; for the provider line
  the user chose "Borrar la línea": drop `tasks.provider` when its value is a removed provider, so
  the default (teamwork) or the `ask_when_missing` preflight question applies, like a new config.

## Scope and constraints

- Removed providers: `github-projects`, `jira` (one Go list, next to `AllowedValues`, so a future
  removal reuses it).
- Cleanup removes, in both scopes:
  - the `tasks.provider: <removed>` line (with its trailing comment);
  - the `tasks.providers.github-projects` and `tasks.providers.jira` sub-blocks, inline
    (`jira: { ... }  # later`) or multi-line, including their own trailing comments;
  - nothing else: every other byte stays as written (comments, padding, other providers).
- It runs on every config write path (`nerv configure` set/validate/init-repo/`--set-model`, the
  wizard save, and whatever `nerv install` writes) and through the shared write layer if one
  exists, with the usual backup. A file that needs no cleanup is untouched.
- Each write that cleaned something prints one line naming what was removed (informational
  output of the action, not a warning about an unchanged config).
- `#52`'s preservation tests are inverted to prove removal; the "other keys keep their bytes"
  assertions stay.

## Tasks

- [ ] T1 Cleanup in the config layer: removed-provider list, pure function that strips the line
  and sub-blocks from YAML bytes (byte-preserving elsewhere), tests for inline/multi-line/
  comment/EOF/no-op cases (route: delegated direct).
- [ ] T2 Wire it into every write path and invert the #52 tests; one informational line when
  something was removed (route: delegated direct).

## Acceptance criteria

- A legacy file loses exactly the provider line and the two sub-blocks on the next write; the
  rest is byte-identical; a clean file is untouched.
- After cleanup `tasks.provider` is absent and resolves as for a new config.
- `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

## Progress

- 2026-10-02: branch created, document written.

## Next step

T1, T2.
