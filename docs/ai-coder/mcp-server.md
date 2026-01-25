# Coder MCP Server

Coder includes a built-in [Model Context Protocol](https://modelcontextprotocol.io/)
(MCP) server that provides AI assistants with tools and context about your Coder
deployment. This enables AI-powered workflows for managing workspaces,
templates, and development environments.

## MCP Registry

Coder is published to the official MCP Registry (`io.github.coder/coder`),
enabling easy installation in supported MCP clients.

### VS Code / GitHub Copilot

1. Open VS Code Command Palette and run **MCP: Add Server...**
1. Select **From MCP Registry**
1. Search for "Coder" and select it
1. Enter your Coder deployment URL when prompted (e.g., `https://coder.example.com`)
1. VS Code will automatically handle OAuth2 authentication

### Claude Desktop

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

> [!NOTE]
> If your MCP client doesn't support OAuth2 discovery, see
> [Manual Configuration](#manual-configuration) for token-based authentication.

## Manual Configuration

For MCP clients that don't support the registry, you can configure the server
manually.

### Prerequisites

Enable the MCP server experiment on your Coder deployment:

```sh
coder server --experiments=mcp-server-http
```

Or set the environment variable:

```sh
CODER_EXPERIMENTS=mcp-server-http
```

### HTTP Transport

The MCP server is available at `/api/experimental/mcp/http` on your Coder
deployment. This endpoint supports
[Streamable HTTP transport](https://modelcontextprotocol.io/specification/2025-03-26/basic/transports#streamable-http).

Example configuration for MCP clients:

```json
{
  "mcpServers": {
    "coder": {
      "url": "https://coder.example.com/api/experimental/mcp/http",
      "headers": {
        "Coder-Session-Token": "&lt;your-session-token&gt;"
      }
    }
  }
}
```

## Authentication

The MCP server supports two authentication methods:

### OAuth2 (Recommended for Interactive Clients)

MCP clients that support [RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728)
(Protected Resource Metadata) can authenticate automatically using OAuth2. The
server advertises its OAuth2 capabilities via the `WWW-Authenticate` header and
`/.well-known/oauth-protected-resource` endpoint.

This enables a seamless "click-to-connect" experience where users authenticate
through their browser without manually managing tokens.

### Session Token (For Programmatic Access)

For clients that don't support OAuth2 discovery, use a session token:

1. Navigate to your Coder deployment
1. Go to **Settings > Tokens**
1. Create a new token
1. Add the token to your MCP client configuration as a `Coder-Session-Token` header

## Available Tools

The MCP server provides the following tools:

### Workspace Management

| Tool | Description |
|------|-------------|
| `coder_list_workspaces` | List workspaces for the authenticated user |
| `coder_get_workspace` | Get details of a specific workspace |
| `coder_create_workspace` | Create a new workspace from a template |
| `coder_create_workspace_build` | Start, stop, or delete a workspace |

### Template Operations

| Tool | Description |
|------|-------------|
| `coder_list_templates` | List available templates |
| `coder_template_version_parameters` | Get parameters for a template version |
| `coder_create_template` | Create a new template |
| `coder_create_template_version` | Create a new template version |
| `coder_update_template_active_version` | Update a template's active version |
| `coder_delete_template` | Delete a template |
| `coder_get_template_version_logs` | Get logs from a template version build |

### File Operations

| Tool | Description |
|------|-------------|
| `coder_workspace_ls` | List files in a workspace directory |
| `coder_workspace_read_file` | Read a file from a workspace |
| `coder_workspace_write_file` | Write a file to a workspace |
| `coder_workspace_edit_file` | Edit a file in a workspace |
| `coder_workspace_edit_files` | Edit multiple files in a workspace |

### Workspace Interaction

| Tool | Description |
|------|-------------|
| `coder_workspace_bash` | Execute bash commands in a workspace |
| `coder_workspace_port_forward` | Get a URL to forward a port |
| `coder_workspace_list_apps` | List apps running in a workspace |
| `coder_get_workspace_agent_logs` | Get workspace agent logs |
| `coder_get_workspace_build_logs` | Get workspace build logs |

### Task Management

| Tool | Description |
|------|-------------|
| `coder_create_task` | Create a new task |
| `coder_list_tasks` | List tasks |
| `coder_get_task_status` | Get the status of a task |
| `coder_get_task_logs` | Get logs from a task |
| `coder_send_task_input` | Send input to a running task |
| `coder_delete_task` | Delete a task |

### User and System

| Tool | Description |
|------|-------------|
| `coder_get_authenticated_user` | Get the authenticated user's details |
| `coder_upload_tar_file` | Upload a tar file (for template versions) |
| `coder_report_task` | Report progress on a task |

## Troubleshooting

### "Unauthorized" errors

- Verify your session token is valid and not expired
- Check that the MCP server experiment is enabled on your deployment
- Ensure your user has appropriate permissions for the requested operations

### Connection timeouts

- Verify your Coder deployment URL is correct and accessible
- Check network connectivity between your MCP client and the Coder server
- Review Coder server logs for any errors

### OAuth2 authentication not working

- Ensure your Coder deployment has the `oauth2` experiment enabled
- Verify your MCP client supports RFC 9728 Protected Resource Metadata
- Check that your browser can reach the Coder authorization endpoint
