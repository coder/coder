# Design Sketch: Incremental Backfill of AI Model Price Costs

## Problem

When an admin adds pricing for a model that was previously unpriced, historical
token usage rows for that model have `cost_micros IS NULL`. The daily spend
table (`ai_user_daily_spend`) was never incremented for those rows. Spend
reports and budget enforcement are inaccurate until those rows are backfilled.

## Design

An incremental rollup job, modeled on the existing `dbrollup` package, runs
periodically and backfills NULL-cost rows in batches. It is fully automatic:
when an admin adds a price via `coder exp model-prices update`, the next rollup
tick starts backfilling historical usage. No manual command required.

### Rollup job

New package: `coderd/database/dbaipricebackfill` (mirrors `dbrollup`/`dbpurge`).

```go
package dbaipricebackfill

const (
    DefaultInterval = 5 * time.Minute
    BatchSize       = 1000
)

type Backfiller struct {
    cancel   context.CancelFunc
    closed   chan struct{}
    db       database.Store
    logger   slog.Logger
    interval time.Duration
    batch    int
}
```

Wired into the server lifecycle the same way as `dbrollup.New`:

```go
// In coderd/coderd.go Options:
AIPriceBackfiller *dbaipricebackfill.Backfiller

// In New():
if options.AIPriceBackfiller == nil {
    options.AIPriceBackfiller = dbaipricebackfill.New(
        options.Logger.Named("aipricebackfill"),
        options.Database,
    )
}

// In API struct:
aIPriceBackfiller *dbaipricebackfill.Backfiller

// In Close():
api.aIPriceBackfiller.Close()
```

New advisory lock ID in `coderd/database/lock.go`:

```go
LockIDAIPriceBackfill
```

### Per-tick logic

Each tick, in one transaction with the advisory lock:

1. **Snapshot prices + compute cost** for up to `BatchSize` NULL-cost rows
   that now have a matching price.

