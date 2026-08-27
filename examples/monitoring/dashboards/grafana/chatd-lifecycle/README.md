# Chatd Chat Lifecycle Grafana Dashboard

A Grafana dashboard for diagnosing where time goes in Coder Agents chat
sessions. It aggregates the `coderd_chatd_stage_duration_seconds{stage,scope}`
histogram into a stage-level flame graph with a selectable summary statistic
(mean, p50, p90, p95, p99), plus summary panels for the whole chat pipeline.

Stage hierarchy:

```text
chat_turn
├── queue_wait          queued message insert -> promotion
├── capacity_wait       concurrent-agent limiter wait
├── acquisition         trigger message -> worker pickup
└── generation_step     one step of a turn (repeats)
    ├── prepare         prompt build, model resolution, context hydration
    ├── mcp_connect     MCP server connection
    ├── provider_attempt  one provider HTTP round trip (per retry)
    │   └── time_to_first_token
    ├── stream          provider stream open -> close
    ├── thinking        reasoning part duration
    ├── tool_call       one local tool call
    ├── commit          step persistence transaction
    └── compaction      auxiliary compaction call
```

Stages overlap in wall time (tool calls and thinking happen inside the
stream), so the flame graph is a stage-time profile, not a strict
decomposition, and quantile statistics are not additive across stages.

## Dimensions

The stage histogram carries four labels, exposed as dashboard variables
where noted:

| Label    | Values                                                                                                                                              | Dashboard variable          |
|----------|-----------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------|
| `stage`  | the 14 stage names above                                                                                                                            | none (fixed hierarchy)      |
| `scope`  | `turn` (part of a chat turn) or `background` (detached async work such as title and summary generation)                                             | none (panels pin one scope) |
| `model`  | resolved model ID, empty before a model is resolved                                                                                                 | `$model` (multi-select)     |
| `effort` | effective reasoning effort sent to the provider (`none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`), empty when the model config sets none | `$effort` (multi-select)    |

Two more variables apply everywhere: `$datasource` selects the Prometheus
data source and `$stat` selects the summary statistic (mean, p50, p90,
p95, p99) for every stat-aware panel.

Stages that run before or outside model resolution (`chat_turn`,
`queue_wait`, `capacity_wait`, `acquisition`, `mcp_connect`, `commit`)
carry empty `model`/`effort` labels. Filtering `$model` or `$effort` to a
specific value therefore zeroes those stages and effectively narrows the
view to the generation stages.

## Panels

### Stage profile

**Stage profile flamegraph ($stat)** - one frame per stage, laid out in
the fixed hierarchy, frame width = the selected `$stat` of that stage's
duration over the dashboard time range. Dimensions: filtered to
`scope="turn"` and the `$model`/`$effort` selections; `$stat` picks the
statistic. How to read: the widest frames under `generation_step` are
where turn time goes; compare `provider_attempt` (request to response
headers) against `stream` (full stream) to separate provider latency
from streaming time. Caveats: stages overlap in wall time (tool calls
and thinking happen inside the stream) and quantiles are not additive,
so a child frame can read wider than its parent at high percentiles;
a stage whose series first appears inside the window reads 0 until its
second sample.

**Stage profile in hierarchy order ($stat)** - the same query as the
flamegraph drawn as horizontal bars in depth-first order with the tree
indented into the labels. Dimensions: identical to the flamegraph. Use
it to read stages too small to see as frames (prepare, commit,
tool_call are typically milliseconds next to multi-second streams) and
as a numeric check on the flamegraph.

### Stage trends

**Stage duration over time ($stat)** - one series per stage, the
selected `$stat` computed over `$__rate_interval`. Dimensions:
`scope="turn"`, `$model`/`$effort` filters, series split by `stage`.
How to read: this is the drill-down for "when did it get slow" - a
regression visible in the profile shows here as a step or trend in the
affected stage. Idle stages drop out rather than plotting NaN.

**Stage time share of chat_turn** - each stage's total time as a
percentage of total `chat_turn` time, from mean rates of the histogram
sums. Dimensions: numerator is `scope="turn"` with `$model`/`$effort`
applied and split by `stage`; the denominator is all `chat_turn` time
without model/effort filters, because `chat_turn` carries empty
model/effort labels. How to read: this is the "where does the time go"
summary - stages overlap, so series can sum past 100%, but a single
stage rising toward 100% of turn time identifies the dominant cost.

**Queue, capacity and acquisition wait (p99)** - p99 of the three
pre-generation waits: `queue_wait` (queued message insert to
promotion), `capacity_wait` (concurrent-agent limiter admission) and
`acquisition` (trigger message insert to worker pickup). Dimensions:
`scope="turn"`, fixed to those three stages, split by `stage`;
`$model`/`$effort` apply but these stages carry empty labels, so
non-All selections blank this panel. How to read: these are scheduling
delays before any model work starts - user-visible latency that no
provider-side optimization can fix.

### Throughput and TTFT

**Time to first token** - p50/p90/p99/mean of `coderd_chatd_ttft_seconds`,
the pre-existing histogram recorded when the first streamed part
arrives. Dimensions: none of the stage labels; this histogram is
labeled by provider/model internally but the panel aggregates across
them, and `$model`/`$effort` do not apply. The `time_to_first_token`
stage in the profile measures the same interval and does honor the
filters. How to read: the primary user-perceived responsiveness metric
for streaming.

**Turn rate and stage sample rate** - completed `chat_turn` per second
plus one rate series per other stage. Dimensions: `scope="turn"`,
`$model`/`$effort` applied, split by `stage`. How to read: throughput
and shape - a stage rate above the turn rate means the stage repeats
within a turn (generation steps, provider attempts, tool calls);
`provider_attempt` rising faster than `stream` indicates retries.

**Background provider calls ($stat)** - rate and selected `$stat`
duration of background-scope `provider_attempt` samples: detached
title/summary/quickgen requests that are excluded from every other
panel. Dimensions: pinned to the background scope of the
`provider_attempt` stage; `$model`/`$effort` are not applied. How to read:
this work costs provider quota and money but no user-facing turn
latency; a spike here with flat turn panels means background load, not
a chat regression.

## Setup

1. **Configure a Prometheus data source** that scrapes your coderd
   Prometheus endpoint (`--prometheus-enable`).
2. **Import**: in Grafana navigate to **Dashboards** -> **Import** ->
   **Upload JSON file** with [`dashboard.json`](./dashboard.json), then map
   the Prometheus data source when prompted.

Per-session drill-down is available by exporting coderd traces
(`--trace` with standard OTLP environment variables) to a tracing backend
such as Tempo; each chat turn is a `chat_turn` root span whose children
mirror the stage hierarchy above.
