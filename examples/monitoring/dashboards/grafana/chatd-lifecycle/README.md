# Chatd Chat Lifecycle Grafana Dashboard

A Grafana dashboard for diagnosing where time goes in Coder Agents chat
sessions. It aggregates the stage histogram
`coderd_chatd_stage_duration_seconds{stage,scope,chat_kind,model,effort}`
into a stage-level flame graph with a selectable summary statistic
(mean, p50, p90, p95, p99), plus summary panels for the whole chat
pipeline and a per-turn partition of turn wall time.

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

The stage histogram carries five labels, exposed as dashboard variables
where noted:

| Label       | Values                                                                                                                                              | Dashboard variable          |
|-------------|-----------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------|
| `stage`     | the 14 stage names above                                                                                                                            | none (fixed hierarchy)      |
| `scope`     | `turn` (part of a chat turn) or `background` (detached async work such as title and summary generation)                                             | none (panels pin one scope) |
| `chat_kind` | `root` (a chat a user drives) or `subagent` (a chat spawned by a parent agent), empty for background work                                           | `$chat_kind` (multi-select) |
| `model`     | resolved model ID, empty before a model is resolved                                                                                                 | `$model` (multi-select)     |
| `effort`    | effective reasoning effort sent to the provider (`none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`), empty when the model config sets none | `$effort` (multi-select)    |

Two more variables apply everywhere: `$datasource` selects the Prometheus
data source and `$stat` selects the summary statistic (mean, p50, p90,
p95, p99) for every stat-aware panel.

`chat_kind` separates root chats, which a user drives, from subagent
chats, which a parent agent spawns and which run as separate chats with
their own turn trees. It is a property of the turn, so every turn-scoped
stage carries it, including the stages recorded before a model is
resolved. `$chat_kind` therefore filters every stage panel in full,
rather than narrowing part of the hierarchy the way `$model` and
`$effort` do. Background provider calls run outside a turn and carry an
empty value, so that panel is never filtered by it.

Stages that run before a model is resolved (`queue_wait`,
`capacity_wait`, `acquisition`, `mcp_connect`, `commit`) always carry
empty `model`/`effort` labels. Panels match those stages without the
`$model`/`$effort` matchers, since a matcher there could only subtract,
so narrowing either variable keeps the waits populated and narrows only
the model-carrying stages (`chat_turn`, `generation_step`, `prepare`,
`provider_attempt`, `time_to_first_token`, `stream`, `thinking`,
`tool_call`, `compaction`). `chat_turn` is stamped with the turn's model
and effort when the turn ends, so it takes the matchers like any other
model-carrying stage.

The turn-end metrics behind the Turn time partition row
(`coderd_chatd_turn_time_seconds`, `coderd_chatd_turn_time_share`,
`coderd_chatd_stage_share_of_turn`) are observed once per turn with the
turn's `chat_kind`, `model` and `effort` already known, so all three
variables apply to them without exception.

## Panels

### Stage profile

**Stage profile flamegraph ($stat)** - one frame per stage, laid out in
the fixed hierarchy, frame width = the selected `$stat` of that stage's
duration over the dashboard time range. Dimensions: filtered to the
turn scope and `$chat_kind`; `$model`/`$effort` apply to the
model-carrying stages only, so a concrete selection narrows the
generation stages while the `chat_turn` root and the pre-model stages
keep their full values. `$stat` picks the statistic. How to read: the
widest frames under `generation_step` are where turn time goes; compare
`provider_attempt` (request to response headers) against `stream` (full
stream) to separate provider latency from streaming time. The `self`
column is the stage's own value for leaf stages and 0 for stages with
children; true self time is not derivable here, because overlapping
stages can make a parent minus its children negative. Caveats: stages
overlap in wall time (tool calls and thinking happen inside the stream)
and quantiles are not additive, so a child frame can read wider than its
parent at high percentiles; a stage whose series first appears inside the
window reads 0 until its second sample.

**Stage profile in hierarchy order ($stat)** - the same query as the
flamegraph drawn as horizontal bars in depth-first order with the tree
indented into the labels. Dimensions: identical to the flamegraph. Use
it to read stages too small to see as frames (prepare, commit,
tool_call are typically milliseconds next to multi-second streams) and
as a numeric check on the flamegraph.

### Stage trends

**Stage duration over time ($stat)** - one series per stage, the
selected `$stat` computed over `$__rate_interval`. Dimensions: the
turn scope and `$chat_kind`, one series per stage;
`$model`/`$effort` apply to the model-carrying stages only, so the
pre-model stages stay plotted under any model selection. How to read:
this is the drill-down for "when did it get slow" - a regression visible
in the profile shows here as a step or trend in the affected stage. Idle
stages drop out rather than plotting NaN.

