# Role display names: full character names, short stable IDs

## Objective

Give every NERV role its full Evangelion character name for display, while
role IDs stay short lowercase slugs usable as agent names, `nerv:<id>`
subagent types and `nerv.yaml` keys.

## Why

User decision (2026-10-09). Full names cannot be IDs: Claude Code agent names
must be lowercase letters and hyphens, and `nerv.yaml` `models:` keys are the
role IDs, so renaming every ID would orphan every user override.

## Scope

| ID (new) | Old ID | Display name |
|---|---|---|
| misato | | Misato Katsuragi |
| ritsuko | | Ritsuko Akagi |
| hyuga | | Makoto Hyuga |
| melchior | melchor | Melchior-Magi 1 |
| balthasar | | Balthasar-Magi 2 |
| casper | | Casper-Magi 3 |
| fuyutsuki | | Kōzō Fuyutsuki |
| kaworu | | Kaworu Nagisa |
| shinji | | Shinji Ikari |
| asuka | | Asuka Langley Sohryu |
| rei | | Rei Ayanami |
| toji | | Tōji Suzuhara |
| maya | | Maya Ibuki |
| kaji | | Ryoji Kaji |
| gendo | kaji-audit | Gendo Ikari |
| aoba | | Shigeru Aoba |

- Group `kaji-passes` is renamed `audit-passes` (members `kaji`, `gendo`).
- Old IDs `melchor`, `kaji-audit` and old group `kaji-passes` stay accepted
  as legacy aliases wherever a user can type or store them (`nerv.yaml`
  `models:` keys, `--set-model`, wizard), resolved to the new ID; a write
  stores the new ID.
- Equivalents, from_phase, duties and pass order are unchanged.

## Constraints

- No orphaned overrides: an existing `models.melchor` or `models.kaji-audit`
  keeps applying to the renamed role.
