# Using Coder MCP with Claude Desktop

This guide shows you how to set up Coder's Model Context Protocol (MCP) server with Claude Desktop, Anthropic's desktop AI assistant.

## Prerequisites

- Claude Desktop installed
- Coder CLI installed and authenticated
- Active Coder deployment

## Setup

### Automatic Configuration (Recommended)

The Coder CLI can automatically configure MCP for Claude Desktop:

```bash
# First, authenticate with your Coder deployment
coder login https://coder.example.com

# Configure Claude Desktop to use Coder MCP
coder exp mcp configure claude-desktop
```

This command will:
- Locate your Claude Desktop configuration file
- Add the Coder MCP server configuration
- Set up the necessary authentication

### Manual Configuration

If automatic configuration doesn't work, you can manually set up MCP:

1. **Locate Claude Desktop's configuration file**:
   - **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
   - **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`
   - **Linux**: `~/.config/Claude/claude_desktop_config.json`

2. **Add Coder MCP server configuration**:
   ```json
   {
     "mcpServers": {
       "coder": {
         "command": "coder",
         "args": ["exp", "mcp", "server"],
         "env": {}
       }
     }
   }
   ```

3. **Restart Claude Desktop** to load the new configuration.

## Using Coder MCP with Claude Desktop

Once configured, Claude can interact with your Coder workspaces through MCP, providing powerful AI assistance for your development workflows.

### Available Capabilities

**Workspace Management**:
- List and inspect your Coder workspaces
- Create new workspaces from templates
- Start, stop, and manage workspace lifecycle
- Monitor workspace status and resource usage

**Development Operations**:
- Execute commands in remote workspaces
- Run build scripts, tests, and deployment processes
- Install packages and manage dependencies
- Access workspace terminals and file systems

**AI-Assisted Development**:
- Analyze code in your actual development environments
- Generate code that works with your specific workspace setup
- Debug issues using live workspace context
- Provide recommendations based on your actual infrastructure

### Example Interactions

**Project setup and initialization**:
```
You: "I need to set up a new Python web application with FastAPI. Can you create a workspace and set up the project structure?"

Claude: I'll help you create a new Coder workspace and set up a FastAPI project. Let me:
1. Create a new workspace using a Python template
2. Install FastAPI and dependencies
3. Set up the basic project structure
4. Create a simple API endpoint to get you started

[Claude uses MCP to create workspace, install dependencies, and set up project]
```

**Debugging and troubleshooting**:
```
You: "My application in the backend workspace is failing to start. Can you help me debug it?"

Claude: I'll help you debug the startup issue. Let me:
1. Check the workspace status
2. Examine the application logs
3. Verify the configuration
4. Identify and fix the issue

[Claude uses MCP to access workspace, check logs, and diagnose the problem]
```

**Code review and optimization**:
```
You: "Can you review the code in my frontend workspace and suggest improvements?"

Claude: I'll review your frontend code and provide suggestions. Let me:
1. Access your workspace and examine the codebase
2. Analyze the code structure and patterns
3. Identify potential improvements
4. Suggest specific optimizations

[Claude uses MCP to access code and provide detailed review]
```

## Claude Desktop-Specific Features

### Conversational Development
Claude Desktop's conversational interface combined with Coder MCP enables:
- Natural language interaction with your development environments
- Context-aware responses based on your actual workspace state
- Iterative development conversations that maintain workspace context

### Multi-Workspace Coordination
Claude can help coordinate work across multiple Coder workspaces:
- Manage microservice architectures spanning multiple workspaces
- Coordinate frontend and backend development
- Handle complex deployment scenarios across environments

### Documentation and Learning
Use Claude with Coder MCP for:
- Generating documentation based on your actual codebase
- Learning about your specific development setup
- Getting explanations of complex workspace configurations

## Configuration Options

### Advanced MCP Settings

Customize the MCP server behavior for Claude Desktop:

```json
{
  "mcpServers": {
    "coder": {
      "command": "coder",
      "args": [
        "exp", "mcp", "server",
        "--log-level", "info",
        "--timeout", "60s"
      ],
      "env": {
        "CODER_MCP_LOG_LEVEL": "info",
        "CODER_MCP_TIMEOUT": "60s"
      }
    }
  }
}
```

### Environment Variables

Set environment variables to customize MCP behavior:

```bash
# Enable debug logging
export CODER_MCP_LOG_LEVEL=debug

# Increase timeout for long operations
export CODER_MCP_TIMEOUT=120s

