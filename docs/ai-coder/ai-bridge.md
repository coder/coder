# AI Bridge

![AI bridge diagram](../images/aibridge/aibridge_diagram.png)

Bridge is a smart proxy for AI. It acts as a man-in-the-middle between your users' coding agents / IDEs
and providers like OpenAI and Anthropic. By intercepting all the AI traffic between these clients and
the upstream APIs, Bridge can record user prompts, token usage, and tool invocations.

Bridge solves 3 key problems:

1. **Centralized authn/z management**: no more issuing & managing API tokens for OpenAI/Anthropic usage.
   Users use their Coder session or API tokens to authenticate with `coderd` (Coder control plane), and
   `coderd` securely communicates with the upstream APIs on their behalf. Use a single key for all users.
2. **Auditing and attribution**: all interactions with AI services, whether autonomous or human-initiated,
   will be audited and attributed back to a user.
3. **Centralized MCP administration**: define a set of approved MCP servers and tools which your users may
   use, and prevent users from using their own.

## When to use AI Bridge

As the library of LLMs and their associated tools grow, administrators are pressured to provide auditing, measure adoption, provide tools through MCP, and track token spend. Disparate SAAS platforms provide _some_ of these for _some_ tools, but there is no centralized, secure solution for these challenges.

If you are an administrator or devops leader looking to:

- Measure AI tooling adoption across teams or projects
- Provide an LLM audit trail to security administrators
- Manage token spend in a central dashboard
- Investigate opportunities for AI automation
- Uncover the high-leverage use cases from experienced engineers

We advise trying Bridge as self-hosted proxy to monitor LLM usage agnostically across AI powered IDEs like Cursor and headless agents like Claude Code.

## Setup

Bridge runs inside the Coder control plane, requiring no separate compute to deploy or scale. Once enabled, `coderd` hosts the bridge in-memory and brokers traffic to your configured AI providers on behalf of authenticated users.

**Required**:

