# MCP Server

Power users can configure Claude Desktop, Cursor, or other external agents to interact with Coder in order to:

- List workspaces
- Create/start/stop workspaces
- Run commands on workspaces
- Check in on agent activity

> [!NOTE]
> See our [toolsdk](https://pkg.go.dev/github.com/coder/coder/v2@v2.24.1/codersdk/toolsdk#pkg-variables) documentation for a full list of tools included in the MCP server

In this model, any custom agent could interact with a remote Coder workspace, or Coder can be used in a remote pipeline or a larger workflow.

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
> The MCP server is authenticated with the same identity as your Coder CLI and can perform any action on the user's behalf. Fine-grained permissions and a remote MCP server are in development. [Contact us](https://coder.com/contact) if this use case is important to you.
