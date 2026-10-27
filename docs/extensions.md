# Extensions and providers

NERV's optional add-ons come in two kinds, and each kind speaks one
contract. Each one ships as its own Claude Code plugin:

- An **extension** (`task`, `release`, `deploy`) adds a capability to the
  core. It talks to the core through its `extension.yaml`.
- A **provider** (`task-teamwork`, `release-conventional-commits`,
  `release-docker-compose`, `release-bash`, `release-github`,
  `release-ssh-compose`, `release-script`) implements one slot of an extension: a tracker, a
  version engine, a build, a git host, a deploy target. It talks to its extension through its `provider.yaml`.

Every extension uses the same `extension.yaml` schema, and every provider,
of every extension, uses the same `provider.yaml` schema. Both share the
config key schema, the name rules, `contract_version` and the
request/response envelope below. `nerv` checks every manifest it ships, and
`nerv install` refuses a tree whose manifests do not hold.

## Layout

Each extension and each provider is a top-level folder of `plugins/` with a
`.claude-plugin/plugin.json` whose `name` is the folder's name, and exactly
one manifest at its root:

| Folder | Manifest | Example |
|---|---|---|
| `<extension>` | `extension.yaml` | `plugins/release/extension.yaml` |
| `<extension>-<provider>` | `provider.yaml` | `plugins/release-github/provider.yaml` |

A provider requires its extension: `nerv install --with release-github`
also installs `deploy` (and `release`, which `deploy` requires). An
extension with `providers: bundled` brings every provider it ships with
it: installing `release` installs `release-conventional-commits`,
`release-docker-compose` and `release-bash`, never asked; with
`providers: suggested` (task) each provider is offered after its
extension. A provider
whose extension is not shipped, whose folder is not
`<extension>-<provider>`, or that does not fit its slot is refused.

## `extension.yaml` (core and extension)

```yaml
name: deploy                   # the folder's name
contract_version: 1
description: Releases and deploys a release artifact   # optional free text; nerv does not read it
suggest_order: 3               # nerv install suggests task (1), release (2), deploy (3); 0 or unset comes last, by name
providers: bundled             # bundled: every provider shipped is installed with the extension, never asked; suggested (the default): each is offered after it
requires: [release]            # extensions installed with this one
runtime: exec                  # exec: the core runs argv; agent: the core launches the agent entry
argv: ["{nerv}", deploy]       # runtime exec; runtime agent names `agent: <plugin>:<agent>` instead
events: []                     # core events the extension subscribes to
config: []                     # the extension's own keys, under <extension>.<key>
provider_fields: []            # extra top-level fields its providers' provider.yaml carry
slots:
  - name: host
    cardinality: one           # one | many | per-target
    default: github            # optional: the provider used when the configuration names none
    events: {}                 # optional: each subscribed event -> the slot operations a provider handling it needs
    operations:
      releaseExists: required  # required | optional
      createRelease: required
```

- **`description`** is optional free text about the extension. nerv does
  not read it: what Claude Code and the marketplace show is the
  `description` of the extension's `.claude-plugin/plugin.json`.
- **`events`** are core events: `run.started`, `verification.started`,
  `audit.ranked`, `issue.deferred`, `wave.started`, `run.closed`,
  `run.halted`. The orchestrator hands each one to every installed
  extension that lists it; an extension with `runtime: agent` is reached by
  launching its `agent` entry. Today only `task` subscribes, through
  `task:dispatcher`.
- **`providers`**: how `nerv install` installs the extension's providers:
  `bundled` installs every provider the tree ships for it together with
  the extension, without a prompt (each listed as `Adding <provider>
  (bundled with <extension>)`); `suggested`, the default, offers each one
  after its extension. Any other value is refused.
