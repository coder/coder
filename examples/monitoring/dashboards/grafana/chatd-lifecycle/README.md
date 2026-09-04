# Chatd Chat Lifecycle Grafana Dashboard

A Grafana dashboard for diagnosing where time goes in Coder Agents chat
sessions. It aggregates the stage histogram
`coderd_chatd_stage_duration_seconds{stage,scope,chat_kind,model,effort}`
into a stage-level flame graph with a selectable summary statistic
(mean, p50, p90, p95, p99), plus summary panels for the whole chat
pipeline and a per-turn partition of turn wall time.

Stage hierarchy, by tree level:

```text
L0  chat_turn                one sample per turn
L1  ├── queue_wait           queued message insert -> promotion
L1  ├── capacity_wait        concurrent-agent limiter wait
L1  ├── acquisition          trigger message -> worker pickup
L1  └── generation_step      one step of a turn (repeats)
L2      ├── prepare          prompt build, model resolution, context hydration
L2      ├── mcp_connect      MCP server connection
L2      ├── provider_attempt one provider HTTP round trip (per retry)
L3      │   └── time_to_first_token   request open -> first streamed part
L2      ├── retry_backoff    wait between provider attempts
L2      ├── stream           provider stream open -> close
L2      ├── thinking         reasoning part duration
L2      ├── tool_call        one local tool call
L2      ├── commit           step persistence transaction
L2      └── compaction       auxiliary compaction call
```

Stages overlap in wall time (tool calls and thinking happen inside the
stream), so the flame graph is a stage-time profile, not a strict
decomposition, and quantile statistics are not additive across stages.

## Dimensions

The stage histogram carries five labels, exposed as dashboard variables
where noted:

| Label       | Values                                                                                                                                              | Dashboard variable          |
|-------------|-----------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------|
| `stage`     | the 15 stage names above                                                                                                                            | none (fixed hierarchy)      |
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
`capacity_wait`, `acquisition`, `mcp_connect`, `commit`,
`retry_backoff`) always carry empty `model`/`effort` labels on
`coderd_chatd_stage_duration_seconds`. Panels match those stages without
the `$model`/`$effort` matchers, since a matcher there could only
subtract, so narrowing either variable keeps the waits populated and
narrows only the model-carrying stages (`chat_turn`, `generation_step`,
`prepare`, `provider_attempt`, `time_to_first_token`, `stream`,
`thinking`, `tool_call`, `compaction`). `chat_turn` is stamped with the
turn's model and effort when the turn ends, so it takes the matchers
like any other model-carrying stage. The exception applies only to the
per-occurrence metric; the turn-end metrics stamp every stage.

The turn-end metrics behind the level rows and the Turn time partition
row (`coderd_chatd_turn_stage_seconds`, `coderd_chatd_turn_stage_count`,
`coderd_chatd_stage_share_of_turn`, `coderd_chatd_turn_time_seconds`,
`coderd_chatd_turn_time_share`) are observed once per turn with the
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

**Stage profile in hierarchy order ($stat)** - the flamegraph query as a
table in depth-first order: a Stage column indented by level, the level
itself, the number of occurrences of the stage in the selected time
range, and the duration drawn as a gauge bar scaled to the widest stage,
so the bar lengths stay comparable to the flamegraph frames. Dimensions:
identical to the flamegraph. The Count column is an exact counter
increase over the range rather than a rate, so a stage that never ran
reads 0 and a stage that ran once reads 1. Use it to read stages
too small to see as frames (prepare, commit, tool_call are typically
milliseconds next to multi-second streams), to tell a rare stage from an
absent one, and as a numeric check on the flamegraph. The indentation
uses fixed-width spacing rather than a drawn tree, so rows at the same
depth line up in any font.

### Reading the levels

The stage tree mixes two units of observation. `chat_turn` is recorded
once per turn, while its descendants are recorded once per occurrence,
and a step-level stage such as `stream` or `tool_call` typically occurs
six to twelve times in a turn. Plotting both on one axis compares a
25-second turn against a 2-second stream and tells you nothing, so the
trend panels are grouped by tree level and every level reads the same
four ways:

