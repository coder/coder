# Smart prebuild autoscaling expressions

Smart prebuild autoscaling lets a preset adjust its prebuild target from recent demand signals instead of relying only on a fixed count or cron schedules. Configure a preset's `desired_instances_expression` to compute the target prebuild count on each reconciliation cycle.

## Overview

Use autoscaling expressions when you want prebuild capacity to respond to real usage, such as bursts of successful claims, rising claim misses, or business-hour traffic patterns. Expressions build on top of the existing prebuild controls:

- **Static counts** still define a baseline pool.
- **Cron schedules** can still raise or lower that baseline by time of day.
- **Expressions** can reference the baseline as `scheduled_target` and then adjust it with live demand signals.

If no expression is configured, prebuilds keep the existing static and schedule-based behavior.

## How it works

Coder evaluates the preset's `desired_instances_expression` during each prebuild reconciliation cycle. The reconciliation interval is controlled by `CODER_WORKSPACE_PREBUILDS_RECONCILIATION_INTERVAL` and defaults to `1m`.

Expressions use the [Expr](https://expr-lang.org/) language. The result must resolve to an integer target. When you scale from floating-point values such as `claim_rate_30m`, use helpers like `ceil()` or `floor()` to convert the result into an integer.

The evaluator always derives `hour` and `weekday` from the preset's scheduling timezone. If the preset does not have a valid scheduling timezone, expression evaluation falls back to `scheduled_target`.

## Available variables

### Baseline and current state

| Variable           | Description                                                                              |
|--------------------|------------------------------------------------------------------------------------------|
| `scheduled_target` | The baseline target from the preset's static prebuild count or the active cron schedule. |
| `running`          | Currently running prebuilds that have not expired.                                       |
| `eligible`         | Running prebuilds that are ready to be claimed.                                          |
| `starting`         | Prebuilds that are currently being created.                                              |
| `stopping`         | Prebuilds that are currently being stopped.                                              |
| `deleting`         | Prebuilds that are currently being deleted.                                              |
| `expired`          | Running prebuilds that have exceeded their TTL.                                          |

### Demand signals

| Variable          | Description                                                                                                   |
|-------------------|---------------------------------------------------------------------------------------------------------------|
| `claims_5m`       | Successful prebuild claims in the last 5 minutes.                                                             |
| `claims_10m`      | Successful prebuild claims in the last 10 minutes.                                                            |
| `claims_30m`      | Successful prebuild claims in the last 30 minutes.                                                            |
| `claims_60m`      | Successful prebuild claims in the last 60 minutes.                                                            |
| `claims_120m`     | Successful prebuild claims in the last 120 minutes.                                                           |
| `misses_5m`       | Failed claim attempts in the last 5 minutes, which indicates demand that could not be served by a prebuild.   |
| `misses_10m`      | Failed claim attempts in the last 10 minutes, which indicates demand that could not be served by a prebuild.  |
| `misses_30m`      | Failed claim attempts in the last 30 minutes, which indicates demand that could not be served by a prebuild.  |
| `misses_60m`      | Failed claim attempts in the last 60 minutes, which indicates demand that could not be served by a prebuild.  |
| `misses_120m`     | Failed claim attempts in the last 120 minutes, which indicates demand that could not be served by a prebuild. |
| `claim_rate_5m`   | Successful claims per minute over the last 5 minutes.                                                         |
| `claim_rate_10m`  | Successful claims per minute over the last 10 minutes.                                                        |
| `claim_rate_30m`  | Successful claims per minute over the last 30 minutes.                                                        |
| `claim_rate_60m`  | Successful claims per minute over the last 60 minutes.                                                        |
| `claim_rate_120m` | Successful claims per minute over the last 120 minutes.                                                       |

### Time-based variables

| Variable  | Description                                                                                       |
|-----------|---------------------------------------------------------------------------------------------------|
| `hour`    | Current hour from `0` to `23` in the preset's scheduling timezone.                                |
| `weekday` | Current day of week in the preset's scheduling timezone, where `0` is Sunday and `6` is Saturday. |

## Built-in functions

The most useful numeric helpers for prebuild autoscaling are:

| Function                 | Description                                                                        |
|--------------------------|------------------------------------------------------------------------------------|
| `min(a, b)`              | Returns the smaller of two values.                                                 |
| `max(a, b)`              | Returns the larger of two values.                                                  |
| `ceil(x)`                | Rounds a number up to the nearest integer. Useful with `claim_rate_*` variables.   |
| `floor(x)`               | Rounds a number down to the nearest integer. Useful with `claim_rate_*` variables. |
| `abs(x)`                 | Returns the absolute value of a number.                                            |
| `clamp(value, min, max)` | Constrains `value` to the inclusive `[min, max]` range.                            |

You can also use normal arithmetic operators such as `+`, `-`, `*`, `/`, and parentheses.

## Expression examples

| Expression                                        | What it does                                                                                                                                                        |
|---------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `min(claims_10m * 10, 20)`                        | Scale to 10 times the number of successful claims in the last 10 minutes, capped at 20 prebuilds.                                                                   |
| `max(scheduled_target, ceil(claim_rate_30m * 8))` | Keep at least the baseline target, or scale up from the 30-minute claim rate. This pattern works well when schedules define a floor and demand adds burst capacity. |
| `max(2, min((claims_60m + misses_60m) * 2, 30))`  | Keep at least 2 prebuilds ready, scale from total recent demand, and cap the result at 30.                                                                          |
| `clamp(claims_10m * 5, 1, 15)`                    | Scale linearly from recent claims while keeping the result between 1 and 15.                                                                                        |

## Precedence rules

Target resolution follows these rules:

1. If `desired_instances_expression` is set, Coder evaluates the expression and uses the result as the target.
1. If no expression is set and a static prebuild count is configured, Coder uses the static count or active schedule target.
1. If both are set, the expression takes precedence and the static-or-scheduled baseline remains available as `scheduled_target`.
1. If neither a prebuild count nor an expression is configured, Coder does not maintain prebuilds for that preset.

## Failure behavior and safety ceiling

Coder validates expression syntax and variable names when a template version is imported. Invalid expressions are logged as warnings during import so they can be fixed before rollout.

If expression evaluation fails at runtime, Coder falls back to `scheduled_target` and logs the failure. This includes environment setup errors, validation failures, and evaluation errors.

After a successful evaluation, Coder clamps the final target to the inclusive range `[0, 100]`. Negative results become `0`, and values above `100` are capped at `100`.

## Cold start considerations

All rolling-window demand variables start at `0` when a preset has no recent claim history. That means a pure demand-based expression can resolve to `0` immediately after you publish a template version or after long idle periods.

To avoid an empty prebuild pool on cold start, either:

- Configure a static baseline prebuild count.
- Use `scheduled_target` in the expression, such as `max(scheduled_target, ceil(claim_rate_30m * 8))`.
- Add an explicit minimum, such as `max(2, min((claims_60m + misses_60m) * 2, 30))`.

## Debugging

To debug autoscaling behavior, search for `coderd.prebuilds:` log entries and inspect the reconciler's `calculated reconciliation state for preset` debug logs.

The most useful structured log fields are:

- `target_source` — Whether the final target came from the schedule baseline (`scheduled`), the expression (`expression`), or a fallback to the schedule baseline after an error (`expression_fallback`).
- `scheduled_target` — The baseline target before expression overrides are applied.
- `expression_present` — Whether the preset has a non-empty expression stored in the database.
- `expression_active` — Whether the reconciler successfully used the expression to set the desired target.
- `expression_error` — The validation, environment, or evaluation error that caused a fallback.

Related documentation:

- Learn how to configure prebuild pools and cron schedules in [Prebuilt workspaces](./prebuilt-workspaces.md).
