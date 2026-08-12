# Reference

> [!NOTE]
> AI Gateway is part of [AI Governance](../ai-governance.md), which is
> included with a Premium license.

## Deployment topologies

AI Gateway can run inside `coderd` or as a standalone data-plane service.
Both topologies run the same Gateway request handling and keep `coderd` as the source of truth for Coder API key validation, provider configuration, and AI session records.
They differ in how requests are routed to the Gateway.

### Embedded Gateway

By default, `coder server` runs an in-memory Gateway instance in the `coderd` process.
AI clients send requests to `<Coder access URL>/api/v2/ai-gateway/<provider-name>/`.
The embedded Gateway uses the same control RPC as a standalone deployment, over an in-process transport rather than a network connection.
It does not use a Gateway key and does not negotiate an API version.

The following diagram shows the embedded topology:

![AI Gateway implementation details](../../images/aibridge/aibridge-implementation-details.png)

### Standalone Gateway

A [standalone deployment](./standalone.md) runs the AI traffic data plane outside the `coderd` process.
Each replica accepts client traffic, sends AI requests directly to upstream providers, and maintains a control connection to `coderd` using a [Gateway key](./standalone.md#create-a-gateway-key).

The control connection carries:

- Coder API key validation, which resolves each request to an active Coder user.
- AI budget checks, which reject requests from users over their spend limit.
- Provider configuration, plus a change signal when the provider set changes.
- AI session records.
- **Deprecated**: the configuration and access tokens used by [injected MCP](./mcp.md).

Standalone replicas do not own authoritative database state.
They keep ephemeral provider snapshots, request caches, provider key pools, and metrics in memory, and emit their own logs and traces.
Each replica writes its own [API dumps](./setup.md#api-dumps) to its own local disk when dumps are enabled.

`coderd` remains required for standalone operation.
A replica becomes unready when its control connection is unavailable, even if its HTTP listener remains healthy.
AI Gateway Proxy remains part of `coder server` and can forward its intercepted traffic to either the embedded Gateway or a standalone endpoint.

## Version compatibility

The control connection between a standalone replica and `coderd` is versioned.
The current version is defined in [`coderd/aibridged/proto/version.go`](https://github.com/coder/coder/blob/main/coderd/aibridged/proto/version.go).

`coderd` validates the version that a standalone replica advertises before it accepts the control connection.
Compatibility follows these rules:

- The Gateway and `coderd` major versions must match.
- The Gateway minor version must be less than or equal to the `coderd` minor version.
- `coderd` rejects a standalone Gateway that advertises a newer minor version.

A rejected replica receives an HTTP 400 response that reports the `client_api_version` and `server_api_version` values.
Coder build versions are not the compatibility criterion.

For upgrade and rollback ordering, refer to [Version compatibility](./standalone.md#version-compatibility) in the standalone deployment guide.

## Supported APIs

API support is divided into two categories:

- **Intercepted**: Requests are intercepted, audited, and augmented.
- **Passthrough**: Requests are proxied directly to the upstream provider without auditing or augmentation.

Where relevant, both streaming and non-streaming requests are supported.
Paths are relative to the provider's base URL, such as `https://ai-gateway.example.com/openai/v1` or `https://ai-gateway.example.com/anthropic`.

### OpenAI

The OpenAI provider also serves the Azure OpenAI, Google, OpenRouter, Vercel, and OpenAI-compatible provider types.

#### Intercepted

- [`/v1/chat/completions`](https://platform.openai.com/docs/api-reference/chat/create)
- [`/v1/responses`](https://platform.openai.com/docs/api-reference/responses/create)

#### Passthrough

- [`/v1/conversations(/*)`](https://platform.openai.com/docs/api-reference/conversations)
- [`/v1/models(/*)`](https://platform.openai.com/docs/api-reference/models/list)
- [`/v1/responses/*`](https://platform.openai.com/docs/api-reference/responses/get)

The legacy [`/v1/completions`](https://platform.openai.com/docs/api-reference/completions) API is deprecated and is not passed through.

### Anthropic

The Anthropic provider also serves the AWS Bedrock provider type.

#### Intercepted

- [`/v1/messages`](https://docs.claude.com/en/api/messages)

#### Passthrough

- [`/v1/messages/count_tokens`](https://docs.claude.com/en/api/messages-count-tokens)
- [`/v1/models(/*)`](https://docs.claude.com/en/api/models-list)
- `/api/event_logging/*`

### GitHub Copilot

#### Intercepted

- `/chat/completions`
- `/responses`
- `/v1/messages`

#### Passthrough

- `/models(/*)`
- `/agents/*`
- `/mcp/*`
- `/.well-known/*`

Any route that is not listed above returns `404`.

## Troubleshooting

To report a bug, file a feature request, or review known issues, visit the [Coder GitHub repository](https://github.com/coder/coder/issues).
For help with AI Gateway, visit the [Coder Discord](https://discord.gg/coder).
