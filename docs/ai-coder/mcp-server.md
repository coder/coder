---
title: MCP Server
---

Coder includes a built-in [Model Context Protocol](https://modelcontextprotocol.io/)
(MCP) server that provides AI assistants with tools and context about your Coder
deployment. This enables AI-powered workflows for managing workspaces,
templates, and development environments.

Coder supports two MCP server modes:

- **[Local MCP Server](#local-mcp-server)**: Runs via the Coder CLI using stdio
  transport. Ideal for local AI tools and IDE integrations.
- **[Remote MCP Server](#remote-mcp-server)**: HTTP-based server exposed by your
  Coder deployment. Supports OAuth2 authentication and is published to the MCP
  Registry.

## Local MCP Server

The local MCP server runs via the Coder CLI and uses stdio transport to
communicate with AI tools.

### Setup

Run the MCP server using the Coder CLI:

```sh
coder exp mcp server
```

### Client Configuration

Configure your MCP client to spawn the Coder CLI:

```json
{
  "mcpServers": {
    "coder": {
      "command": "coder",
      "args": ["exp", "mcp", "server"]
    }
  }
}
```

The CLI automatically uses your existing Coder authentication (from `coder login`).

### Claude Desktop Example

Add to your Claude Desktop configuration file:

<div class="tabs">

#### macOS

Edit `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "coder": {
      "command": "coder",
      "args": ["exp", "mcp", "server"]
    }
  }
}
```

#### Windows

Edit `%APPDATA%\Claude\claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "coder": {
      "command": "coder.exe",
      "args": ["exp", "mcp", "server"]
    }
  }
}
```

</div>

## Remote MCP Server

The remote MCP server is an HTTP endpoint exposed by your Coder deployment at
`/api/experimental/mcp/http`. This enables MCP clients to connect to Coder
without running the CLI locally.

The endpoint implements the [Streamable HTTP transport](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports) in stateless mode.
It supports MCP specification versions from `2024-11-05` through `2026-07-28` and doesn't issue `Mcp-Session-Id` headers.
It answers `GET` and `DELETE` with `405 Method Not Allowed`, without a standalone server-event stream or explicit session termination.
The default toolset exposes tools and prompts.
The workspace toolset includes `structuredContent` alongside the JSON text result
for object results. Other toolsets and result shapes use text.
MCP resources, elicitation, and the MCP Tasks extension aren't implemented.

### Choose a toolset

For workspace operations, add `?toolset=workspace` to the endpoint URL:

```txt
https://coder.example.com/api/experimental/mcp/http?toolset=workspace
```

This toolset includes workspace operations and the discovery tools for selecting an organization and template.
It excludes template administration and Coder Agents chats and prompts.
Without a `toolset` parameter, the endpoint exposes the standard toolset.
The `?toolset=chatgpt` toolset exposes only `search` and `fetch`.

### Prerequisites

The remote MCP HTTP endpoint requires the OAuth2 provider and the `mcp-server-http` experiment on your Coder deployment:

```sh
coder server --oauth2-provider-enable --experiments=mcp-server-http
```

Or set the environment variables:

```sh
CODER_OAUTH2_PROVIDER_ENABLE=true
CODER_EXPERIMENTS=mcp-server-http
```

For the YAML and Helm forms of the provider setting, refer to [Enable OAuth2 Provider](../admin/integrations/oauth2-provider/index.md#enable-oauth2-provider).
That page does not cover the experiment; set it with the top-level [`experiments`](../reference/cli/server/index.md#--experiments) YAML key.

### MCP Registry

Coder is published to the official [MCP Registry](https://github.com/modelcontextprotocol/registry)
as `io.github.coder/coder`, enabling easy installation in supported MCP clients.

#### VS Code / GitHub Copilot

1. Open VS Code Command Palette and run **MCP: Add Server...**
1. Select **From MCP Registry**
1. Search for "Coder" and select it
1. Enter your Coder deployment hostname when prompted (e.g., `coder.example.com`)
1. VS Code will automatically handle OAuth2 authentication

#### Claude Desktop (Remote)

Add to your Claude Desktop configuration file (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "coder": {
      "url": "https://coder.example.com/api/experimental/mcp/http"
    }
  }
}
```

Claude Desktop will automatically discover OAuth2 endpoints and prompt you to
authenticate through your browser.

### Manual Configuration

For MCP clients that don't support the registry or OAuth2 discovery, configure
the server manually with a session token:

```json
{
  "mcpServers": {
    "coder": {
      "url": "https://coder.example.com/api/experimental/mcp/http",
      "headers": {
        "Coder-Session-Token": "<your-session-token>"
      }
    }
  }
}
```

To create a session token:

1. Navigate to your Coder deployment
1. Go to **Settings > Tokens**
1. Create a new token
1. Add the token to your MCP client configuration

## Authentication

The MCP server supports two authentication methods:

### OAuth2 (Recommended for Interactive Clients)

MCP clients that support [RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728)
(Protected Resource Metadata) can authenticate automatically using OAuth2. The
server advertises its OAuth2 capabilities via the `WWW-Authenticate` header and
`/.well-known/oauth-protected-resource` endpoint.

This enables a seamless connect-and-authenticate experience where users sign in through their browser without manually managing tokens.

> [!NOTE]
> OAuth2 requires `CODER_OAUTH2_PROVIDER_ENABLE=true` on your Coder deployment.

### Session Token (For Programmatic Access)

For clients that don't support OAuth2 discovery, or for programmatic access, use
a session token as shown in the [Manual Configuration](#manual-configuration)
section.

## Available Tools

The MCP server exposes tools across several areas:

- **Workspace management**: list, inspect, create, and build workspaces
- **Template operations**: list, inspect, create, and manage templates and versions
- **File operations**: read, write, and edit files in a workspace
- **Workspace interaction**: run commands, forward ports, list apps, and read logs
- **Coder Agents chats**: create chats, send messages, read transcripts and status, interrupt, archive, and list available models
- **User and system**: authenticated user details, organization memberships, tar uploads, and task reporting

The full, authoritative set of tools, including their names, descriptions, and
arguments, is defined in Coder's
[`toolsdk` package](../../codersdk/toolsdk/toolsdk.go). Refer to it for the
current list, since the available tools can change between releases.

### Observe workspace readiness

Call `coder_workspace_readiness` before execution to check the workspace build and workspace agent startup state.
It's available in the standard and workspace toolsets and the authenticated CLI MCP server.
The tool doesn't start the workspace, connect to its workspace agent, or extend workspace activity.
Tools that read workspace files, list directories or apps, or get port URLs can start a stopped workspace and advertise that side effect.

Supply the workspace as `[owner/]workspace[.agent]`.
If the workspace has multiple workspace agents, select one with the `.agent` suffix.
For example, these arguments observe `alice/dev.main` for up to 30&nbsp;seconds:

```json
{
  "workspace": "alice/dev.main",
  "wait_ms": 30000
}
```

Omit `wait_ms` or set it to `0` to request one snapshot.
Positive values bound observation, including API requests, to at most 30&nbsp;seconds.
Single-snapshot API requests also have a 30&nbsp;second limit.
The tool returns immediately when ready or when the observed state requires action.
If the wait expires after a snapshot, the response includes that snapshot with `wait_expired: true`.

The response includes workspace and build IDs, build status, and the selected workspace agent's connection and startup states when available.
Its `state` and `reason` distinguish pending startup, build failure, disconnection, startup errors, and startup timeouts.
A `ready: true` result means the control plane reports a running build, a connected workspace agent, and successful startup.
It doesn't probe connectivity or guarantee that a subsequent operation succeeds.

## Available Prompts

The standard toolset also exposes
[prompts](https://modelcontextprotocol.io/specification/2026-07-28/server/prompts)
for common Coder Agents chat workflows. Clients that support prompts surface
them for you to invoke, for example as slash commands:

- `coder_agents_delegate`: delegate a coding task to a Coder Agents chat and
  monitor it to completion
- `coder_agents_check`: check the status and recent activity of an existing
  Coder Agents chat

## Troubleshooting

### "Unauthorized" errors

- Verify your session token is valid and not expired
- Check that the MCP server experiment is enabled on your deployment
- Ensure your user has appropriate permissions for the requested operations

### Connection timeouts

- Verify your Coder deployment URL is correct and accessible
- Check network connectivity between your MCP client and the control plane
- Review the logs from `coderd`, the process that runs the control plane, for any errors

### OAuth2 authentication not working

- Ensure your Coder deployment has `CODER_OAUTH2_PROVIDER_ENABLE=true` set
- Verify your MCP client supports RFC 9728 Protected Resource Metadata
- Check that your browser can reach the Coder authorization endpoint
