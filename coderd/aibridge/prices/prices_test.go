package prices_test

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridge/prices"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
)

// testSeedJSON is a synthetic seed used by tests instead of the embedded
// one, so assertions don't depend on whatever values currently live in the
// embedded seed.
const testSeedJSON = `[
  {
    "provider": "anthropic",
    "model": "claude-opus-4-7",
    "input_price": 5000000,
    "output_price": 25000000,
    "cache_read_price": 500000,
    "cache_write_price": 6250000
  },
  {
    "provider": "openai",
    "model": "gpt-4o",
    "input_price": 2500000,
    "output_price": 10000000,
    "cache_read_price": 1250000,
    "cache_write_price": null
  }
]`

func TestSeedFromBytes(t *testing.T) {
	t.Parallel()

	t.Run("SeedsFreshDatabase", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db, _ := dbtestutil.NewDB(t)

		require.NoError(t, prices.SeedFromBytes(ctx, db, []byte(testSeedJSON)))

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
		require.Equal(t, opus.CreatedAt, opus.UpdatedAt)

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
		require.Zero(t, gpt.CacheWritePrice.Int64)
	})

	t.Run("Idempotent", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db, _ := dbtestutil.NewDB(t)

		require.NoError(t, prices.SeedFromBytes(ctx, db, []byte(testSeedJSON)))
		first, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "openai", Model: "gpt-4o",
		})
		require.NoError(t, err)

		require.NoError(t, prices.SeedFromBytes(ctx, db, []byte(testSeedJSON)))
		second, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "openai", Model: "gpt-4o",
		})
		require.NoError(t, err)

		// A re-seed that changes nothing must not touch the row at all.
		require.Equal(t, first.InputPrice, second.InputPrice)
		require.Equal(t, first.OutputPrice, second.OutputPrice)
		require.Equal(t, first.CreatedAt, second.CreatedAt)
		require.Equal(t, first.UpdatedAt, second.UpdatedAt)
	})

	t.Run("OverwritesExistingPrices", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db, _ := dbtestutil.NewDB(t)

		// Pre-seed with deliberately wrong values for all four price columns.
		// cache_write_price is set to a non-NULL value here even though the
		// embedded seed leaves it NULL for OpenAI; Seed must replace it with
		// NULL to keep the table in sync with the seed.
		require.NoError(t, db.UpsertAIModelPrices(ctx, []byte(`[{
			"provider": "openai",
			"model": "gpt-4o",
			"input_price": 1,
			"output_price": 2,
			"cache_read_price": 3,
			"cache_write_price": 4
		}]`)))
		before, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "openai", Model: "gpt-4o",
		})
		require.NoError(t, err)

		require.NoError(t, prices.SeedFromBytes(ctx, db, []byte(testSeedJSON)))

		got, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "openai", Model: "gpt-4o",
		})
		require.NoError(t, err)
		require.Equal(t, int64(2_500_000), got.InputPrice.Int64)
		require.Equal(t, int64(10_000_000), got.OutputPrice.Int64)
		require.Equal(t, int64(1_250_000), got.CacheReadPrice.Int64)
		require.False(t, got.CacheWritePrice.Valid)
		require.Zero(t, got.CacheWritePrice.Int64)
		require.Equal(t, before.CreatedAt, got.CreatedAt)
		require.True(t, got.UpdatedAt.After(before.UpdatedAt))
	})

	t.Run("LeavesOrphanRowsUntouched", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db, _ := dbtestutil.NewDB(t)

		// Insert a row for a (provider, model) the seed doesn't cover. After
		// Seed it should still be there with its values intact.
		require.NoError(t, db.UpsertAIModelPrices(ctx, []byte(`[{
			"provider": "test-provider",
			"model": "test-model-not-in-seed",
			"input_price": 12345,
			"output_price": 67890,
			"cache_read_price": null,
			"cache_write_price": null
		}]`)))

		require.NoError(t, prices.SeedFromBytes(ctx, db, []byte(testSeedJSON)))

		got, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "test-provider", Model: "test-model-not-in-seed",
		})
		require.NoError(t, err)
		require.Equal(t, int64(12345), got.InputPrice.Int64)
		require.Equal(t, int64(67890), got.OutputPrice.Int64)
	})

	// Verifies the chain: AsAIBridged context -> dbauthz wrapper auth check
	// -> subjectAibridged's permission grant. A missing or wrong action on
	// the subject would surface as "unauthorized: rbac: forbidden" here, even
	// though the unit tests above (which bypass dbauthz) would still pass.
	t.Run("AuthorizedAsAIBridged", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		rawDB, _ := dbtestutil.NewDB(t)
		authzDB := dbauthz.New(rawDB, rbac.NewStrictAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), coderdtest.AccessControlStorePointer())

		require.NoError(t, prices.SeedFromBytes(dbauthz.AsAIBridged(ctx), authzDB, []byte(testSeedJSON)))

		// Read back via the raw DB.
		got, err := rawDB.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "openai", Model: "gpt-4o",
		})
		require.NoError(t, err)
		require.True(t, got.InputPrice.Valid)
		require.Equal(t, int64(2_500_000), got.InputPrice.Int64)
	})

	// Every price column counts toward the comparison, and a NULL on either
	// side counts as a difference.
	t.Run("UpdatedAtTracksPriceChanges", func(t *testing.T) {
		t.Parallel()

		key := database.GetAIModelPriceByProviderModelParams{Provider: "openai", Model: "gpt-4o"}
		seed := func(priceFields string) []byte {
			return fmt.Appendf(nil, `[{"provider": %q, "model": %q, %s}]`, key.Provider, key.Model, priceFields)
		}

		tests := []struct {
			name             string
			initial, updated string
		}{
			{
				name:    "InputPriceChanged",
				initial: `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`,
				updated: `"input_price": 111, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`,
			},
			{
				name:    "InputPriceSetFromNull",
				initial: `"input_price": null, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`,
				updated: `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`,
			},
			{
				name:    "InputPriceClearedToNull",
				initial: `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`,
				updated: `"input_price": null, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`,
			},
			{
				name:    "OutputPriceChanged",
				initial: `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`,
				updated: `"input_price": 100, "output_price": 222, "cache_read_price": 300, "cache_write_price": 400`,
			},
			{
				name:    "OutputPriceSetFromNull",
				initial: `"input_price": 100, "output_price": null, "cache_read_price": 300, "cache_write_price": 400`,
				updated: `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`,
			},
			{
				name:    "OutputPriceClearedToNull",
				initial: `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`,
				updated: `"input_price": 100, "output_price": null, "cache_read_price": 300, "cache_write_price": 400`,
			},
			{
				name:    "CacheReadPriceChanged",
				initial: `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`,
				updated: `"input_price": 100, "output_price": 200, "cache_read_price": 333, "cache_write_price": 400`,
			},
			{
				name:    "CacheReadPriceSetFromNull",
				initial: `"input_price": 100, "output_price": 200, "cache_read_price": null, "cache_write_price": 400`,
				updated: `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`,
			},
			{
				name:    "CacheReadPriceClearedToNull",
				initial: `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`,
				updated: `"input_price": 100, "output_price": 200, "cache_read_price": null, "cache_write_price": 400`,
			},
			{
				name:    "CacheWritePriceChanged",
				initial: `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`,
				updated: `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": 444`,
			},
			{
				name:    "CacheWritePriceSetFromNull",
				initial: `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": null`,
				updated: `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`,
			},
			{
				name:    "CacheWritePriceClearedToNull",
				initial: `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`,
				updated: `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": null`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitShort)
				db, _ := dbtestutil.NewDB(t)

				require.NoError(t, prices.SeedFromBytes(ctx, db, seed(tt.initial)))
				before, err := db.GetAIModelPriceByProviderModel(ctx, key)
				require.NoError(t, err)

				require.NoError(t, prices.SeedFromBytes(ctx, db, seed(tt.updated)))
				after, err := db.GetAIModelPriceByProviderModel(ctx, key)
				require.NoError(t, err)

				require.True(t, after.UpdatedAt.After(before.UpdatedAt), "updated_at should advance when a price changes")
			})
		}
	})
}

