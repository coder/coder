# Using Coder MCP with WindSurf

This guide shows you how to set up Coder's Model Context Protocol (MCP) server with WindSurf, the AI-powered development environment.

## Prerequisites

- WindSurf installed
- Coder CLI installed and authenticated
- Active Coder deployment

## Setup

### Automatic Configuration (Recommended)

The Coder CLI can automatically configure MCP for WindSurf:

```bash
# First, authenticate with your Coder deployment
coder login https://coder.example.com

# Configure WindSurf to use Coder MCP
coder exp mcp configure windsurf
```txt

This command will:
- Locate your WindSurf configuration directory
- Add the Coder MCP server to WindSurf's MCP settings
- Set up the necessary authentication

### Manual Configuration

If automatic configuration doesn't work, you can manually set up MCP:

1. **Locate WindSurf's MCP configuration**:
   - **macOS**: `~/Library/Application Support/WindSurf/User/globalStorage/mcp.json`
   - **Windows**: `%APPDATA%\WindSurf\User\globalStorage\mcp.json`
   - **Linux**: `~/.config/WindSurf/User/globalStorage/mcp.json`

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

1. **Restart WindSurf** to load the new configuration.

## Using Coder MCP in WindSurf

Once configured, WindSurf's AI can interact with your Coder workspaces through MCP, enhancing its already powerful AI capabilities.

### Available Capabilities

**Workspace Management**:

- List and organize your Coder workspaces
- Create new workspaces from templates
- Start, stop, and manage workspace lifecycle
- Monitor workspace status and resource utilization

**AI-Enhanced Development**:

- Execute commands in remote Coder workspaces
- Run AI-assisted code generation in cloud environments
- Perform automated testing and deployment
- Access powerful cloud resources for AI workloads

**Environment Integration**:

- Access workspace file systems and configurations
- Manage environment variables and secrets
- Monitor running processes and services
- Integrate with Coder's security and compliance features

### Example Interactions

**AI-powered project setup**:

```txt
You: "Create a new Next.js workspace with TypeScript and set up a modern web app structure"
WindSurf AI: Creates a Coder workspace, installs Next.js with TypeScript, sets up project structure, and configures development environment
```txt

**Intelligent code generation**:
```txt
You: "Generate a REST API for user management in my backend workspace"
WindSurf AI: Connects to your backend workspace, analyzes existing code, and generates API endpoints with proper error handling and validation
```txt

**Cross-workspace development**:
```txt
You: "Deploy my frontend changes and update the backend API to match"
WindSurf AI: Coordinates changes across multiple Coder workspaces, ensuring consistency between frontend and backend
```txt

## WindSurf-Specific Features

### Enhanced AI Context
With Coder MCP, WindSurf's AI gains access to:
- Live code from your actual development environments
- Real-time workspace status and configuration
- Actual project dependencies and environment setup
- Historical workspace usage patterns

### Cloud-Powered AI Development
Combining WindSurf's AI with Coder's cloud infrastructure:
- Run AI workloads on powerful cloud resources
- Scale development environments based on AI processing needs
- Access GPU-enabled workspaces for machine learning projects
- Leverage enterprise-grade security for AI development

### Collaborative AI Development
WindSurf + Coder enables:
- Team collaboration on AI-assisted projects
- Shared workspace templates for consistent AI development
- Centralized management of AI development environments
- Audit trails for AI-generated code and changes

## Configuration Options

### Advanced MCP Settings

Customize the MCP server for optimal WindSurf integration:

```json
{
  "mcpServers": {
    "coder": {
      "type": "stdio",
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
```txt

### Environment Variables

Optimize MCP for AI workloads:

```bash
# Enable detailed logging for AI operations
export CODER_MCP_LOG_LEVEL=debug

# Increase timeout for long-running AI tasks
export CODER_MCP_TIMEOUT=300s

# Set custom workspace preferences
export CODER_MCP_DEFAULT_TEMPLATE=ai-development
```txt

## Troubleshooting

### MCP Server Connection Issues

1. **Verify Coder CLI setup**:
   ```bash
   coder version
   coder whoami
   coder workspaces list
   ```

