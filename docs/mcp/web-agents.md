# Using Coder MCP with Web-Based AI Agents

This guide shows you how to set up Coder's Model Context Protocol (MCP) server with web-based AI agents like claude.ai, ChatGPT, and other browser-based AI assistants.

## Prerequisites

- A running Coder deployment with experimental features enabled
- Coder CLI installed and authenticated
- A web browser with access to AI assistant platforms

## Setup

### Enable HTTP MCP Server

Web-based AI agents require the HTTP MCP server, which is currently experimental:

```bash
# Enable experimental features on your Coder server
CODER_EXPERIMENTS="oauth2,mcp-server-http" coder server

# Or using CLI flags
coder server --experiments=oauth2,mcp-server-http
```

### Access the MCP Server

Once enabled, the HTTP MCP server will be available at:

```
https://your-coder-deployment.com/api/experimental/mcp/http
```

### Authentication Setup

Web-based agents authenticate using OAuth2:

1. **Navigate to your Coder deployment's OAuth2 settings**
2. **Create a new OAuth2 application** for your AI agent
3. **Configure the redirect URI** based on your AI agent's requirements
4. **Note the client ID and client secret** for configuration

## Supported Web-Based AI Agents

### Claude.ai (Anthropic)

Claude.ai supports MCP through its web interface:

1. **Open claude.ai** in your browser
2. **Go to Settings** > **Integrations** > **MCP Servers**
3. **Add a new MCP server**:
   - **Name**: Coder
   - **URL**: `https://your-coder-deployment.com/api/experimental/mcp/http`
   - **Authentication**: OAuth2
4. **Complete OAuth2 authentication** when prompted

### ChatGPT (OpenAI)

> **Note**: ChatGPT's MCP support is limited and may not be compatible with the HTTP MCP server at this time.

For ChatGPT integration:
1. Check OpenAI's documentation for current MCP support
2. Use custom GPTs with API integrations as an alternative
3. Consider using the OpenAI API with local MCP tools

### Other Web-Based Agents

For other web-based AI agents that support MCP:

1. **Check the agent's MCP documentation** for setup instructions
2. **Use the HTTP MCP endpoint**: `https://your-coder-deployment.com/api/experimental/mcp/http`
3. **Configure OAuth2 authentication** using your Coder deployment's OAuth2 settings

## Using Coder MCP with Web Agents

Once configured, web-based AI agents can interact with your Coder workspaces through the HTTP MCP server.

### Available Capabilities

**Workspace Operations**:
- List and filter your Coder workspaces
- Create new workspaces from templates
- Start, stop, and manage workspace lifecycle
- Monitor workspace status and resource usage

**Remote Command Execution**:
- Execute commands in any accessible workspace
- Run build scripts, tests, and deployment commands
- Install packages and manage dependencies
- Access workspace file systems and configurations

**Development Assistance**:
- Analyze code in your actual development environments
- Generate code that works with your specific setup
- Debug issues using live workspace context
- Provide infrastructure-aware recommendations

### Example Interactions

**Project initialization**:
```
You: "I need to create a new React workspace and set up a modern web application with TypeScript and Tailwind CSS."

AI Agent: I'll help you create a new Coder workspace and set up your React application. Let me:
1. Create a new workspace using a Node.js template
2. Initialize a new React project with TypeScript
3. Install and configure Tailwind CSS
4. Set up the basic project structure

[Agent uses HTTP MCP to create workspace and execute setup commands]
```

**Cross-workspace development**:
```
You: "Deploy my frontend changes to staging and update the backend API documentation to reflect the new endpoints."

AI Agent: I'll coordinate the deployment across your workspaces:
1. Build and deploy frontend changes from your frontend workspace
2. Update API documentation in your backend workspace
3. Verify the deployment is successful
4. Run integration tests to ensure everything works together

[Agent uses HTTP MCP to coordinate across multiple workspaces]
```

**Infrastructure management**:
```
You: "Check the status of all my workspaces and optimize resource allocation."

AI Agent: I'll analyze your workspace usage and optimize resources:
1. List all workspaces with their current status
2. Check resource utilization (CPU, memory, storage)
3. Identify underutilized or overloaded workspaces
4. Suggest resource optimization strategies

[Agent uses HTTP MCP to gather workspace metrics and provide recommendations]
```

## Configuration Options

### OAuth2 Configuration

Configure OAuth2 settings for web agent authentication:

```yaml
# Example OAuth2 configuration
oauth2:
  github:
    client_id: "your-client-id"
    client_secret: "your-client-secret"
    redirect_url: "https://your-coder-deployment.com/api/v2/oauth2/github/callback"
    scopes:
      - "read:user"
      - "user:email"
```

### HTTP MCP Server Settings

Customize the HTTP MCP server behavior:

```bash
# Environment variables for HTTP MCP server
export CODER_MCP_HTTP_TIMEOUT=120s
export CODER_MCP_HTTP_MAX_REQUESTS=100
export CODER_MCP_HTTP_RATE_LIMIT=10

# Start server with custom settings
CODER_EXPERIMENTS="oauth2,mcp-server-http" coder server
```

