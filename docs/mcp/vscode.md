# Using Coder MCP with Visual Studio Code

This guide shows you how to set up Coder's Model Context Protocol (MCP) server with Visual Studio Code and AI extensions like GitHub Copilot, Codeium, or other AI assistants.

## Prerequisites

- Visual Studio Code installed
- Coder CLI installed and authenticated
- An AI extension that supports MCP (varies by extension)

## Setup Methods

### Method 1: Automatic Configuration (Recommended)

The Coder CLI can automatically configure MCP for supported VS Code extensions:

```bash
# First, authenticate with your Coder deployment
coder login https://coder.example.com

# Configure MCP for VS Code (when supported)
coder exp mcp configure vscode
```

> **Note**: Automatic configuration support varies by AI extension. Check with your specific AI extension's documentation for MCP support.

### Method 2: Manual Configuration

For extensions that support custom MCP servers, you can manually configure the connection:

1. **Start the Coder MCP server**:
   ```bash
   coder exp mcp server
   ```
   
   The server will start on `stdio` by default and display connection details.

2. **Configure your AI extension**:
   - Open VS Code settings (Ctrl/Cmd + ,)
   - Search for your AI extension's MCP settings
   - Add the Coder MCP server configuration

### Method 3: Using VS Code Settings

Some AI extensions allow MCP configuration through VS Code's settings.json:

```json
{
  "your-ai-extension.mcpServers": {
    "coder": {
      "command": "coder",
      "args": ["exp", "mcp", "server"],
      "env": {}
    }
  }
}
```

## Supported AI Extensions

### GitHub Copilot
GitHub Copilot's MCP support is evolving. Check the [GitHub Copilot documentation](https://docs.github.com/en/copilot) for the latest MCP integration options.

### Codeium
Codeium may support MCP through their VS Code extension. Refer to [Codeium's documentation](https://codeium.com/vscode_tutorial) for MCP setup instructions.

### Continue.dev
Continue.dev has experimental MCP support. See their [MCP documentation](https://docs.continue.dev/walkthroughs/mcp) for setup details.

## Using Coder MCP in VS Code

Once configured, your AI assistant can interact with Coder through MCP:

### Available Commands

- **List workspaces**: View all your Coder workspaces
- **Create workspace**: Create new development environments
- **Start/Stop workspaces**: Manage workspace lifecycle
- **Execute commands**: Run commands in your workspaces
- **Check status**: Monitor workspace and agent activity

### Example Interactions

**Creating a new workspace**:
```
User: "Create a new Python workspace for my machine learning project"
AI: Uses Coder MCP to create a workspace with Python and ML tools
```

**Running commands**:
```
User: "Run the tests in my backend workspace"
AI: Executes test commands in the specified Coder workspace
```

**Checking workspace status**:
```
User: "What's the status of my workspaces?"
AI: Lists all workspaces with their current state and resource usage
```

## Troubleshooting

### MCP Server Not Starting

1. **Check Coder CLI authentication**:
   ```bash
   coder whoami
   ```

2. **Verify MCP server manually**:
   ```bash
   coder exp mcp server --help
   ```

3. **Check VS Code extension logs**:
   - Open VS Code Developer Tools (Help > Toggle Developer Tools)
   - Check the Console for MCP-related errors

### AI Extension Not Connecting

1. **Verify MCP support**: Ensure your AI extension supports MCP
2. **Check configuration**: Review your extension's MCP settings
3. **Restart VS Code**: Sometimes a restart is needed after configuration changes

### Permission Issues

1. **Check Coder permissions**: Ensure your user has appropriate workspace permissions
2. **Verify authentication**: Re-authenticate with Coder if needed:
   ```bash
   coder login https://coder.example.com
   ```

## Best Practices

### Security
- Keep your Coder CLI credentials secure
- Regularly rotate authentication tokens
- Review AI assistant permissions and access patterns

### Performance
- Use workspace templates optimized for AI development
- Consider workspace resource allocation for AI workloads
- Monitor workspace usage and costs

### Development Workflow
- Create dedicated workspaces for different projects
- Use Coder's workspace templates for consistent environments
- Leverage Coder's collaboration features for team AI development

## Advanced Configuration

### Custom MCP Server Options

You can customize the MCP server behavior:

```bash
# Start MCP server with custom options
coder exp mcp server --log-level debug --timeout 30s
```

### Environment Variables

Configure MCP behavior through environment variables:

```bash
export CODER_MCP_LOG_LEVEL=debug
export CODER_MCP_TIMEOUT=60s
coder exp mcp server
```

## Next Steps

- Explore [Coder Templates](https://registry.coder.com) optimized for AI development
- Learn about [AI coding best practices](../ai-coder/best-practices.md) with Coder
- Set up [Coder Tasks](../ai-coder/tasks.md) for background AI agent execution
- Review [security considerations](../ai-coder/security.md) for AI development

## Support

For VS Code-specific MCP issues:

1. Check your AI extension's documentation for MCP support
2. [Contact Coder Support](https://coder.com/contact) for Coder MCP server issues
3. [Report bugs](https://github.com/coder/coder/issues) on the Coder GitHub repository
