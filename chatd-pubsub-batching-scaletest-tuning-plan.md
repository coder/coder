# chatd Postgres pubsub batching scaletest tuning plan

## Purpose

This document describes how we will choose the default scaletest and rollout
numbers for chatd's Postgres pubsub batching path.

The initial tuning scope is the operator-facing knobs we already agreed to
expose:

- flush interval;
- batch size; and
- queue size.

If we later expose an overload wait budget as a real knob, we can tune that in a
follow-up pass. It is not required to get the first useful defaults.

We are **not** revisiting semantics during this campaign. v1 keeps the
correctness-first behavior we agreed on:

- no semantic deduplication or coalescing;
- no automatic retry on failed flushes; and
- no silent drops.

## What "best possible figures" means

We are not looking for the numerically largest batches or the lowest transaction
count in isolation. We want the config on the best reliability/latency/resource
trade-off frontier.

In priority order, the winning config should:

1. drive chat stream 5xxs and queue-cap publish errors to zero, or as close to
   zero as the workload allows;
2. materially reduce shared DB pool starvation and NOTIFY-lock pressure versus
   unbatched Postgres;
3. keep added cross-replica delivery delay small enough that chat UX does not
   meaningfully regress; and
4. stay stable through short DB stalls or sender reconnects without hiding
   problems behind retries or silent loss.

One explicit anti-goal: do **not** optimize on TTTS alone. Earlier scaletests
already showed that pathological Postgres backpressure can improve TTTS while
making overall reliability much worse.

## Fixed test shape

To make the knob comparisons meaningful, every candidate run in the main tuning
campaign should keep the following fixed unless the document explicitly says
otherwise:

- same git SHA;
- same CloudSQL shape;
- same coderd replica count and node size;
- same `MAX_OPEN` / DB pool settings;
- same fake template / workload generator behavior;
- same LLM mock latency;
- same chat turn mix and tool-call mix; and
- same run duration, warm-up window, and measurement window.

The main tuning workload should be the clean chat workload from the March 31
study, because it removed the extra loadgen noise and the real-template polling
confounder. In practice that means starting with the same 600-workspace,
5,400-chat, 54,000-turn shape that produced the clearest Redis-vs-Postgres
comparison.

After we have finalists, we should run one higher-pressure scenario at roughly
125% to 150% of the main workload to verify overload behavior and recovery.

## Baselines we should always keep

Before tuning the new knobs, we should capture three anchor runs on the same
workload shape:

1. **Unbatched Postgres**: today's control.
2. **Seed batched Postgres**: the first reasonable default guess.
3. **Redis reference**: optional but strongly preferred if operationally easy,
   because it remains the best "lock-free delivery" reference point.

Those anchors keep us honest. The batched Postgres tuning work is successful
only if it clearly beats the unbatched Postgres control on reliability and pool
health, and gets materially closer to the Redis reference where that comparison
is relevant.

## Knobs and initial search space

We should begin with a balanced seed config and then do a coarse-to-fine search.

### Seed config

Start with:

- flush interval: `10ms`;
- batch size: `32`; and
- queue size: `1024`.

Why this is a reasonable first guess:

- a `10ms` timer is short enough to keep added delivery delay modest;
- a batch size of `32` is large enough to absorb ordinary bursts without forcing
  giant latency-first batches; and
- a queue size of `1024` is large enough to absorb transient sender stalls, but
  still bounded enough to expose sustained overload instead of hiding it.

### Coarse sweep ranges

Start with these candidate ranges:

- flush interval: `5ms`, `10ms`, `20ms`, `40ms`;
- batch size: `8`, `16`, `32`, `64`, `128`; and
- queue size: `256`, `512`, `1024`, `2048`.

These are intentionally broad. We can narrow them after the first sweep.

## Metrics we must collect for every run

The tuning campaign depends on both the existing scaletest outputs and the new
batch-specific metrics from the feature work.

### Primary outcome metrics

These decide whether a setting is acceptable at all:

- chat stream 5xx rate;
- publish rejections caused by hitting the batch queue hard cap;
- DB pool wait time / wait count / max-open saturation on coderd;
- blocked sessions or active time attributable to the Postgres NOTIFY path;
- batch flush errors; and
- whether the queue drains cleanly again after transient pressure.

### Secondary outcome metrics

These rank the acceptable candidates against each other:

- chat endpoint latency p50/p95/p99;
- pubsub send latency p50/p95/p99;
- batch queue depth p50/p95/p99 and time spent near the cap;
- flush duration;
- flush size distribution;
- fraction of flushes triggered by timer vs capacity vs pressure relief;
- sender reconnect count / sender-connected state; and
- coderd CPU, memory, and network overhead.

### Informational metrics

These are useful for diagnosis but should not dominate the final decision:

- TTTS;
- chat worker status timing;
- per-query latencies for hot DB lookups; and
- overall database active time.

## Run protocol

Every tuning run should follow the same protocol:

1. bring up a fresh environment or reset the environment to a comparable state;
2. run a warm-up period long enough to reach steady traffic;
3. measure on a fixed steady-state window; and
4. save both raw metrics and a short written summary for the run.

A good default structure is:

- `5m` warm-up;
- `10m` primary measurement window; and
- `5m` post-run observation window for queue drain and cleanup behavior.

If later data shows the system takes longer to stabilize, extend the warm-up and
measurement windows consistently for all subsequent runs.

## Search strategy

We should tune one dimension at a time first, then verify interactions around
the finalists.