2. **Backfill daily spend** for past-day rows only (today's spend is deferred
   to tomorrow's rollup — avoids mid-session budget surprises).

The `cost_micros IS NULL` predicate is both the filter and the idempotency
guarantee: rows already backfilled have non-NULL cost and are skipped. Partial
failures roll back and retry next tick.

### SQL queries

Two new queries in `coderd/database/queries/aicostcontrol.sql`:

#### Query 1: Backfill cost snapshots

```sql
-- name: BackfillAIModelPriceCosts :exec
-- Backfills cost_micros and price snapshots for up to @batch_size token usage
-- rows that have NULL cost (model was unpriced at request time) and now have a
-- matching price in ai_model_prices. Mirrors computeCost in cost.go:
-- tokens * price_per_million / 1_000_000, with NULL prices treated as 0.
UPDATE aibridge_token_usages tu
SET
    input_price_micros       = p.input_price,
    output_price_micros      = p.output_price,
    cache_read_price_micros  = p.cache_read_price,
    cache_write_price_micros = p.cache_write_price,
    cost_micros = (
        tu.input_tokens              * COALESCE(p.input_price, 0)
      + tu.output_tokens             * COALESCE(p.output_price, 0)
      + tu.cache_read_input_tokens   * COALESCE(p.cache_read_price, 0)
      + tu.cache_write_input_tokens  * COALESCE(p.cache_write_price, 0)
    ) / 1000000
FROM aibridge_interceptions ai
JOIN ai_model_prices p ON p.provider = ai.provider AND p.model = ai.model
WHERE tu.interception_id = ai.id
  AND tu.cost_micros IS NULL
LIMIT @batch_size;
```

#### Query 2: Backfill daily spend (past days only)

```sql
-- name: BackfillAIDailySpend :exec
-- Aggregates newly-backfilled costs into ai_user_daily_spend for past days
-- only. Today's spend is deferred to the next day's rollup to avoid
-- mid-session budget enforcement surprises. Uses the same ON CONFLICT
-- accumulation pattern as IncrementUserAIDailySpend.
INSERT INTO ai_user_daily_spend (user_id, effective_group_id, day, spend_micros)
SELECT
    ai.initiator_id,
    tu.effective_group_id,
    ((tu.created_at AT TIME ZONE 'UTC')::date) AS day,
    SUM(tu.cost_micros) AS spend_micros
FROM aibridge_token_usages tu
JOIN aibridge_interceptions ai ON ai.id = tu.interception_id
WHERE tu.cost_micros IS NOT NULL
  AND tu.effective_group_id IS NOT NULL
  AND ((tu.created_at AT TIME ZONE 'UTC')::date) < ((NOW() AT TIME ZONE 'UTC')::date)
  AND NOT EXISTS (
      SELECT 1 FROM ai_user_daily_spend ds
      WHERE ds.user_id = ai.initiator_id
        AND ds.effective_group_id = tu.effective_group_id
        AND ds.day = ((tu.created_at AT TIME ZONE 'UTC')::date)
        AND ds.spend_backfilled = true
  )
GROUP BY ai.initiator_id, tu.effective_group_id, day
ON CONFLICT (user_id, effective_group_id, day) DO UPDATE SET
    spend_micros = ai_user_daily_spend.spend_micros + EXCLUDED.spend_micros;
```

Wait — the `NOT EXISTS` + `spend_backfilled` marker approach requires a new
column on `ai_user_daily_spend`, which complicates things. A simpler approach:
track backfill progress via a separate marker or rely on the fact that
`BackfillAIModelPriceCosts` already sets `cost_micros` to non-NULL, so the
daily spend query only needs to find rows where cost was backfilled *and*
spend hasn't been accumulated yet.

Alternative: add a `cost_backfilled_at TIMESTAMPTZ` column to
`aibridge_token_usages` that is set when the cost is backfilled (not when it
was originally computed). The daily spend query then filters on
`cost_backfilled_at IS NOT NULL` to find rows that need spend accumulation,
and a separate query marks them as spend-accumulated. But this adds two
columns.

Simplest approach: **two-phase within the same transaction**. Phase 1 computes
costs. Phase 2 accumulates spend for the rows that were just updated (same
transaction, so the cost_micros is now non-NULL). The trick is identifying
*which* rows were just updated. Use a CTE:

```sql
-- name: BackfillAIModelPriceCostsAndSpend :exec
-- Single-transaction backfill: compute costs for NULL-cost rows, then
-- accumulate past-day spend for the rows that were just backfilled.
-- Today's spend is deferred to avoid mid-session budget surprises.
WITH backfilled AS (
    UPDATE aibridge_token_usages tu
    SET
        input_price_micros       = p.input_price,
        output_price_micros      = p.output_price,
        cache_read_price_micros  = p.cache_read_price,
        cache_write_price_micros = p.cache_write_price,
        cost_micros = (
            tu.input_tokens              * COALESCE(p.input_price, 0)
          + tu.output_tokens             * COALESCE(p.output_price, 0)
          + tu.cache_read_input_tokens   * COALESCE(p.cache_read_price, 0)
          + tu.cache_write_input_tokens  * COALESCE(p.cache_write_price, 0)
        ) / 1000000
    FROM aibridge_interceptions ai
    JOIN ai_model_prices p ON p.provider = ai.provider AND p.model = ai.model
    WHERE tu.interception_id = ai.id
      AND tu.cost_micros IS NULL
    LIMIT @batch_size
    RETURNING tu.id, tu.interception_id, tu.effective_group_id,
              tu.cost_micros, tu.created_at
),
past_day AS (
    SELECT
        ai.initiator_id,
        b.effective_group_id,
        ((b.created_at AT TIME ZONE 'UTC')::date) AS day,
        SUM(b.cost_micros) AS spend_micros
    FROM backfilled b
    JOIN aibridge_interceptions ai ON ai.id = b.interception_id
    WHERE b.effective_group_id IS NOT NULL
      AND ((b.created_at AT TIME ZONE 'UTC')::date) < ((NOW() AT TIME ZONE 'UTC')::date)
    GROUP BY ai.initiator_id, b.effective_group_id, day
)
INSERT INTO ai_user_daily_spend (user_id, effective_group_id, day, spend_micros)
SELECT initiator_id, effective_group_id, day, spend_micros
FROM past_day
ON CONFLICT (user_id, effective_group_id, day) DO UPDATE SET
    spend_micros = ai_user_daily_spend.spend_micros + EXCLUDED.spend_micros;
```

This is the recommended approach: single CTE-based query, one transaction,
idempotent (NULL-cost rows are the resume marker), past-day-only spend
accumulation.

### Partial index for performance

```sql
CREATE INDEX idx_aibridge_token_usages_null_cost
    ON aibridge_token_usages (id)
    WHERE cost_micros IS NULL;
```

Keeps repeated scans cheap as backfilled rows drop out of the index.

### Rollup tick structure

```go
func (b *Backfiller) start(ctx context.Context) {
    defer close(b.closed)

    do := func() {
        err := b.db.InTx(func(tx database.Store) error {
            ok, err := tx.TryAcquireLock(ctx, database.LockIDAIPriceBackfill)
            if err != nil {
                return err
            }
            if !ok {
                return nil
            }
            return tx.BackfillAIModelPriceCostsAndSpend(ctx, int64(b.batch))
        }, database.DefaultTXOptions().WithID("ai_price_backfill"))
        if err != nil && !database.IsQueryCanceledError(err) && ctx.Err() == nil {
            b.logger.Error(ctx, "failed to backfill AI model price costs", slog.Error(err))
        }
    }

    // Same ticker pattern as dbrollup: immediate run, then on interval.
    // ...
}
```

### Effective group resolution

Rows where `effective_group_id` is also NULL (no group was resolved at request
time) get their cost snapshot backfilled but no daily spend. This is correct —
those rows can't be attributed to a group for budget purposes, and they're
also excluded from `ExportOrganizationAISpend` (which joins on
`groups.id = tu.effective_group_id`).

Resolving `effective_group_id` retroactively would require calling
`budget.ResolveUserEffectiveGroup` with *current* group memberships, which may
differ from the memberships at request time. This is out of scope — the
backfill only fills in cost for rows that already have an `effective_group_id`.

### What this does NOT do

- **Does not fire budget threshold detection.** The rollup writes spend but
  does not call `detectBudgetThresholdCrossings`. Threshold notifications are
  for live spend events, not historical corrections. The budget check on the
  user's next live request naturally includes the backfilled past-day spend.
- **Does not backfill today's spend.** Today's unpriced usage gets its cost
  snapshot backfilled (for reporting accuracy) but daily spend accumulation is
  deferred until tomorrow's rollup. This prevents a user mid-session from
  suddenly hitting a budget limit due to retroactive spend from today.
- **Does not resolve `effective_group_id` for rows where it's NULL.** Those
  rows get cost snapshots only.
- **Does not handle the case where a model's price changes** (only handles
  models that were previously unpriced — `cost_micros IS NULL`). Price
  *changes* (where `cost_micros` is already non-NULL) are not backfilled;
  the old price snapshot is the historical record.