- History (released CHANGELOG entries, other features' odd docs) is not
  rewritten.
- Generated artifacts in English.

## Tasks

- [x] T1 — catalogue: `DisplayName` field, ID renames, group rename, legacy
  alias resolution, agent files renamed, every test that pins them, and the
  prose the root tests pin (route: delegated, writer trigger: 2+ non-trivial
  files).
- [x] T2 — display names on user surfaces (`--print`, wizard, docs roles
  table, agent descriptions) and remaining orchestration/command/docs prose
  renames, plus CHANGELOG (route: delegated, same writer).

- [x] T3 — review follow-ups (user authorized 2026-10-09): Kaji falls back
  to legacy pass file names (`pass-melchor-round-N.json`,
  `pass-kaji-audit-round-N.json`) when the new ones are missing in a round
  started before the upgrade; test the layered user+project merge and
  `apply-models` with mixed legacy and new keys (route: delegated, writer
  trigger: prose + Go tests).

## Acceptance criteria

- `go test ./...`, `go vet ./...` pass; `gofmt -l .` clean.
- No live reference to `melchor`, `kaji-audit` or `kaji-passes` outside
  history, the alias table and its tests.
- A `nerv.yaml` holding `melchor:` / `kaji-audit:` overrides resolves them to
  `melchior` / `gendo`.

## Delivery

Branch `feat/role-display-names` from 555add4. Forecast about 450 authored
changed lines (29 files hold 144 references to the renamed IDs, plus the
alias code and tests). Strategy: `single-pr`, as in the two previous role
features; a mechanical rename does not split cohesively.

## Progress

Branch created from 555add4.

### T1 (delegated writer)

- Alias design: `config.LegacyRoleAliases()` (`melchor` -> `melchior`,
  `kaji-audit` -> `gendo`), `config.LegacyGroupAliases()` (`kaji-passes` ->
  `audit-passes`) and `config.CanonicalRole`. `ReadModelsOverrides` keys
  every entry by its canonical ID (the new key wins over a legacy one in
  either order), so `ModelTable`, `apply-models`, `--print` and the wizard
  see the renamed role; `SetModel` canonicalises the `--set-model` role
  before validating, so every write stores one entry under the new ID;
  `ResolveRoleTarget` (wizard) resolves legacy role and group names.
  `ValidateRole` stays strict on current IDs.
- `RoleInfo.DisplayName` added with the scope table's names.
- Agent files renamed with `git mv` (`melchior.md`, `gendo.md`); every
  plugin ID reference, pass file name (`pass-gendo-round-N.json`) and
  character prose renamed; docs roles table rows renamed (pinned by the
  root test). New guard `TestPlugin_NamesNoLegacyRoleIDs`.
- RED: after the test-first edits `go test ./...` failed to build
  `internal/config` (undefined `DisplayName`, `CanonicalRole`,
  `LegacyRoleAliases`, `LegacyGroupAliases`) and failed root prose tests,
  `TestSetModel_LegacyAliases`, `TestSetModel_FromPhase_Display`,
  `TestPrint_ModelsRowsCarryPurposeAndEquivalent`,
  `TestPluginDefaults_RealEmbeddedFS_16RolesAobaSonnetLow` and three
  wizard table tests.
- GREEN: `go test ./...` ok (all packages), `go vet ./...` clean,
  `gofmt -l .` empty. `TestRun_ModelsSection_AcceptsLegacyNamesAndWritesNewIDs`
  and the plugin guard were added after GREEN (the guard was checked to
  fail on an injected `nerv:kaji-audit` line).
- Size: 29 files changed, 571 insertions(+), 189 deletions(-) (above the
  ~400 heuristic: the catalogue map realigns and the alias tests are
  table-driven; no split, as planned for `single-pr`).
- Commit: `31a6c90` feat(config)!: rename melchor and kaji-audit, add role
  display names (30 files changed, 685 insertions(+), 189 deletions(-),
  feature doc included).

### T2 (same delegated writer)

- `--print` models rows carry `display_name`; the wizard's models table has
  a NAME column, padded in runes (`fmt`'s `%-*s` pads by bytes and would
  misalign "Kōzō Fuyutsuki" / "Tōji Suzuhara"); `/nerv:configure` and
  `/nerv:status` tables show the name; `docs/configuration.md` roles table
  gained a Name column plus a paragraph on IDs vs names and the legacy
  aliases (new key wins); every agent's `description:` starts with
  "<display name>, " and its first heading with "<display name> — ".
- Remaining `docs/integration.md`, `bench/journeys.md` and
  `docs/configuration.md` references renamed; CHANGELOG `[Unreleased]`
  gained Added (display names) and Changed (renames, aliases, group).
- RED: `go test ./...` failed `TestDocsRolesTable_MatchesCatalogue` (6
  cells), `TestProse_NamesNoLegacyRoleIDs` (bench/docs),
  `TestAgents_IntroduceTheirDisplayName`,
  `TestPrint_ModelsRowsCarryPurposeAndEquivalent` (`display_name`) and
  `TestRun_ModelsSection_TableShowsPurposeEquivalentAndGroupLegend` (NAME).
- GREEN: `go test ./...` ok, `go vet ./...` clean, `gofmt -l .` empty.
- Size before the doc update: 27 files changed, 270 insertions(+),
  131 deletions(-).
- Commit: `8a236e5` docs: show role display names and finish the
  melchior/gendo rename (28 files changed, 297 insertions(+),
  134 deletions(-), feature doc included).

### Review

- Native review of 555add4..d227dba (medium, reliability lens, user
  granted): approved and acknowledged. Two informational suggestions:
  R3-001 no test covers `apply-models` or the user/project merge with mixed
  legacy and new keys; R3-002 an audit round in flight across the upgrade
  keeps pass files under the old names (`pass-melchor-*`,
  `pass-kaji-audit-*`), which the compiler would report missing.
- Parent spot check: `go test ./...` ok.

### T3 (delegated writer)

- Pass-file fallback: `nerv-artifacts.md` gained "Legacy pass names", the
  one canonical statement: a round started before the rename may hold
  `pass-melchor-round-N.json` / `pass-kaji-audit-round-N.json` (and the
  matching `audit-pass-*` Engram keys); Kaji, or Ikari on resume, reads the
  legacy name only when the new one is absent and counts it as the
  `melchior` / `gendo` pass whatever its `pass` field says. `kaji.md`
  points to it. The legacy-name guard now tolerates exactly those tokens,
  only in that file (checked to still fail on an injected
  `pass-melchor-round-N.json` in `kaji.md`).
- RED: `TestAuditPasses_LegacyPassNameFallbackDocumented` failed (5 missing
  strings in `nerv-artifacts.md`, no pointer in `kaji.md`); GREEN after the
  prose.
- Layered merge: no Go code merges the user and project `models:` blocks;
  Ikari does it in prose (`SKILL.md` "Model and effort per role", which
  `/nerv:status` reuses) and `apply-models` reads the user scope only. The
  Go tests are characterization tests, green on first run (the one initial
  failure was the test's own assertion tripping on a trailing frontmatter
  comment, not a product bug):
  `TestReadModelsOverrides_LayeredScopesMeetUnderCanonicalIDs` (project
  legacy over user new, project new over user legacy, user legacy kept
  when the project is silent) and
  `TestApplyModels_LegacyRoleKeysApplyToRenamedAgents` (legacy user key
  rewrites `melchior.md`; new key wins over legacy in one file).
- Bug found in prose: Ikari's per-launch model resolution looked up only
  `models.<current id>`, so a project-scope legacy override (the only way a
  project file sets a model) was ignored. RED:
  `TestOrchestrator_ModelResolutionReadsLegacyRoleKeys` failed; fixed by
  one sentence in `SKILL.md` (read the legacy key per file when the current
  ID has no entry, before falling back to the next file); GREEN. The guard
  tolerates `models.melchor` / `models.kaji-audit` only in `SKILL.md`.
- Side observation (out of scope): `apply-models` keeps an agent file's
  trailing `model:` comment, so `melchior.md` overridden to haiku still
  says "alias for Claude Fable 5.1".
- Checks: `go test ./...` ok, `go vet ./...` clean, `gofmt -l .` empty.
- Commits: `af0bd9b` fix(audit): let kaji read pre-rename pass files (3
  files, +68/-1); `0bfdde8` test(config): cover layered models scopes and
  apply-models with legacy role keys (2 files, +104); `97b2920`
  fix(orchestrator): read legacy models keys in each scope before layering
  (2 files, +33/-12).

### T3 review

- The cumulative candidate from 3f53343 (2786 lines) stopped with
  `lens_context_budget_exceeded` (terminal, no authority created).
- Native review of the T3 slice bef2f50..adc8829 (medium, reliability lens,
  user granted): approved and acknowledged. Advisory findings:
  R3-applymodels-key-order (WARNING: the new-key-wins case only puts the new
  key first), R3-layering-reimplemented-in-test (the layering test merges in
  the test body, not through product code), R3-prose-presence-only (the
  orchestrator prose test only checks the legacy keys appear).

## Next step

User decides on push and PR (`single-pr`).
