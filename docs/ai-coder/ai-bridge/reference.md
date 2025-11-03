# Reference

## Implementation Details

`coderd` runs an in-memory instance of `aibridged`, whose logic is mostly contained in https://github.com/coder/aibridge. In future releases we will support running external instances for higher throughput and complete memory isolation from `coderd`.

<details>
<summary>See a diagram of how AI Bridge interception works</summary>

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

![AI Bridge implementation details](../../images/aibridge/aibridge-implementation-details.png)

## Supported APIs

API support is broken down into two categories:

- **Intercepted**: requests are intercepted, audited, and augmented - full AI Bridge functionality
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

To report a bug, file a feature request, or view a list of known issues, please visit our [GitHub repository for AI Bridge](https://github.com/coder/aibridge). If you encounter issues with AI Bridge during early access, please reach out to us via [Discord](https://discord.gg/coder).
