# Chatd Chat Lifecycle Grafana Dashboard

A Grafana dashboard for diagnosing where time goes in Coder Agents chat
sessions. It plots the stage histogram
`coderd_chatd_stage_duration_seconds{stage,scope,chat_kind,model}`
and the turn-end rollups one tree level at a time with a selectable
summary statistic (mean, p50, p90, p95, p99), plus summary panels for the
whole chat pipeline and a per-turn partition of turn wall time.

Stage hierarchy, by tree level. `[basic]` marks the stages observed at
`--chat-stage-metrics=basic`; the rest are span-only until the level is
`full` (see [Metric levels](#metric-levels)). The default level is `off`,
which exposes no stage or turn families:

```text
L0  chat_turn                [basic] one sample per turn
L1  ├── queue_wait           [basic] queued message insert -> promotion
L1  ├── capacity_wait        [basic] concurrent-agent limiter wait
L1  ├── acquisition          [basic] trigger message -> worker pickup
L1  └── generation_step      [full]  one step of a turn (repeats)
L2      ├── prepare          [full]  prompt build, model resolution, context hydration
L3      │   └── mcp_connect  [basic] MCP server connection
L2      ├── retry_backoff    [basic] wait between provider attempts
L2      ├── stream           [basic] provider stream open -> close
L3      │   ├── time_to_first_token   [basic] request open -> first streamed part
L3      │   └── provider_attempt      [basic] one provider HTTP round trip, closed on headers (per retry)
L2      ├── thinking         [full]  reasoning part duration (window inside stream)
L2      ├── tool_call        [basic] one local tool call
L2      ├── commit           [basic] step persistence transaction
L2      └── compaction       [full]  auxiliary compaction call
```

The tree follows span parentage. Stages overlap in wall time (tool calls
and thinking happen inside the stream; `time_to_first_token` and
`provider_attempt` cover nearly the same window under `stream`), so
per-stage values are a stage-time profile, not a strict decomposition,
and quantile statistics are not additive across stages.

## Metric levels

`--chat-stage-metrics` (`CODER_CHAT_STAGE_METRICS`) on `coder server`
selects how much of the instrumentation is exposed as Prometheus series.
Tracing spans are emitted at every level.

| Level           | Families                                                                                      | Stages on `stage_duration_seconds` |
|-----------------|-----------------------------------------------------------------------------------------------|------------------------------------|
| `off` (default) | `coderd_chatd_stage_metrics_level` only                                                       | none                               |
| `basic`         | `stage_duration_seconds`, `turn_time_seconds`, `stage_anomalies_total`, `stage_metrics_level` | the `[basic]` stages above         |
| `full`          | basic plus `turn_stage_seconds`, `turn_stage_count`, `stage_share_of_turn`, `turn_time_share` | all 15                             |

The label schema is the same at every level, so a query written for
`full` returns "No data" rather than an error at `basic`. The dashboard
is laid out accordingly: every row that is expanded by default works at
`basic`; rows whose title ends in `(full level only)` are collapsed and
hold the panels that need the `full` families. The stat panel at the top
shows the level each replica reports on
`coderd_chatd_stage_metrics_level`.

Series cost per `(chat_kind, model)` pair is roughly 250 at `basic`
(plus a constant ~230 per replica for the model-less wait and connect
stages) and roughly 1,200 at `full`. Multiply by replicas and by the
number of models in use.

### Alerting without the dashboard

Alerts only need `coderd_chatd_stage_duration_seconds` for the stages in
question. The buckets are round numbers (`0.05 0.1 0.25 0.5 1 2.5 5 10
20 30 60 120 300 600 1800 3600`), so a threshold that matches a bucket
edge is exact rather than interpolated:

```yaml
groups:
  - name: chatd-latency
    rules:
      # More than 10% of first tokens took over 10s in the last 10m,
      # with enough samples to mean something.
      - alert: ChatdSlowTimeToFirstToken
        expr: |
          1 - (
            sum by (model) (rate(coderd_chatd_stage_duration_seconds_bucket{stage="time_to_first_token", scope="turn", le="10"}[10m]))
            /
            sum by (model) (rate(coderd_chatd_stage_duration_seconds_count{stage="time_to_first_token", scope="turn"}[10m]))
          ) > 0.10
          and
          sum by (model) (increase(coderd_chatd_stage_duration_seconds_count{stage="time_to_first_token", scope="turn"}[10m])) > 20
        for: 5m
      # p95 MCP connect above 5s. mcp_connect carries no model label.
      - alert: ChatdSlowMCPConnect
        expr: |
          histogram_quantile(0.95, sum by (le) (rate(coderd_chatd_stage_duration_seconds_bucket{stage="mcp_connect"}[10m]))) > 5
        for: 10m
```

`time_to_first_token` is only observed when a first token arrived, so a
provider that hangs without streaming does not show up here; pair it with
`stream`, which is observed regardless, or with `provider_attempt` errors
in traces. Saturation paging belongs to the
`coderd_chatd_agents_queued_for_capacity` gauge, not to `capacity_wait`,
which is recorded only after the wait ends.

An operator who wants only these alerts can keep the two stages and drop
the rest at scrape time. Keep `chat_turn` if the dashboard's `$chat_kind`
and `$model` variables should still populate; they are sourced from it.

```yaml
metric_relabel_configs:
  - source_labels: [__name__, stage]
    regex: 'coderd_chatd_stage_duration_seconds.*;(time_to_first_token|mcp_connect|chat_turn)'
    action: keep
  - source_labels: [__name__]
    regex: 'coderd_chatd_(turn_.*|stage_share_of_turn)'
    action: drop
```

## Dimensions

The stage histogram carries four labels, exposed as dashboard variables
where noted:

| Label       | Values                                                                                                  | Dashboard variable          |
|-------------|---------------------------------------------------------------------------------------------------------|-----------------------------|
| `stage`     | the 15 stage names above                                                                                | none (fixed hierarchy)      |
| `scope`     | `turn` (part of a chat turn) or `background` (detached async work such as title and summary generation) | none (panels pin one scope) |
| `chat_kind` | `root` (a chat a user drives) or `subagent` (a chat spawned by a parent agent)                          | `$chat_kind` (multi-select) |
| `model`     | resolved model ID, empty for stages that are not tied to a model call                                   | `$model` (multi-select)     |

Two more variables apply everywhere: `$datasource` selects the Prometheus
data source and `$stat` selects the summary statistic (mean, p50, p90,
p95, p99) for every stat-aware panel.

Reasoning effort is deliberately not a metric label: it multiplies series
per model while changing the behavior of only the model-call stages. It
is a span attribute (`reasoning_effort`) on every stage span, so
per-effort analysis is a trace query (for example
`{ name = "time_to_first_token" && span.reasoning_effort = "high" }` in
Tempo).

`chat_kind` separates root chats, which a user drives, from subagent
chats, which a parent agent spawns and which run as separate chats with
their own turn trees whose wall time overlaps the parent's `tool_call`.
It is a property of the turn, so every turn-scoped stage carries it,
including the stages recorded before a model is resolved. `$chat_kind`
therefore filters every stage panel in full, rather than narrowing part
of the hierarchy the way `$model` does. Background provider calls carry
the kind of the chat that spawned them and are pinned to
`scope="background"` in their own panel.

Stages that are not tied to a model call (`queue_wait`, `capacity_wait`,
`acquisition`, `mcp_connect`, `commit`, `retry_backoff`) always carry an
empty `model` label on `coderd_chatd_stage_duration_seconds`. Panels
match those stages without the `$model` matcher, since a matcher there
could only subtract, so narrowing the variable keeps the waits populated
and narrows only the model-carrying stages (`chat_turn`,
`generation_step`, `prepare`, `provider_attempt`, `time_to_first_token`,
`stream`, `thinking`, `tool_call`, `compaction`). `chat_turn` is stamped
with the turn's model when the turn ends, so it takes the matcher like
any other model-carrying stage. The exception applies only to the
per-occurrence metric; the turn-end metrics stamp every stage.

The turn-end metrics behind the collapsed level rows and the Turn time
partition row (`coderd_chatd_turn_stage_seconds`,
`coderd_chatd_turn_stage_count`, `coderd_chatd_stage_share_of_turn`,
`coderd_chatd_turn_time_seconds`, `coderd_chatd_turn_time_share`) are
observed once per turn with the turn's `chat_kind` and `model` already
known, so both variables apply to them without exception.

## Panels

### Reading the levels

The stage tree mixes two units of observation. `chat_turn` is recorded
once per turn, while its descendants are recorded once per occurrence,
and a step-level stage such as `stream` or `tool_call` typically occurs
six to twelve times in a turn. Plotting both on one axis compares a
25-second turn against a 2-second stream and tells you nothing, so the
trend panels are grouped by tree level and every level can be read four
ways:

| Panel                   | Metric                                | Question                                 | Level |
|-------------------------|---------------------------------------|------------------------------------------|-------|
| Duration per occurrence | `coderd_chatd_stage_duration_seconds` | how long does one of these take          | basic |
| Seconds per turn        | `coderd_chatd_turn_stage_seconds`     | how much of a turn does it add up to     | full  |
| Occurrences per turn    | `coderd_chatd_turn_stage_count`       | how often does it happen in a turn       | full  |
| Share of turn           | `coderd_chatd_stage_share_of_turn`    | what fraction of the turn does it occupy | full  |

The first is per occurrence, the other three are per turn, recorded when
the turn ends. Seconds per turn is roughly occurrences per turn times
duration per occurrence, so the three together separate "each one is
slow" from "it happens too often". At `basic` only the first column is
populated, and the Turn time partition row is the per-turn view; the
other three live in each level's collapsed `(full level only)` row.
Stages still overlap within a level, so shares and seconds at one level
do not sum to the turn; the Turn time partition row is the view that
does add up.

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
| `preparation`         | prompt build, model resolution and MCP connects     |
| `persistence`         | the step commit transaction                         |
| `chatd_overhead`      | the step's own work outside every other stage       |
| `unattributed`        | turn time no category claimed                       |

The categories are exclusive and sum to the turn, so these panels do add
up, unlike the stage panels. `unattributed` is the completeness check:
if it grows, real turn time is happening outside every instrumented
stage. The opposite failure, categories that sum to more than the turn,
is emitted as measured with zero `unattributed` and counted in
`coderd_chatd_stage_anomalies_total{reason="overattributed"}`; that
counter also records stage observations dropped for inverted clocks and
turns dropped for a non-positive duration.

The partition is computed once per turn from the turn's own stages, so
it is exact per turn. The dashboard has no panel that divides aggregate
stage seconds by aggregate `chat_turn` counts: the two observations for
one turn land in different scrapes, and such a ratio mixes turns and can
read well past 100% while long turns are in flight.

The category partition is available at `basic` because `prepare`,
`commit`, `queue_wait`, and the other stages feed their categories
regardless of whether the stage itself is observed on the histogram.

**Turn time mix by model** - shown in the Level 0 row above, one
100%-stacked bar per model, each category's total seconds over the range
divided by all categories' total seconds. Dimensions: `$chat_kind` and
`$model` apply; the grouping is fixed to `model`, because Grafana
transformation options do not interpolate dashboard variables and the
matrix transform needs a static row field. How to read: the fastest way
to compare where models spend a turn, for example a model with a large
`time_to_first_token` share against one dominated by `streaming`.

**Seconds per turn by category** - mean seconds per turn in each
category, stacked, with total turn duration as a line. Category seconds
are divided by the turn count taken from the `unattributed` category's
`_count`, since every category is observed once per turn even when it is
zero. How to read: the stack height is the mean turn duration, so the
line should sit on top of the stack; a gap means the current variable
selection dropped categories.

**Unattributed turn time** - mean unattributed seconds per turn. How to
read: this is the completeness check for the stage model, so treat a
rising line as an instrumentation gap rather than a workload change.

**Category share per turn ($stat)** (full level only) - the selected
`$stat` of each category's share of a turn, from
`coderd_chatd_turn_time_share`. How to read: the mix bar shows where
aggregate time goes, this shows how much a category varies per turn, so
a small mean with a large p99 marks a bursty cost such as a slow tool
call or a retry storm in a minority of turns. Quantiles are per category,
so unlike the mean shares they do not sum to 100%.

### Level 1: Turn children

Members: `acquisition`, `queue_wait`, `capacity_wait` in the visible
row; `generation_step` in the full-level row. The three scheduling waits
happen once per turn before generation starts; `generation_step` repeats
once per step.

`capacity_wait` appears in the duration per occurrence panel only. It is
measured by the acquisition loop before the turn exists, so no turn
records it, and its window lies inside `acquisition`, which the turn
does record under the `scheduling` category.

`capacity_wait` is a lower bound. The refusal history behind it lives
in memory on each replica, while the capacity limit is deployment-wide,
so the wait is measured from the acquiring replica's own first refusal.
A replica that admitted the chat on its first attempt records nothing,
and a restart discards the history. All replicas are woken together
when a chat becomes acquirable, so the undercount is bounded by about
one acquisition interval (30s by default) rather than growing with the
replica count; do not scale the value by replicas. The distortion that
matters is censoring: short waits are the ones most likely to be
missing, which skews the recorded distribution long. For a
deployment-accurate view of time spent waiting on capacity, compare
`acquisition` quantiles while `coderd_chatd_agents_queued_for_capacity`
is above zero against the same quantiles while it is zero.

Because these are the direct children of the turn, their share panel (in
the full-level row) is the quickest answer to "was this turn slow because
of scheduling or because of generation". At `basic`, the `scheduling`
category in the partition row answers the same question in aggregate.

### Level 2: Step children

Members: `retry_backoff`, `stream`, `tool_call`, `commit` in the visible
row; `prepare`, `thinking`, `compaction` in the full-level row.

These are the stages inside one generation step and they overlap each
other, so read them as a profile of the step. `thinking` and `tool_call`
are reconstructed from timestamps after the fact and attach to the step
even though their windows fall inside `stream` and the tool phase.

### Level 3: Prepare and stream children

Members: `mcp_connect` (inside `prepare`), `time_to_first_token` and
`provider_attempt` (inside `stream`). All three are observed at `basic`.

`provider_attempt` is one HTTP round trip to the provider and closes when
response headers arrive; `time_to_first_token` closes on the first
streamed part, so the two overlap almost entirely. An HTTP retry inside
one stream adds a second `provider_attempt` but no second
`time_to_first_token`. `mcp_connect` is not tied to a model call and is
matched without `$model`; the other two carry the label.

### Throughput and TTFT

**Time to first token** - p50/p90/p99/mean of `coderd_chatd_ttft_seconds`,
the pre-existing histogram recorded when the first streamed part
arrives. Dimensions: none of the stage labels; this histogram is
labeled by provider/model internally but the panel aggregates across
them, and `$model` and `$chat_kind` do not apply. The
`time_to_first_token` stage in the profile measures the same interval
and does honor the filters. How to read: the primary user-perceived
responsiveness metric for streaming.

**Background provider calls ($stat)** - rate and selected `$stat`
duration of background-scope `provider_attempt` samples: detached
title/summary/quickgen requests that are excluded from every other
panel. Dimensions: pinned to the background scope of the
`provider_attempt` stage; `$model` and `$chat_kind` are not applied,
because background work runs outside a turn. How to read: this work
costs provider quota and money but no user-facing turn latency; a spike
here with flat turn panels means background load, not a chat regression.

## Setup

1. **Configure a Prometheus data source** that scrapes your coderd
   Prometheus endpoint (`--prometheus-enable`).
2. **Import**: in Grafana navigate to **Dashboards** -> **Import** ->
   **Upload JSON file** with [`dashboard.json`](./dashboard.json), then map
   the Prometheus data source when prompted.
3. **Pick a level**: stage metrics are off by default. Set
   `--chat-stage-metrics=basic` on the coderd replicas to fill every
   expanded row, or `full` to also populate the collapsed rows.

Per-session drill-down is available by exporting coderd traces
(`--trace` with standard OTLP environment variables) to a tracing backend
such as Tempo; each chat turn is a `chat_turn` root span whose children
mirror the stage hierarchy above, with `reasoning_effort`, `tool_name`,
and `generation_action` as span attributes.
