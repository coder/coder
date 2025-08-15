# MCP Server Reference

This page provides technical reference information for Coder's Model Context Protocol (MCP) server implementation. For setup guides with specific AI tools and IDEs, see the [MCP documentation](../mcp/index.md).

## Overview

Coder's MCP server enables AI assistants to interact with Coder workspaces and infrastructure through the standardized Model Context Protocol. The server provides tools for:

- **Workspace Management**: List, create, start, stop, and delete workspaces
- **Command Execution**: Run commands in remote workspaces
- **Resource Monitoring**: Check workspace status and resource usage
- **Agent Activity**: Monitor AI agent operations and history

> [!NOTE]
> See our [toolsdk documentation](https://pkg.go.dev/github.com/coder/coder/v2/codersdk/toolsdk#pkg-variables) for a complete list of tools included in the MCP server.

## Architecture

The Coder MCP server acts as a bridge between AI assistants and the Coder API, providing secure and controlled access to development environments.

## Server Modes

Coder provides two MCP server modes to support different AI assistant architectures:

### Local MCP Server (stdio)

The local MCP server runs on your machine and communicates with AI assistants through standard input/output. This mode is ideal for desktop applications and local development tools.

**Automatic Configuration**:

```sh
# First authenticate with Coder
coder login https://coder.example.com

# Configure specific AI tools
coder exp mcp configure claude-desktop
coder exp mcp configure cursor
```

**Manual Server Start**:

```sh
# Start MCP server for manual configuration
coder exp mcp server
```

**Authentication**: Uses your Coder CLI credentials and inherits your user permissions.

### Remote MCP Server (HTTP)

The HTTP MCP server runs on your Coder deployment and provides web-based access for browser-based AI assistants.

**Enable HTTP MCP Server**:

```sh
# Enable experimental features
CODER_EXPERIMENTS="oauth2,mcp-server-http" coder server
```

**Endpoint**: `https://your-coder-deployment.com/api/experimental/mcp/http`

**Authentication**: Uses OAuth2 for secure web-based authentication.

> [!NOTE]
> Both server modes operate with the authenticated user's permissions. Fine-grained MCP-specific permissions are in development. [Contact us](https://coder.com/contact) if this is important for your use case.

## Available Tools

The Coder MCP server exposes a comprehensive set of tools through the Model Context Protocol. These tools are automatically available to any connected AI assistant.

### Workspace Tools

- `list_workspaces` - List all accessible workspaces
- `create_workspace` - Create new workspaces from templates
- `start_workspace` - Start stopped workspaces
- `stop_workspace` - Stop running workspaces
- `delete_workspace` - Delete workspaces
- `get_workspace_status` - Check workspace status and resource usage

### Command Execution Tools

- `execute_command` - Run commands in workspace terminals
- `get_command_output` - Retrieve command execution results
- `list_processes` - List running processes in workspaces

### File System Tools

- `read_file` - Read file contents from workspaces
- `write_file` - Write files to workspace file systems
- `list_directory` - List directory contents

### Template and Resource Tools

- `list_templates` - List available workspace templates
- `get_template_info` - Get detailed template information
- `list_template_versions` - List template version history

For the complete and up-to-date list of available tools, see the [toolsdk documentation](https://pkg.go.dev/github.com/coder/coder/v2/codersdk/toolsdk#pkg-variables).

## Configuration Options

### Local MCP Server Options

Customize the local MCP server behavior:

```sh
# Enable debug logging
coder exp mcp server --log-level debug

# Set custom timeout
coder exp mcp server --timeout 60s

# Specify workspace filter
coder exp mcp server --workspace-filter "owner:me"
```

### HTTP MCP Server Options

Configure the HTTP MCP server through environment variables:

```sh
# Set custom timeout for HTTP requests
export CODER_MCP_HTTP_TIMEOUT=120s

# Configure rate limiting
export CODER_MCP_HTTP_RATE_LIMIT=100

# Enable detailed logging
export CODER_MCP_LOG_LEVEL=debug
```

## Security Considerations

### Authentication and Authorization

- **Local Mode**: Uses Coder CLI credentials and user permissions
- **HTTP Mode**: Requires OAuth2 authentication with proper scopes
- **Permission Inheritance**: MCP operations inherit the authenticated user's Coder permissions
- **Audit Logging**: All MCP operations are logged through Coder's audit system

### Best Practices

- Regularly rotate authentication credentials
- Monitor MCP usage through Coder's audit logs
- Use workspace templates with appropriate security configurations
- Implement proper secret management in workspaces
- Review AI assistant access patterns regularly

## Troubleshooting

### Common Issues

**MCP Server Won't Start**:

- Verify Coder CLI authentication: `coder whoami`
- Check experimental features are enabled for HTTP mode
- Review server logs for error messages

**AI Assistant Can't Connect**:

- Verify MCP server is running and accessible
- Check authentication credentials and permissions
- Ensure network connectivity to Coder deployment

**Permission Denied Errors**:

- Verify user has appropriate workspace permissions
- Check Coder RBAC settings
- Ensure OAuth2 scopes are correctly configured (HTTP mode)

### Debug Mode

Enable debug logging for troubleshooting:

```sh
# Local MCP server
coder exp mcp server --log-level debug

# HTTP MCP server
CODER_MCP_LOG_LEVEL=debug CODER_EXPERIMENTS="oauth2,mcp-server-http" coder server
```

## Getting Started

For step-by-step setup instructions with specific AI tools and IDEs, see:

- [MCP Overview](../mcp/index.md) - Introduction and concepts
- [VSCode Setup](../mcp/vscode.md) - Visual Studio Code integration
- [Cursor Setup](../mcp/cursor.md) - Cursor AI editor integration
- [Zed Setup](../mcp/zed.md) - Zed editor integration
- [WindSurf Setup](../mcp/windsurf.md) - WindSurf AI environment integration
- [Claude Desktop Setup](../mcp/claude-desktop.md) - Claude Desktop integration
- [Web Agents Setup](../mcp/web-agents.md) - Browser-based AI assistants

## Support

For technical support with Coder MCP:

- [Contact Coder Support](https://coder.com/contact)
- [Join our Discord Community](https://discord.gg/coder)
- [Report Issues on GitHub](https://github.com/coder/coder/issues)
- [Model Context Protocol Specification](https://modelcontextprotocol.io/)
