package pricebook_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge/prices/pricebook"
)

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("DistinguishesNullFromZero", func(t *testing.T) {
		t.Parallel()

		rows, err := pricebook.Parse([]byte(`[
			{
				"provider": "openai",
				"model": "gpt",
				"input_price": 0,
				"output_price": null,
				"cache_read_price": 1,
				"cache_write_price": 2
			}
		]`))
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, "openai", rows[0].Provider)
		require.Equal(t, "gpt", rows[0].Model)
		require.Equal(t, int64Ptr(0), rows[0].InputPrice)
		require.Nil(t, rows[0].OutputPrice)
		require.Equal(t, int64Ptr(1), rows[0].CacheReadPrice)
		require.Equal(t, int64Ptr(2), rows[0].CacheWritePrice)
	})

	t.Run("MissingPricesAreNull", func(t *testing.T) {
		t.Parallel()

		rows, err := pricebook.Parse([]byte(`[{"provider": "anthropic", "model": "claude"}]`))
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Nil(t, rows[0].InputPrice)
		require.Nil(t, rows[0].OutputPrice)
		require.Nil(t, rows[0].CacheReadPrice)
		require.Nil(t, rows[0].CacheWritePrice)
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()

		rows, err := pricebook.Parse([]byte(`[]`))
		require.NoError(t, err)
		require.Empty(t, rows)
	})

	t.Run("Malformed", func(t *testing.T) {
		t.Parallel()

		_, err := pricebook.Parse([]byte(`not json`))
		require.Error(t, err)
	})
}

func TestRowKey(t *testing.T) {
	t.Parallel()

	// Provider and model stay separate fields, so a provider identifier that
	// itself contains a slash cannot collide with a namespaced model.
	a := pricebook.Row{
		Provider: "azure",
		Model:    "openai/gpt-5",
	}
	b := pricebook.Row{
		Provider: "azure/openai",
		Model:    "gpt-5",
	}
	require.Equal(t, pricebook.Key{Provider: "azure", Model: "openai/gpt-5"}, a.Key())
	require.NotEqual(t, a.Key(), b.Key())
}

func int64Ptr(v int64) *int64 {
	return &v
}
