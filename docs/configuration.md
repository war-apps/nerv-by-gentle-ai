# Configuration schema

Every Nerv by Gentle-AI setting lives in one of two files with the same
schema: user scope `~/.config/ai-workflow/config.yml` (personal
defaults, never committed) and project scope `<repo>/.ai-workflow.yml`
(committed). Project overrides user key by key; a missing key falls back
to the user file, then to the built-in default. `~/.config/ai-workflow` is
`os.UserConfigDir()`: `$XDG_CONFIG_HOME` when set on Linux,
`~/Library/Application Support` on macOS and `%AppData%` on Windows. The
repository's `.nerv/` folder is runtime only (artifacts, the orchestrator
lock under `.nerv/locks/`, logs) and ignores itself with a `.nerv/.gitignore`
of `*`. Each run's ODD feature document (`odd/tasks/<feature>.md`) and NERV
run folder (`odd/<feature>/nerv/`) are committed per `artifacts.commit`.

The files used before, `~/.claude/nerv/nerv.yaml` and
`<repo>/.nerv/nerv.yaml`, are no longer read. When only an old file exists,
every command behaves as unconfigured and prints one line naming it and
saying that `nerv migrate` moves it.

```yaml
# ~/.config/ai-workflow/config.yml (user) and <repo>/.ai-workflow.yml (project): same schema
enabled: true                       # project scope only: activates NERV in this repo
skills:                             # stacks per consuming role; names must exist in .atl/skill-registry.md
  testing: [tdd, playwright-best-practices]                        # ritsuko, kaworu, maya
  code: [dotnet-best-practices, typescript-best-practices]         # pilots
  best-practices: [best-practices, solid-principles, clean-code-guard]  # balthasar
  architecture: [hexagonal-architecture, c4-architecture]          # melchior
  audit: [security-review, clean-code-guard]                       # audit passes, melchior audit
models:                             # per-role model and effort; project overrides user, key by key
  misato: { model: fable, effort: high }
  melchior: { from: jd-judge-b }     # inherit gentle-ai's assignment for that phase (state.json)
  aoba: { model: sonnet, effort: low }
critical_paths: [auth/, payments/, migrations/, infra/]            # Hyuga auto-critical
artifacts:
  commit: at-close                  # when odd/tasks/<feature>.md and odd/<feature>/nerv/ are committed: with-change | at-close | never (default: at-close)
git:
  base_branch: develop              # default base for the worktree offer
  worktree: ask                     # ask | always | never
  worktree_pattern: ".claude/worktrees/{slug}"   # where task worktrees are created; {slug} {branch} {prefix} {id} {repo}
                                                  # `nerv configure`'s wizard offers default / herdr / custom; herdr
                                                  # reuses its own [worktrees] directory as <directory>/{repo}/{slug}
  branch_pattern: "feature/{prefix}-{id}-{slug}"   # {prefix}/{id} come from the run.started task_ref (tw-123)
  commit_ref_pattern: "({PREFIX}-{id})"
tasks:                              # read only by the optional task plugin (task@nerv); core ignores it
  provider: teamwork                # provider for bare task ids; "none" turns the task plugin off in this scope
  ask_when_missing: true            # at run.started with no task, the dispatcher asks for one
  subtasks_per_wave: false          # whether wave.started creates one subtask per FULL wave
  events:                           # per-event switches, on (default) | off
    verification.started: off       # e.g. leave the testing stage move manual
  providers:                        # one block per installed provider; keys come from its provider.yaml
    teamwork:
      assignee_id: 686035           # user scope
      project_id: 1271726           # project scope
      tasklist_id: 3951970          # project scope
      stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }
  sources:                          # extra work sources for listings (replaces ~/.claude/work/sources.md)
    - { name: erp-proveedores, type: google-sheets, ... }
ship:                               # ship plugin; project scope only, never merged with the user file; see release-commands.md
  projects:
    - { name: api, path: ., build: { mode: compose, file: docker-compose.yml, images: ["ghcr.io/acme/app:${version}"] } }
deploy:                             # deploy plugin; see deploy.md
  host: github
  targets:
    - { name: staging, provider: ssh-compose, config: { environment: staging } }
```

