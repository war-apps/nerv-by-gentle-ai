# task-teamwork

The Teamwork provider of the NERV task extension (Claude Code plugin
`task-teamwork`, provider name `teamwork`, slot `provider`). The `/task:*`
commands run its procedures (`procedures/*.md`) with Teamwork's MCP tools,
the `twprojects-*` tools listed in `provider.yaml`.

## Install

```sh
nerv install --with task,task-teamwork
```

A Teamwork administrator enables MCP under **Settings → AI** first.

## Connect to Teamwork

The provider takes the tools from any of three sources; pick the one that
suits you. The task dispatcher checks the tools before every dispatch and
answers `blocked` with `missing MCP tools: ...` when none of them serves
all of them. Any other server's `twprojects-*` tools do not count.

| Source | Sign-in | Claude Code tool names |
|---|---|---|
| A. claude.ai Teamwork connector | in claude.ai, once per account | `mcp__claude_ai_Teamwork_com__twprojects-*` |
| B. the plugin's MCP server | OAuth from Claude Code, per machine | `mcp__plugin_task-teamwork_teamwork__twprojects-*` |
| C. a server with a fixed token | none: a bearer token from an environment variable | `mcp__teamwork__twprojects-*` |

When more than one serves every tool, a dispatch uses one of them:
C, then B, then A.

Check the result with `claude mcp list`: the source you chose shows
`✔ Connected`.

### A. The claude.ai Teamwork connector

