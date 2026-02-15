# Reference

## Architecture

AI Bridge runs as an in-process component within `coderd` (the Coder control
plane). It does not require a separate deployment or infrastructure.

### Request lifecycle

1. A template admin configures AI Bridge base URLs and session tokens in the
   workspace template (Terraform). This propagates to every workspace
   automatically.
2. The AI client (Claude Code, Codex, Cline, etc.) sends API requests to the
   Bridge endpoint using the developer's Coder session token — not a provider
   API key.
3. Bridge authenticates the user via Coder's identity system.
4. Bridge logs the prompt and request metadata.
5. If MCP servers are configured, Bridge fetches available tools and injects them
   into the request.
6. Bridge forwards the modified request to the upstream LLM provider using the
   enterprise API key.
7. The LLM response (including any tool calls) flows back through Bridge.
8. If the response includes MCP tool calls, Bridge executes them on behalf of
   the user and sends results back to the LLM.
9. Bridge logs usage (tokens, tools, model, user) and returns the final response
   to the agent.

### Key architectural points

- **Developers never hold provider API keys.** They authenticate with their
  existing Coder credentials.
- **The enterprise holds one key per provider.** These are configured centrally
  in `coderd` via environment variables or Helm values.
- **Template-level configuration.** Configure AI Bridge once in a workspace
  template, and it propagates to every workspace created from that template.
- **Universal client compatibility.** Works with any AI tool that supports base
  URL overrides, including Claude Code, Codex, Cline, Zed, and more. For tools
  that don't support base URL overrides, [AI Bridge Proxy](./ai-bridge-proxy/index.md)
  can intercept traffic transparently.

## Implementation Details

`coderd` runs an in-memory instance of `aibridged`, whose logic is mostly contained in https://github.com/coder/aibridge. In future releases we will support running external instances for higher throughput and complete memory isolation from `coderd`.

![AI Bridge implementation details](../../images/aibridge/aibridge-implementation-details.png)

## Supported APIs

API support is broken down into two categories:

- **Intercepted**: requests are intercepted, audited, and augmented - full AI Bridge functionality
- **Passthrough**: requests are proxied directly to the upstream, no auditing or augmentation takes place

Where relevant, both streaming and non-streaming requests are supported.

### OpenAI

#### Intercepted

- [`/v1/chat/completions`](https://platform.openai.com/docs/api-reference/chat/create)
- [`/v1/responses`](https://platform.openai.com/docs/api-reference/responses/create)

#### Passthrough

- [`/v1/models(/*)`](https://platform.openai.com/docs/api-reference/models/list)

### Anthropic

#### Intercepted

- [`/v1/messages`](https://docs.claude.com/en/api/messages)

#### Passthrough

- [`/v1/models(/*)`](https://docs.claude.com/en/api/models-list)

## Troubleshooting

To report a bug, file a feature request, or view a list of known issues, please visit our [GitHub repository for AI Bridge](https://github.com/coder/aibridge). If you encounter issues with AI Bridge, please reach out to us via [Discord](https://discord.gg/coder).