`ship:` configures the ship plugin (`nerv version next` and `nerv ship`,
also `/ship:version`, `/ship:setup` and `/ship:build`): the version engine
(branches and channels, release rules, note keywords, tag format,
changelog), the projects and their dependencies, and each project's
`build.mode` (`compose` or `script`; there are no presets) with its
stages, hooks and coverage. Its full reference is
[release-commands.md](release-commands.md#configuration-ship).

`deploy:` configures the deploy plugin (`nerv deploy`, `/deploy:run`,
`/deploy:setup`): the git host provider, the targets with their provider
config, and the release commit's message and remote. Provider keys with
`scope: user` (credentials, local paths) go in the user file under
`deploy.providers.<provider>.`. Its full reference is
[deploy.md](deploy.md#configuration).

The old `release:` and `pipeline:` sections are no longer read: a command
that finds one refuses and points to `/ship:setup` (or `/deploy:setup`).
`nerv migrate` translates them, and `nerv ship setup --import` imports a
semantic-release config; see
[release-commands.md](release-commands.md#migrating-from-release-and-semantic-release)
and [pipeline.md](pipeline.md).

`models:` assigns a per-role `model` and `effort` override, with project
overriding user key by key like every other section — each entry is an
inline map, one role per line, with `model`, `effort`, or `from` keys;
`from: <gentle-ai-phase>` inherits that phase's `model`/`effort` from
`~/.gentle-ai/state.json`'s `claude_phase_assignments`, and an explicit
`model`/`effort` on the same line wins over the inherited one. A role
absent from both `models:` files keeps the plugin default (see
[integration.md](integration.md#roles)). `model` and `effort` are
applied differently, because Claude Code itself treats them differently:
`model` is a per-call launch parameter, so Ikari reads the resolved
`models:` map (both scopes merged) and passes it to every agent launch —
it takes effect immediately, no reinstall needed. `effort` is honored only
from the launched agent file's own frontmatter, so it can only be applied
by rewriting the cached agent files themselves: `nerv apply-models` reads
`models:` from the **user-scope** file only (`~/.config/ai-workflow/config.yml`)
and rewrites `model:`/`effort:` in each affected `<role>.md` under the
plugin cache. Project-scope `models:` therefore applies to `model` only —
`effort` is not applicable at project scope, since project config cannot
reach into a user's local plugin cache. `nerv install` runs this same apply
step automatically at the end of its own cache refresh.
`/nerv:status` reports the resolved `models:` table (both scopes merged)
and warns when the cached agent frontmatter has drifted from it, so a
pending `nerv apply-models` run is visible without inspecting the cache by
hand.

## `tasks:` (task plugin)

Core never reads `tasks:`. The block belongs to the optional task plugin
(`task@nerv`) and its provider plugins, such as `task-teamwork@nerv`. The
full key reference is in the task plugin's contract
(`plugins/task/skills/task/SKILL.md`, section 1).

- **Per-event switches.** `tasks.events.<event>: off` stops one pipeline
  event from reaching any provider, per repository (project file) or for
  everyone (user file). A run can also switch events off for itself; the
  dispatcher then answers `skipped` with `reason: switched off`.
- **Provider settings.** `tasks.providers.<name>.*` holds the keys that the
  provider's `provider.yaml` declares under `config`, each with its scope
  (`user` or `repo`), type, default and prompt. `tasks.providers.<name>.enabled:
  false` keeps a provider installed but stops sending it events.
- **Setting them.** When `task@nerv` is enabled, the `nerv configure`
  wizard shows a "Task providers" section that prompts each enabled
  provider's scalar keys, plus `tasks.provider` when more than one
  provider is enabled. `nerv configure --init-repo <path>
  --repo-provider-config <provider>.<key>=<value>` (repeatable) writes
  `tasks.providers.<provider>.<key>` into the repository's
  `.ai-workflow.yml`.

**Migration.** A `tasks:` block written by an older release keeps working:

- with the task plugin installed, it is read under the layout above, so the
  Teamwork keys under `tasks.providers.teamwork.*` keep their paths;
- without it, core ignores the block and never deletes it;
- copies of the `/task:*` commands that an older release put in
  `~/.claude/commands/task/` are reported by `nerv install` and removed only
  after you confirm. The `task` plugin now serves those commands itself.

### Roles and their gentle-ai equivalents

Every role below can be overridden under `models:`. A role is keyed by its
ID — the short lowercase slug that is also its agent name, its `nerv:<id>`
subagent type and its `models:` key — and shown with its full character
name (the Name column; `display_name` on each `nerv configure --print`
models row, the NAME column of the wizard's models table).

Two roles and one group were renamed: `melchor` is now `melchior`,
`kaji-audit` is now `gendo`, and the group `kaji-passes` is now
`audit-passes`. The old names stay accepted as legacy aliases wherever you
type or store a role: a `models.melchor` or `models.kaji-audit` key in
either configuration file keeps applying to the renamed role (`nerv configure --print`,
`nerv apply-models` and the wizard read it as `melchior` / `gendo`),
`nerv configure --set-model melchor=...` and the wizard's role prompt
accept the old role and group names, and any write stores the new ID once.
When a file holds both the old and the new key for a role, the new key
wins and the old one is dropped on the next write.

The gentle-ai
equivalents are the gentle-ai v4 agents whose duties the role covers (the
`jd-*` agents and the native `review-*` agents); roles with none show none.
Every gentle-ai v4 agent (the `GentleAIV4Agents` list in
`internal/config/models.go`) is claimed by at least one role, and a test
fails if one is left unclaimed. No two roles share an equivalent except the
pilots, which claim `jd-fix-agent` because fix routing goes through the
owning pilot, with kaworu writing the RED test first. Equivalents are informational only and never a `from:<phase>` value:
the native review agents (`review-*`) are not keys of
`claude_phase_assignments`. Only the `jd-judge` equivalents (melchior's
`jd-judge-b`, balthasar's `jd-judge-a`) double as a `from:` suggestion,
exposed separately as `from_phase`; only melchior and balthasar carry one.
The roles whose equivalents are only native `review-*` agents
(`gendo`, and `fuyutsuki` for his read-only refute mode) and the
pilots get no suggestion. casper and kaji (the compiler) claim no
equivalent, so no role judges work it authored or compiles. Neither changes how a model is
resolved. The same data is shown in the wizard's models table, in
`/nerv:configure` and `/nerv:status`, and as `display_name` / `purpose` /
`gentle_ai_equivalents` (an array) / `from_phase` on each
`nerv configure --print` models row. `gentle_ai_equivalent` carries the same
list joined with `, ` for readers of the original single-string field. Roles are addressable by group: `magi` (the three voters),
`pilots` (the implementers), `audit-passes` (the audit compiler and its
pass) and `all`.

| Role | Name | Group | Purpose | gentle-ai equivalent | Default model / effort |
|---|---|---|---|---|---|
| `misato` | Misato Katsuragi | | authors the plan (specs, design and tasks in the ODD document) | none | fable / high |
| `ritsuko` | Ritsuko Akagi | | intelligence, test planning, end-of-run docs | none | opus / high |
| `hyuga` | Makoto Hyuga | | task criticality, dependency waves, tracking | none | sonnet / medium |
| `melchior` | Melchior-Magi 1 | `magi` | MAGI vote: structure and security; security audit | `jd-judge-b`, `review-risk` | fable / high |
| `balthasar` | Balthasar-Magi 2 | `magi` | MAGI vote: software principles; readability audit | `jd-judge-a`, `review-readability` | sonnet / medium |
| `casper` | Casper-Magi 3 | `magi` | MAGI vote: process and documentation | none | sonnet / medium |
| `fuyutsuki` | Kōzō Fuyutsuki | | governance veto on new skills/scripts/commands; refutes severe audit findings | `review-refuter` | sonnet / medium |
| `kaworu` | Kaworu Nagisa | `pilots` | writes the failing tests first | `jd-fix-agent` | sonnet / medium |
| `shinji` | Shinji Ikari | `pilots` | backend pilot | `jd-fix-agent` | sonnet / medium |
| `asuka` | Asuka Langley Sohryu | `pilots` | frontend pilot | `jd-fix-agent` | sonnet / medium |
| `rei` | Rei Ayanami | `pilots` | data pilot (persistence, observability) | `jd-fix-agent` | sonnet / medium |
| `toji` | Tōji Suzuhara | `pilots` | infrastructure pilot (CI/CD, containers) | `jd-fix-agent` | sonnet / medium |
| `maya` | Maya Ibuki | | quality gate (tests, lint, build) | none | sonnet / medium |
| `kaji` | Ryoji Kaji | `audit-passes` | audit compiler | none | opus / high |
| `gendo` | Gendo Ikari | `audit-passes` | audit pass: test coverage, reliability, correctness, resilience, performance | `review-reliability`, `review-resilience` | sonnet / medium |
| `aoba` | Shigeru Aoba | | commits, PRs and run telemetry | none | sonnet / low |

### Configuring models and effort

The wizard's **Models** section (see "Setup" in [the README](../README.md))
prints the resolved table (role, model, effort, source — `override`,
`gentle-ai:<phase>`, or `default`; `gentle-ai:<phase> (missing; plugin
default)` when `~/.gentle-ai/state.json` has phase assignments but not
that phase — plus each role's full name, what it does and its gentle-ai
equivalent) and a
legend for the group shortcuts, then lets you
edit it role by role, or
by group (`magi`, `pilots`, `audit-passes`, `all`), until you type `done`.
For each role it asks for a model (`sonnet`/`opus`/`haiku`/`fable`/
`inherit`, a custom `claude-...` id, or `from:` a gentle-ai phase listed
from `~/.gentle-ai/state.json`, with the role's own `from_phase` listed
first and marked `(equivalent)`; roles without one get no mark) and an effort (`low`/`medium`/`high`/
`xhigh`/`max`), with Enter keeping the current value; `reset <role|group>`
clears an override back to the plugin default. Confirming writes the block
to the user-scope `config.yml`, after backing up the file to
`<path>.bak-models-<yyyyMMdd-HHmmss>`, then offers to run
`nerv apply-models` immediately.

Non-interactively, `nerv configure --set-model role=model[/effort]` (or
`role=from:<gentle-ai-phase>`, or `role=default` to clear an override)
applies one role's override at a time to the same user-scope file; pass it
repeatedly for a batch. This always edits the user-scope file — project
scope has no dedicated command, since project-scope `models:` only ever
affects `model` (see above). Edit the project `.ai-workflow.yml` `models:`
block by hand for that case, using the same inline-map syntax and `from:`
behavior described above.

You can also skip both the wizard and `--set-model`, and edit the
`models:` block by hand in either configuration file, then run
`nerv apply-models` yourself. Either way, restart Claude Code afterwards
for the change to take effect.
