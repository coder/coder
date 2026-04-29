# aibridge

aibridge provides an HTTP handler that intercepts AI client requests bound for upstream AI providers (Anthropic, OpenAI, Copilot). It records token usage, prompts, and tool invocations per user. Optionally supports centralized [MCP](https://modelcontextprotocol.io/) tool injection with allowlist/denylist filtering.

The handler is mounted by a host process. Today that host is `coderd`, which [mounts the handler](../enterprise/coderd/coderd.go#L294) at `/api/v2/aibridge/<provider>/*`. Running aibridge as a separate process is planned for the future.

## Architecture

```
┌─────────────────┐     ┌───────────────────────────────────────────┐
│    AI Client    │     │                    aibridge               │
│  (Claude Code,  │────▶│  ┌─────────────────┐    ┌─────────────┐   │
│   Cursor, etc.) │     │  │  RequestBridge  │───▶│  Providers  │   │
└─────────────────┘     │  │  (http.Handler) │    │  (Anthropic │   │
                        │  └─────────────────┘    │   OpenAI)   │   │
                        │                         └──────┬──────┘   │
                        │                                │          │
                        │                                ▼          │    ┌─────────────┐
                        │  ┌─────────────────┐    ┌─────────────┐   │    │  Upstream   │
                        │  │    Recorder     │◀───│ Interceptor │─── ───▶│    API      │
                        │  │ (tokens, tools, │    │ (streaming/ │   │    │ (Anthropic  │
                        │  │  prompts)       │    │  blocking)  │   │    │   OpenAI)   │
                        │  └────────┬────────┘    └──────┬──────┘   │    └─────────────┘
                        │           │                    │          │
                        │           ▼             ┌──────▼──────┐   │
                        │  ┌ ─ ─ ─ ─ ─ ─ ─ ┐      │  MCP Proxy  │   │
                        │  │    Database   │      │   (tools)   │   │
                        │  └ ─ ─ ─ ─ ─ ─ ─ ┘      └─────────────┘   │
                        └───────────────────────────────────────────┘
```

### Components

- **RequestBridge**: The main `http.Handler` that routes requests to providers
- **Provider**: Defines bridged routes (intercepted) and passthrough routes (proxied)
- **Interceptor**: Handles request/response processing and streaming
- **Recorder**: Interface for capturing usage data (tokens, prompts, tools)
- **MCP Proxy** (optional): Connects to MCP servers to list tool, inject them into requests, and invoke them in an inner agentic loop

## Request Flow

1. Client sends request to `/anthropic/v1/messages` or `/openai/v1/chat/completions`
2. **Actor extraction**: Request must have an actor in context (via `AsActor()`). The host is responsible for authenticating the caller before invoking the handler.
3. **Upstream call**: Request forwarded to the AI provider
4. **Response relay**: Response streamed/sent to client
5. **Recording**: Token usage, prompts, and tool invocations recorded

**With MCP enabled**: Tools from configured MCP servers are centrally defined and injected into requests (prefixed `bmcp_`). Allowlist/denylist regex patterns control which tools are available. When the model selects an injected tool, the gateway invokes it in an inner agentic loop, and continues the conversation loop until complete.

Passthrough routes (`/v1/models`, `/v1/messages/count_tokens`) are reverse-proxied directly.

## Observability

### Prometheus Metrics

Create metrics with `NewMetrics(prometheus.Registerer)`:

| Metric                               | Type      | Description                                                              |
|--------------------------------------|-----------|--------------------------------------------------------------------------|
| `interceptions_total`                | Counter   | Intercepted request count                                                |
| `interceptions_inflight`             | Gauge     | Currently processing requests                                            |
| `interceptions_duration_seconds`     | Histogram | Request duration                                                         |
| `passthrough_total`                  | Counter   | Non-intercepted requests forwarded to the upstream                       |
| `prompts_total`                      | Counter   | User prompt count                                                        |
| `tokens_total`                       | Counter   | Token usage (input, output, cache read/write, provider extras)           |
| `injected_tool_invocations_total`    | Counter   | Injected MCP tool invocations performed by the handler                   |
| `non_injected_tool_selections_total` | Counter   | Client-defined tool selections returned by the model                     |
| `circuit_breaker_state`              | Gauge     | Circuit breaker state per provider/endpoint (0=closed, 0.5=half, 1=open) |
| `circuit_breaker_trips_total`        | Counter   | Times the circuit breaker transitioned to open                           |
| `circuit_breaker_rejects_total`      | Counter   | Requests rejected due to an open circuit breaker                         |

### Recorder Interface

Implement `Recorder` to persist usage data to your database:

- `aibridge_interceptions` - request metadata (provider, model, initiator, timestamps)
- `aibridge_token_usages` - input/output and cache read/write token counts per response
- `aibridge_user_prompts` - user prompts
- `aibridge_tool_usages` - tool invocations (injected and client-defined)
- `aibridge_model_thoughts` - model reasoning content (thinking, reasoning summaries, commentary)

```go
type Recorder interface {
    RecordInterception(ctx context.Context, req *InterceptionRecord) error
    RecordInterceptionEnded(ctx context.Context, req *InterceptionRecordEnded) error
    RecordTokenUsage(ctx context.Context, req *TokenUsageRecord) error
    RecordPromptUsage(ctx context.Context, req *PromptUsageRecord) error
    RecordToolUsage(ctx context.Context, req *ToolUsageRecord) error
    RecordModelThought(ctx context.Context, req *ModelThoughtRecord) error
}
```

## Supported Routes

Each provider instance is mounted under `/api/v2/aibridge/<name>`, where `<name>` is the provider's configured name. For example, with an Anthropic provider named `my-anthropic`, its `/messages` endpoint would be reachable at `/api/v2/aibridge/my-anthropic/v1/messages`.

If a name is not set, the route path defaults to the provider's type: `anthropic`, `openai`, or `copilot`. The table below uses the default names.

`(/*)` denotes a route that handles both the exact path and any subpaths. A trailing `/*` denotes subpaths only.

| Provider  | Route                                 | Type                  |
|-----------|---------------------------------------|-----------------------|
| Anthropic | `/anthropic/v1/messages`              | Bridged (intercepted) |
| Anthropic | `/anthropic/v1/messages/count_tokens` | Passthrough           |
| Anthropic | `/anthropic/v1/models(/*)`            | Passthrough           |
| Anthropic | `/anthropic/api/event_logging/*`      | Passthrough           |
| OpenAI    | `/openai/v1/chat/completions`         | Bridged (intercepted) |
| OpenAI    | `/openai/v1/responses`                | Bridged (intercepted) |
| OpenAI    | `/openai/v1/responses/*`              | Passthrough           |
| OpenAI    | `/openai/v1/conversations(/*)`        | Passthrough           |
| OpenAI    | `/openai/v1/models(/*)`               | Passthrough           |
| Copilot   | `/copilot/chat/completions`           | Bridged (intercepted) |
| Copilot   | `/copilot/responses`                  | Bridged (intercepted) |
| Copilot   | `/copilot/models(/*)`                 | Passthrough           |
| Copilot   | `/copilot/agents/*`                   | Passthrough           |
| Copilot   | `/copilot/mcp/*`                      | Passthrough           |
| Copilot   | `/copilot/.well-known/*`              | Passthrough           |
