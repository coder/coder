package modelprices_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/modelprices"
)

func TestTransform(t *testing.T) {
	t.Parallel()

	// upstreamJSON covers all the test cases in a single payload:
	//   - a normal model with all four price fields (anthropic/claude-fable-5),
	//   - a partial-pricing model (openai/gpt-4o, missing cache_write),
	//   - a model with no cost key (openai/gpt-no-prices), which is skipped,
	//   - a model with cost but all-null price fields (openai/gpt-null-cost),
	//     which is skipped,
	//   - a model with an explicit zero price (anthropic/claude-zero),
	//     which is included because zero is "populated",
	//   - a third provider (alibaba) that Transform must NOT filter out (it
	//     transforms all providers; filtering is the caller's job).
	const upstreamJSON = `{
		"alibaba": {
			"models": {
				"qwen-max": {
					"name": "Qwen Max",
					"limit": {"context": 128000, "output": 8192},
					"cost": {"input": 1, "output": 1}
				}
			}
		},
		"anthropic": {
			"models": {
				"claude-fable-5": {
					"name": "Claude Fable 5",
					"limit": {"context": 1000000, "output": 128000},
					"cost": {"input": 10, "output": 50, "cache_read": 1, "cache_write": 12.5}
				},
				"claude-zero": {
					"name": "Claude Zero",
					"limit": {"context": 200000, "output": 64000},
					"cost": {"input": 0, "output": 0}
				}
			}
		},
		"openai": {
			"models": {
				"gpt-4o": {
					"name": "GPT-4o",
					"limit": {"context": 128000, "output": 16384},
					"cost": {"input": 2.5, "output": 10, "cache_read": 1.25}
				},
				"gpt-no-prices": {
					"name": "GPT No Prices"
				},
				"gpt-null-cost": {
					"name": "GPT Null Cost",
					"limit": {"context": 128000, "output": 16384},
					"cost": {}
				}
			}
		}
	}`

	rows, err := modelprices.Transform([]byte(upstreamJSON))
	require.NoError(t, err)

	// Sorted by (provider, model): alibaba/qwen-max, anthropic/claude-fable-5,
	// anthropic/claude-zero, openai/gpt-4o. The no-cost and all-null-cost
	// models are absent.
	require.Len(t, rows, 4)

	// alibaba/qwen-max is NOT filtered; Transform processes all providers.
	require.Equal(t, "alibaba", rows[0].Provider)
	require.Equal(t, "qwen-max", rows[0].Model)
	require.Equal(t, int64(1_000_000), *rows[0].InputPrice)
	require.Equal(t, int64(1_000_000), *rows[0].OutputPrice)
	require.Nil(t, rows[0].CacheReadPrice)
	require.Nil(t, rows[0].CacheWritePrice)

	// anthropic/claude-fable-5: all four fields, exact micro-units.
	fable := rows[1]
	require.Equal(t, "anthropic", fable.Provider)
	require.Equal(t, "claude-fable-5", fable.Model)
	require.Equal(t, int64(10_000_000), *fable.InputPrice)
	require.Equal(t, int64(50_000_000), *fable.OutputPrice)
	require.Equal(t, int64(1_000_000), *fable.CacheReadPrice)
	require.Equal(t, int64(12_500_000), *fable.CacheWritePrice)

	// anthropic/claude-zero: explicit zeros are populated, so the model is
	// included and the price pointers are *int64(0), not nil.
	zero := rows[2]
	require.Equal(t, "anthropic", zero.Provider)
	require.Equal(t, "claude-zero", zero.Model)
	require.NotNil(t, zero.InputPrice)
	require.Equal(t, int64(0), *zero.InputPrice)
	require.NotNil(t, zero.OutputPrice)
	require.Equal(t, int64(0), *zero.OutputPrice)
	require.Nil(t, zero.CacheReadPrice)
	require.Nil(t, zero.CacheWritePrice)

	// openai/gpt-4o: partial pricing; cache_write is absent (nil).
	gpt := rows[3]
	require.Equal(t, "openai", gpt.Provider)
	require.Equal(t, "gpt-4o", gpt.Model)
	require.Equal(t, int64(2_500_000), *gpt.InputPrice)
	require.Equal(t, int64(10_000_000), *gpt.OutputPrice)
	require.Equal(t, int64(1_250_000), *gpt.CacheReadPrice)
	require.Nil(t, gpt.CacheWritePrice)
}

func TestTransformNegativePriceTreatedAsMissing(t *testing.T) {
	t.Parallel()

	// A negative price is treated as missing (nil), matching the generator's
	// toMicros behavior. The model is still included because it has other
	// non-nil price fields.
	const upstreamJSON = `{
		"anthropic": {
			"models": {
				"claude-neg": {
					"name": "Claude Neg",
					"cost": {"input": -1, "output": 5}
				}
			}
		}
	}`

	rows, err := modelprices.Transform([]byte(upstreamJSON))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Nil(t, rows[0].InputPrice)
	require.Equal(t, int64(5_000_000), *rows[0].OutputPrice)
}

func TestTransformNoPricedModels(t *testing.T) {
	t.Parallel()

	// Every model is missing a cost block or has an all-null cost block, so
	// the output is empty (but not an error; Transform does not validate
	// that at least one row has pricing; that's the generator's job).
	const upstreamJSON = `{
		"anthropic": {
			"models": {
				"claude-free": {"name": "Claude Free"},
				"claude-null": {"name": "Claude Null", "cost": {}}
			}
		}
	}`

	rows, err := modelprices.Transform([]byte(upstreamJSON))
	require.NoError(t, err)
	require.Empty(t, rows)
}

// TestTransformRounding verifies that a fractional USD-per-million-token
// price rounds correctly to micro-units. 0.075 * 1_000_000 = 75000.
func TestTransformRounding(t *testing.T) {
	t.Parallel()

	const upstreamJSON = `{
		"anthropic": {
			"models": {
				"claude-round": {
					"cost": {"input": 0.075, "output": 0.075}
				}
			}
		}
	}`

	rows, err := modelprices.Transform([]byte(upstreamJSON))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(75_000), *rows[0].InputPrice)
	require.Equal(t, int64(75_000), *rows[0].OutputPrice)
}

func TestTransformInvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := modelprices.Transform([]byte("not json"))
	require.Error(t, err)
}

func TestTransformSortedByProviderThenModel(t *testing.T) {
	t.Parallel()

	// Deliberately out-of-order providers and models to confirm the sort.
	const upstreamJSON = `{
		"openai": {
			"models": {
				"gpt-z": {"cost": {"input": 1}},
				"gpt-a": {"cost": {"input": 1}}
			}
		},
		"anthropic": {
			"models": {
				"claude-z": {"cost": {"input": 1}},
				"claude-a": {"cost": {"input": 1}}
			}
		}
	}`

	rows, err := modelprices.Transform([]byte(upstreamJSON))
	require.NoError(t, err)
	require.Len(t, rows, 4)

	want := []struct{ provider, model string }{
		{"anthropic", "claude-a"},
		{"anthropic", "claude-z"},
		{"openai", "gpt-a"},
		{"openai", "gpt-z"},
	}
	for i, w := range want {
		require.Equal(t, w.provider, rows[i].Provider, "row %d provider", i)
		require.Equal(t, w.model, rows[i].Model, "row %d model", i)
	}
}
