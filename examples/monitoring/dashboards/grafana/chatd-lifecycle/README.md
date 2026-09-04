# Chatd Chat Lifecycle Grafana Dashboard

A Grafana dashboard for diagnosing where time goes in Coder Agents chat
sessions. It reads the chat lifecycle metrics that `coder server` exposes
when the `chat-stage-metrics` experiment is enabled
(`--experiments=chat-stage-metrics` or `--experiments=*`):

| Family                                                                                            | What it records                                                  |
|---------------------------------------------------------------------------------------------------|------------------------------------------------------------------|
| `coderd_chatd_stage_duration_seconds{stage,scope,chat_kind}`                                      | one sample per stage occurrence                                  |
| `coderd_chatd_model_stage_duration_seconds{stage,provider_type,chat_kind,model}`                  | the provider-bound stages again, per model                       |
| `coderd_chatd_turn_time_seconds_total{category,chat_kind}`, `coderd_chatd_turns_total{chat_kind}` | an exclusive partition of each completed turn's wall time        |
| `coderd_chatd_turn_outcomes_total{outcome,chat_kind}`                                             | `completed`, `interrupted`, `error`, `abandoned` per closed turn |
| `coderd_chatd_stage_anomalies_total{reason}`                                                      | dropped or clamped observations                                  |

Tracing spans are emitted whether or not the experiment is enabled.

## Stages

```text
L0  chat_turn                one sample per turn
L1  ├── queue_wait           queued message insert -> promotion
L1  ├── capacity_wait        concurrent-agent limiter wait
L1  ├── acquisition          trigger message -> worker pickup
L1  └── generation_step      [span-only] one step of a turn (repeats)
L2      ├── prepare          [span-only] prompt build, model resolution, context hydration
L3      │   └── mcp_connect  MCP server connection
L2      ├── retry_backoff    wait between provider attempts
L2      ├── stream           provider stream open -> close
L3      │   ├── time_to_first_token   request open -> first streamed part
L3      │   └── provider_attempt      one provider HTTP round trip (per retry)
L2      ├── thinking         [span-only] reasoning part duration (window inside stream)
L2      ├── tool_call        one local tool call
L2      ├── commit           step persistence transaction
L2      └── compaction       [span-only] auxiliary compaction call
```

The tree follows span parentage and stages overlap in wall time, so
per-stage values are a profile, not a decomposition, and quantiles are
not additive across stages. The dashboard plots each tree level on its
own axis because `chat_turn` is observed once per turn while a step
stage such as `stream` occurs many times per turn. Stages marked
`[span-only]` have no histogram and appear only in traces.

The model histogram observes `time_to_first_token`, `stream`, and
`provider_attempt`. Every
sample there is also on `stage_duration_seconds`, so the stage panels
are complete without it and `$model` applies only to the Model row.

### Turn time partition

The turn-end counters divide each completed turn's wall time into
exclusive categories that sum to the turn: `scheduling`,
`time_to_first_token`, `streaming`, `tool_execution`, `provider_error`,
`retry_backoff`, `compaction`, `preparation`, `persistence`,
`chatd_overhead`, and `unattributed`. `unattributed` is the completeness
check: if it grows, turn time is being spent outside every instrumented
stage. The partition is computed per turn from that turn's stages, so it
is exact; the dashboard has no panel that divides aggregate stage seconds
by aggregate turn counts, which mixes turns and can exceed 100% while
long turns are in flight. Only `completed` turns are partitioned; the
other outcomes are visible in the Turn outcomes panel.

## Dimensions

| Label           | Values                                                                            | Dashboard variable          |
|-----------------|-----------------------------------------------------------------------------------|-----------------------------|
| `stage`         | the stage names above                                                             | none (fixed hierarchy)      |
| `scope`         | `turn`, or `background` for detached title, summary, and status-label generation  | none (panels pin one scope) |
| `chat_kind`     | `root` (a chat a user drives) or `subagent` (a chat spawned by a parent agent)    | `$chat_kind`                |
| `provider_type` | configured type of the model's AI provider (`anthropic`, `bedrock`, `azure`, ...) | none (panels group by it)   |
| `model`         | the model config's model ID                                                       | `$model`                    |

`$datasource` and `$stat` (mean, p50, p90, p95, p99) apply to every
stat-aware panel.

`provider_type` is the value the AI Gateway metrics report under that
label. The pre-existing `coderd_chatd_*{provider,model}` families report
the wire protocol as `provider` (`anthropic` or `openai` for Bedrock,
`openai-compat` for OpenAI-compatible types), so do not join the two by
provider.

Reasoning effort and organization are span attributes
(`reasoning_effort`, `organization_name`), not metric labels; query them
in a tracing backend, for example
`{ name = "time_to_first_token" && span.reasoning_effort = "high" }` in
Tempo. `chat_kind` is empty only for a queue wait recorded when the
chat row could not be reloaded after promotion.

## Caveats

- `time_to_first_token` observes only windows in which a first content
  part arrived. A failed attempt ends its span with the error, is not
  observed, and its time lands in the `provider_error` category. A
  provider that hangs without streaming or failing shows up in `stream`
  instead.
