# Using Coder MCP with Zed

This guide shows you how to set up Coder's Model Context Protocol (MCP) server with Zed, the high-performance collaborative code editor.

## Prerequisites

- Zed installed
- Coder CLI installed and authenticated
- Active Coder deployment

## Setup

### Automatic Configuration (Recommended)

The Coder CLI can automatically configure MCP for Zed:

```bash
# First, authenticate with your Coder deployment
coder login https://coder.example.com

# Configure Zed to use Coder MCP
coder exp mcp configure zed
```txt

This command will:
- Locate your Zed configuration directory
- Add the Coder MCP server to Zed's settings
- Configure the necessary authentication

### Manual Configuration

If automatic configuration doesn't work, you can manually set up MCP:

1. **Open Zed settings**:
   - Press `Cmd+,` (macOS) or `Ctrl+,` (Linux/Windows)
   - Or go to `Zed > Settings` in the menu

1. **Add MCP server configuration**:
   ```json
   {
     "experimental": {
       "mcp": {
         "servers": {
           "coder": {
             "command": "coder",
             "args": ["exp", "mcp", "server"],
             "env": {}
           }
         }
       }
     }
   }
   ```

1. **Save settings** and restart Zed.

### Alternative: Configuration File

You can also edit Zed's configuration file directly:

- **macOS**: `~/Library/Application Support/Zed/settings.json`
- **Linux**: `~/.config/zed/settings.json`
- **Windows**: `%APPDATA%\Zed\settings.json`

## Using Coder MCP in Zed

Once configured, Zed's AI assistant can interact with your Coder workspaces through MCP.

### Available Capabilities

**Workspace Operations**:

- List and filter your Coder workspaces
- Create workspaces from templates
- Start, stop, and manage workspace lifecycle
- Monitor workspace resource usage and status

**Development Commands**:

- Execute commands in remote workspaces
- Run build scripts, tests, and deployment commands
- Install packages and manage dependencies
- Access workspace terminals and processes

**Environment Management**:

- Access workspace file systems and configurations
- Monitor running services and applications
- Manage environment variables and secrets

### Example Interactions

**Setting up a new project**:

```txt
You: "Create a Rust workspace for my new CLI tool project"
Zed AI: Creates a Coder workspace with Rust toolchain, sets up cargo project structure, and configures development environment
```txt

**Running development tasks**:
```txt
You: "Build and test my application in the backend workspace"
Zed AI: Executes cargo build and cargo test in your specified Coder workspace, showing output and results
```txt

**Managing multiple environments**:
```txt
You: "Check which workspaces are running and their resource usage"
Zed AI: Lists all workspaces with their status, CPU, memory usage, and uptime
```txt

## Zed-Specific Features

### AI Assistant Integration
Zed's AI assistant can now:
- Reference live code from your Coder workspaces
- Understand your actual development environment setup
- Provide context-aware suggestions based on your workspace configuration

### Collaborative Development
With Zed's collaboration features and Coder MCP:
- Share Coder workspaces with team members through Zed
- Collaborate on code in remote development environments
- Coordinate development across multiple workspaces

### Performance Optimization
Zed's high-performance architecture combined with Coder:
- Fast interaction with remote workspaces
- Efficient handling of large codebases in Coder environments
- Optimized AI responses using live workspace context

## Configuration Options

### Advanced MCP Settings

Customize the MCP server behavior:

```json
{
  "experimental": {
    "mcp": {
      "servers": {
        "coder": {
          "command": "coder",
          "args": [
            "exp", "mcp", "server",
            "--log-level", "info",
            "--timeout", "30s"
          ],
          "env": {
            "CODER_MCP_LOG_LEVEL": "info",
            "CODER_MCP_TIMEOUT": "30s"
          }
        }
      }
    }
  }
}
```txt

### Environment Variables

Configure MCP behavior through environment variables:

```bash
# Enable debug logging
export CODER_MCP_LOG_LEVEL=debug

# Set custom timeout
export CODER_MCP_TIMEOUT=60s

# Start Zed with custom environment
zed
```txt

## Troubleshooting

### MCP Server Not Starting

1. **Verify Coder CLI installation**:
   ```bash
   coder version
   coder whoami
   ```

1. **Test MCP server manually**:

   ```bash
   coder exp mcp server --help
   ```

1. **Check Zed configuration**:
   - Ensure JSON syntax is correct in settings
   - Verify the MCP configuration is in the right location
   - Check Zed's developer console for errors

### Zed Not Recognizing MCP

1. **Check Zed version**: Ensure you're using a version with MCP support
1. **Verify experimental features**: Make sure MCP is enabled in experimental settings
1. **Restart Zed**: A full restart may be needed after configuration changes

### Connection Issues

1. **Check network connectivity**: Ensure you can reach your Coder deployment
1. **Verify authentication**: Re-authenticate with Coder:

   ```bash
   coder login https://coder.example.com
   ```

1. **Check firewall settings**: Ensure MCP traffic isn't blocked

### Performance Issues

1. **Optimize workspace resources**: Ensure adequate CPU and memory allocation
1. **Check network latency**: High latency can affect MCP performance
1. **Monitor workspace load**: Heavy workspace usage can slow MCP responses

## Best Practices

### Security

- Secure your Coder CLI credentials and tokens
- Use workspace templates with appropriate security configurations
- Regularly audit AI assistant access to workspaces
- Enable Coder's audit logging for compliance

### Performance

- Use lightweight workspace templates for faster operations
- Optimize workspace resource allocation based on project needs
- Implement workspace auto-stop policies to manage costs
- Monitor and tune MCP timeout settings

### Development Workflow

- Create standardized workspace templates for different project types
- Use consistent naming conventions for workspaces and projects
- Leverage Zed's collaboration features with Coder workspaces
- Set up automated workspace provisioning workflows

## Advanced Usage

### Custom Workspace Templates

Create Coder templates optimized for Zed development:

```hcl
# Terraform template for Zed + AI development
resource "coder_workspace" "zed_dev" {
  name = "zed-development"
  
  # Optimized for AI development
  cpu    = 4
  memory = 8192
  
  # Pre-configure development tools
  startup_script = <<-EOF
    # Install language servers for Zed
    rustup component add rust-analyzer
    npm install -g typescript-language-server
    
    # Set up AI development dependencies
    pip install openai anthropic langchain
  EOF
}
```txt

### Integration with Zed Extensions

Combine Coder MCP with Zed extensions:
- Use language server extensions with Coder workspace environments
- Integrate version control extensions with Coder's Git support
- Combine AI coding assistants with Coder's remote development capabilities

### Team Collaboration

Set up team workflows with Zed and Coder:
- Share workspace templates across the team
- Use Zed's collaboration features with shared Coder workspaces
- Implement code review workflows using both platforms

## Next Steps

- Explore [Coder Templates](https://registry.coder.com) optimized for your tech stack
- Learn about [AI coding best practices](../ai-coder/best-practices.md) with Coder
- Set up [Coder Tasks](../ai-coder/tasks.md) for background AI operations
- Review [security considerations](../ai-coder/security.md) for AI development
- Check out Zed's [collaboration features](https://zed.dev/docs/collaboration) with Coder

## Support

For Zed-specific MCP issues:

1. Check [Zed's documentation](https://zed.dev/docs) for MCP support details
1. [Contact Coder Support](https://coder.com/contact) for Coder MCP server issues
1. [Join our Discord](https://discord.gg/coder) for community support
4. [Report bugs](https://github.com/coder/coder/issues) on the Coder GitHub repository
5. Check [Zed's GitHub](https://github.com/zed-industries/zed) for Zed-specific issues
