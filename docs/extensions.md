# Extensions and providers

NERV's optional plugins come in two kinds, and each kind speaks one
contract:

- An **extension** (`task`, `release`, `deploy`) adds a capability to the
  core. It talks to the core through its `extension.yaml`.
- A **provider** (`task-teamwork`, `deploy-github`, `deploy-ssh-compose`)
  implements one slot of an extension: a tracker, a git host, a deploy
  target. It talks to its extension through its `provider.yaml`.

Every extension uses the same `extension.yaml` schema, and every provider,
of every extension, uses the same `provider.yaml` schema. Both share the
config key schema, the name rules, `contract_version` and the
request/response envelope below. `nerv` checks every manifest it ships, and
`nerv install` refuses a tree whose manifests do not hold.

## Layout

Each plugin is a top-level folder of `plugins/` with a
`.claude-plugin/plugin.json` whose `name` is the folder's name, and exactly
one manifest at its root:

| Folder | Manifest | Example |
|---|---|---|
| `<extension>` | `extension.yaml` | `plugins/deploy/extension.yaml` |
| `<extension>-<provider>` | `provider.yaml` | `plugins/deploy-github/provider.yaml` |

A provider requires its extension: `nerv install --with deploy-github`
also installs `deploy` (and `release`, which `deploy` requires). A provider
whose extension is not shipped, whose folder is not
`<extension>-<provider>`, or that does not fit its slot is refused.

## `extension.yaml` (core and extension)

```yaml
name: deploy                   # the folder's name
contract_version: 1
suggest_order: 3               # nerv install suggests task (1), release (2), deploy (3); 0 or unset comes last, by name
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
    operations:
      releaseExists: required  # required | optional
      createRelease: required
```

- **`events`** are core events: `run.started`, `verification.started`,
  `audit.ranked`, `issue.deferred`, `wave.started`, `run.closed`,
  `run.halted`. The orchestrator hands each one to every installed
  extension that lists it; an extension with `runtime: agent` is reached by
  launching its `agent` entry. Today only `task` subscribes, through
  `task:dispatcher`.
- **`cardinality`**: `one` provider at a time (deploy's `host`), `many`
  active at once (task's `provider`), or one per configured target
  (`per-target`, deploy's `target`).
- **`operations`** lists every operation of the slot. A provider declares
  each one; only an `optional` one may be `not_implemented`.
- **`provider_fields`** are `{name, type, required}` entries (types as for
  config keys). They let an extension give its providers top-level fields
  the common schema does not have, such as task's `task_ref_prefix`,
  `stages` and `events`. A provider field must not reuse a common field
  name. The extension's own code gives them their meaning.

The shipped extensions:

| Extension | Runtime | Requires | Events | Slots |
|---|---|---|---|---|
| `task` | agent, `task:dispatcher` | | the 7 core events | `provider` (many): the 13 task operations, all optional |
| `release` | exec, `nerv release` | | | none yet |
| `deploy` | exec, `nerv deploy` | `release` | | `host` (one): `releaseExists`, `createRelease`; `target` (per-target): `preflight`, `publish`, `verify` optional, `deploy` required |

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
    - argv: ["{nerv}", provider, deploy-ssh-compose, preflight]
  publish:
    - argv: [docker, push, "{item}"]
      for_each: images
  deploy:
    - argv: ["{nerv}", provider, deploy-ssh-compose, deploy]
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
events: [run.started]
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
  (what `nerv configure` asks) and a valid `default`. Where the values live
  is the extension's: `tasks.providers.<name>.<key>` for task providers,
  `deploy.targets[].config.<key>` or `deploy.providers.<name>.<key>` for
  deploy providers.
- Unknown fields are refused, at the top level and inside every block. A
  top-level field that is not common must be one of the extension's
  `provider_fields`.

### Native providers

A provider nerv implements itself is an exec provider whose steps run the
hidden `{nerv} provider <extension>-<name> <operation>`, with the request on
stdin and the response on stdout. It has no special case in its extension:
`deploy-ssh-compose` is one.

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