- `capacity_wait` is a lower bound. The refusal history is in memory on
  each replica while the limit is deployment-wide, so a replica that
  admits a chat on its first attempt records nothing and a restart
  discards the history. Because all replicas wake together when a chat
  becomes acquirable, the undercount is bounded by about one acquisition
  interval (30s by default); do not scale by replica count. Short waits
  are the ones most likely to be missing, which skews the distribution
  long. For a deployment-accurate view, compare `acquisition` quantiles
  while `coderd_chatd_agents_queued_for_capacity` is above zero against
  the same quantiles while it is zero. Entitled deployments have no
  limit and never record it.
- `mcp_connect` is one sample per generation step covering every
  configured MCP server, is never errored, and is absent for chats
  without MCP servers.
- `provider_attempt` closes on response headers and `time_to_first_token`
  on the first streamed part, so the two overlap almost entirely; an
  HTTP retry adds a second attempt but no second TTFT.
- The Turn time mix panel groups by `chat_kind` with a fixed grouping
  because Grafana transformations do not interpolate variables.

## Series cost

Only `model_stage_duration_seconds` carries `model`, and no family
carries an organization label. Worst case per replica, each `chat_kind`
costs about 180 series (11 stages x 15 series, plus 16 partition
counters) and each `(chat_kind, model)` costs 45 (3 stages x 15).
Background `provider_attempt` adds one histogram set per chat kind and per
model. A coderd replica exposes roughly 3,400 `coderd_*` series before
chat instrumentation; both chat kinds with five models add about 800
series per replica. Series appear only for tuples that see traffic.

Operators who want alerts without the dashboard can drop the remaining
stages at scrape time. These rules keep `chat_turn`,
`time_to_first_token`, `mcp_connect`, and `capacity_wait` on the stage
histogram, `time_to_first_token` on the model histogram, and the
outcome and anomaly families; the Turn time partition, Level 2, and
most Model panels go empty:

```yaml
metric_relabel_configs:
  - source_labels: [__name__, stage]
    regex: 'coderd_chatd_stage_duration_seconds.*;(acquisition|queue_wait|stream|provider_attempt|tool_call|commit|retry_backoff|generation_step|prepare|thinking|compaction)'
    action: drop
  - source_labels: [__name__, stage]
    regex: 'coderd_chatd_model_stage_duration_seconds.*;(stream|provider_attempt|thinking|compaction)'
    action: drop
  - source_labels: [__name__]
    regex: 'coderd_chatd_(turn_time_seconds_total|turns_total)'
    action: drop
```

## Alerting

Bucket edges are round numbers (`0.1 0.25 0.5 1 2.5 5 10 30 60 300 1800
3600`), so a threshold on an edge is exact. Prometheus 3 stores classic `le` values as floats
(`10.0`), hence the regex matcher:

```yaml
groups:
  - name: chatd-latency
    rules:
      # More than 10% of first tokens over 10s in the last 10m, per model.
      - alert: ChatdSlowTimeToFirstToken
        expr: |
          1 - (
            sum by (provider_type, model) (rate(coderd_chatd_model_stage_duration_seconds_bucket{stage="time_to_first_token", le=~"10(\\.0)?"}[10m]))
            /
            sum by (provider_type, model) (rate(coderd_chatd_model_stage_duration_seconds_count{stage="time_to_first_token"}[10m]))
          ) > 0.10
          and
          sum by (provider_type, model) (increase(coderd_chatd_model_stage_duration_seconds_count{stage="time_to_first_token"}[10m])) > 20
        for: 5m
      # p95 MCP connect above 5s.
      - alert: ChatdSlowMCPConnect
        expr: |
          histogram_quantile(0.95, sum by (le) (rate(coderd_chatd_stage_duration_seconds_bucket{stage="mcp_connect"}[10m]))) > 5
        for: 10m
```

Saturation paging belongs to the `coderd_chatd_agents_queued_for_capacity`
gauge, not to `capacity_wait`, which is recorded only after the wait ends.

## Setup

1. Configure a Prometheus data source that scrapes the coderd Prometheus
   endpoint (`--prometheus-enable`).
2. In Grafana, go to **Dashboards** -> **Import** -> **Upload JSON file**,
   choose [`dashboard.json`](./dashboard.json), and map the Prometheus
   data source when prompted.
3. Enable the `chat-stage-metrics` experiment on the coderd replicas
   (`CODER_EXPERIMENTS=chat-stage-metrics`). Without it the chatd stage
   families are not registered and every panel reads "No data".

For per-session drill-down, export coderd traces (`--trace` with the
standard OTLP environment variables) to a backend such as Tempo. Each
turn is a `chat_turn` root span whose children mirror the stage tree,
with `reasoning_effort`, `tool_name`, and `generation_action` as span
attributes. `--trace` exports over OTLP gRPC and the endpoint needs a
scheme (`OTEL_EXPORTER_OTLP_ENDPOINT=http://tempo:4317`); a scheme-less
value is accepted and exports nothing. A turn trace also carries several
hundred `DB QUERY` and AI Bridge `Intercept` spans; filter with
`{ span.scope = "turn" }` to read the stage tree alone.