1. In claude.ai, open **Settings → Connectors**
   ([claude.ai/customize/connectors](https://claude.ai/customize/connectors)),
   add **Teamwork.com** and sign in.
2. Sign in to Claude Code with the same claude.ai account (`/login`).
   `claude mcp list` shows `claude.ai Teamwork.com: ✔ Connected`.

Nothing to do on the machine itself, so it also works over SSH and on a
console-only server. It needs a claude.ai subscription login: with an API
key, Claude Code does not load claude.ai connectors.

### B. The plugin's MCP server (OAuth)

The plugin ships Teamwork's official MCP server
([github.com/Teamwork/mcp](https://github.com/Teamwork/mcp)) in its
`.mcp.json` as `teamwork` (`https://mcp.ai.teamwork.com`), with no
credential: Claude Code lists it as `plugin:task-teamwork:teamwork` and
each user signs in once.

> **Known issue (Claude Code 2.1.312):** Teamwork's authorization server
> rejects the redirect URI Claude Code registers, so this sign-in fails
> before the browser opens:
> `Dynamic Client Registration rejected (HTTP 400): {"error":"invalid_redirect_uri","error_description":"redirect URI host \"localhost:63397\" is no longer supported (sunset 2026-08-01)"}`.
> Claude Code always registers `http://localhost:<port>/callback` and its
> host cannot be changed. Until either side changes, use A or C.

On a machine with a browser:

1. In Claude Code, run `/mcp`.
2. Select `plugin:task-teamwork:teamwork`, then **Authenticate**.
3. Sign in to Teamwork in the browser that opens.

Claude Code waits for Teamwork's answer on a temporary
`http://localhost:<port>/callback` server, with a random port. Only the
port can be fixed (`oauth.callbackPort` in `.mcp.json`), and a fixed port
would make two users of a shared machine collide. Claude Code stores the
token and refreshes it on its own.

**Without a browser on that machine** (SSH, a console-only server): run
`claude mcp login plugin:task-teamwork:teamwork --no-browser` (over SSH,
connect with `ssh -t`). It prints the authorization URL: open it in a
browser on any machine, sign in, and when the browser fails to reach
`localhost`, copy the full URL from its address bar and paste it at the
prompt.

**Through VS Code Remote-SSH**, VS Code can carry the redirect back:

1. Open the remote folder in VS Code (Remote-SSH), open its integrated
   terminal (**View → Terminal**) and run `claude` there, or use the
   Claude Code extension in that window. A terminal multiplexer (tmux,
   herdr) or a plain SSH session has no link to VS Code.
2. Run `/mcp`, select `plugin:task-teamwork:teamwork`, then
   **Authenticate**, and open the authorization URL in your browser
   (Ctrl+click it in the terminal).
3. Find the port in the URL's
   `redirect_uri=http%3A%2F%2Flocalhost%3A<port>%2Fcallback` parameter.
   In VS Code's **Ports** tab, check that it is forwarded, or **Forward a
   Port** with it; *Forwarded Address* must be `localhost:<port>`, the
   same number.
4. Sign in to Teamwork. The redirect reaches Claude Code through the
   forward and the sign-in finishes; if the browser had already failed,
   reload the page once the port is forwarded.
5. Close the forward in the **Ports** tab.

### C. A fixed bearer token from an environment variable

No interactive sign-in on the machine that runs Claude Code: the server
receives the same token on every request.

1. On any machine with a browser, get a bearer token with Teamwork's
   helper (or follow
   [Teamwork's app login flow](https://apidocs.teamwork.com/guides/teamwork/app-login-flow)):

   ```sh
   npm i @teamwork/get-bearer-token@latest -g
   teamwork-get-bearer-token
   ```

2. On the machine that runs Claude Code, export it as `TEAMWORK_MCP_TOKEN`
   in your shell profile (for example `~/.bashrc`), or under `env` in
   `~/.claude/settings.json`:

   ```sh
   export TEAMWORK_MCP_TOKEN='<token>'
   ```

3. Register the server at user scope with a header that references the
   variable. The single quotes keep `${TEAMWORK_MCP_TOKEN}` unexpanded, so
   Claude Code stores the reference, not the token, and expands it each
   time it connects:

   ```sh
   claude mcp add --scope user --transport http teamwork https://mcp.ai.teamwork.com \
     --header 'Authorization: Bearer ${TEAMWORK_MCP_TOKEN}'
   ```

4. Restart Claude Code.

The plugin's server keeps showing `Needs authentication`; leave it signed
out. If the variable is not set, Claude Code warns in `claude mcp list`
and `/mcp` and sends the `${TEAMWORK_MCP_TOKEN}` text as is, so Teamwork
rejects the requests. Teamwork's documentation does not say whether the
token expires. Keep it out of every repository; to revoke it, delete it in
Teamwork and run `claude mcp remove --scope user teamwork`.

## Configure a repository

The task extension reads `tasks:` from the repository's
`.ai-workflow/config.yaml` only. The smallest configuration that sends a
repository's tasks to Teamwork:

```yaml
tasks:
  provider: teamwork
  providers:
    teamwork:
      project_id: 1271726    # this repository's Teamwork project
      tasklist_id: 3951970   # this repository's tasklist for new tasks
```

It needs no `enabled: true`: that key turns NERV's orchestrator on in the
repository, and the task commands work without it. Three ways to write it, none of them the whole
`nerv configure` wizard:

- **`/task:setup`** in Claude Code, in the repository: it picks the
  provider, proposes the section, offers Teamwork's project and tasklist
  listings for the ids, writes only `tasks:` and checks the connection.
- **`nerv configure --init-repo`**, from a terminal:

  ```sh
  nerv configure --init-repo <path> \
    --repo-provider-config teamwork.project_id=<id> \
    --repo-provider-config teamwork.tasklist_id=<id>
  ```

  It also writes `enabled: true`, and `tasks.provider` only when the user
  configuration seeds it (`defaults.tasks.provider: teamwork`).
- **By hand.**

The personal keys, such as
`assignee_id`, go in the user configuration under
`tasks.providers.teamwork` (see `docs/configuration.md`). Then open Claude
Code in the repository and run `/task:me`.

## Deleting a timelog

The MCP tools cannot delete a timelog, so `deleteTimelog` calls Teamwork's
REST API with a Teamwork API key: the `TEAMWORK_API_KEY` environment
variable, else `TEAMWORK_API_KEY` in the `env` of the `teamwork` server in
`~/.claude.json`.
