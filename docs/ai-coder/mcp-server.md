# Coder MCP Server

Coder includes a built-in [Model Context Protocol](https://modelcontextprotocol.io/)
(MCP) server that provides AI assistants with tools and context about your Coder
deployment. This enables AI-powered workflows for managing workspaces,
templates, and development environments.

## MCP Registry

Coder is published to the [MCP Registry](https://registry.modelcontextprotocol.io/),
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
      "url": "https://coder.example.com/api/experimental/mcp/http",
      "headers": {
        "Coder-Session-Token": "<your-session-token>"
      }
    }
  }
}
```

Generate a session token from your Coder deployment at **Settings > Tokens**.

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
        "Coder-Session-Token": "<your-session-token>"
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

The MCP server provides tools for:

- **Workspace Management**: List, create, start, stop, and delete workspaces
- **Template Operations**: Browse and manage templates
- **User Information**: Get details about the authenticated user
- **File Operations**: Read and write files in workspaces
- **Command Execution**: Run commands in workspace terminals

Use your AI assistant's tool discovery feature to see the full list of available
tools and their parameters.

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
