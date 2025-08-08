# Model Context Protocol (MCP) with Coder

The Model Context Protocol (MCP) is an open standard that enables AI assistants to securely connect to external data sources and tools. Coder provides a comprehensive MCP server that allows AI agents to interact with your development environments, workspaces, and infrastructure.

## What is Coder MCP?

Coder MCP is a server implementation that exposes Coder's functionality through the Model Context Protocol, enabling AI assistants to:

- **Manage Workspaces**: List, create, start, stop, and delete development workspaces
- **Execute Commands**: Run commands directly in your Coder workspaces
- **Monitor Activity**: Check on agent activity and workspace status
- **Access Resources**: Interact with your cloud development environments securely

## Benefits of Using Coder MCP

### Secure Remote Development

- Keep your code and secrets in controlled, isolated environments
- Leverage powerful cloud resources without local machine limitations
- Maintain consistent development environments across your team

### Enhanced AI Productivity

- AI agents can directly interact with your actual development environment
- No need to copy code or context - agents work with live workspaces
- Seamless integration with existing Coder infrastructure

### Enterprise-Ready

- Built on Coder's enterprise-grade security and compliance features
- Fine-grained access controls and audit logging
- Self-hosted solution that keeps your data in your infrastructure

## How It Works

Coder MCP acts as a bridge between AI assistants and your Coder deployment:

1. **AI Assistant** connects to Coder MCP server
2. **MCP Server** authenticates and authorizes requests
3. **Coder API** executes operations on workspaces and infrastructure
4. **Results** are returned to the AI assistant for further processing

```mermaid
graph LR
    A[AI Assistant] --> B[Coder MCP Server]
    B --> C[Coder API]
    C --> D[Workspaces]
    C --> E[Infrastructure]
```txt

## Supported AI Tools and IDEs

Coder MCP works with various AI assistants and development environments:

- **[VSCode](./vscode.md)** - Microsoft Visual Studio Code with AI extensions
- **[Cursor](./cursor.md)** - AI-first code editor
- **[Zed](./zed.md)** - High-performance collaborative code editor
- **[WindSurf](./windsurf.md)** - AI-powered development environment
- **[Claude Desktop](./claude-desktop.md)** - Anthropic's desktop AI assistant
- **[Web-based Agents](./web-agents.md)** - Browser-based AI assistants like claude.ai

## Getting Started

### Prerequisites

- A running Coder deployment
- Coder CLI installed and authenticated
- An AI assistant or IDE that supports MCP

### Quick Setup

1. **Configure your AI tool**: Choose your preferred AI assistant or IDE from the list above
2. **Set up MCP connection**: Follow the specific guide for your chosen tool
3. **Start coding**: Your AI assistant can now interact with your Coder workspaces

### Local MCP Server

For local development and testing:

```bash
# Authenticate with your Coder deployment
coder login https://coder.example.com

# Start the MCP server
coder exp mcp server
```txt

### Remote MCP Server

For web-based AI assistants, enable the HTTP MCP server:

```bash
# Enable experimental features
CODER_EXPERIMENTS="oauth2,mcp-server-http" coder server
```txt

The MCP server will be available at:
```txt
https://coder.example.com/api/experimental/mcp/http
```txt

## Security and Authentication

Coder MCP inherits Coder's robust security model:

- **OAuth2 Authentication**: Secure token-based authentication for web agents
- **CLI Authentication**: Uses your existing Coder CLI credentials for local tools
- **Role-Based Access**: Respects your Coder RBAC permissions
- **Audit Logging**: All MCP operations are logged for compliance

> **Note**: The MCP server operates with the same permissions as the authenticated user. Fine-grained MCP-specific permissions are in development.

## Available Tools and Capabilities

The Coder MCP server provides access to a comprehensive set of tools. See the [toolsdk documentation](https://pkg.go.dev/github.com/coder/coder/v2@latest/codersdk/toolsdk#pkg-variables) for the complete list of available tools.

## Next Steps

- Choose your preferred AI tool and follow its setup guide
- Explore the [best practices](../ai-coder/best-practices.md) for AI coding with Coder
- Learn about [security considerations](../ai-coder/security.md) when using AI agents
- Check out [Coder Tasks](../ai-coder/tasks.md) for background AI agent execution

## Support and Feedback

Coder MCP is currently in beta. For questions, issues, or feature requests:

- [Contact Coder Support](https://coder.com/contact)
- [Join our Discord Community](https://discord.gg/coder)
- [Report Issues on GitHub](https://github.com/coder/coder/issues)
