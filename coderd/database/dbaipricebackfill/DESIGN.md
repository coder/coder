# Design Sketch: `valid_from` on AI Model Prices

## Problem

When an admin adds pricing for a model that was previously unpriced, historical
token usage rows have `cost_micros = NULL`. Without a validity period, the
system either (a) leaves them NULL forever (understated spend), or (b) requires
a backfill rollup job to retroactively compute and accumulate spend —
introducing complexity around idempotency, past-day-only spend, budget
threshold suppression, and partial indexes.

A `valid_from` column dissolves the problem at the schema level. The price is
valid from a point in time. Past usage that predates the price is correctly
NULL — the price didn't exist yet. The admin can optionally choose retroactive
validity with `--valid-from <past>`, accepting the consequences.

## Design

### Schema change

Add `valid_from` to `ai_model_prices` and change the primary key:

```sql
-- Migration: 000NNN_ai_model_prices_valid_from.up.sql
ALTER TABLE ai_model_prices
    ADD COLUMN valid_from TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Drop the old PK and create a new one that allows multiple price rows
-- per (provider, model) — one per validity period.
ALTER TABLE ai_model_prices
    DROP CONSTRAINT ai_model_prices_pkey,
    ADD PRIMARY KEY (provider, model, valid_from);
```

Existing rows get `valid_from = NOW()` at migration time (the `DEFAULT NOW()`
populates them). This is correct — existing prices are valid from the moment
of migration, not retroactively.

### Price lookup (most recent valid row)

The current `GetAIModelPriceByProviderModel` returns a single row by
`(provider, model)`. With `valid_from`, it becomes a "most recent valid" lookup:

```sql
-- name: GetAIModelPriceByProviderModel :one
SELECT *
FROM ai_model_prices
WHERE provider = @provider AND model = @model
  AND valid_from <= NOW()
ORDER BY valid_from DESC
LIMIT 1;
```

If no row matches (no price valid at the current time), `sql.ErrNoRows` — cost
is NULL, same as today. This is correct: the price wasn't valid yet.

### Upsert behavior

The current `UpsertAIModelPrices` uses `ON CONFLICT (provider, model) DO UPDATE`
— it overwrites the price columns on conflict. With the new PK
`(provider, model, valid_from)`, the conflict key changes. Two options:

**Option A: `ON CONFLICT DO NOTHING`** — insert new price periods, never
clobber existing ones. If the admin re-applies the same seed with the same
`valid_from`, rows are silently skipped. If they use a different `valid_from`,
a new row is inserted. This is the safest default.

**Option B: `ON CONFLICT (provider, model, valid_from) DO UPDATE`** — update
the price columns for the same validity period. This lets the admin correct a
price they just set without creating a new row. The `updated_at` column tracks
the correction.

Recommended: **Option B** — the admin can correct a just-set price, and new
`valid_from` values create new rows naturally.

### Startup seeder

The existing seeder (`coderd/aibridge/prices/prices.go`) runs at server
startup and upserts the embedded price book. With `valid_from`, it should only
seed prices that don't exist yet — it should not clobber admin-set prices.

Two approaches:

1. **Seed with `ON CONFLICT DO NOTHING`** — the seeder inserts rows only if
   `(provider, model, valid_from)` doesn't exist. Admin-set prices survive
   restart. The seeder uses `valid_from = NOW()` for the first seed, then
   subsequent restarts are no-ops (the row already exists).

2. **Check before seed** — query for existing prices before upserting. More
   explicit but more queries.

Recommended: **Option 1** — `ON CONFLICT DO NOTHING` in the seeder's upsert.
Simple, idempotent, non-destructive.

### CLI changes

#### `coder exp model-prices update <seed> [--yes] [--valid-from <timestamp>]`

- **Default:** `--valid-from` is `NOW()`. Past unpriced usage stays NULL.
  New requests use the new price. No retroactive impact.