// TestSeed exercises the real embedded prices.json so we catch a corrupted,
// empty, or unparseable seed file at test time rather than at server startup.
// Intentionally makes no assertions about specific prices, since those drift
// whenever the seed is regenerated from upstream.
func TestSeed(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	require.NoError(t, prices.Seed(ctx, db))
}

// TestIsDefaultPriced reads the real embedded price book, so it uses a model
// the generator injects rather than one that could drift out of upstream.
func TestIsDefaultPriced(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider string
		model    string
		want     bool
	}{
		{
			name:     "ModelInThePriceBook",
			provider: "anthropic",
			model:    "claude-opus-5",
			want:     true,
		},
		{
			name:     "ModelNotInThePriceBook",
			provider: "anthropic",
			model:    "not-a-real-model",
			want:     false,
		},
		{
			// The book is keyed on both columns, so the same model under
			// another provider is a different entry.
			name:     "SameModelUnderAnotherProvider",
			provider: "openai",
			model:    "claude-opus-5",
			want:     false,
		},
		{
			name:     "UnknownProvider",
			provider: "unknown-provider",
			model:    "claude-opus-5",
			want:     false,
		},
		{
			name:     "Empty",
			provider: "",
			model:    "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, prices.IsDefaultPriced(tt.provider, tt.model))
		})
	}
}
