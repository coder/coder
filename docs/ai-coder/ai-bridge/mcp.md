# MCP Tools Injection

> **Early Access** — MCP tool injection is available but the API surface may
> change in future releases.

[Model Context Protocol (MCP)](https://modelcontextprotocol.io/docs/getting-started/intro)
is a mechanism for connecting AI applications to external systems.

AI Bridge can connect to configured MCP servers, list their available tools, and
inject those tools into incoming LLM requests automatically. This means agents
receive centrally managed tools without any per-developer MCP configuration.

## How it works

1. An administrator configures External Auth providers with MCP endpoints
   (e.g., GitHub, Jira, internal services).
2. Allow/deny regex patterns define which tools are available to agents.
3. When an AI client sends a request through Bridge, Bridge fetches the
   available tools from configured MCP servers and injects them into the LLM
   request.
4. The LLM sees and can invoke these tools in its response.
5. When the LLM response includes tool calls, Bridge executes them on behalf of
   the user using their OAuth tokens (via Coder's External Auth).
6. Tool results are sent back to the LLM to continue the conversation.

### Key points

- **Clients don't configure MCP.** The AI client only configures a base URL
  override — nothing MCP-related. Tool injection is managed centrally.
- **User-scoped authentication.** Tool calls are executed using the user's own
  OAuth tokens, so actions are attributed and permissioned correctly.
- **Centralized tool governance.** Administrators control which MCP tools are
  available via allow/deny patterns. New tools can be added or revoked centrally
  without touching workspace templates.

> [!NOTE]
> Only MCP servers which support OAuth2 Authorization are supported currently.
>
> [_Streamable HTTP_](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports#streamable-http) is the only supported transport currently. In future releases we will support the (now deprecated) [_Server-Sent Events_](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports#backwards-compatibility) transport.

## Prerequisites

MCP tool injection requires two experimental features to be enabled:

```bash
CODER_EXPERIMENTS=oauth2,mcp-server-http
```

You must also have External Auth configured for the services you want to expose
as MCP tools.

AI Bridge makes use of [External Auth](../../admin/external-auth/index.md)
applications to define OAuth2 connections to upstream services. If your
External Auth application hosts a remote MCP server, you can configure AI Bridge
to connect to it, retrieve its tools and inject them into requests automatically
— all while using each individual user's access token.

## Configuration

### Enable MCP injection

```bash
CODER_AI_BRIDGE_MCP_INJECT=true
```

### Configure MCP servers

MCP servers are configured through Coder's External Auth system. Each External
Auth provider that exposes an MCP endpoint can have its tools injected into
Bridge requests.

### Tool filtering

Use allow/deny regex patterns to control which tools are available:

```bash
CODER_AI_BRIDGE_MCP_TOOL_ALLOW=".+"          # Allow all tools (default)
CODER_AI_BRIDGE_MCP_TOOL_DENY="dangerous_.*" # Deny tools matching pattern
```

## Example

For example, GitHub has a
[remote MCP server](https://github.com/github/github-mcp-server?tab=readme-ov-file#remote-github-mcp-server)
and we can use it as follows:

```bash
CODER_EXTERNAL_AUTH_0_TYPE=github
CODER_EXTERNAL_AUTH_0_CLIENT_ID=...
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=...
# Tell AI Bridge where it can find this service's remote MCP server.
CODER_EXTERNAL_AUTH_0_MCP_URL=https://api.githubcopilot.com/mcp/
```

See the diagram in [Implementation Details](./reference.md#implementation-details)
for more information.

With GitHub configured as an External Auth provider with MCP:

1. A developer runs Claude Code in their workspace.
2. Claude Code sends a request to Bridge.
3. Bridge fetches tools from the GitHub MCP server (e.g.,
   `create_pull_request`, `list_issues`, `search_code`).
4. Bridge injects these tools into the LLM request.
5. Claude decides to call `create_pull_request` with proposed changes.
6. Bridge executes the tool call using the developer's GitHub OAuth token.
7. The PR is created and the result is sent back to Claude.

The developer never configured MCP, and the GitHub token was never exposed to
the agent directly.

### Per-provider tool filtering

You can also control which tools are injected per External Auth provider by
using an allow and/or a deny regular expression on the tool names:

```env
CODER_EXTERNAL_AUTH_0_MCP_TOOL_ALLOW_REGEX=(.+_gist.*)
CODER_EXTERNAL_AUTH_0_MCP_TOOL_DENY_REGEX=(create_gist)
```

In the above example, all tools containing `_gist` in their name will be
allowed, but `create_gist` is denied.

The logic works as follows:

- If neither the allow/deny patterns are defined, all tools will be injected.
- The deny pattern takes precedence.
- If only a deny pattern is defined, all tools are injected except those
  explicitly denied.

In the above example, if you prompted your AI model with "list your available
github tools by name", it would reply something like:

> Certainly! Here are the GitHub-related tools that I have available:
>
> ```text
> 1. bmcp_github_update_gist
> 2. bmcp_github_list_gists
> ```

AI Bridge marks automatically injected tools with a prefix `bmcp_` ("bridged
MCP"). It also namespaces all tool names by the ID of their associated External
Auth application (in this case `github`).

## Tool injection

If a model decides to invoke a tool and it has a `bmcp_` prefix and AI Bridge
has a connection with the related MCP server, it will invoke the tool. The tool
result will be passed back to the upstream AI provider, and this will loop until
the model has all of its required data. These inner loops are not relayed back to
the client; all it sees is the result of this loop. See
[Implementation Details](./reference.md#implementation-details).

In contrast, tools which are defined by the client (i.e. the
[`Bash` tool](https://docs.claude.com/en/docs/claude-code/settings#tools-available-to-claude)
defined by _Claude Code_) cannot be invoked by AI Bridge, and the tool call from
the model will be relayed to the client, after which it will invoke the tool.

If you have [Coder MCP Server](../mcp-server.md) enabled, as well as have
[`CODER_AIBRIDGE_INJECT_CODER_MCP_TOOLS=true`](../../reference/cli/server#--aibridge-inject-coder-mcp-tools)
set, Coder's MCP tools will be injected into intercepted requests.

## Troubleshooting

- **Too many tools**: should you receive an error like `Invalid 'tools': array
  too long. Expected an array with maximum length 128, but got an array with
  length 132 instead`, you can reduce the number by filtering out tools using
  the allow/deny patterns documented in the [tool filtering](#tool-filtering)
  section.

- **Coder MCP tools not being injected**: in order for Coder MCP tools to be
  injected, the internal MCP server needs to be active. Follow the instructions
  in the [MCP Server](../mcp-server.md) page to enable it and ensure
  `CODER_AIBRIDGE_INJECT_CODER_MCP_TOOLS` is set to `true`.

- **External Auth tools not being injected**: this is generally due to the
  requesting user not being authenticated against the
  [External Auth](../../admin/external-auth/index.md) app; when this is the
  case, no attempt is made to connect to the MCP server.

## Current scope

- MCP tool injection is in Early Access and the API surface may change.
- Tools are injected into every LLM call made through Bridge when enabled.
  Per-request or per-user tool scoping is not yet available.
- Tool execution adds latency to the request cycle, as Bridge must make
  round-trips to MCP servers.

## Next steps

- [Setup](./setup.md) — Enable AI Bridge and configure providers.
- [Client Configuration](./clients/index.md) — Configure AI tools to route
  through Bridge.
- [Monitoring](./monitoring.md) — Track tool invocations in AI Bridge logs.
