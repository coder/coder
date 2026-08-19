package pricebook_test

import (
	"bytes"
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

func TestWrite(t *testing.T) {
	t.Parallel()

	t.Run("OnDiskForm", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		require.NoError(t, pricebook.Write(&buf, []pricebook.Row{{
			Provider:   "openai",
			Model:      "gpt",
			InputPrice: int64Ptr(0),
		}}))
		require.Equal(t, `[
  {
    "provider": "openai",
    "model": "gpt",
    "input_price": 0,
    "output_price": null,
    "cache_read_price": null,
    "cache_write_price": null
  }
]
`, buf.String())
	})

	t.Run("RoundTrips", func(t *testing.T) {
		t.Parallel()

		// Zero and null must survive a write followed by a read, since the
		// generated artifact is the input to both the diff tool and the seeder.
		rows := []pricebook.Row{
			{
				Provider:        "anthropic",
				Model:           "claude",
				InputPrice:      int64Ptr(3_000_000),
				OutputPrice:     int64Ptr(0),
				CacheReadPrice:  nil,
				CacheWritePrice: int64Ptr(1),
			},
			{
				Provider: "openai",
				Model:    "gpt",
			},
		}

		var buf bytes.Buffer
		require.NoError(t, pricebook.Write(&buf, rows))
		got, err := pricebook.Parse(buf.Bytes())
		require.NoError(t, err)
		require.Equal(t, rows, got)
	})
}

func TestRowSamePrices(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		a    pricebook.Row
		b    pricebook.Row
		want bool
	}{
		{
			name: "identical",
			a: pricebook.Row{
				InputPrice:  int64Ptr(1),
				OutputPrice: int64Ptr(2),
			},
			b: pricebook.Row{
				InputPrice:  int64Ptr(1),
				OutputPrice: int64Ptr(2),
			},
			want: true,
		},
		{
			name: "all null",
			a:    pricebook.Row{},
			b:    pricebook.Row{},
			want: true,
		},
		{
			// Identity is not part of the comparison.
			name: "different models same prices",
			a: pricebook.Row{
				Provider:   "openai",
				Model:      "gpt",
				InputPrice: int64Ptr(1),
			},
			b: pricebook.Row{
				Provider:   "anthropic",
				Model:      "claude",
				InputPrice: int64Ptr(1),
			},
			want: true,
		},
		{
			name: "input differs",
			a:    pricebook.Row{InputPrice: int64Ptr(1)},
			b:    pricebook.Row{InputPrice: int64Ptr(2)},
			want: false,
		},
		{
			name: "output differs",
			a:    pricebook.Row{OutputPrice: int64Ptr(1)},
			b:    pricebook.Row{OutputPrice: int64Ptr(2)},
			want: false,
		},
		{
			name: "cache read differs",
			a:    pricebook.Row{CacheReadPrice: int64Ptr(1)},
			b:    pricebook.Row{CacheReadPrice: int64Ptr(2)},
			want: false,
		},
		{
			name: "cache write differs",
			a:    pricebook.Row{CacheWritePrice: int64Ptr(1)},
			b:    pricebook.Row{CacheWritePrice: int64Ptr(2)},
			want: false,
		},
		{
			// Zero is a populated price, distinct from an absent one.
			name: "null and zero",
			a:    pricebook.Row{},
			b:    pricebook.Row{InputPrice: int64Ptr(0)},
			want: false,
		},
		{
			name: "zero and null",
			a:    pricebook.Row{InputPrice: int64Ptr(0)},
			b:    pricebook.Row{},
			want: false,
		},
		{
			name: "null and nonzero",
			a:    pricebook.Row{InputPrice: nil},
			b:    pricebook.Row{InputPrice: int64Ptr(5)},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, tc.a.SamePrices(tc.b))
			// The relation is symmetric.
			require.Equal(t, tc.want, tc.b.SamePrices(tc.a))
		})
	}
}
