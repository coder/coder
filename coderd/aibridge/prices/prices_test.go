package prices_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge/prices"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	t.Run("SeedsFreshDatabase", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db, _ := dbtestutil.NewDB(t)

		require.NoError(t, prices.Load(ctx, db))

		// Spot-check a fully-populated row.
		opus, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "anthropic",
			Model:    "claude-opus-4-7",
		})
		require.NoError(t, err)
		require.Equal(t, int64(5_000_000), opus.InputPrice.Int64)
		require.Equal(t, int64(25_000_000), opus.OutputPrice.Int64)
		require.Equal(t, int64(500_000), opus.CacheReadPrice.Int64)
		require.Equal(t, int64(6_250_000), opus.CacheWritePrice.Int64)

		// Spot-check a row where the seed has a NULL price (OpenAI does not
		// publish a cache_write_price). The column should land as SQL NULL.
		gpt, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "openai",
			Model:    "gpt-4o",
		})
		require.NoError(t, err)
		require.Equal(t, int64(2_500_000), gpt.InputPrice.Int64)
		require.Equal(t, int64(10_000_000), gpt.OutputPrice.Int64)
		require.Equal(t, int64(1_250_000), gpt.CacheReadPrice.Int64)
		require.False(t, gpt.CacheWritePrice.Valid)
	})

	t.Run("Idempotent", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db, _ := dbtestutil.NewDB(t)

		require.NoError(t, prices.Load(ctx, db))
		first, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "openai", Model: "gpt-4o",
		})
		require.NoError(t, err)

		require.NoError(t, prices.Load(ctx, db))
		second, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "openai", Model: "gpt-4o",
		})
		require.NoError(t, err)

		// Prices must be identical across runs and CreatedAt must be
		// preserved (only updated_at moves on a no-op upsert).
		require.Equal(t, first.InputPrice, second.InputPrice)
		require.Equal(t, first.OutputPrice, second.OutputPrice)
		require.Equal(t, first.CreatedAt, second.CreatedAt)
	})

	t.Run("OverwritesExistingPrices", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db, _ := dbtestutil.NewDB(t)

		// Pre-seed with deliberately wrong values for all four price columns.
		// cache_write_price is set to a non-NULL value here even though the
		// embedded seed leaves it NULL for OpenAI; Load must replace it with
		// NULL to keep the table in sync with the seed.
		require.NoError(t, db.UpsertAIModelPrice(ctx, database.UpsertAIModelPriceParams{
			Provider:        "openai",
			Model:           "gpt-4o",
			InputPrice:      sql.NullInt64{Int64: 1, Valid: true},
			OutputPrice:     sql.NullInt64{Int64: 2, Valid: true},
			CacheReadPrice:  sql.NullInt64{Int64: 3, Valid: true},
			CacheWritePrice: sql.NullInt64{Int64: 4, Valid: true},
		}))

		require.NoError(t, prices.Load(ctx, db))

		got, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "openai", Model: "gpt-4o",
		})
		require.NoError(t, err)
		require.Equal(t, int64(2_500_000), got.InputPrice.Int64)
		require.Equal(t, int64(10_000_000), got.OutputPrice.Int64)
		require.Equal(t, int64(1_250_000), got.CacheReadPrice.Int64)
		require.False(t, got.CacheWritePrice.Valid)
	})

	t.Run("LeavesOrphanRowsUntouched", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db, _ := dbtestutil.NewDB(t)

		// Insert a row for a (provider, model) the seed doesn't cover. After
		// Load it should still be there with its values intact.
		require.NoError(t, db.UpsertAIModelPrice(ctx, database.UpsertAIModelPriceParams{
			Provider:    "test-provider",
			Model:       "test-model-not-in-seed",
			InputPrice:  sql.NullInt64{Int64: 12345, Valid: true},
			OutputPrice: sql.NullInt64{Int64: 67890, Valid: true},
		}))

		require.NoError(t, prices.Load(ctx, db))

		got, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "test-provider", Model: "test-model-not-in-seed",
		})
		require.NoError(t, err)
		require.Equal(t, int64(12345), got.InputPrice.Int64)
		require.Equal(t, int64(67890), got.OutputPrice.Int64)
	})
}