### CORS Configuration

Configure CORS for web agent access:

```bash
# Allow specific origins for web agents
export CODER_ACCESS_URL="https://your-coder-deployment.com"
export CODER_WILDCARD_ACCESS_URL="*.your-domain.com"
```

## Security Considerations

### Authentication and Authorization

- **OAuth2 Security**: Use strong client secrets and secure redirect URIs
- **Token Management**: Implement proper token rotation and expiration
- **Scope Limitation**: Grant minimal necessary permissions to AI agents
- **Audit Logging**: Enable comprehensive logging for all MCP operations

### Network Security

- **HTTPS Only**: Ensure all MCP communication uses HTTPS
- **Firewall Rules**: Restrict access to the MCP endpoint as needed
- **Rate Limiting**: Implement rate limiting to prevent abuse
- **IP Allowlisting**: Consider restricting access to known AI agent IPs

### Data Protection

- **Sensitive Data**: Be cautious about exposing sensitive workspace data
- **Access Controls**: Use Coder's RBAC to limit workspace access
- **Data Retention**: Understand how AI agents handle and store data
- **Compliance**: Ensure setup meets your organization's compliance requirements

## Troubleshooting

### HTTP MCP Server Not Starting

1. **Check experimental features**:
   ```bash
   coder server --help | grep experiments
   ```

2. **Verify server configuration**:
   ```bash
   curl -I https://your-coder-deployment.com/api/experimental/mcp/http
   ```

3. **Check server logs** for MCP-related errors

### OAuth2 Authentication Issues

1. **Verify OAuth2 configuration**:
   - Check client ID and secret
   - Verify redirect URIs
   - Ensure proper scopes are configured

2. **Test OAuth2 flow manually**:
   ```bash
   curl -X POST https://your-coder-deployment.com/api/v2/oauth2/authorize
   ```

### Web Agent Connection Problems

1. **Check CORS settings**: Ensure the web agent's domain is allowed
2. **Verify network connectivity**: Test access to the MCP endpoint
3. **Review browser console**: Check for JavaScript errors or network issues
4. **Test with curl**: Verify the endpoint is accessible:
   ```bash
   curl -H "Authorization: Bearer your-token" \
        https://your-coder-deployment.com/api/experimental/mcp/http
   ```

### Performance Issues

1. **Monitor server resources**: Check CPU, memory, and network usage
2. **Optimize workspace allocation**: Ensure adequate resources for workspaces
3. **Implement caching**: Use appropriate caching strategies for MCP responses
4. **Rate limiting**: Adjust rate limits based on usage patterns

## Best Practices

### Security
- Regularly rotate OAuth2 credentials
- Monitor and audit AI agent access patterns
- Implement proper secret management
- Use least-privilege access principles
- Keep Coder deployment and dependencies updated

### Performance
- Monitor MCP server performance and resource usage
- Implement appropriate rate limiting and throttling
- Use efficient workspace templates
- Consider geographic distribution for global teams

### Operational
- Document AI agent configurations and access patterns
- Implement monitoring and alerting for MCP operations
- Establish incident response procedures
- Regular backup and disaster recovery testing

## Advanced Usage

### Custom Web Agent Integration

For custom web applications that need MCP integration:

```javascript
// Example JavaScript integration
const mcpClient = {
  baseUrl: 'https://your-coder-deployment.com/api/experimental/mcp/http',
  token: 'your-oauth2-token',
  
  async listWorkspaces() {
    const response = await fetch(`${this.baseUrl}/workspaces`, {
      headers: {
        'Authorization': `Bearer ${this.token}`,
        'Content-Type': 'application/json'
      }
    });
    return response.json();
  },
  
  async executeCommand(workspaceId, command) {
    const response = await fetch(`${this.baseUrl}/workspaces/${workspaceId}/execute`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${this.token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ command })
    });
    return response.json();
  }
};
```

### Enterprise Integration

For enterprise deployments:

- Integrate with existing SSO providers
- Implement custom authentication flows
- Set up monitoring and compliance reporting
- Configure high availability and load balancing

## Next Steps

- Explore [Coder Templates](https://registry.coder.com) for web development
- Learn about [AI coding best practices](../ai-coder/best-practices.md)
- Set up [Coder Tasks](../ai-coder/tasks.md) for background operations
- Review [security considerations](../ai-coder/security.md) for AI development
- Check out [OAuth2 provider documentation](../../admin/integrations/oauth2-provider.md)

## Support

For web agent MCP issues:

1. [Contact Coder Support](https://coder.com/contact) for Coder MCP server issues
2. [Join our Discord](https://discord.gg/coder) for community support
3. [Report bugs](https://github.com/coder/coder/issues) on GitHub
4. Check your AI agent's documentation for platform-specific MCP support
5. Review the [Model Context Protocol specification](https://modelcontextprotocol.io/)

## Additional Resources

- [Model Context Protocol Documentation](https://modelcontextprotocol.io/)
- [Coder OAuth2 Provider Guide](../../admin/integrations/oauth2-provider.md)
- [Coder API Documentation](../../reference/api.md)
- [Web Security Best Practices](../../admin/security.md)
