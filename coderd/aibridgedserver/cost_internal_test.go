package aibridgedserver

import (
	"database/sql"
	"testing"

	"github.com/coder/coder/v2/coderd/database"
)

func TestComputeCost(t *testing.T) {
	t.Parallel()

	nullInt64 := func(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }

	tests := []struct {
		name                                                         string
		price                                                        database.AIModelPrice
		inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int64
		want                                                         int64
	}{
		{
			name: "all priced",
			price: database.AIModelPrice{
				InputPrice:      nullInt64(3_000_000),
				OutputPrice:     nullInt64(6_000_000),
				CacheReadPrice:  nullInt64(300_000),
				CacheWritePrice: nullInt64(3_750_000),
			},
			inputTokens:      100,
			outputTokens:     200,
			cacheReadTokens:  50,
			cacheWriteTokens: 10,
			// 300 + 1200 + 15 + 37 (10*3_750_000/1e6 = 37, integer division).
			want: 1552,
		},
		{
			name: "null cache write price treated as zero",
			price: database.AIModelPrice{
				InputPrice:      nullInt64(3_000_000),
				OutputPrice:     nullInt64(6_000_000),
				CacheReadPrice:  nullInt64(300_000),
				CacheWritePrice: sql.NullInt64{Valid: false},
			},
			inputTokens:      100,
			outputTokens:     200,
			cacheReadTokens:  50,
			cacheWriteTokens: 10,
			// 300 + 1200 + 15 + 0.
			want: 1515,
		},
		{
			name:             "all prices null is zero cost",
			price:            database.AIModelPrice{},
			inputTokens:      100,
			outputTokens:     200,
			cacheReadTokens:  50,
			cacheWriteTokens: 10,
			want:             0,
		},
		{
			name: "zero tokens is zero cost",
			price: database.AIModelPrice{
				InputPrice:  nullInt64(3_000_000),
				OutputPrice: nullInt64(6_000_000),
			},
			want: 0,
		},
		{
			name: "integer division truncates",
			price: database.AIModelPrice{
				// 1 token at 1 micro-unit per million tokens rounds down to 0.
				InputPrice: nullInt64(1),
			},
			inputTokens: 1,
			want:        0,
		},
		{
			name: "price just below one micro-unit per token floors to zero",
			price: database.AIModelPrice{
				InputPrice: nullInt64(999_999),
			},
			inputTokens: 1, // 1 * 999_999 = 999_999, below 1_000_000
			want:        0,
		},
		{
			name: "sub-unit price summed across tokens still floors to zero",
			price: database.AIModelPrice{
				InputPrice: nullInt64(999),
			},
			inputTokens: 1000, // 1000 * 999 = 999_000, below 1_000_000
			want:        0,
		},
		{
			name: "sub-unit price crosses one micro-unit once the product reaches 1e6",
			price: database.AIModelPrice{
				InputPrice: nullInt64(999),
			},
			inputTokens: 1002, // 1002 * 999 = 1_000_998
			want:        1,
		},
		{
			// Stress the per-term numerator near the int64 ceiling. At a $75/M
			// model the overflow point is ~123e9 tokens (123e9 * 75e6 = 9.225e18,
			// just over int64 max 9.223e18); 122e9 stays just under.
			name: "large token count at a high price does not overflow",
			price: database.AIModelPrice{
				InputPrice: nullInt64(75_000_000), // $75 per 1M tokens
			},
			inputTokens: 122_000_000_000, // 122e9 * 75e6 = 9.15e18 < int64 max
			want:        9_150_000_000_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := computeCost(tt.price, tt.inputTokens, tt.outputTokens, tt.cacheReadTokens, tt.cacheWriteTokens)
			if got != tt.want {
				t.Fatalf("computeCost = %d, want %d", got, tt.want)
			}
		})
	}
}
