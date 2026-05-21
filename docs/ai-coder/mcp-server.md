# MCP Server

Power users can configure [claude.ai](https://claude.ai), Claude Desktop, Cursor, or other external agents to interact with Coder in order to:

- List workspaces
- Create/start/stop workspaces
- Run commands on workspaces
- Check in on agent activity

> [!NOTE]
> See our [toolsdk](https://pkg.go.dev/github.com/coder/coder/v2/codersdk/toolsdk#pkg-variables) documentation for a full list of tools included in the MCP server

In this model, any custom agent could interact with a remote Coder workspace, or Coder can be used in a remote pipeline or a larger workflow.

## Local MCP server

The Coder CLI has options to automatically configure MCP servers for you. On your local machine, run the following command:

```sh
# First log in to Coder. 
coder login <https://coder.example.com>

# Configure your client with the Coder MCP
coder exp mcp configure claude-desktop # Configure Claude Desktop to interact with Coder
coder exp mcp configure cursor # Configure Cursor to interact with Coder
```

For other agents, run the MCP server with this command:

```sh
coder exp mcp server
```

> [!NOTE]
> The MCP server is authenticated with the same identity as your Coder CLI and can perform any action on the user's behalf. Fine-grained permissions are in development. [Contact us](https://coder.com/contact) if this use case is important to you.

## Remote MCP server

Coder can expose an MCP server via HTTP. This is useful for connecting web-based agents, like https://claude.ai/, to Coder. This is an experimental feature and is subject to change.

To enable this feature, activate the `oauth2` and `mcp-server-http` experiments using an environment variable or a CLI flag:

```sh
CODER_EXPERIMENTS="oauth2,mcp-server-http" coder server
# or
coder server --experiments=oauth2,mcp-server-http
```

The Coder server will expose the MCP server at:

```txt
https://coder.example.com/api/experimental/mcp/http
```

> [!NOTE]
> At this time, the remote MCP server is not compatible with web-based ChatGPT.

Users can authenticate applications to use the remote MCP server with [OAuth2](../admin/integrations/oauth2-provider.md). An authenticated application can perform any action on the user's behalf. Fine-grained permissions are in development.