# Start Claude Desktop with custom environment
open -a "Claude Desktop"
```

## Troubleshooting

### MCP Server Not Connecting

1. **Verify Coder CLI authentication**:
   ```bash
   coder whoami
   coder workspaces list
   ```

2. **Test MCP server manually**:
   ```bash
   coder exp mcp server --log-level debug
   ```

3. **Check Claude Desktop configuration**:
   - Verify the configuration file exists and has correct syntax
   - Ensure the file path is correct for your operating system
   - Check Claude Desktop's logs for MCP-related errors

### Claude Not Recognizing MCP Tools

1. **Restart Claude Desktop**: A full restart is often needed after configuration changes
2. **Check MCP server status**: Ensure the MCP server is running and accessible
3. **Verify permissions**: Ensure Claude Desktop has necessary permissions to execute the Coder CLI

### Performance Issues

1. **Optimize workspace resources**: Ensure workspaces have adequate resources
2. **Check network connectivity**: Verify stable connection to your Coder deployment
3. **Monitor MCP timeouts**: Adjust timeout settings for long-running operations

### Authentication Problems

1. **Re-authenticate with Coder**:
   ```bash
   coder login https://coder.example.com
   ```

2. **Check token expiration**: Verify your Coder authentication token is still valid
3. **Verify permissions**: Ensure your user has appropriate workspace permissions

## Best Practices

### Security
- Keep your Coder CLI credentials secure and up to date
- Use workspace templates with appropriate security configurations
- Regularly review Claude's access to your workspaces
- Enable audit logging for compliance and monitoring
- Implement proper secret management in workspaces

### Effective AI Interaction
- Be specific about which workspace you want Claude to work with
- Provide context about your project goals and constraints
- Ask Claude to explain its actions when working with your workspaces
- Use iterative conversations to refine development tasks

### Development Workflow
- Create standardized workspace templates for different project types
- Use consistent naming conventions for workspaces and projects
- Establish clear boundaries for what Claude should and shouldn't modify
- Implement version control best practices for AI-assisted changes

## Advanced Usage

### Custom Workspace Templates

Create Coder templates optimized for AI-assisted development:

```hcl
# Terraform template for AI-assisted development
resource "coder_workspace" "claude_dev" {
  name = "claude-development"
  
  # Optimized for AI development workflows
  cpu    = 4
  memory = 8192
  
  # Pre-install development tools
  startup_script = <<-EOF
    # Install common development tools
    apt-get update && apt-get install -y git curl wget
    
    # Install language-specific tools
    curl -fsSL https://deb.nodesource.com/setup_18.x | bash -
    apt-get install -y nodejs
    
    # Install Python and pip
    apt-get install -y python3 python3-pip
    
    # Set up development environment
    echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
  EOF
}
```

### Automated Workflows

Use Claude with Coder MCP for automated development workflows:

```bash
# Example: Automated project setup script
#!/bin/bash

# This script can be executed by Claude through MCP
echo "Setting up new project workspace..."

# Create workspace
coder create my-project --template web-development

# Wait for workspace to be ready
coder ssh my-project -- 'echo "Workspace ready"'

# Initialize project
coder ssh my-project -- 'git init && npm init -y'

echo "Project setup complete!"
```

### Integration with Development Tools

Combine Claude Desktop with other development tools:
- Use Claude to generate code that integrates with your existing CI/CD pipelines
- Have Claude help set up monitoring and logging in your Coder workspaces
- Use Claude to generate documentation that reflects your actual workspace setup

## Next Steps

- Explore [Coder Templates](https://registry.coder.com) for different development stacks
- Learn about [AI coding best practices](../ai-coder/best-practices.md) with Coder
- Set up [Coder Tasks](../ai-coder/tasks.md) for background AI operations
- Review [security considerations](../ai-coder/security.md) for AI development
- Check out Claude's [advanced features](https://docs.anthropic.com/claude/docs) for development workflows

## Support

For Claude Desktop-specific MCP issues:

1. Check [Claude Desktop documentation](https://docs.anthropic.com/claude/docs/claude-desktop) for MCP support
2. [Contact Coder Support](https://coder.com/contact) for Coder MCP server issues
3. [Join our Discord](https://discord.gg/coder) for community support
4. [Report bugs](https://github.com/coder/coder/issues) on the Coder GitHub repository
5. Contact [Anthropic Support](https://support.anthropic.com/) for Claude Desktop-specific issues

## Additional Resources

- [Model Context Protocol Documentation](https://modelcontextprotocol.io/)
- [Claude Desktop User Guide](https://docs.anthropic.com/claude/docs/claude-desktop)
- [Coder CLI Reference](../../reference/cli.md)
- [Workspace Templates Guide](../../tutorials/templates.md)
