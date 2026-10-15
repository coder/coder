package aibridgedserver

import (
	"database/sql"
	"errors"
	"math"
	"testing"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/database"
)

func TestComputeCost(t *testing.T) {
	t.Parallel()

	nullInt64 := func(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }

	const oneMicroPerToken = 1_000_000
	bound := maxCostMicros.IntPart()

	tests := []struct {
		name                                                         string
		price                                                        priceSet
		inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int64
		cacheWrite1hTokens                                           int64
		want                                                         int64
		wantOutOfRange                                               bool
	}{
		{
			name: "all priced",
			price: priceSet{
				input:      nullInt64(3_000_000),
				output:     nullInt64(6_000_000),
				cacheRead:  nullInt64(300_000),
				cacheWrite: nullInt64(3_750_000),
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
			price: priceSet{
				input:      nullInt64(3_000_000),
				output:     nullInt64(6_000_000),
				cacheRead:  nullInt64(300_000),
				cacheWrite: sql.NullInt64{Valid: false},
			},
			inputTokens:      100,
			outputTokens:     200,
			cacheReadTokens:  50,
			cacheWriteTokens: 10,
			// 300 + 1200 + 15 + 0.
			want: 1515,
		},
		{
			// 4 of the 10 cache writes have a 1h TTL: 6*3_750_000/1e6 = 22
			// plus 4*6_000_000/1e6 = 24.
			name: "1h cache writes use the 1h price",
			price: priceSet{
				cacheWrite:   nullInt64(3_750_000),
				cacheWrite1h: nullInt64(6_000_000),
			},
			cacheWriteTokens:   10,
			cacheWrite1hTokens: 4,
			want:               46,
		},
		{
			name:             "all prices null is zero cost",
			price:            priceSet{},
			inputTokens:      100,
			outputTokens:     200,
			cacheReadTokens:  50,
			cacheWriteTokens: 10,
			want:             0,
		},
		{
			name: "zero tokens is zero cost",
			price: priceSet{
				input:  nullInt64(3_000_000),
				output: nullInt64(6_000_000),
			},
			want: 0,
		},
		{
			name: "integer division truncates",
			price: priceSet{
				// 1 token at 1 micro-unit per million tokens rounds down to 0.
				input: nullInt64(1),
			},
			inputTokens: 1,
			want:        0,
		},
		{
			name: "price just below one micro-unit per token floors to zero",
			price: priceSet{
				input: nullInt64(999_999),
			},
			inputTokens: 1, // 1 * 999_999 = 999_999, below 1_000_000
			want:        0,
		},
		{
			name: "sub-unit price summed across tokens still floors to zero",
			price: priceSet{
				input: nullInt64(999),
			},
			inputTokens: 1000, // 1000 * 999 = 999_000, below 1_000_000
			want:        0,
		},
		{
			name: "sub-unit price crosses one micro-unit once the product reaches 1e6",
			price: priceSet{
				input: nullInt64(999),
			},
			inputTokens: 1002, // 1002 times 999 is 1_000_998.
			want:        1,
		},
		{
			// Stress the per-term numerator near the int64 ceiling. At a $75/M
			// model the overflow point is ~123e9 tokens (123e9 * 75e6 = 9.225e18,
			// just over int64 max 9.223e18); 122e9 stays just under.
			name: "large token count at a high price does not overflow",
			price: priceSet{
				input: nullInt64(75_000_000), // $75 per 1M tokens
			},
			inputTokens: 122_000_000_000, // 122e9 * 75e6 = 9.15e18 < int64 max
			want:        9_150_000_000_000,
		},
		{
			// Each category costs 37.5 micro-units, so truncating per category
			// gives 37 + 37 = 74.
			name: "each category truncates before the sum",
			price: priceSet{
				input:  nullInt64(37_500_000),
				output: nullInt64(37_500_000),
			},
			inputTokens:  1,
			outputTokens: 1,
			want:         74,
		},
		{
			name:        "cost exactly at the bound is in range",
			price:       priceSet{input: nullInt64(oneMicroPerToken)},
			inputTokens: bound,
			want:        bound,
		},
		{
			name:           "cost one micro-unit above the bound is out of range",
			price:          priceSet{input: nullInt64(oneMicroPerToken)},
			inputTokens:    bound + 1,
			wantOutOfRange: true,
		},
		{
			name:           "cost of int64 max is out of range",
			price:          priceSet{input: nullInt64(oneMicroPerToken)},
			inputTokens:    math.MaxInt64,
			wantOutOfRange: true,
		},
		{
			// Each category fits on its own; only their sum exceeds the bound,
			// so the range check has to run on the total.
			name: "sum of in-range categories above the bound is out of range",
			price: priceSet{
				input:  nullInt64(oneMicroPerToken),
				output: nullInt64(oneMicroPerToken),
			},
			inputTokens:    bound/2 + 1,
			outputTokens:   bound/2 + 1,
			wantOutOfRange: true,
		},
		{
			// The cost column forbids negatives, so an implausible token count
			// is rejected here rather than failing the insert.
			name: "negative cost is out of range",
			price: priceSet{
				input: nullInt64(3_000_000),
			},
			inputTokens:    -1_000_000,
			wantOutOfRange: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := computeCost(tt.price, usageTokens{
				input:        tt.inputTokens,
				output:       tt.outputTokens,
				cacheRead:    tt.cacheReadTokens,
				cacheWrite:   tt.cacheWriteTokens,
				cacheWrite1h: tt.cacheWrite1hTokens,
			})
			if tt.wantOutOfRange {
				if !errors.Is(err, errCostOutOfRange) {
					t.Fatalf("computeCost error = %v, want errCostOutOfRange", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("computeCost error = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("computeCost = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestValidateTokenUsage(t *testing.T) {
	t.Parallel()

	bound := maxAllowedTokenUsage

	tests := []struct {
		name           string
		request        *proto.RecordTokenUsageRequest
		wantOutOfRange bool
	}{
		{
			// A frontier-sized request, with one category at zero.
			name: "plausible counts",
			request: &proto.RecordTokenUsageRequest{
				InputTokens: 1_000_000, OutputTokens: 128_000,
				CacheReadInputTokens: 500_000,
			},
		},
		{
			// The bound is inclusive.
			name: "every category exactly at the bound",
			request: &proto.RecordTokenUsageRequest{
				InputTokens: bound, OutputTokens: bound,
				CacheReadInputTokens: bound, CacheWriteInputTokens: bound,
			},
		},
		{
			name:           "above the bound",
			request:        &proto.RecordTokenUsageRequest{InputTokens: bound + 1},
			wantOutOfRange: true,
		},
		{
			// The last category is checked too, not just the first.
			name:           "negative cache write",
			request:        &proto.RecordTokenUsageRequest{CacheWriteInputTokens: -1},
			wantOutOfRange: true,
		},
		{
			name:           "negative 1h cache write",
			request:        &proto.RecordTokenUsageRequest{CacheWriteOneHourInputTokens: -1},
			wantOutOfRange: true,
		},
		{
			name:           "negative prompt tokens",
			request:        &proto.RecordTokenUsageRequest{PromptTokens: -1},
			wantOutOfRange: true,
		},
		{
			// A negative offset by a larger positive still totals in range, so
			// the check has to run per category rather than on the sum.
			name: "negative input offset by positive output",
			request: &proto.RecordTokenUsageRequest{
				InputTokens: -1_000_000, OutputTokens: 2_000_000,
			},
			wantOutOfRange: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateTokenUsage(tt.request)
			if tt.wantOutOfRange {
				if !errors.Is(err, errTokenUsageOutOfRange) {
					t.Fatalf("validateTokenUsage error = %v, want errTokenUsageOutOfRange", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateTokenUsage error = %v, want nil", err)
			}
		})
	}
}

func TestPricesForPrompt(t *testing.T) {
	t.Parallel()

	nullInt64 := func(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }

	base := database.AIModelPrice{
		Model:             "m",
		InputPrice:        nullInt64(1_000),
		OutputPrice:       nullInt64(2_000),
		CacheReadPrice:    nullInt64(100),
		CacheWritePrice:   nullInt64(1_250),
		CacheWrite1hPrice: nullInt64(2_000),
	}
	withTiers := func(tiers string) database.AIModelPrice {
		row := base
		row.Tiers = []byte(tiers)
		return row
	}

	tests := []struct {
		name   string
		row    database.AIModelPrice
		prompt int64
		want   priceSet
	}{
		{
			name:   "no tiers uses base prices",
			row:    withTiers(`[]`),
			prompt: 1_000_000,
			want: priceSet{
				input: nullInt64(1_000), output: nullInt64(2_000), cacheRead: nullInt64(100),
				cacheWrite: nullInt64(1_250), cacheWrite1h: nullInt64(2_000),
			},
		},
		{
			name: "no 1h price uses the cache write price",
			row: database.AIModelPrice{
				InputPrice:      nullInt64(1_000),
				CacheWritePrice: nullInt64(1_250),
				Tiers:           []byte(`[]`),
			},
			want: priceSet{input: nullInt64(1_000), cacheWrite: nullInt64(1_250), cacheWrite1h: nullInt64(1_250)},
		},
		{
			// Tiers are read in threshold order even when the stored array
			// is not sorted, so the highest tier under the prompt wins.
			name: "highest tier below the prompt applies",
			row: withTiers(`[
				{"threshold_tokens": 500, "input_price": 5000, "output_price": 5000, "cache_read_price": 500, "cache_write_price": 5000, "cache_write_1h_price": 5000},
				{"threshold_tokens": 200, "input_price": 3000, "output_price": 3000, "cache_read_price": 300, "cache_write_price": 3000, "cache_write_1h_price": 3000}
			]`),
			prompt: 501,
			want: priceSet{
				input: nullInt64(5_000), output: nullInt64(5_000), cacheRead: nullInt64(500),
				cacheWrite: nullInt64(5_000), cacheWrite1h: nullInt64(5_000), tierThreshold: nullInt64(500),
			},
		},
		{
			// Gemini tiers carry no cache write price.
			name:   "missing tier prices use the base prices",
			row:    withTiers(`[{"threshold_tokens": 200, "input_price": 3000, "output_price": null, "cache_read_price": 300}]`),
			prompt: 201,
			want: priceSet{
				input: nullInt64(3_000), output: nullInt64(2_000), cacheRead: nullInt64(300),
				cacheWrite: nullInt64(1_250), cacheWrite1h: nullInt64(1_250), tierThreshold: nullInt64(200),
			},
		},
		{
			// The fallback for a missing tier 1h price is the tier cache
			// write price, not the base 1h price.
			name:   "tier without 1h price uses the tier cache write price",
			row:    withTiers(`[{"threshold_tokens": 200, "cache_write_price": 4000}]`),
			prompt: 201,
			want: priceSet{
				input: nullInt64(1_000), output: nullInt64(2_000), cacheRead: nullInt64(100),
				cacheWrite: nullInt64(4_000), cacheWrite1h: nullInt64(4_000), tierThreshold: nullInt64(200),
			},
		},
		{
			name:   "undecodable tiers use base prices",
			row:    withTiers(`{"threshold_tokens": 200}`),
			prompt: 201,
			want: priceSet{
				input: nullInt64(1_000), output: nullInt64(2_000), cacheRead: nullInt64(100),
				cacheWrite: nullInt64(1_250), cacheWrite1h: nullInt64(2_000),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &Server{logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})}
			got := s.pricesForPrompt(t.Context(), tt.row, tt.prompt)
			if got != tt.want {
				t.Fatalf("pricesForPrompt = %+v, want %+v", got, tt.want)
			}
		})
	}
}