1. **Test MCP server independently**:

   ```bash
   coder exp mcp server --log-level debug
   ```

1. **Check WindSurf configuration**:
   - Verify MCP configuration file syntax
   - Ensure WindSurf has necessary permissions
   - Check WindSurf's developer console for errors

### AI Performance Issues

1. **Optimize workspace resources**:
   - Ensure adequate CPU and memory for AI workloads
   - Consider GPU-enabled workspaces for ML tasks
   - Monitor workspace resource utilization

1. **Network optimization**:
   - Check network latency to Coder deployment
   - Ensure stable internet connection
   - Consider workspace location relative to your location

### WindSurf AI Not Using MCP

1. **Verify MCP integration**: Ensure WindSurf recognizes the MCP server
1. **Check AI model settings**: Some AI models may need explicit MCP enablement
1. **Restart WindSurf**: Full restart may be needed after configuration changes

## Best Practices

### Security

- Secure Coder CLI credentials and authentication tokens
- Use workspace templates with appropriate security configurations
- Regularly audit AI assistant access to sensitive workspaces
- Enable comprehensive audit logging for compliance
- Implement proper secret management in workspaces

### Performance

- Use workspace templates optimized for AI development
- Allocate appropriate resources based on AI workload requirements
- Implement intelligent workspace auto-stop policies
- Monitor and optimize MCP timeout settings for AI operations
- Consider workspace caching strategies for frequently used environments

### AI Development Workflow

- Create specialized workspace templates for different AI use cases
- Use consistent naming conventions for AI projects and workspaces
- Implement version control best practices for AI-generated code
- Set up automated testing for AI-generated components
- Establish code review processes for AI-assisted development

## Advanced Usage

### AI-Optimized Workspace Templates

Create Coder templates specifically for AI development with WindSurf:

```hcl
# Terraform template for AI development
resource "coder_workspace" "ai_dev" {
  name = "windsurf-ai-development"
  
  # High-performance configuration for AI workloads
  cpu    = 8
  memory = 16384
  
  # GPU support for ML workloads
  gpu_enabled = true
  gpu_type    = "nvidia-t4"
  
  # Pre-install AI development stack
  startup_script = <<-EOF
    # Install AI/ML frameworks
    pip install torch tensorflow transformers
    pip install openai anthropic langchain
    
    # Install development tools
    npm install -g typescript @types/node
    
    # Set up Jupyter for experimentation
    pip install jupyter jupyterlab
  EOF
}
```txt

### Integration with AI Workflows

Set up automated AI development workflows:

```bash
# Example: Automated model training workflow
#!/bin/bash

# Create workspace for training
coder create ml-training --template ai-gpu

# Start training job
coder ssh ml-training -- python train_model.py

# Monitor training progress
coder ssh ml-training -- tensorboard --logdir ./logs
```txt

### Team AI Development

Implement team-wide AI development practices:
- Share AI-optimized workspace templates
- Establish common AI development environments
- Implement collaborative model development workflows
- Set up centralized model and dataset management

## Next Steps

- Explore [AI-optimized Coder Templates](https://registry.coder.com) for machine learning
- Learn about [AI coding best practices](../ai-coder/best-practices.md) with Coder
- Set up [Coder Tasks](../ai-coder/tasks.md) for background AI model training
- Review [security considerations](../ai-coder/security.md) for AI development
- Check out WindSurf's [AI features documentation](https://windsurf.ai/docs) for advanced usage

## Support

For WindSurf-specific MCP issues:

1. Check [WindSurf's documentation](https://windsurf.ai/docs) for MCP support details
1. [Contact Coder Support](https://coder.com/contact) for Coder MCP server issues
1. [Join our Discord](https://discord.gg/coder) for community support and AI development discussions
4. [Report bugs](https://github.com/coder/coder/issues) on the Coder GitHub repository
5. Check WindSurf's support channels for WindSurf-specific issues

## Additional Resources

- [WindSurf AI Development Guide](https://windsurf.ai/docs/ai-development)
- [Coder AI Templates](https://registry.coder.com/templates?category=ai)
- [MCP Protocol Specification](https://modelcontextprotocol.io/)
- [AI Development Best Practices](../ai-coder/best-practices.md)
