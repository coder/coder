# Monitoring

AI Bridge records the last `user` prompt, token usage, and every tool invocation for each intercepted request. Each capture is tied to a single "interception" that maps back to the authenticated Coder identity, making it easy to attribute spend and behaviour.

![User Prompt logging](../../images/aibridge/grafana_user_prompts_logging.png)

![User Leaderboard](../../images/aibridge/grafana_user_leaderboard.png)

We provide an example Grafana dashboard that you can import as a starting point for your metrics. See [the Grafana dashboard README](https://github.com/coder/coder/blob/main/examples/monitoring/dashboards/grafana/aibridge/README.md).

These logs and metrics can be used to determine usage patterns, track costs, and evaluate tooling adoption.

## Exporting Data

AI Bridge interception data can be exported for external analysis, compliance reporting, or integration with log aggregation systems.

### REST API

You can retrieve AI Bridge interceptions via the Coder API with filtering and pagination support.

```sh
curl -X GET "https://coder.example.com/api/v2/aibridge/interceptions?q=initiator:me" \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN"
```

Available query filters:

- `client` - Filter by client name.
  <details>
  <summary>Possible <code>client</code> values</summary>

  > [!NOTE]
  > Client classification is done on best effort basis using the `User-Agent` header;
  not all clients send these headers in an easily-identifiable manner.

  - `Claude Code`
  - `Codex`
  - `Zed`
  - `GitHub Copilot (VS Code)`
  - `GitHub Copilot (CLI)`
  - `Kilo Code`
  - `Roo Code`
  - `Cursor`
  - `Unknown`

  </details><br>
- `initiator` - Filter by user ID or username
- `provider` - Filter by AI provider (e.g., `openai`, `anthropic`)
- `model` - Filter by model name
- `started_after` - Filter interceptions after a timestamp
- `started_before` - Filter interceptions before a timestamp

See the [API documentation](../../reference/api/aibridge.md) for full details.

### CLI

Export interceptions as JSON using the CLI:

```sh
coder aibridge interceptions list --initiator me --limit 1000
```

You can filter by time range, provider, model, and user:

```sh
coder aibridge interceptions list \
  --started-after "2025-01-01T00:00:00Z" \
  --started-before "2025-02-01T00:00:00Z" \
  --provider anthropic
```

See `coder aibridge interceptions list --help` for all options.

### Structured logging

AI Bridge can emit structured log entries for every interception record. This is
the primary mechanism for streaming data into external SIEM or observability
systems (Splunk, Elastic, Grafana Loki, etc.).

Enable structured logging with the server flag:

```bash
CODER_AIBRIDGE_STRUCTURED_LOGS=true
```

When enabled, each interception produces a structured log entry containing the
user, model, provider, token usage, prompt content, and tool invocations. These
entries can be collected by any log aggregation system that reads server logs.

## Integration with external systems

Coder positions AI Bridge as a **signal source**, not a data warehouse.
Enterprise deployments are expected to export AI Bridge data into their existing
SIEM/analytics platforms for correlation, alerting, and long-term retention.

### Recommended integration pattern

1. **Enable structured logging** to emit per-request records to your log
   aggregation system.
2. **Configure OTEL tracing** (see [below](#tracing)) to send traces to your
   collector.
3. **Scrape Prometheus metrics** from the Coder server for aggregate dashboards.
4. **Import the Grafana dashboard** as a starting point for visualization.
5. **Correlate with Agent Boundaries logs** in your SIEM by joining on shared
   fields (user, workspace ID, timestamp) for a complete picture of AI
   interactions and network access.

### What to monitor

| Category | Examples |
|----------|----------|
| **Adoption** | Active AI users, requests per day, model distribution |
| **Cost** | Token usage by user, team, model, provider |
| **Compliance** | Prompt audit trails, tool invocation history |
| **Security** | Unusual usage patterns, unexpected model access, anomalous request volume |

> **Note:** Correlating AI Bridge interceptions with
> [Agent Boundaries audit logs](../agent-boundaries/index.md#audit-logs)
> currently requires exporting both log streams to an external analytics
> platform and joining on shared fields (user, workspace, timestamp). Built-in
> cross-referencing is planned for a future release.

## Data Retention

AI Bridge data is retained for **60 days by default**. Configure the retention
period to balance storage costs with your organization's compliance and analysis
needs.

For configuration options and details, see [Data Retention](./setup.md#data-retention)
in the AI Bridge setup guide.

## Tracing

AI Bridge supports tracing via [OpenTelemetry](https://opentelemetry.io/),
providing visibility into request processing, upstream API calls, and MCP server
interactions. Traces can be exported to any OTEL-compatible backend: Grafana
Tempo, Datadog, New Relic, Jaeger, or any other collector.

### Enabling Tracing

AI Bridge tracing is enabled when tracing is enabled for the Coder server.
To enable tracing set `CODER_TRACE_ENABLE` environment variable or
[--trace](https://coder.com/docs/reference/cli/server#--trace) CLI flag:

```sh
export CODER_TRACE_ENABLE=true
```

```sh
coder server --trace
```
To configure a specific trace endpoint:

```bash
export CODER_TRACE_ENABLE=true
export CODER_TRACE_ENDPOINT=<your-otel-collector-endpoint>
```

### What is Traced

AI Bridge creates spans for the following operations:

| Span Name                                   | Description                                          |
|---------------------------------------------|------------------------------------------------------|
| `CachedBridgePool.Acquire`                  | Acquiring a request bridge instance from the pool    |
| `Intercept`                                 | Top-level span for processing an intercepted request |
| `Intercept.CreateInterceptor`               | Creating the request interceptor                     |
| `Intercept.ProcessRequest`                  | Processing the request through the bridge            |
| `Intercept.ProcessRequest.Upstream`         | Forwarding the request to the upstream AI provider   |
| `Intercept.ProcessRequest.ToolCall`         | Executing a tool call requested by the AI model      |
| `Intercept.RecordInterception`              | Recording creating interception record               |
| `Intercept.RecordPromptUsage`               | Recording prompt/message data                        |
| `Intercept.RecordTokenUsage`                | Recording token consumption                          |
| `Intercept.RecordToolUsage`                 | Recording tool/function calls                        |
| `Intercept.RecordInterceptionEnded`         | Recording the interception as completed              |
| `ServerProxyManager.Init`                   | Initializing MCP server proxy connections            |
| `StreamableHTTPServerProxy.Init`            | Setting up HTTP-based MCP server proxies             |
| `StreamableHTTPServerProxy.Init.fetchTools` | Fetching available tools from MCP servers            |

Example trace of an interception using Jaeger backend:

![Trace of interception](../../images/aibridge/jaeger_interception_trace.png)

### Capturing Logs in Traces

> **Note:** Enabling log capture may generate a large volume of trace events.

To include log messages as trace events, enable trace log capture
by setting `CODER_TRACE_LOGS` environment variable or using
[--trace-logs](https://coder.com/docs/reference/cli/server#--trace-logs) flag:

```sh
export CODER_TRACE_ENABLE=true
export CODER_TRACE_LOGS=true
```

```sh
coder server --trace --trace-logs
```
## Prometheus metrics

AI Bridge exposes metrics via the Coder server's Prometheus endpoint. When
Prometheus is enabled on your deployment (`CODER_PROMETHEUS_ENABLE=true`), AI
Bridge metrics are included automatically.

Metrics include request counts, token usage, latency histograms, and error
rates — all labeled by user, model, and provider.

For general Prometheus setup, see the
[Coder Prometheus documentation](https://coder.com/docs/admin/integrations/prometheus).

## Rate limiting

AI Bridge supports rate limiting to protect upstream providers and your
deployment:

| Setting | Description | Default |
|---------|-------------|---------|
| `CODER_AIBRIDGE_MAX_RPS` | Maximum requests per second per replica. Set to `0` to disable. | `0` |
| `CODER_AIBRIDGE_MAX_CONCURRENT` | Maximum concurrent requests per replica. Set to `0` to disable. | `0` |

## Next steps

- [Setup](./setup.md) — Enable AI Bridge and configure providers.
- [Reference](./reference.md) — Full list of configuration options.
- [Agent Boundaries audit logs](../agent-boundaries/index.md#audit-logs) —
  Correlate network-level logs with AI Bridge data.
