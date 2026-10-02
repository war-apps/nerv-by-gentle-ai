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

- [x] T1 Cleanup in the config layer: removed-provider list, pure function that strips the line
  and sub-blocks from YAML bytes (byte-preserving elsewhere), tests for inline/multi-line/
  comment/EOF/no-op cases (route: delegated direct).
  RED `7a1633b`: build failed, `undefined: config.RemovedTaskProviders` and
  `config.StripRemovedTaskProviders`. GREEN `ae0e388` (`internal/config/legacy.go`, 207 lines with tests).
- [x] T2 Wire it into every write path and invert the #52 tests; one informational line when
  something was removed (route: delegated direct).
  RED `017c513`: build failures (`Save` returned 3 values, `Result.Removed` undefined) and
  `TestRunConfigure_Set_ReportsRemovedLegacyProviders` failing. GREEN `4b31fa6`: the cleanup
  lives in `configstore.Store.Save`, the one write point behind `configure.Set`/`SetModel` (and
  so the wizard's user-config section) and the wizard's models section; `Result.Removed`
  (`removed`, omitted when empty) and `configure.RemovedLine` carry the one-line report.
  `--init-repo` only creates a missing project file and never rewrites an existing one, and
  `--print`/`install` only read, so no cleanup runs there.

- [x] T3 (added 2026-10-02, user decision "vamos por la recomendacion") `nerv configure
  --init-repo` cleans an existing project `nerv.yaml` (with backup, the informational Removed
  line, and no rewrite when nothing needs cleaning); a missing file is created as today. Project
  files are otherwise never rewritten, so this is their only cleanup path (route: delegated
  direct).
  RED `8bf53fe`: `TestInitRepo_ExistingLegacyConfig_IsCleanedWithBackup` and
  `TestRunConfigure_InitRepo_CleansExistingLegacyProjectConfig` failing (file untouched, nothing
  in `Removed`/`Backup`). GREEN `8228784`: `InitRepo` strips an existing project file through
  `config.StripRemovedTaskProviders` and writes it with `atomicfile.Save` and the store's
  `bak-configure-` backup; a clean file stays untouched with today's warning; a missing file is
  created as before. `--repo-provider` with an existing file still changes nothing but the
  cleanup. The #52-era test pinning "left untouched" was inverted in the same commit. Docs
  `78c0173`.

## Acceptance criteria

- A legacy file loses exactly the provider line and the two sub-blocks on the next write; the
  rest is byte-identical; a clean file is untouched.
- After cleanup `tasks.provider` is absent and resolves as for a new config.
- `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

## Progress

- 2026-10-02: branch created, document written.
- 2026-10-02: T1 and T2 done; `go test ./...`, `go vet ./...`, `gofmt -l .` clean. A no-op write
  (nothing to change) still touches nothing, so a legacy file is cleaned on its next real write.
- 2026-10-02: native review accepted five in-scope findings (R3): preceding-block comment
  swallowed, non-block value forms orphaning child lines, one-line flow `providers` map not
  stripped, no-op save writing for cleanup, `removed` JSON field unasserted. Route: delegated
  direct (one writer). RED `e03545e`: 13 new `TestStripRemovedTaskProviders` cases and
  `TestStore_Save_UnchangedLegacyDocumentIsNoop` failing (the two new cmd tests passed already:
  they pin existing behavior). GREEN `bdb2fc0`: only comment lines at the key's own indentation
  directly above go with it; a removed key drops every deeper-indented line (block scalar,
  anchor, tag, plain continuation), keeping the trailing separating blank; a one-line flow
  `providers` map loses its removed entries with their separators; `Save` writes only when the
  requested document differs from the original, then strips. Documented limit: a multi-line flow
  `providers` map is left untouched and reports nothing (code comment and test).
  `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

- 2026-10-02: T3 done; `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

- 2026-10-02: native review of `e9c481f..56344cd` (hardening + T3, medium, reliability lens)
  approved and acknowledged. Accepted both warnings: R3-flow-quote-apostrophe (a quote only opens
  a quoted scalar right after `{`, `[`, `,` or `:`, in both `stripFlowProviders` and `flowDepth`)
  and R3-initrepo-read-error-now-fatal (`--init-repo` on an existing but unreadable config warns
  and skips the cleanup, as before). RED `test(config): cover apostrophes in flow scalars and an
  unreadable project config` (observed: jira entry kept, `is a directory` error), GREEN `fix(config):
  treat in-scalar quotes as text and keep --init-repo non-fatal on unreadable configs`.
  `go test ./...`, `go vet ./...`, `gofmt -l .` clean.

## Next step

PR to `develop` (about 1,500 authored lines: stacked slices, cached strategy).