### Phase 0: Sanity check the feature

Run the seed config once and verify the basics before spending time on a full
sweep:

- batching is actually enabled for chatd;
- the dedicated sender connection is not borrowing from the shared app pool;
- batched flush metrics are present;
- queue depth stays bounded;
- no ordering or correctness regressions are visible; and
- the run is meaningfully better than unbatched Postgres on obvious NOTIFY-lock
  symptoms.

If this phase fails, fix instrumentation or implementation before tuning.

### Phase 1: Flush interval sweep

Hold batch size and queue size constant at the seed values, then sweep the flush
interval.

Initial interval candidates:

- `5ms`;
- `10ms`;
- `20ms`; and
- `40ms`.

Decision rule for this phase:

- reject any interval that introduces obvious latency regression without a clear
  reliability or pool-health win;
- reject any interval that still shows queue-cap publish errors at the target
  load; and
- among the acceptable candidates, prefer the **shortest** interval that is
  still near the best performer on reliability and pool-health metrics.

That rule deliberately biases us toward lower added latency once reliability is
roughly tied.

### Phase 2: Batch size sweep

Fix the best interval from Phase 1, then sweep batch size.

Initial batch-size candidates:

- `8`;
- `16`;
- `32`;
- `64`; and
- `128`.

Decision rule for this phase:

- look for the knee of the curve, where larger batches stop buying meaningful
  improvements in queue depth, pool wait, or NOTIFY contention;
- reject batch sizes that noticeably worsen endpoint or pubsub latency without a
  compensating reliability benefit; and
- among near-ties, prefer the **smaller** batch size.

This prevents us from choosing an oversized batch just because it produces a
slightly prettier transaction count.

### Phase 3: Queue size sweep

Fix the best interval and batch size, then sweep queue size.

Initial queue-size candidates:

- `256`;
- `512`;
- `1024`; and
- `2048`.

Decision rule for this phase:

- at the main target load, choose the **smallest** queue size that shows zero
  publish rejections and no unstable backlog growth;
- at the overload run, confirm that the queue remains bounded, that overload is
  surfaced explicitly, and that the queue recovers promptly when pressure drops;
- do not choose a large queue just to hide a sender stall for longer.

The queue is there to absorb bursts, not to paper over a sustained throughput
mismatch.

### Phase 4: Interaction check around the finalists

Once each dimension has a provisional winner, run a small local matrix around
that point.

For example, if the provisional winner is `10ms / 32 / 1024`, run nearby
neighbors such as:

- `5ms / 32 / 1024`;
- `20ms / 32 / 1024`;
- `10ms / 16 / 1024`;
- `10ms / 64 / 1024`;
- `10ms / 32 / 512`; and
- `10ms / 32 / 2048`.

This is how we make sure a one-axis sweep did not miss a better nearby point.

### Phase 5: Repeat finalists for variance control

The finalists should not be chosen from one lucky run.

For the top two or three candidate configs:

- rerun each at least twice more on the same workload shape; and
- alternate the run order if practical, so time-of-day or warm-cluster drift
  does not bias one config.

The final chosen config should also get one repeat overload run.

## How we choose the winner

We should treat the final decision as a Pareto-frontier choice, not a single
metric sort.

### Hard reject conditions

Reject any candidate that shows any of the following at the main target load:

- queue-cap publish errors;
- flush failures in steady state;
- materially worse chat stream 5xxs than another nearby candidate;
- obvious endpoint-latency regression without a corresponding reliability win;
- unbounded or very slow queue recovery; or
- evidence that the sender path is still competing with the shared app pool.

### Ranking acceptable candidates

Among the candidates that clear the hard rejects, rank in this order:

1. lower chat stream 5xx rate;
2. lower DB pool wait / saturation and lower NOTIFY-lock pressure;
3. lower queue depth and cleaner recovery after pressure;
4. lower added pubsub / endpoint latency; and
5. smaller, simpler knob values.

That last rule matters. If two configs are effectively tied, pick the one with
smaller batches, smaller queues, and shorter timers.

## What to do with overload results

Overload runs are not there to prove that the system can never fail. They are
there to verify that failure is bounded and explicit.

At overload, we want to see:

- bounded queue depth;
- explicit publish errors when the hard cap is truly hit;
- no silent loss;
- no retry loops masking the failure; and
- fast recovery once load drops back below capacity.

A candidate that looks good at the target load but behaves pathologically in the
overload run should not ship as the default.

## Data capture and reporting

For every run, record:

- git SHA;
- workload shape;
- all batching knob values;
- warm-up and measurement windows;
- environment details that differed from the baseline, if any; and
- a short operator summary of what looked good or bad.

Run names should be structured and easy to compare, for example:

- `pg-batched-i005-b032-q1024-run1`;
- `pg-batched-i020-b032-q1024-run1`; and
- `pg-batched-i010-b064-q0512-run2`.

After the campaign, write a short summary that includes:

1. the winning default values;
2. why they won;
3. which nearby configs lost and why; and
4. whether batching alone fully solved the problem or only moved the bottleneck.

## Exit criteria

We are done tuning when all of the following are true:

- we have a stable winner across repeat runs;
- the winner clearly beats unbatched Postgres on reliability and pool health;
- the winner does not introduce an unacceptable latency regression;
- overload behavior is bounded and explicit; and
- adjacent configs no longer offer a compelling improvement.

If no candidate meets those criteria, we should say so directly and document the
next limiting factor instead of pretending the knob sweep found a production
answer.
