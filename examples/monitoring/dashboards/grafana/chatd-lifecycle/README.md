# Chatd Chat Lifecycle Grafana Dashboard

A Grafana dashboard for diagnosing where time goes in Coder Agents chat
sessions. It plots the stage histogram
`coderd_chatd_stage_duration_seconds{stage,scope,chat_kind}`
and the turn-end rollups one tree level at a time with a selectable
summary statistic (mean, p50, p90, p95, p99), plus summary panels for the
whole chat pipeline, a per-turn partition of turn wall time, and a
per-model view of the provider-bound stages from
`coderd_chatd_model_stage_duration_seconds{stage,provider_type,chat_kind,model}`.

Stage hierarchy, by tree level. `[basic]` marks the stages observed at
the default `--chat-stage-metrics=basic`; the rest are span-only until
the level is `full` (see [Metric levels](#metric-levels)):

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
L3      │   ├── time_to_first_token   [basic] request open -> first streamed part; a failed attempt is span-only
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

| Level             | Families                                                                                                                                                                                  | Stages on `stage_duration_seconds` | Stages on `model_stage_duration_seconds`            |
|-------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------------------------|-----------------------------------------------------|
| `off`             | `coderd_chatd_stage_metrics_level` only                                                                                                                                                   | none                               | none                                                |
| `basic` (default) | `stage_duration_seconds` and `model_stage_duration_seconds` (12 buckets), `turn_time_seconds_total`, `turns_total`, `turn_outcomes_total`, `stage_anomalies_total`, `stage_metrics_level` | the `[basic]` stages above         | `time_to_first_token`, `stream`, `provider_attempt` |
| `full`            | basic with 16 buckets on both stage histograms, plus `turn_time_seconds`, `turn_time_share`, `turn_stage_seconds`                                                                         | all 15                             | basic plus `thinking` and `compaction`              |

The label schema is the same at every level, so a query written for
`full` returns "No data" rather than an error at `basic`. The dashboard
is laid out accordingly: every row that is expanded by default works at
`basic`; rows whose title ends in `(full level only)` are collapsed and
hold the panels that need the `full` families. The stat panel at the top
shows how many replicas report each level on
`coderd_chatd_stage_metrics_level`. The gauge has one series per level
and only the configured one reads 1, so a replica restarted at a new
level overwrites its old series on the next scrape; the panel counts
the series equal to 1 per level.

Only `model_stage_duration_seconds` carries the `model` label, so the
series count scales with the number of models only for the stages
where the model explains the duration. No family carries an
organization label. Worst case, when every stage occurs in every
tuple, per replica:

| Level   | Per `chat_kind`                                                                               | Per `(chat_kind, model)` |
|---------|-----------------------------------------------------------------------------------------------|--------------------------|
| `basic` | ~180 (11 stages x 15 series, plus 16 partition counters)                                      | 45 (3 stages x 15)       |
| `full`  | ~900 (15 stages x 19, plus the three per-turn distributions over 13 stages and 11 categories) | 95 (5 stages x 19)       |

A histogram series set is its bucket edges plus `+Inf`, `_sum`, and
`_count`. The counts are for `scope="turn"`; background
`provider_attempt` adds one more set per chat kind and per model, and
title generation adds a tuple with an empty chat kind. For scale, a
coderd replica exposes roughly 3,400 `coderd_*` series on its own
before chat instrumentation; a deployment using both chat kinds and
five models adds about 800 series per replica at `basic` and about
2,750 at `full`. Series only appear for tuples that see traffic.

### Alerting without the dashboard

Alerts only need `coderd_chatd_stage_duration_seconds` for the stages in
question, or `coderd_chatd_model_stage_duration_seconds` for a per-model
threshold on a provider-bound stage. The buckets are round numbers (`0.1 0.25 0.5 1 2.5 5 10 30 60
300 1800 3600` at `basic`; `full` adds `0.05 20 120 600`), so a
threshold that matches a bucket edge is exact rather than interpolated
at either level:

```yaml
groups:
  - name: chatd-latency
    rules:
      # More than 10% of first tokens took over 10s in the last 10m,
      # with enough samples to mean something. Per model, so this reads
      # the model histogram; the same expression on
      # stage_duration_seconds gives the deployment-wide figure.
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

Prometheus 3 stores classic histogram `le` values in float form
(`10.0`), so exact-edge matchers need the regex above; `histogram_quantile`
is unaffected.

The `time_to_first_token` stage histogram records only windows a first
content part closed. A failed attempt ends the span with the error, is
not observed, and its time is categorized as `provider_error` in the
turn partition. A provider that hangs without streaming and without
failing does not show up here either; pair it with `stream`, which is
observed regardless, or with `provider_attempt` errors in traces.
Saturation paging belongs to the `coderd_chatd_agents_queued_for_capacity`
gauge, not to `capacity_wait`, which is recorded only after the wait
ends.

An operator who wants only these alerts can drop the other stages at
scrape time. The rules below are drop-only: a `keep` rule on the stage
family would drop every other series on the target, including the
non-chatd metrics coderd exposes. What survives is
`stage_duration_seconds` for `chat_turn`, `time_to_first_token`,
`mcp_connect`, and `capacity_wait`, `model_stage_duration_seconds` for
`time_to_first_token`, plus `stage_anomalies_total`,
`turn_outcomes_total`, the level gauge, and every metric outside chatd.
`chat_turn` is kept so the Turn duration and Turns per minute panels
keep working (the chat kind variable is sourced from any turn-scoped
stage), `time_to_first_token` on the model histogram because the model
variable is sourced from it, and `capacity_wait` so an alert on
capacity wait time can be written against it. With all three rules in
place the panels left standing are Turn duration, Turns per minute,
Turn outcomes, the Level 1 and Level 3 duration panels minus
`provider_attempt`, and Time to first token; the Turn time partition
row, Turn time mix, Level 2, the rest of the Model row, Background
provider calls, and the collapsed rows are empty.

```yaml
metric_relabel_configs:
  - source_labels: [__name__, stage]
    regex: 'coderd_chatd_stage_duration_seconds.*;(acquisition|queue_wait|stream|provider_attempt|tool_call|commit|retry_backoff|generation_step|prepare|thinking|compaction)'
    action: drop
  - source_labels: [__name__, stage]
    regex: 'coderd_chatd_model_stage_duration_seconds.*;(stream|provider_attempt|thinking|compaction)'
    action: drop
  - source_labels: [__name__]
    regex: 'coderd_chatd_(turn_time_.*|turns_total|turn_stage_.*)'
    action: drop
```

## Dimensions

The stage histogram carries three labels and the model histogram four,
exposed as dashboard variables where noted:

| Label           | Families                       | Values                                                                                                  | Dashboard variable          |
|-----------------|--------------------------------|---------------------------------------------------------------------------------------------------------|-----------------------------|
| `stage`         | both                           | the 15 stage names above; five of them on the model histogram                                           | none (fixed hierarchy)      |
| `scope`         | `stage_duration_seconds`       | `turn` (part of a chat turn) or `background` (detached async work such as title and summary generation) | none (panels pin one scope) |
| `chat_kind`     | both, and the turn families    | `root` (a chat a user drives) or `subagent` (a chat spawned by a parent agent)                          | `$chat_kind` (multi-select) |
| `provider_type` | `model_stage_duration_seconds` | configured type of the model's AI provider (`anthropic`, `bedrock`, `azure`, ...)                       | none (panels group by it)   |
| `model`         | `model_stage_duration_seconds` | the model config's model ID                                                                             | `$model` (multi-select)     |

`provider_type` is the same value the AI Gateway metrics report under
that label. The pre-existing `coderd_chatd_*{provider,model}` families
report the wire protocol the client speaks as `provider`, which is
`anthropic` or `openai` for Bedrock and `openai-compat` for the
OpenAI-compatible provider types, so do not join the two by provider.

No chatd metric carries an organization label; the organization is an
attribute on every stage span (`organization_name`) for trace queries.
Title generation runs without a chat: its background `provider_attempt`
carries an empty `chat_kind`. Summary and status-label generation run
for a chat and carry that chat's kind.

Two more variables apply everywhere: `$datasource` selects the Prometheus
data source and `$stat` selects the summary statistic (mean, p50, p90,
p95, p99) for every stat-aware panel.

Reasoning effort is deliberately not a metric label: it multiplies series
per model while changing the behavior of only the model-call stages. It
is a span attribute (`reasoning_effort`) on every stage span that has
resolved a model, so
per-effort analysis is a trace query (for example
`{ name = "time_to_first_token" && span.reasoning_effort = "high" }` in
Tempo). The attribute appears when the model config declares a
`reasoning_effort` block; a model config without one produces spans
without the attribute, whatever the user selected.

`chat_kind` separates root chats, which a user drives, from subagent
chats, which a parent agent spawns and which run as separate chats with
their own turn trees whose wall time overlaps the parent's `tool_call`.
It is a property of the turn, so every turn-scoped stage carries it,
including the stages recorded before a model is resolved. `$chat_kind`
therefore filters every stage panel in full. Background provider
calls carry the kind of the chat that spawned them and are pinned to
`scope="background"` in their own panel.

`model` is a label only on `coderd_chatd_model_stage_duration_seconds`,
which observes the stages whose duration is the provider's work on a
model: `time_to_first_token`, `stream`, `provider_attempt`, and at
`full` also `thinking` and `compaction`. Every sample there is also a
sample on `stage_duration_seconds`, so the stage panels are complete
without it and `$model` applies only to the Model row. A turn is not a
model-scoped unit (subagents, compaction summaries, and model switches
can run inside one), so the turn-end families
(`coderd_chatd_turn_time_seconds_total`, `coderd_chatd_turns_total`,
`coderd_chatd_turn_outcomes_total`, `coderd_chatd_turn_stage_seconds`,
`coderd_chatd_turn_time_seconds`, `coderd_chatd_turn_time_share`) carry
`chat_kind` only. The first model a turn resolves is an attribute on
its `chat_turn` span.

## Panels

### Reading the levels

The stage tree mixes two units of observation. `chat_turn` is recorded
once per turn, while its descendants are recorded once per occurrence,
and a step-level stage such as `stream` or `tool_call` typically occurs
six to twelve times in a turn. Plotting both on one axis compares a
25-second turn against a 2-second stream and tells you nothing, so the
trend panels are grouped by tree level and every level can be read two
ways:

| Panel                   | Metric                                | Question                             | Level |
|-------------------------|---------------------------------------|--------------------------------------|-------|
| Duration per occurrence | `coderd_chatd_stage_duration_seconds` | how long does one of these take      | basic |
| Seconds per turn        | `coderd_chatd_turn_stage_seconds`     | how much of a turn does it add up to | full  |

The first is per occurrence, the second is per turn, recorded when the
turn ends. Seconds per turn divided by duration per occurrence is
roughly how often the stage happens in a turn, so the two together
separate "each one is slow" from "it happens too often"; the exact
occurrence count for one turn is in its trace. At `basic` only the
first is populated, and the Turn time partition row is the per-turn
view; Seconds per turn lives in each level's collapsed `(full level
only)` row. Stages still overlap within a level, so seconds at one
level do not sum to the turn; the Turn time partition row is the view
that does add up.

### Level 0: Turn

Members: `chat_turn`.

**Turn duration (`$stat`)** - wall time of a whole turn, split by chat
kind. This is the denominator every other level is measured against.
For a per-model view of latency, use the Model row: a turn can span
several models, so it is not split by one.

**Turns per minute** - closed turns per minute, split by chat kind.
`chat_turn` is observed on every close, so every closed turn is counted
whatever its outcome. Read it beside turn duration: duration moving
with flat throughput is a latency regression, both moving together is
usually a workload change.

**Turn outcomes** - closed turns per minute by outcome from
`coderd_chatd_turn_outcomes_total`, stacked. Every closed turn is
counted exactly once: `completed` turns finished normally and are the
ones whose time partition is recorded; `interrupted` turns were cut off
by a cancellation; `error` turns stopped on any other failure;
`abandoned` turns were closed before they finished without a failure or
cancellation recorded against them. The stacked total equals Turns per
minute. Only `completed` turns appear in the partition and in
`turns_total`, so the other three together are the turns the rest of
this row does not see. Dimensions: `$chat_kind` applies.

**Turn time mix by chat kind** - the exclusive category partition of a
turn per chat kind, described in the next section.

### Level 0: Turn time partition

The stage hierarchy overlaps in wall time, so it can tell you which
stages are slow but not how a turn's seconds divide up. The turn-end
metrics answer that with an exclusive partition of turn wall time,
observed once per turn:

| Category              | Turn time spent                                                                                         |
|-----------------------|---------------------------------------------------------------------------------------------------------|
| `scheduling`          | queueing, capacity admission and worker pickup                                                          |
| `time_to_first_token` | provider request open until the first streamed part, when one arrived                                   |
| `streaming`           | first part until the stream closes                                                                      |
| `tool_execution`      | local tool calls                                                                                        |
| `provider_error`      | attempts that ended in a provider error, including the `time_to_first_token` window of a failed attempt |
| `retry_backoff`       | waiting between provider attempts                                                                       |
| `compaction`          | auxiliary compaction calls                                                                              |
| `preparation`         | prompt build, model resolution and MCP connects                                                         |
| `persistence`         | the step commit transaction                                                                             |
| `chatd_overhead`      | the step's own work outside every other stage                                                           |
| `unattributed`        | turn time no category claimed                                                                           |

The categories are exclusive and sum to the turn, so these panels do add
up, unlike the stage panels. `unattributed` is the completeness check:
if it grows, real turn time is happening outside every instrumented
stage. The opposite failure, categories that sum to more than the turn,
is emitted as measured with zero `unattributed` and counted in
`coderd_chatd_stage_anomalies_total{reason="overattributed"}`; that
counter also records stage observations dropped for inverted clocks,
turns dropped for a non-positive duration, and turn anchors clamped to
the previous turn's anchor (`stale_anchor`).

The partition is computed once per turn from the turn's own stages, so
it is exact per turn. The dashboard has no panel that divides aggregate
stage seconds by aggregate `chat_turn` counts: the two observations for
one turn land in different scrapes, and such a ratio mixes turns and can
read well past 100% while long turns are in flight.

The category partition is available at `basic` because `prepare`,
`commit`, `queue_wait`, and the other stages feed their categories
regardless of whether the stage itself is observed on the histogram.

**Turn time mix by chat kind** - shown in the Level 0 row above, one
100%-stacked bar per chat kind, each category's total seconds over the
range divided by all categories' total seconds. Dimensions: `$chat_kind`
applies; the grouping is fixed to `chat_kind`, because
Grafana transformation options do not interpolate dashboard variables
and the matrix transform needs a static row field. How to read: the
fastest way to compare where root and subagent turns spend their time,
for example subagents dominated by `tool_execution` against root turns
dominated by `streaming`.

**Seconds per turn by category** - mean seconds per turn in each
category, stacked, with total turn duration as a line. Category seconds
from `coderd_chatd_turn_time_seconds_total` are divided by the turn
count from `coderd_chatd_turns_total`. How to read: the stack height is the mean turn duration, so the
line should sit on top of the stack; a gap means the current variable
selection dropped categories.

**Unattributed turn time** - mean unattributed seconds per turn. How to
read: this is the completeness check for the stage model, so treat a
rising line as an instrumentation gap rather than a workload change.

**Category share per turn (`$stat`)** (full level only) - the selected
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
is above zero against the same quantiles while it is zero. The limits
themselves are fixed at 5 concurrent root chats and 10 subagent chats;
an enterprise `agent_runtime_hours` entitlement removes them, so
`capacity_wait` is never recorded on entitled deployments.

Because these are the direct children of the turn, their Seconds per
turn panel (in the full-level row) against Turn duration is the
quickest answer to "was this turn slow because of scheduling or because
of generation". At `basic`, the `scheduling` category in the partition
row answers the same question in aggregate.

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
`time_to_first_token`. `time_to_first_token` and `provider_attempt` are
also observed on the model histogram, which the Model row reads;
`mcp_connect` is not a model call and appears only here.

`mcp_connect` populates only when an MCP server is configured for the
chat; a chat without MCP servers records no sample. It is one sample per
generation step covering the connects to every configured server, and
it is never errored. Servers on loopback or private addresses are
refused unless their ranges are listed in `--mcp-allowed-private-cidrs`;
a refused address shortens the sample rather than failing it.

### Model

Three panels read `coderd_chatd_model_stage_duration_seconds`, the
one stage family labeled by `provider_type` and `model`, and take
`$chat_kind` and `$model`; the fourth, Background provider calls, reads
`stage_duration_seconds` and takes `$chat_kind` only. The pre-existing
`coderd_chatd_ttft_seconds{provider,model}` histogram measures the same
first-token interval without the chat kind and is not used by the
dashboard.

**Time to first token** - p50/p90/p99/mean of the `time_to_first_token`
stage across the selected models: request open to the first streamed
part, for windows a first part closed. How to read: the primary
user-perceived responsiveness metric for streaming; split it by model
with the next panel's pattern if one model is suspected.

**Stream duration by model (`$stat`)** - the selected `$stat` of the
`stream` stage per model: provider stream open to close, including
time to first token. How to read: the per-model latency view; a slow
model shows here without being blended into Turn duration.

**Provider attempts per minute by provider type and model** - rate of
`provider_attempt` samples. One attempt is one HTTP request to the
provider, so a rate above the stream rate is retries. How to read:
paired with `retry_backoff` in Level 2, this is the provider health
view per model.

**Background provider calls (`$stat`)** - rate and selected `$stat`
duration of background-scope `provider_attempt` samples on
`stage_duration_seconds`: detached title, summary, and status-label
requests that are excluded from every other panel. Dimensions: pinned
to the background scope of the `provider_attempt` stage; `$chat_kind`
applies, `$model` does not. Summary and status-label generation carry
the kind of the chat they ran for; title generation runs without a
chat and carries an empty `chat_kind`, so it is shown while the
variable is set to All and drops out once a kind is selected. How to
read: this
work costs provider quota and money but no user-facing turn latency; a
spike here with flat turn panels means background load, not a chat
regression.

## Setup

1. **Configure a Prometheus data source** that scrapes your coderd
   Prometheus endpoint (`--prometheus-enable`).
2. **Import**: in Grafana navigate to **Dashboards** -> **Import** ->
   **Upload JSON file** with [`dashboard.json`](./dashboard.json), then map
   the Prometheus data source when prompted.
3. **Pick a level**: the default `--chat-stage-metrics=basic` fills every
   expanded row. Set `full` on the coderd replicas to populate the
   collapsed rows.

Per-session drill-down is available by exporting coderd traces
(`--trace` with standard OTLP environment variables) to a tracing backend
such as Tempo; each chat turn is a `chat_turn` root span whose children
mirror the stage hierarchy above, with `reasoning_effort`, `tool_name`,
and `generation_action` as span attributes.

`--trace` exports over OTLP gRPC, so point the exporter at the gRPC
port, for example `OTEL_EXPORTER_OTLP_ENDPOINT=http://tempo:4317`. The
value needs a scheme: a scheme-less endpoint such as `tempo:4317` is
accepted and exports nothing, silently.

A turn trace carries several hundred `DB QUERY` and AI Bridge
`Intercept` spans around a few dozen stage spans. In Tempo, filter to
the stages with `{ span.scope = "turn" }` in the search, or use the span
filter in the trace view, to read the stage tree without the noise.