**Stage share of turn ($stat)** - the selected `$stat` of each stage's
per-turn share of its turn, from `coderd_chatd_stage_share_of_turn`,
which records one fraction per stage when a turn ends. Dimensions: one
series per stage, and the chat kind, model and effort variables all
apply,
because the metric is stamped with the turn's identity at turn end. How
to read: this is the exact form of the share question - every sample
comes from one completed turn, so there is no phase skew between
numerator and denominator. Stages overlap, so the shares do not sum to
100%, and at p99 several stages can each approach the whole turn.

**Queue, capacity and acquisition wait (p99)** - p99 of the three
pre-generation waits: `queue_wait` (queued message insert to
promotion), `capacity_wait` (concurrent-agent limiter admission) and
`acquisition` (trigger message insert to worker pickup). Dimensions:
`scope="turn"` and `$chat_kind`, fixed to those three stages, split by
`stage`; all three run before a model is resolved, so `$model`/`$effort`
are not applied and the panel stays populated under any model selection.
How to read: these are scheduling delays before any model work starts -
user-visible latency that no provider-side optimization can fix.

### Turn time partition

The stage hierarchy overlaps in wall time, so it can tell you which
stages are slow but not how a turn's seconds divide up. The turn-end
metrics answer that with an exclusive partition of turn wall time,
observed once per turn:

| Category              | Turn time spent                                     |
|-----------------------|-----------------------------------------------------|
| `scheduling`          | queueing, capacity admission and worker pickup      |
| `time_to_first_token` | provider request open until the first streamed part |
| `streaming`           | first part until the stream closes                  |
| `tool_execution`      | local tool calls                                    |
| `provider_error`      | attempts that ended in a provider error             |
| `retry_backoff`       | waiting between provider attempts                   |
| `compaction`          | auxiliary compaction calls                          |
| `chatd_overhead`      | prompt build, persistence and other chatd work      |
| `unattributed`        | turn time no category claimed                       |

The categories are exclusive and sum to the turn, so these panels do add
up, unlike the stage panels. `unattributed` is the completeness check:
if it grows, real turn time is happening outside every instrumented
stage.

These per-turn histograms replaced the earlier time-share panels that
divided aggregated stage seconds by aggregated `chat_turn` seconds: the
two observations for one turn land at different times, so any aggregate
ratio mixes turns and reads well past 100%.

**Turn time mix by model** - one 100%-stacked bar per model, each
category's total seconds over the range divided by all categories'
total seconds. Dimensions: `$chat_kind`, `$model` and `$effort` all
apply; the grouping is fixed to `model`, because Grafana transformation
options do not interpolate dashboard variables and the matrix transform
needs a static row field. How to read: the fastest way to compare where
models spend a turn, for example a model with a large
`time_to_first_token` share against one dominated by `streaming`.

**Seconds per turn by category** - mean seconds per turn in each
category, stacked, with total turn duration as a line. Category seconds
are divided by the turn count taken from the `unattributed` category's
`_count`, since every category is observed once per turn even when it is
zero. How to read: the stack height is the mean turn duration, so the
line should sit on top of the stack; a gap means the current variable
selection dropped categories.

**Unattributed turn time** - mean unattributed seconds per turn and its
mean share of a turn. How to read: this is the completeness check for
the stage model, so treat a rising line as an instrumentation gap rather
than a workload change.

**Category share per turn ($stat)** - the selected `$stat` of each
category's share of a turn, from `coderd_chatd_turn_time_share`. How to
read: the mix bar shows where aggregate time goes, this shows how much a
category varies per turn, so a small mean with a large p99 marks a
bursty cost such as a slow tool call or a retry storm in a minority of
turns. Quantiles are per category, so unlike the mean shares they do not
sum to 100%.

### Throughput and TTFT

**Time to first token** - p50/p90/p99/mean of `coderd_chatd_ttft_seconds`,
the pre-existing histogram recorded when the first streamed part
arrives. Dimensions: none of the stage labels; this histogram is
labeled by provider/model internally but the panel aggregates across
them, and `$model`, `$effort` and `$chat_kind` do not apply. The
`time_to_first_token` stage in the profile measures the same interval
and does honor the filters. How to read: the primary user-perceived
responsiveness metric for streaming.

**Turn rate and stage sample rate** - completed `chat_turn` per second
plus one rate series per other stage. Dimensions: `scope="turn"` and
`$chat_kind`, split by `stage`, with `$model`/`$effort` applied to the
model-carrying stages only. How to read: throughput and shape - a stage
rate above the turn rate means the stage repeats within a turn
(generation steps, provider attempts, tool calls); `provider_attempt`
rising faster than `stream` indicates retries.

**Background provider calls ($stat)** - rate and selected `$stat`
duration of background-scope `provider_attempt` samples: detached
title/summary/quickgen requests that are excluded from every other
panel. Dimensions: pinned to the background scope of the
`provider_attempt` stage; `$model`, `$effort` and `$chat_kind` are not
applied, because background work runs outside a turn. How to read:
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
