---
title: MCP server
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
Object results include `structuredContent` alongside the JSON text result.
Other result shapes use text.
MCP resources, elicitation, and the MCP Tasks extension aren't implemented.

### Choose a toolset

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
It's available in the standard toolset and the authenticated CLI MCP server.
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
Single-snapshot API requests are also limited to 30&nbsp;seconds.
The tool returns immediately when ready or when the observed state requires action.
If the wait expires after a snapshot, the response includes that snapshot with `wait_expired: true`.

The response includes workspace and build IDs, build status, and the selected workspace agent's connection and startup states when available.
Its `state` and `reason` distinguish pending startup, build failure, disconnection, startup errors, and startup timeouts.
A `ready: true` result means the control plane reports a running build, a connected workspace agent, and successful startup.
It doesn't probe connectivity or guarantee that a subsequent operation succeeds.

### Run work with a durable identity

Tracked execution requires control-plane and workspace-agent builds that include the execution-session, tracked-process, and artifact capabilities described here.
An older workspace agent cannot provide these guarantees.

Use `coder_acquire_workspace_execution` to create a workspace or attach an execution session to an existing workspace.
Supply an organization, owner, unique `request_id`, future `lease_expires_at`, and explicit `retained` value.
For creation, supply template and workspace parameters in `create`.
For adoption, supply `workspace_id`; an existing workspace cannot become disposable through adoption.

Declare absolute `result_paths` before execution.
For a template with multiple workspace agents, supply its unique `result_agent_name`, or a known `result_agent_id` for an existing workspace.
The result collector must match that selector.
If you omit both selectors, preservation requires exactly one workspace agent.

Keep the acquisition request unchanged when retrying after a lost response.
The same request identity returns the original session, workspace, and acquisition build.
Changed input with the same identity returns a conflict.
The acquisition build reports provisioning and quota failures.

An execution session's `source_unavailable: true` result can mean your current permissions don't allow reading its workspace or acquisition build.
It doesn't prove that either resource was deleted.

After checking readiness, call `coder_start_workspace_command` with the execution session, workspace agent, command, and a new command `request_id`.
Keep the returned execution ID to observe output or request cancellation.
Retrying the original start request recovers its receipt without starting the command again.

Legacy MCP bash and file mutation tools reject changes while any disposable execution session is incomplete or results are being preserved.
Use tracked commands in disposable workspaces so cleanup can observe their lifetime.
File reads remain available. Adopted workspaces otherwise permit legacy mutations.

| Tool                                       | Purpose                                          |
|--------------------------------------------|--------------------------------------------------|
| `coder_get_workspace_command`              | Observe status and bounded output.               |
| `coder_cancel_workspace_command`           | Request cancellation and observe acknowledgment. |
| `coder_renew_workspace_execution_session`  | Extend an open session's lease.                  |
| `coder_retain_workspace_execution_session` | Protect the workspace from automatic cleanup.    |
| `coder_retry_workspace_execution_session`  | Retry a failed preservation or deletion attempt. |

An observation timeout doesn't stop the command.
A running or unknown command has no invented exit code.
Cancellation is complete only when the response reports an actual exit or confirms that the command never started.
An unknown outcome is not permission to run the same work with a new identity.

Command output is bounded.
When output is truncated, the response reports the original, retained, and omitted byte counts.
Use declared result files for outputs that must remain complete and available after workspace deletion.
When output history expires, a matching workspace agent can still report a recorded exit, but output remains unavailable.
A restarted workspace agent cannot prove the outcome of an earlier instance's work.
An unresolved command blocks new export and automatic cleanup indefinitely.
Previously preserved artifacts remain subject to their retention.
You can still replay a successful export's metadata.
There is no operation that acknowledges an old agent's unknown work as stopped.
An uncertain external chat hook remains recorded as uncertain; retrying does not dispatch it again.
Its dispatch ID helps correlate consumer logs. Hook uncertainty alone does not block workspace cleanup.
If cleanup closes workspace admission while a hook is pending, the chat cannot commit new workspace work afterward.
Tracked identities are limited to 16,384 per workspace agent instance; reaching the limit rejects new tracked work.

### Retrieve complete results

Call `coder_export_workspace_execution` with the session ID and observed revision after work settles.
When reusing an existing workspace, first stop any preexisting commands or other writers outside the execution session.
Export waits for tracked work; it cannot establish whether an untracked process has finished.
Export closes workspace admission while collecting results.
After a successful export from an adopted workspace, new sessions and existing sibling sessions or chats may continue work.
The exported session stays closed and its artifacts remain an immutable snapshot.
Legacy mutations can resume in adopted workspaces once collection finishes.
A disposable workspace stays closed after export; use another workspace for subsequent work.
Sessions admitted before export can preserve their own declared results.

Export verifies every declared result before accepting the result set.
It doesn't delete the source workspace or change explicit retention.
Retry the same revision to recover the accepted export's metadata after a lost response.
A successful replay returns metadata for the original result set, even if artifact access has expired.
Replay doesn't extend expiry or collect source files again.
Check the expiry, then read the artifacts to verify that their bytes are still available.

If preservation fails, you can call `coder_export_workspace_execution` again to retry the same generation, even without an automatic cleanup policy.
A failed adopted export permits new sessions and legacy mutations to repair source files, then retry export.
The failed execution session stays closed to new commands. Disposable workspaces remain closed until cleanup completes.

Use `coder_workspace_artifact_list` to retrieve artifact IDs, sizes, MIME types, SHA-256 checksums, and expiry information.
Use `coder_workspace_artifact_read` to retrieve base64-encoded bytes in chunks of at most 65,536 bytes.
Continue from `next_offset` until `eof` is true, then verify the complete bytes against the listed checksum.
Artifact reads remain available after the workspace is deleted, until access expires.
Expiry ends access through artifact reads.
It does not physically purge stored bytes.
Older generations keep their stored bytes.

If required artifacts expire while their source workspace still exists, call `coder_retry_workspace_execution_session` with its current revision and a later `artifact_expires_at`.
The new expiry must extend finite retention beyond the current lease and current time.
The response identifies a new closed preservation generation; export that revision to collect the required files again.
Old artifact IDs keep their original expiry.
Recovery never substitutes missing files or reopens the exported session.
The replacement generation stays closed to new commands. Collection also fences other workspace admission while it runs.

Explicit retention prevents automatic cleanup.
There is no release control to undo retention.
Retention does not prevent an authorized manual workspace deletion.

A disposable acquisition alone doesn't enable automatic cleanup.
An administrator must enable `--workspace-execution-cleanup` or `CODER_WORKSPACE_EXECUTION_CLEANUP=true`.
The setting defaults to false and does not change existing licensing or browser-only restrictions.
An explicit `execution_deadline` bounds command execution; omitting it lets work finish before cleanup.
Active leases, tracked work, and running or queued chats protect the workspace from automatic deletion.
Leases do not override administrator or template stop rules, including autostop and dormancy.
A stop or restart can leave command outcomes unknown and defer automatic cleanup indefinitely.

> [!WARNING]
> Do not use an ordinary workspace delete operation as a substitute for execution-session cleanup.
> Explicit workspace deletion can remove source results before preservation completes.

## Available Prompts

The standard toolset also exposes [prompts](https://modelcontextprotocol.io/specification/2026-07-28/server/prompts) for common Coder Agents chat workflows.
Clients that support prompts surface them for you to invoke, for example as slash commands:

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