| Panel                   | Metric                                | Question                                 |
|-------------------------|---------------------------------------|------------------------------------------|
| Duration per occurrence | `coderd_chatd_stage_duration_seconds` | how long does one of these take          |
| Seconds per turn        | `coderd_chatd_turn_stage_seconds`     | how much of a turn does it add up to     |
| Occurrences per turn    | `coderd_chatd_turn_stage_count`       | how often does it happen in a turn       |
| Share of turn           | `coderd_chatd_stage_share_of_turn`    | what fraction of the turn does it occupy |

The first is per occurrence, the other three are per turn, recorded when
the turn ends. Seconds per turn is roughly occurrences per turn times
duration per occurrence, so the three together separate "each one is
slow" from "it happens too often". Stages still overlap within a level,
so shares and seconds at one level do not sum to the turn; the Turn time
partition row is the view that does add up.

The Stage profile row above is the one deliberate exception: the flame
graph and its bar chart show all levels together, because a profile is
about relative width across the tree rather than about trends.

### Level 0: Turn

Members: `chat_turn`.

**Turn duration ($stat)** - wall time of a whole turn, split by model.
This is the denominator every other level is measured against, and the
model split keeps a slow model from hiding inside a blended line.

**Turns per minute** - completed turns per minute, split by model. Read
it beside turn duration: duration moving with flat throughput is a
latency regression, both moving together is usually a workload change.

**Turn time mix by model** - the exclusive category partition of a turn
per model, described in the next section.

### Level 0: Turn time partition

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
stage. The opposite failure, categories that sum to more than the turn,
is emitted as measured with zero `unattributed` and counted in
`coderd_chatd_stage_anomalies_total{reason="overattributed"}`; that
counter also records stage observations dropped for inverted clocks and
turns dropped for a non-positive duration.

These per-turn histograms replaced the earlier time-share panels that
divided aggregated stage seconds by aggregated `chat_turn` seconds: the
two observations for one turn land at different times, so any aggregate
ratio mixes turns and reads well past 100%.

**Turn time mix by model** - shown in the Level 0 row above, one
100%-stacked bar per model, each
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

### Level 1: Turn children

Members: `acquisition`, `queue_wait`, `capacity_wait`,
`generation_step`. The three scheduling waits happen once per turn
before generation starts; `generation_step` repeats once per step.

`capacity_wait` appears in the duration per occurrence panel only. It is
measured by the acquisition loop before the turn exists, so no turn
records it, and its window lies inside `acquisition`, which the turn
does record under the `scheduling` category.

Because these are the direct children of the turn, their share panel is
the quickest answer to "was this turn slow because of scheduling or
because of generation".

### Level 2: Step children

Members: `prepare`, `mcp_connect`, `provider_attempt`, `stream`,
`retry_backoff`, `thinking`, `tool_call`, `commit`, `compaction`.

These are the stages inside one generation step and they overlap each
other, so read them as a profile of the step. `provider_attempt` is
listed here rather than at level 3: in a trace it wraps the stream, but
it measures one full provider round trip, so it belongs beside `stream`
at step level. It appears at one level only, so the occurrence counts
stay comparable within the row.

### Level 3: Stream children

Members: `time_to_first_token`.

The part of a provider round trip before the first token. It keeps the
same four panels as the other levels so the rows compare directly, even
though the level has a single member.

### Throughput and TTFT

**Time to first token** - p50/p90/p99/mean of `coderd_chatd_ttft_seconds`,
the pre-existing histogram recorded when the first streamed part
arrives. Dimensions: none of the stage labels; this histogram is
labeled by provider/model internally but the panel aggregates across
them, and `$model`, `$effort` and `$chat_kind` do not apply. The
`time_to_first_token` stage in the profile measures the same interval
and does honor the filters. How to read: the primary user-perceived
responsiveness metric for streaming.

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