1. A **premium** licensed Coder deployment
1. Feature must be [enabled](#activation) using the server flag
1. One or more [provider](#providers) API keys must be configured

### Activation

You will need to enable AI Bridge explicitly:

```sh
CODER_AIBRIDGE_ENABLED=true coder server
# or
coder server --aibridge-enabled=true
```

### Providers

Bridge currently supports OpenAI and Anthropic APIs.

**API Key**:

The single key used to authenticate all requests from Bridge to OpenAI/Anthropic APIs.

- `CODER_AIBRIDGE_OPENAI_KEY` or `--aibridge-openai-key`
- `CODER_AIBRIDGE_ANTHROPIC_KEY` or `--aibridge-anthropic-key`

**Base URL**:

The API to which Bridge will relay requests.

- `CODER_AIBRIDGE_OPENAI_BASE_URL` or `--aibridge-openai-base-url`, defaults to `https://api.openai.com/v1/`
- `CODER_AIBRIDGE_ANTHROPIC_BASE_URL` or `--aibridge-anthropic-base-url`, defaults to `https://api.anthropic.com/`

Bridge is compatible with _[Google Vertex AI](https://cloud.google.com/vertex-ai?hl=en)_, _[AWS Bedrock](https://aws.amazon.com/bedrock/)_, and other LLM brokers. You may specify the base URL(s) above to the appropriate API endpoint for your provider.

---

> [!NOTE]
> See [Supported APIs](#supported-apis) section below for a comprehensive list.

## Client Configuration

Once AI Bridge is enabled on the server, your users need to configure their AI coding tools to use it. This section explains how users should configure their clients to connect to AI Bridge.

### Setting Base URLs

The exact configuration method varies by client — some use environment variables, others use configuration files or UI settings:

- **OpenAI-compatible clients**: Set the base URL (commonly via the `OPENAI_BASE_URL` environment variable) to `https://coder.example.com/api/v2/aibridge/openai/v1`
- **Anthropic-compatible clients**: Set the base URL (commonly via the `ANTHROPIC_BASE_URL` environment variable) to `https://coder.example.com/api/v2/aibridge/anthropic`

Replace `coder.example.com` with your actual Coder deployment URL.

### Authentication

Instead of distributing provider-specific API keys (OpenAI/Anthropic keys) to users, they authenticate to AI Bridge using their **Coder session token** or **API key**:

- **OpenAI clients**: Users set `OPENAI_API_KEY` to their Coder session token or API key
- **Anthropic clients**: Users set `ANTHROPIC_API_KEY` to their Coder session token or API key

Users can generate a Coder API key using:

```sh
coder tokens create
```

Template admins can pre-configure authentication in templates using [`data.coder_workspace_owner.me.session_token`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/workspace_owner#session_token-1) to automatically configure the workspace owner's credentials.

#### Compatibility Notes

Most AI coding assistants that support custom base URLs can work with AI Bridge. However, client-specific configuration requirements vary:

- Some clients require specific URL formats (e.g. try removing the `/v1` suffix)
- Some clients may proxy requests through their own servers, limiting compatibility (e.g. Cursor)
- Some clients may not support custom base URLs at all (e.g. Copilot CLI, Sourcegraph Amp)

Consult your specific AI client's documentation for details on configuring custom API endpoints.

## Collected Data

Bridge collects:

- The last `user` prompt of each request
- All token usage (associated with each prompt)
- Every tool invocation

All of these records are associated to an "interception" record, which maps 1:1 with requests received from clients but may involve several interactions with upstream providers. Interceptions are associated with a Coder identity, allowing you to map consumption and cost with teams or individuals in your organization:

![User Prompt logging](../images/aibridge/grafana_user_prompts_logging.png)

These logs can be used to determine usage patterns, track costs, and evaluate tooling adoption.

This data is currently accessible through the API and CLI, which we advise administrators export to their observability platform of choice. We've configured a Grafana dashboard to display Claude Code usage internally which can be imported as a starting point for your tooling adoption metrics.

![User Leaderboard](../images/aibridge/grafana_user_leaderboard.png)

We provide an example Grafana dashboard that you can import as a starting point for your tooling adoption metrics. See [here](https://github.com/coder/coder/blob/main/examples/monitoring/dashboards/grafana/aibridge/README.md).

## Implementation Details

`coderd` runs an in-memory instance of `aibridged`, whose logic is mostly contained in https://github.com/coder/aibridge. In future releases we will support running external instances for higher throughput and complete memory isolation from `coderd`.

<details>
<summary>See a diagram of how Bridge interception works</summary>

```mermaid

sequenceDiagram
    actor User
    participant Client
    participant Bridge

    User->>Client: Issues prompt
    activate Client

    Note over User, Client: Coder session key used<br>as AI token
    Client-->>Bridge: Sends request

    activate Bridge
    Note over Client, Bridge: Coder session key <br>passed along

    Note over Bridge: Authenticate
    Note over Bridge: Parse request

    alt Rejected
        Bridge-->>Client: Send response
        Client->>User: Display response
    end

    Note over Bridge: If first request, establish <br>connection(s) with MCP server(s)<br>and list tools

    Note over Bridge: Inject MCP tools

    Bridge-->>AIProvider: Send modified request

    activate AIProvider

    AIProvider-->>Bridge: Send response

    Note over Client: Client is unaware of injected<br>tools and invocations,<br>just receives one long response

    alt Has injected tool calls
        loop
            Note over Bridge: Invoke injected tool
            Bridge-->>AIProvider: Send tool result
            AIProvider-->>Bridge: Send response
        end
    end

    deactivate AIProvider

    Bridge-->>Client: Relay response
    deactivate Bridge

    Client->>User: Display response
    deactivate Client
```

</details>

## MCP

[Model Context Protocol (MCP)](https://modelcontextprotocol.io/docs/getting-started/intro) is a mechanism for connecting AI applications to external systems.

Bridge can connect to MCP servers and inject tools automatically, enabling you to centrally manage the list of tools you wish to grant your users.

> [!NOTE]
> Only MCP servers which support OAuth2 Authorization are supported currently. In future releases we will support [optional authorization](https://modelcontextprotocol.io/specification/2025-06-18/basic/authorization#protocol-requirements).
>
> [_Streamable HTTP_](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports#streamable-http) is the only supported transport currently. In future releases we will support the (now deprecated) [_Server-Sent Events_](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports#backwards-compatibility) transport.

Bridge makes use of [External Auth](../admin/external-auth/index.md) applications, as they define OAuth2 connections to upstream services. If your External Auth application hosts a remote MCP server, you can configure Bridge to connect to it, retrieve its tools and inject them into requests automatically - all while using each individual user's access token.

For example, GitHub has a [remote MCP server](https://github.com/github/github-mcp-server?tab=readme-ov-file#remote-github-mcp-server) and we can use it as follows.

```bash
CODER_EXTERNAL_AUTH_0_TYPE=github
CODER_EXTERNAL_AUTH_0_CLIENT_ID=...
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=...
# Tell Bridge where it can find this service's remote MCP server.
CODER_EXTERNAL_AUTH_0_MCP_URL=https://api.githubcopilot.com/mcp/
```

See the diagram in [Implementation Details](#implementation-details) for more information.

You can also control which tools are injected by using an allow and/or a deny regular expression on the tool names:

```bash
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
> 1. `bmcp_github_update_gist`
> 2. `bmcp_github_list_gists`

Bridge marks automatically injected tools with a prefix `bmcp_` ("bridged MCP"). It also namespaces all tool names by the ID of their associated External Auth application (in this case `github`).

## Tool Injection

If a model decides to invoke a tool and it has a `bmcp_` suffix and Bridge has a connection with the related MCP server, it will invoke the tool. The tool result will be passed back to the upstream AI provider, and this will loop until the model has all of its required data. These inner loops are not relayed back to the client; all it seems is the result of this loop. See [Implementation Details](#implementation-details).

In contrast, tools which are defined by the client (i.e. the [`Bash` tool](https://docs.claude.com/en/docs/claude-code/settings#tools-available-to-claude) defined by _Claude Code_) cannot be invoked by Bridge, and the tool call from the model will be relayed to the client, after which it will invoke the tool.

If you have the `oauth2` and `mcp-server-http` experiments enabled, Coder's own [internal MCP tools](mcp-server.md) will be injected automatically.

### Troubleshooting

- **Too many tools**: should you receive an error like `Invalid 'tools': array too long. Expected an array with maximum length 128, but got an array with length 132 instead`, you can reduce the number by filtering out tools using the allow/deny patterns documented in the [MCP](#mcp) section.

- **Coder MCP tools not being injected**: in order for Coder MCP tools to be injected, the internal MCP server needs to be active. Follow the instructions in the [MCP Server](mcp-server.md) page to enable it.

- **External Auth tools not being injected**: this is generally due to the requesting user not being authenticated against the External Auth app; when this is the case, no attempt is made to connect to the MCP server.

## Known Issues / Limitations

- Codex CLI currently does not work with Bridge due to a JSON marshaling issue: https://github.com/coder/aibridge/issues/19
- Claude Code web searches do not report correctly: https://github.com/coder/aibridge/issues/11

## Supported APIs

API support is broken down into two categories:

- **Intercepted**: requests are intercepted, audited, and augmented - full Bridge functionality
- **Passthrough**: requests are proxied directly to the upstream, no auditing or augmentation takes place

Where relevant, both streaming and non-streaming requests are supported.

### OpenAI

**Intercepted**:

- [`/v1/chat/completions`](https://platform.openai.com/docs/api-reference/chat/create)

**Passthrough**:

- [`/v1/models(/*)`](https://platform.openai.com/docs/api-reference/models/list)
- [`/v1/responses`](https://platform.openai.com/docs/api-reference/responses/create) _(Interception support coming in **Beta**)_

### Anthropic

**Intercepted**:

- [`/v1/messages`](https://docs.claude.com/en/api/messages)

**Passthrough**:

- [`/v1/models(/*)`](https://docs.claude.com/en/api/models-list)

## Troubleshooting

To report a bug, file a feature request, or view a list of known issues, please visit our [GitHub repository for Bridge](https://github.com/coder/aibridge). If you encounter issues with Bridge during early access, please reach out to us via [Discord](https://discord.gg/coder).
