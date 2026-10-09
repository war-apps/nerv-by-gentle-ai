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
- [ ] T2 — display names on user surfaces (`--print`, wizard, docs roles
  table, agent descriptions) and remaining orchestration/command/docs prose
  renames, plus CHANGELOG (route: delegated, same writer).

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
- Commit: recorded in the next commit (amend not allowed).

## Next step

T2: display names on user surfaces, remaining docs/bench prose, CHANGELOG.