- **`cardinality`**: `one` provider at a time (deploy's `host`, release's
  `version`), `many` active at once (task's `provider`), or one per
  configured target (`per-target`: deploy's `target`, release's `build`,
  one per project).
- **`default`**: the provider used when the configuration names none
  (release's `version`: `conventional-commits`). It does not decide what is
  installed; `providers` does.
- **`operations`** lists every operation of the slot. A provider declares
  each one; only an `optional` one may be `not_implemented`.
- **slot `events`** (optional) maps each core event a provider of the slot
  may handle to the slot operations that event needs. Its keys must be
  events the extension subscribes to, and its operations must be
  operations of the slot. A provider that lists an event in its own
  `events` must implement every operation the event needs (task:
  `audit.ranked` needs `comment`).
- **`provider_fields`** are `{name, type, required}` entries (types as for
  config keys). They let an extension give its providers top-level fields
  the common schema does not have, such as task's `task_ref_prefix` and
  `stages`. A provider field must not reuse a common field
  name. The extension's own code gives them their meaning.

The shipped extensions:

| Extension | Runtime | Requires | Providers | Events | Slots |
|---|---|---|---|---|---|
| `task` | agent, `task:dispatcher` | | suggested | the 7 core events | `provider` (many): the 13 task operations, all optional |
| `release` | exec, `nerv release` | | bundled | | `version` (one, default `conventional-commits`): `next`, `branch` required, `validate`, `import` optional; `build` (per-target, one per project): `plan` required, `validate` optional |
| `deploy` | exec, `nerv deploy` | `release` | bundled | | `host` (one): `releaseExists`, `createRelease`; `target` (per-target): `preflight`, `publish`, `verify` optional, `deploy` required |

### The release slots

`nerv release`, `nerv version next` and `nerv release setup` call the
release providers; all three shipped ones are native exec providers
(`nerv provider release-<name> <operation>`). The payloads and answers are
`internal/releaseplan/contract.go`'s:

| Slot, operation | Payload | Answer (`data`) |
|---|---|---|
| `version` `validate` | `projects` (the `release.projects[]` identity keys: `name`, `path`, `shared_paths`, `scopes`, `tag_format`, `changelog`, `depends_on`) | `failed` with `{problems: [...]}`, each named by its path in `.ai-workflow/config.yaml` |
| `version` `next` | `dir` (the release clone), `branch`, `projects`, `only` (the projects asked for), `version` (`nerv release --version`) | `{results: [...]}`: per project, the outputs `nerv version next` prints, the release type, the CHANGELOG file and section |
| `version` `branch` | `dir`, `branch`, `projects` | `{name, prerelease, channel}` |
| `version` `import` | `dir` | `{source, config, warnings}`: `config` is the provider's keys as a YAML mapping, which `nerv release setup --import` writes under `release.providers.<engine>`; `skipped` when there is nothing to import, `blocked` with `{manual}` for a configuration to move by hand |
| `build` `validate` | `project`, `at` (`release.projects[i]`), `stages` (the stages the project writes commands for) | as `version` `validate` |
| `build` `plan` | as `validate` | `{steps: [{stage, name, exec}], image_prefix, stage_env, paths}`: the provider's own commands, run without a shell before the project's command of the stage; the env added under the project's stage commands; the files it reads, which `nerv release` checks on origin's branch |

Every call's `config` is the provider's keys, resolved: the version
engine's `release.providers.<provider>`; a build provider's
`release.providers.<provider>` overridden by
`release.projects[].providers.<provider>`; each checked against the
provider's `config` (an undeclared key or a value of another type is
refused) with the scalar defaults applied. `release.version` selects the
engine (the slot's default when unset) and `release.build` /
`release.projects[].build` the build; an unknown provider is refused,
naming the slot's providers.

| Provider | Slot | Keys | Operations |
|---|---|---|---|
| `conventional-commits` | `version` | `branches`, `release_rules`, `note_keywords`, `tag_format`, `repository_url`, `changelog` | all four: internal/versioning, semantic-release 24 with the `conventionalcommits` preset |
| `docker-compose` | `build` | `file` (required), `test_service` | `validate` (the file, the service name, no `build`/`test`/`package` stage command), `plan` (`docker compose -f <file> build`, `run --rm <test_service>`) |
| `bash` | `build` | `shared_host` | `validate`, `plan` (no command of its own; with `shared_host` the build-server environment for `restore`, `lint`, `build`, `test` and `package`) |

## `provider.yaml` (extension and provider)

```yaml
name: ssh-compose              # the folder is <extension>-<name>
extension: deploy
slot: target                   # a slot of the extension's extension.yaml
contract_version: 1
runtime: exec                  # exec or agent
requires: [docker, ssh]        # exec: the executables it needs on PATH
config:                        # key, scope (user | repo), type, default, prompt
  - key: environment
    scope: repo
    type: string
    prompt: The environment
operations:                    # every slot operation
  preflight:
    - argv: ["{nerv}", provider, release-ssh-compose, preflight]
  publish:
    - argv: [docker, push, "{item}"]
      for_each: images
  deploy:
    - argv: ["{nerv}", provider, release-ssh-compose, deploy]
  verify: not_implemented
```

An agent provider names its MCP tools and a procedure file per operation:

```yaml
name: teamwork
extension: task
slot: provider
contract_version: 1
runtime: agent
mcp:                           # agent: the server and the exact tools its procedures call
  server: teamwork
  tools: [teamwork_get_task]
operations:
  getTask: procedures/get-task.md
  comment: not_implemented
task_ref_prefix: TW            # task's provider_fields
events: [run.started]          # the core events it handles, checked against its slot's events
```

- **Runtime `exec`**: each operation is a list of steps that Go runs
  directly, never through a shell. `argv[0]` is `{nerv}` or an executable
  listed in `requires`, and an element is a literal or one whole
  `{variable}`. The variables, the `for_each` lists and the `when`
  conditions belong to the extension (deploy's are in
  [deploy.md](deploy.md#providers)). `requires` is exec only.
- **Runtime `agent`**: each operation is a relative `.md` procedure inside
  the provider, which the extension's agent follows with the MCP tools
  `mcp` names. `mcp` is agent only.
- **`config`**: the key schema shared with `extension.yaml`. `type` is
  `string`, `int`, `bool`, `map` or `list`; a scalar key needs a `prompt`
  (what `nerv configure` asks); its `default` is optional and, when set,
  must be a valid value of its `type`. Where the values live
  is the extension's: `tasks.providers.<name>.<key>` for task providers,
  `release.providers.<name>.<key>` (and
  `release.projects[].providers.<name>.<key>` for a build provider) for
  release providers, `deploy.targets[].config.<key>` or
  `deploy.providers.<name>.<key>` for deploy providers. A `scope: repo` key
  is read from the project's `.ai-workflow/config.yaml` only; a
  `scope: user` key is personal and is also read from the user
  `config.yaml` (see
  [configuration.md](configuration.md#extension-sections-tasks-release-deploy)).
- **`events`** (optional): the core events the provider handles. Each must
  be an event of its slot's `events`, and none may need an operation the
  provider declares `not_implemented`.
- Unknown fields are refused, at the top level and inside every block. A
  top-level field that is not common must be one of the extension's
  `provider_fields`.

### Native providers

A provider nerv implements itself is an exec provider whose steps run the
hidden `{nerv} provider <extension>-<name> <operation>`, with the request on
stdin and the response on stdout. It has no special case in its extension:
`release-ssh-compose`, `release-script` and the three release providers are such
providers.

## Project extensions and providers

A repository can add its own extensions and providers next to the ones nerv
ships, in its committed `.ai-workflow/` folder:

```
.ai-workflow/
  config.yaml
  extensions/<name>/extension.yaml
  providers/<extension>-<name>/provider.yaml    # plus its scripts and procedures
```

- Each manifest is checked with exactly the contract and slot rules above,
  and its errors read like a shipped one's, named by the repository path
  (`.ai-workflow/providers/deploy-rsync/provider.yaml: operations.deploy:
  missing (implement it or declare not_implemented)`).
- A project provider belongs to a shipped extension (`deploy`, `release`,
  `task`) or to a project extension.
- A name nerv already ships is refused (`.ai-workflow/providers/release-github:
  nerv already ships an extension or provider named release-github; give the
  project's another name`), and so is a folder that resolves outside the
  repository.
- They are not Claude Code plugins: `nerv install` does not materialize them. `nerv
  release`, `nerv version next`, `nerv release setup` and `nerv deploy` read
  them from the repository, with its configuration; `nerv configure
  --init-repo --repo-provider-config` and the wizard's repository step take
  the keys of its task providers; the `task:dispatcher` reads its task
  providers from `.ai-workflow/providers/task-*/provider.yaml`.
- An exec provider runs its own files through `{provider_dir}/<path>`, a
  file inside the provider's folder; a bare `{provider_dir}` element (the
  folder itself) is refused. nerv resolves it to an absolute path when it loads the
  provider, and refuses a path that leaves the folder (`..`, or a symlink
  out of it) or does not exist. The command is still `{nerv}` or an
  executable named in `requires`, so a script runs through its interpreter.
  A provider nerv ships cannot use `{provider_dir}`.

A deploy target that copies the release with the repository's own script:

```yaml
# .ai-workflow/providers/deploy-rsync/provider.yaml
name: rsync
extension: deploy
slot: target
contract_version: 1
runtime: exec
requires: [bash]
config:
  - key: host
    scope: repo
    type: string
    prompt: The host to copy the release to
operations:
  preflight:
    - argv: [bash, "{provider_dir}/preflight.sh"]
  publish: not_implemented
  deploy:
    - argv: [bash, "{provider_dir}/deploy.sh", "{config.host}", "{version}"]
  verify: not_implemented
```

```yaml
# .ai-workflow/config.yaml
deploy:
  targets:
    - name: lab
      provider: rsync
      config:
        host: lab.example.com
```

Each script reads the request on its stdin and may print the response on
its stdout, as any exec step does. Project scripts run with your
permissions, like the repository's own build and deploy commands: review
them as you review those.

## Request and response

An operation is called with one request, the JSON object an exec step reads
on stdin:

```json
{
  "contract": 1,
  "extension": "deploy",
  "slot": "target",
  "provider": "ssh-compose",
  "operation": "verify",
  "event": "run.started",
  "config": {"environment": "staging"},
  "payload": {"target": "staging", "artifact": {}, "artifact_dir": "..."}
}
```

`event` is set when the operation handles a core event; `config` is the
provider's resolved config; `payload` is the extension's input for the
operation.

It answers with one response, which an exec step prints as one JSON object
on stdout (a step that prints none answers by its exit code):

```json
{"status": "ok", "message": "staging: api healthy", "data": {}}
```

| Status | Meaning |
|---|---|
| `ok` | Done. |
| `skipped` | Nothing to do, with the reason in `message`. |
| `blocked` | It cannot run (a missing tool, a missing connector). |
| `failed` | It ran and failed, with the error in `message`. |
| `needs_input` | It needs an answer: `message` is the question. |

`data` carries the operation's answer (deploy's `releaseExists` answers
`{"exists": false}`, a target's `preflight` `{"requires": ["sshpass"]}`).
An extension that cannot ask the user, such as `nerv deploy`, treats
`blocked` and `needs_input` as failures.
