---
title: Bring your own MCP servers
---

This page is for developers who want their own MCP servers in Coder Agents chats without asking an administrator to register each one.
If you use `claude mcp add` in Claude Code, a `.mcp.json` file in your workspace is the closest equivalent.
You write the file once, from your dotfiles or a startup script, and new chats in that workspace get its tools.

## Choose an approach

Coder Agents can get MCP tools from three places:

| Approach                                                                                 | Who sets it up                                       | Lasts across chats                             | Authentication                                  |
|------------------------------------------------------------------------------------------|------------------------------------------------------|------------------------------------------------|-------------------------------------------------|
| Workspace `.mcp.json`                                                                    | You                                                  | Yes, for every new chat in the workspace       | Environment variables (stdio) or headers (HTTP) |
| [Organization MCP servers](./extending-agents.md#connect-to-mcp-servers)                 | An organization admin registers it, then you connect | Yes, based on the server's availability policy | OAuth2, API key, custom headers, or OIDC        |
| [Inline MCP servers](./platform-controls/mcp-servers.md#inline-mcp-servers-experimental) | You, through the chats API                           | No, you declare it on each chat                | Headers only                                    |

Use an organization MCP server when the provider needs a browser sign-in.
A workspace `.mcp.json` file has no OAuth2 flow.

The rest of this page covers the workspace `.mcp.json` file.

## Before you begin

The agent reads `.mcp.json` from one place: the top level of the workspace agent's working directory.
The template sets this directory with the `dir` argument of its `coder_agent` resource, for example `/home/coder/project`.

- If the template sets `dir`, put your file at `<dir>/.mcp.json`.
- If the template doesn't set `dir`, the agent reads no `.mcp.json` at all.
  Ask your template administrator to set `dir`.

The agent doesn't read `~/.claude.json`, subdirectories, or parent directories.
When the template sets `dir`, a new workspace terminal opens in that directory.

## Write the file

`.mcp.json` uses the same `mcpServers` format as Claude Code.
Each key is a server name, and Coder names that server's tools `<server>__<tool>`.
A server name can't contain `__` or start or end with `_`.

```json
{
  "mcpServers": {
    "github": {
      "command": "github-mcp-server",
      "args": ["stdio"]
    },
    "example": {
      "type": "http",
      "url": "https://mcp.example.com/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

A server with `command` runs as a local process (stdio) in the workspace, started in the working directory.
A server with `url` connects over streamable HTTP from the workspace, so it can reach anything the workspace can reach, including `localhost`.
To choose the transport explicitly, set `type` to `stdio`, `http`, or `sse`.

### Keep tokens out of the file

Store each token as a [user secret](../../user-guides/user-secrets.md) with an environment variable target.
Coder injects user secrets into every workspace you own, and stdio servers inherit them from the agent.

For example, the GitHub MCP server reads `GITHUB_PERSONAL_ACCESS_TOKEN`.
A user secret with that name needs no entry in `.mcp.json`:

```sh
echo -n "$TOKEN" | coder secret create github-mcp-token \
  --env GITHUB_PERSONAL_ACCESS_TOKEN
```

Name the secret after the variable your server expects.
The `env` block in `.mcp.json` can't map one name to another here.
Coder expands `${VAR}` in `env` values from the agent's own process environment, which doesn't include user secrets.
Coder doesn't expand variables in `command`, `args`, `url`, or `headers`.

HTTP servers take credentials in `headers`, which Coder reads literally.
To keep those tokens out of your dotfiles repository, have your startup script copy them from a user secret into the file, as the next section shows.
The token then sits in plain text in the workspace, so the script restricts the file to your user.

## Create the file when the workspace starts

A script that writes `.mcp.json` at startup gives every workspace you create the same servers.
User secrets are available to startup scripts, so the script can fill in header tokens.

This script writes both servers from the earlier example.
Set `WORKDIR` to your agent's working directory, and replace `EXAMPLE_API_TOKEN` with the name of your user secret.

```sh
#!/usr/bin/env bash
set -euo pipefail

WORKDIR="$HOME/project"

mkdir -p "$WORKDIR"
umask 077
cat > "$WORKDIR/.mcp.json" <<JSON
{
  "mcpServers": {
    "github": {
      "command": "github-mcp-server",
      "args": ["stdio"]
    },
    "example": {
      "type": "http",
      "url": "https://mcp.example.com/mcp",
      "headers": { "Authorization": "Bearer ${EXAMPLE_API_TOKEN}" }
    }
  }
}
JSON
chmod 600 "$WORKDIR/.mcp.json"
```

Run the script from one of these places:

- **Dotfiles**: save it as `install.sh` in your [dotfiles repository](../../user-guides/workspace-dotfiles.md), or call it from your existing install script.
  The template must run `coder dotfiles`, usually through the [dotfiles module](https://registry.coder.com/modules/dotfiles).
- **Personalize script**: save it as `~/personalize`.
  The template must include the [personalize module](https://registry.coder.com/modules/personalize).

The agent connects to the servers after startup scripts finish.
It also watches the file, so a `.mcp.json` that appears later still loads without a workspace restart.

### If the working directory is a Git repository

The script writes into your project, so keep the file out of commits:

```sh
echo ".mcp.json" >> "$WORKDIR/.git/info/exclude"
```

If the repository already commits a `.mcp.json`, do not overwrite it.
Merge your servers into the committed file, then tell Git to ignore your local change:

```sh
jq -s '.[0] * .[1]' "$WORKDIR/.mcp.json" ~/my-mcp.json > ~/.mcp.merged.json
chmod 600 ~/.mcp.merged.json
mv ~/.mcp.merged.json "$WORKDIR/.mcp.json"
git -C "$WORKDIR" update-index --skip-worktree .mcp.json
```

In this example, `~/my-mcp.json` holds your own `mcpServers` entries.

## Reuse a Claude Code setup

`claude mcp add` writes to `~/.claude.json` by default, which Coder doesn't read.
To write `.mcp.json` instead, run it with `--scope project` from the working directory:

```sh
cd "$WORKDIR"
claude mcp add --scope project --transport http example https://mcp.example.com/mcp
```

A file from Claude Code works in Coder with three differences:

- Coder accepts `type` values of `stdio`, `http`, and `sse`.
  Change `streamable-http` to `http`.
- Coder expands variables only in `env` values, as [Keep tokens out of the file](#keep-tokens-out-of-the-file) describes.
- Coder doesn't ask you to approve the servers in `.mcp.json`.
  It starts every server the file lists, so review a `.mcp.json` from a repository before the agent runs it.

## Check that your servers loaded

Start a new chat in the workspace and ask the agent which tools it has.
Tools from your file have names like `github__<tool>`.

If a server is missing:

1. Confirm that `.mcp.json` sits at the top level of the working directory.
1. Run the server's `command` in a workspace terminal to confirm that it starts.
1. Read the workspace agent logs for the connection error.

The agent skips a server that doesn't connect within 30&nbsp;seconds, and the other servers still load.
Each tool call has a 60&nbsp;second timeout and needs a running workspace.

## Update your servers

Edit `.mcp.json` at any time.
The agent reloads its servers without a workspace restart, and new chats get the new tools right away.

An existing chat keeps the tools it started with.
An edit to `.mcp.json` alone doesn't mark that chat out of date, so start a new chat to use the new tools.
For the full lifecycle, refer to [How the workspace shares context with chats](./extending-agents.md#how-the-workspace-shares-context-with-chats).

> [!NOTE]
> Template administrators can point the agent at other files with the experimental `CODER_AGENT_EXP_MCP_CONFIG_FILES` variable, set in the agent's process environment.
> It takes a comma-separated list of paths and replaces the default `.mcp.json`.
> Relative paths resolve against the working directory, and absolute or `~/` paths work even when the template doesn't set `dir`.
> For example, `.mcp.json,~/.coder/mcp.json` keeps the project file and adds one in your home directory.

## Learn more

- [Workspace MCP tools](./extending-agents.md#workspace-mcp-tools)
- [MCP servers](./platform-controls/mcp-servers.md) for administrators
- [User secrets](../../user-guides/user-secrets.md)
- [Dotfiles](../../user-guides/workspace-dotfiles.md)