- **`--valid-from <past>`:** sets `valid_from` on all rows in the seed to the
  specified timestamp. The price is retroactively valid from that point.

  Interactive mode warns before applying:
  ```
  You are setting prices valid from 2025-01-01. This means:
  - Past usage from 2025-01-01 onward will be priced at these rates.
  - Users whose backfilled spend exceeds their budget may be blocked
    on their next request.
  Continue? (y/N)
  ```

  With `--yes`, the warning is skipped.

- **No `--valid-from` and no `valid_from` in the seed file:** defaults to
  `NOW()`. The seed file may also carry `valid_from` per row, overriding the
  flag.

#### `coder exp model-prices list`

Add a `valid_from` column to the table output. JSON output already includes
all fields.

#### `coder exp model-prices import`

No change — the `import` command transforms models.dev data into wire-format
rows. The `valid_from` field is optional in the output (omitted = `NOW()` when
applied).

### What this replaces

The `dbaipricebackfill` rollup job (previously sketched in this doc) is no
longer needed. The entire backfill machinery — advisory lock, periodic tick,
CTE query, partial index, past-day-only spend logic, threshold suppression —
is replaced by a single column and a WHERE clause.

### Cost calculation implications

The cost calculation path (`resolveTokenUsageCost` → `GetAIModelPriceByProviderModel`
→ `computeCost`) is unchanged in structure. The only difference is the SQL
query: `WHERE ... AND valid_from <= NOW() ORDER BY valid_from DESC LIMIT 1`
instead of `WHERE provider = $1 AND model = $2`.

The price snapshot on `aibridge_token_usages` (`input_price_micros` etc.) is
taken from the `valid_from` row that was active at the time of the request.
This is correct — it records the price that was in effect.

### Budget enforcement with retroactive pricing

When `--valid-from <past>` is used, past token usage rows that were NULL-cost
are NOT automatically backfilled by this design. The cost lookup at request
time uses `valid_from <= NOW()`, which picks up the retroactive price for
*new* requests. But *past* requests already have `cost_micros = NULL` and are
not re-computed.

If the admin wants past usage to reflect the retroactive price, a separate
backfill step is still needed. However, the `valid_from` design makes this
opt-in rather than automatic:

- The admin can run a one-time SQL migration/CLI command to backfill
  `cost_micros` for rows where `created_at >= valid_from AND cost_micros IS NULL`.
- The backfill is simple: join `aibridge_token_usages` →
  `aibridge_interceptions` → `ai_model_prices` (matching on `valid_from`),
  compute cost, accumulate daily spend.
- This is a manual, explicit step — not an automatic rollup job.

The key insight: `valid_from` makes the *default* behavior correct (past
usage stays NULL = "price didn't exist yet"), and makes retroactive pricing
an explicit admin decision with clear consequences. The rollup job was trying
to solve a problem that `valid_from` prevents from existing in the first
place.

## Files to create/modify (sketch — not implemented)

| File | Action |
|------|--------|
| `coderd/database/migrations/000NNN_ai_model_prices_valid_from.up.sql` | Add column, change PK |
| `coderd/database/queries/aicostcontrol.sql` | Update `GetAIModelPriceByProviderModel` (add `valid_from <= NOW() ORDER BY`), update `UpsertAIModelPrices` (new conflict key) |
| `coderd/database/queries.sql.go` | Regenerate (`make gen/db`) |
| `coderd/aibridge/prices/prices.go` | Seeder uses `ON CONFLICT DO NOTHING` |
| `coderd/aibridge/prices/data/prices.json` | No change (valid_from defaults at seed time) |
| `cli/exp_modelprices.go` | Add `--valid-from` flag, warning prompt |
| `cli/exp_modelprices_test.go` | Test `--valid-from` default and past |
| `codersdk/aimodelprices.go` | Add `ValidFrom time.Time` field |

## Out of scope

- Automatic backfill rollup job (replaced by explicit admin `--valid-from`)
- `effective_group_id` resolution for NULL rows (unchanged — still not
  retroactively resolved)
- Historical price querying ("what was the price on day X") — the
  `(provider, model, valid_from)` PK supports this, but no query is added