### Edge cases

- **Rows whose model never gets a price:** stay NULL forever. The rollup
  no-ops on them each tick. The partial index keeps this cheap.
- **Price changes after initial backfill:** if an admin updates a model's
  price after it was already backfilled, the backfilled rows keep their
  original (now-stale) price snapshot. Only *new* requests use the updated
  price. This matches the point-in-time snapshot design of the cost columns.
- **Server restart:** the rollup resumes on next tick. No state to recover —
  `cost_micros IS NULL` is the resume marker.
- **HA deployment:** the advisory lock ensures only one instance runs the
  backfill at a time.

## Files to create/modify (sketch — not implemented)

| File | Action |
|------|--------|
| `coderd/database/lock.go` | Add `LockIDAIPriceBackfill` |
| `coderd/database/queries/aicostcontrol.sql` | Add `BackfillAIModelPriceCostsAndSpend` query |
| `coderd/database/queries.sql.go` | Regenerate (`make gen/db`) |
| `coderd/database/querier.go` | Regenerate |
| `coderd/database/dbauthz/dbauthz.go` | Regenerate |
| `coderd/database/migrations/000NNN_ai_token_usage_null_cost_index.up.sql` | Create partial index |
| `coderd/database/dbaipricebackfill/dbaipricebackfill.go` | Create rollup job |
| `coderd/database/dbaipricebackfill/dbaipricebackfill_test.go` | Create tests |
| `coderd/coderd.go` | Wire into server lifecycle |
