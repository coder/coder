# MCP

[Model Context Protocol (MCP)](https://modelcontextprotocol.io/docs/getting-started/intro) is a mechanism for connecting AI applications to external systems.

AI Bridge can connect to MCP servers and inject tools automatically, enabling you to centrally manage the list of tools you wish to grant your users.

> [!NOTE]
> Only MCP servers which support OAuth2 Authorization are supported currently.
>
> [_Streamable HTTP_](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports#streamable-http) is the only supported transport currently. In future releases we will support the (now deprecated) [_Server-Sent Events_](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports#backwards-compatibility) transport.

AI Bridge makes use of [External Auth](../../admin/external-auth/index.md) applications, as they define OAuth2 connections to upstream services. If your External Auth application hosts a remote MCP server, you can configure AI Bridge to connect to it, retrieve its tools and inject them into requests automatically - all while using each individual user's access token.

For example, GitHub has a [remote MCP server](https://github.com/github/github-mcp-server?tab=readme-ov-file#remote-github-mcp-server) and we can use it as follows.

```bash
CODER_EXTERNAL_AUTH_0_TYPE=github
CODER_EXTERNAL_AUTH_0_CLIENT_ID=...
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=...
# Tell AI Bridge where it can find this service's remote MCP server.
CODER_EXTERNAL_AUTH_0_MCP_URL=https://api.githubcopilot.com/mcp/
```

See the diagram in [Implementation Details](./reference.md#implementation-details) for more information.

You can also control which tools are injected by using an allow and/or a deny regular expression on the tool names:

```env
CODER_EXTERNAL_AUTH_0_MCP_TOOL_ALLOW_REGEX=(.+_gist.*)
CODER_EXTERNAL_AUTH_0_MCP_TOOL_DENY_REGEX=(create_gist)
```

In the above example, all tools containing `_gist` in their name will be allowed, but `create_gist` is denied.

The logic works as follows:

- If neither the allow/deny patterns are defined, all tools will be injected.
- The deny pattern takes precedence.
- If only a deny pattern is defined, all tools are injected except those explicitly denied.

In the above example, if you prompted your AI model with "list your available github tools by name", it would reply something like:

> Certainly! Here are the GitHub-related tools that I have available:
>
> ```text
> 1. bmcp_github_update_gist
> 2. bmcp_github_list_gists
> ```

AI Bridge marks automatically injected tools with a prefix `bmcp_` ("bridged MCP"). It also namespaces all tool names by the ID of their associated External Auth application (in this case `github`).

## Tool Injection

If a model decides to invoke a tool and it has a `bmcp_` suffix and AI Bridge has a connection with the related MCP server, it will invoke the tool. The tool result will be passed back to the upstream AI provider, and this will loop until the model has all of its required data. These inner loops are not relayed back to the client; all it seems is the result of this loop. See [Implementation Details](./reference.md#implementation-details).

In contrast, tools which are defined by the client (i.e. the [`Bash` tool](https://docs.claude.com/en/docs/claude-code/settings#tools-available-to-claude) defined by _Claude Code_) cannot be invoked by AI Bridge, and the tool call from the model will be relayed to the client, after which it will invoke the tool.

If you have the `oauth2` and `mcp-server-http` experiments enabled, Coder's own [internal MCP tools](../mcp-server.md) will be injected automatically.

### Troubleshooting

- **Too many tools**: should you receive an error like `Invalid 'tools': array too long. Expected an array with maximum length 128, but got an array with length 132 instead`, you can reduce the number by filtering out tools using the allow/deny patterns documented in the [MCP](#mcp) section.

- **Coder MCP tools not being injected**: in order for Coder MCP tools to be injected, the internal MCP server needs to be active. Follow the instructions in the [MCP Server](../mcp-server.md) page to enable it.

- **External Auth tools not being injected**: this is generally due to the requesting user not being authenticated against the [External Auth](../../admin/external-auth/index.md) app; when this is the case, no attempt is made to connect to the MCP server.
