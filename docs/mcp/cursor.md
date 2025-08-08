# Using Coder MCP with Cursor

This guide shows you how to set up Coder's Model Context Protocol (MCP) server with Cursor, the AI-first code editor.

## Prerequisites

- Cursor installed
- Coder CLI installed and authenticated
- Active Coder deployment

## Setup

### Automatic Configuration (Recommended)

The Coder CLI can automatically configure MCP for Cursor:

```bash
# First, authenticate with your Coder deployment
coder login https://coder.example.com

# Configure Cursor to use Coder MCP
coder exp mcp configure cursor
```txt

This command will:
- Locate your Cursor configuration directory
- Add the Coder MCP server to Cursor's MCP configuration
- Set up the necessary authentication

### Manual Configuration

If automatic configuration doesn't work, you can manually set up MCP:

1. **Locate Cursor's MCP configuration file**:
   - **macOS**: `~/Library/Application Support/Cursor/User/globalStorage/mcp.json`
   - **Windows**: `%APPDATA%\Cursor\User\globalStorage\mcp.json`
   - **Linux**: `~/.config/Cursor/User/globalStorage/mcp.json`

1. **Add Coder MCP server configuration**:
   ```json
   {
     "mcpServers": {
       "coder": {
         "type": "stdio",
         "command": "coder",
         "args": ["exp", "mcp", "server"],
         "env": {}
       }
     }
   }
   ```

1. **Restart Cursor** to load the new configuration.

## Using Coder MCP in Cursor

Once configured, Cursor's AI can interact with your Coder workspaces through MCP.

### Available Capabilities

**Workspace Management**:

- List all your Coder workspaces
- Create new workspaces from templates
- Start, stop, and delete workspaces
- Check workspace status and resource usage

**Command Execution**:

- Run commands in any workspace
- Execute build scripts and tests
- Install dependencies and packages
- Manage development processes

**Environment Interaction**:

- Access workspace file systems
- Monitor running processes
- Check environment variables and configuration

### Example Interactions

**Creating a development environment**:

```txt
You: "I need a new React workspace for my frontend project"
Cursor AI: Creates a new Coder workspace using a React template, installs dependencies, and sets up the development environment
```txt

**Running tests across workspaces**:
```txt
You: "Run the test suite in my backend workspace"
Cursor AI: Connects to your backend workspace via MCP and executes the test commands, showing results
```txt

**Managing multiple environments**:
```txt
You: "Show me the status of all my workspaces and start the ones that are stopped"
Cursor AI: Lists workspace statuses and starts any stopped workspaces
```txt

## Cursor-Specific Features

### AI Chat Integration
Cursor's AI chat can now reference and interact with your Coder workspaces:
- Ask questions about workspace configurations
- Get help with environment-specific issues
- Receive suggestions based on your actual development setup

### Code Context Enhancement
With MCP, Cursor can:
- Access live code from your Coder workspaces
- Understand your actual development environment
- Provide more accurate suggestions and completions

### Multi-Workspace Development
Cursor can help manage complex projects spanning multiple Coder workspaces:
- Coordinate changes across frontend and backend workspaces
- Manage microservice deployments
- Handle cross-workspace dependencies

## Configuration Options

### Custom MCP Server Settings

You can customize the MCP server behavior in your configuration:

```json
{
  "mcpServers": {
    "coder": {
      "type": "stdio",
      "command": "coder",
      "args": [
        "exp", "mcp", "server",
        "--log-level", "debug",
        "--timeout", "60s"
      ],
      "env": {
        "CODER_MCP_LOG_LEVEL": "debug"
      }
    }
  }
}
```txt

### Environment Variables

Set environment variables to customize MCP behavior:

```bash
# Set log level for debugging
export CODER_MCP_LOG_LEVEL=debug

# Increase timeout for long-running operations
export CODER_MCP_TIMEOUT=120s
```txt

## Troubleshooting

### MCP Server Not Connecting

1. **Check Coder CLI authentication**:
   ```bash
   coder whoami
   ```

1. **Test MCP server manually**:

   ```bash
   coder exp mcp server --help
   ```

1. **Verify Cursor configuration**:
   - Check that the MCP configuration file exists
   - Ensure the file has correct JSON syntax
   - Restart Cursor after making changes

### Cursor AI Not Recognizing MCP

1. **Check Cursor version**: Ensure you're using a version that supports MCP
1. **Restart Cursor**: Sometimes a full restart is needed
1. **Check Cursor logs**: Look for MCP-related errors in Cursor's developer console

### Permission Issues

1. **Verify Coder permissions**: Ensure your user can create and manage workspaces
1. **Check authentication**: Re-authenticate if needed:

   ```bash
   coder login https://coder.example.com
   ```

### Performance Issues

1. **Optimize workspace templates**: Use lightweight templates for faster startup
1. **Monitor resource usage**: Check workspace CPU and memory allocation
1. **Adjust MCP timeout**: Increase timeout for slow operations

## Best Practices

### Security

- Keep your Coder CLI credentials secure
- Use workspace templates with appropriate security configurations
- Regularly review AI assistant access patterns
- Enable audit logging for compliance requirements

### Performance

- Use Coder workspace templates optimized for your development stack
- Consider workspace resource allocation based on AI workload requirements
- Implement workspace auto-stop policies to manage costs

### Development Workflow

- Create project-specific workspace templates
- Use consistent naming conventions for workspaces
- Leverage Coder's collaboration features for team development
- Set up automated workspace provisioning for common project types

## Advanced Usage

### Custom Workspace Templates

Create Coder templates optimized for AI-assisted development:

```hcl
# Example Terraform template for AI development
resource "coder_workspace" "dev" {
  # AI-optimized configuration
  cpu    = 4
  memory = 8192
  
  # Pre-install AI development tools
  startup_script = <<-EOF
    # Install AI development dependencies
    pip install openai anthropic
    npm install -g @cursor/cli
  EOF
}
```txt

### Integration with CI/CD

Use Cursor + Coder MCP for automated development workflows:
- AI-assisted code review in Coder workspaces
- Automated testing and deployment through MCP
- Intelligent environment provisioning based on project needs

## Next Steps

- Explore [Coder Templates](https://registry.coder.com) for AI development
- Learn about [AI coding best practices](../ai-coder/best-practices.md)
- Set up [Coder Tasks](../ai-coder/tasks.md) for background AI operations
- Review [security considerations](../ai-coder/security.md) for AI development

## Support

For Cursor-specific MCP issues:

1. Check [Cursor's MCP documentation](https://docs.cursor.com/mcp) (when available)
1. [Contact Coder Support](https://coder.com/contact) for Coder MCP server issues
1. [Join our Discord](https://discord.gg/coder) for community support
4. [Report bugs](https://github.com/coder/coder/issues) on GitHub
