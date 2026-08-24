package prices_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridge/prices"
	"github.com/coder/coder/v2/coderd/aibridge/prices/pricebook"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
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

	t.Run("SeededPricesAreDefault", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db, _ := dbtestutil.NewDB(t)

		require.NoError(t, prices.SeedFromBytes(ctx, db, []byte(testSeedJSON)))

		got, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "openai", Model: "gpt-4o",
		})
		require.NoError(t, err)
		require.Equal(t, database.AIModelPriceSourceDefault, got.Source)
		require.Equal(t, int64(2_500_000), got.InputPrice.Int64)
		require.Equal(t, int64(10_000_000), got.OutputPrice.Int64)
		require.Equal(t, int64(1_250_000), got.CacheReadPrice.Int64)
		require.False(t, got.CacheWritePrice.Valid)
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
		require.NoError(t, db.UpsertAIModelPrices(ctx, database.UpsertAIModelPricesParams{
			Seed: []byte(`[{
				"provider": "openai",
				"model": "gpt-4o",
				"input_price": 1,
				"output_price": 2,
				"cache_read_price": 3,
				"cache_write_price": 4
			}]`),
			Source: database.AIModelPriceSourceDefault,
		}))
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
		require.NoError(t, db.UpsertAIModelPrices(ctx, database.UpsertAIModelPricesParams{
			Seed: []byte(`[{
				"provider": "test-provider",
				"model": "test-model-not-in-seed",
				"input_price": 12345,
				"output_price": 67890,
				"cache_read_price": null,
				"cache_write_price": null
			}]`),
			Source: database.AIModelPriceSourceDefault,
		}))

		require.NoError(t, prices.SeedFromBytes(ctx, db, []byte(testSeedJSON)))

		got, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "test-provider", Model: "test-model-not-in-seed",
		})
		require.NoError(t, err)
		require.Equal(t, int64(12345), got.InputPrice.Int64)
		require.Equal(t, int64(67890), got.OutputPrice.Int64)
	})

	t.Run("RowTagsMatchSQLColumns", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db, _ := dbtestutil.NewDB(t)

		// The upsert extracts each field from the raw JSON by name, so the
		// struct tags on pricebook.Row and the query have to agree. Building
		// the seed from the struct rather than a JSON literal is what makes a
		// renamed tag fail here: the query would look for a name the seed no
		// longer carries and write NULL instead. Each price gets a distinct
		// value so a query that reads the wrong field also fails.
		var seed bytes.Buffer
		require.NoError(t, pricebook.Write(&seed, []pricebook.Row{{
			Provider:        "test-provider",
			Model:           "tag-contract",
			InputPrice:      ptr.Ref(int64(11)),
			OutputPrice:     ptr.Ref(int64(22)),
			CacheReadPrice:  ptr.Ref(int64(33)),
			CacheWritePrice: ptr.Ref(int64(44)),
		}}))
		require.NoError(t, prices.SeedFromBytes(ctx, db, seed.Bytes()))

		got, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "test-provider", Model: "tag-contract",
		})
		require.NoError(t, err)
		require.Equal(t, int64(11), got.InputPrice.Int64)
		require.Equal(t, int64(22), got.OutputPrice.Int64)
		require.Equal(t, int64(33), got.CacheReadPrice.Int64)
		require.Equal(t, int64(44), got.CacheWritePrice.Int64)
	})

	t.Run("LeavesCustomPricesUntouched", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db, _ := dbtestutil.NewDB(t)

		// Price a model the seed also covers.
		require.NoError(t, db.UpsertAIModelPrices(ctx, database.UpsertAIModelPricesParams{
			Seed: []byte(`[{
				"provider": "openai",
				"model": "gpt-4o",
				"input_price": 1,
				"output_price": 2,
				"cache_read_price": 3,
				"cache_write_price": 4
			}]`),
			Source: database.AIModelPriceSourceCustom,
		}))
		before, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "openai", Model: "gpt-4o",
		})
		require.NoError(t, err)
		require.Equal(t, database.AIModelPriceSourceCustom, before.Source)
		require.Equal(t, int64(1), before.InputPrice.Int64)
		require.Equal(t, int64(2), before.OutputPrice.Int64)
		require.Equal(t, int64(3), before.CacheReadPrice.Int64)
		require.Equal(t, int64(4), before.CacheWritePrice.Int64)

		// Re-applying the price book writes its own row and leaves this one be.
		require.NoError(t, prices.SeedFromBytes(ctx, db, []byte(testSeedJSON)))

		got, err := db.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
			Provider: "openai", Model: "gpt-4o",
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), got.InputPrice.Int64)
		require.Equal(t, int64(2), got.OutputPrice.Int64)
		require.Equal(t, int64(3), got.CacheReadPrice.Int64)
		require.Equal(t, int64(4), got.CacheWritePrice.Int64)
		require.Equal(t, database.AIModelPriceSourceCustom, got.Source)
		require.Equal(t, before.UpdatedAt, got.UpdatedAt)
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
