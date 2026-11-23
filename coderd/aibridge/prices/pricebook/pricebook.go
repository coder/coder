// Package pricebook defines the serialized AI Gateway model price book.
package pricebook

import (
	"encoding/json"
	"io"
	"slices"
)

// Row is one provider and model entry in the generated price book. Pointer
// fields preserve the distinction between a missing price and an explicit zero.
//
// NOTE: the batch SQL upsert extracts the fields it seeds from the raw JSON by
// name, so the tags here and that query must stay in sync. TestSeedFromBytes/
// RowTagsMatchSQLColumns in the parent package guards that.
type Row struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	InputPrice      *int64 `json:"input_price"`
	OutputPrice     *int64 `json:"output_price"`
	CacheReadPrice  *int64 `json:"cache_read_price"`
	CacheWritePrice *int64 `json:"cache_write_price"`
	// PricingExtras holds the prices beyond the four base prices. nil when the
	// model has none.
	PricingExtras *PricingExtras `json:"pricing_extras,omitempty"`
}

// PricingExtras is the shape of the ai_model_prices.pricing_extras column.
// Absent prices are omitted rather than written as null, and readers ignore
// unknown keys, so new price types can be added without a schema change.
type PricingExtras struct {
	ExtraPrices ExtraPrices `json:"extra_prices,omitzero"`
	// Tiers are long-context prices, sorted by ThresholdTokens.
	Tiers []Tier `json:"tiers,omitempty"`
}

// ExtraPrices are base prices for token types beyond the four price columns.
type ExtraPrices struct {
	// CacheWrite1h is the price of a cache write with a 1-hour TTL. nil when a
	// 1-hour write costs the cache write price.
	CacheWrite1h *int64 `json:"cache_write_1h,omitempty"`
}

// Tier holds the prices for a request whose prompt is over ThresholdTokens
// tokens.
type Tier struct {
	ThresholdTokens int64      `json:"threshold_tokens"`
	Prices          TierPrices `json:"prices"`
}

// TierPrices are the prices of a long-context tier. A nil price uses the base
// price of the Row, except a nil CacheWrite1h, which uses the cache write
// price that applies to the tier.
type TierPrices struct {
	Input        *int64 `json:"input,omitempty"`
	Output       *int64 `json:"output,omitempty"`
	CacheRead    *int64 `json:"cache_read,omitempty"`
	CacheWrite   *int64 `json:"cache_write,omitempty"`
	CacheWrite1h *int64 `json:"cache_write_1h,omitempty"`
}

// NewPricingExtras returns the extras for a 1-hour cache write price and
// long-context tiers, or nil when there are none, so that a model without
// extras stores SQL NULL.
func NewPricingExtras(cacheWrite1h *int64, tiers []Tier) *PricingExtras {
	if cacheWrite1h == nil && len(tiers) == 0 {
		return nil
	}
	return &PricingExtras{
		ExtraPrices: ExtraPrices{CacheWrite1h: cacheWrite1h},
		Tiers:       tiers,
	}
}

// Key identifies a model within a provider.
type Key struct {
	Provider string
	Model    string
}

// Key returns the provider and model identity of the row.
func (r Row) Key() Key {
	return Key{Provider: r.Provider, Model: r.Model}
}

func (k Key) String() string {
	return k.Provider + "/" + k.Model
}

// Parse decodes a generated price book.
func Parse(data []byte) ([]Row, error) {
	var rows []Row
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// Write encodes a price book in the on-disk form of the generated artifact.
func Write(w io.Writer, rows []Row) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

// SamePrices reports whether two rows carry identical prices. Provider and
// model are ignored, so it is only meaningful for rows with the same Key. A
// price moving to or from null counts as a difference, matching the null
// handling in the batch SQL upsert.
func (r Row) SamePrices(other Row) bool {
	return equalPrice(r.InputPrice, other.InputPrice) &&
		equalPrice(r.OutputPrice, other.OutputPrice) &&
		equalPrice(r.CacheReadPrice, other.CacheReadPrice) &&
		equalPrice(r.CacheWritePrice, other.CacheWritePrice) &&
		r.PricingExtras.samePrices(other.PricingExtras)
}

func (e *PricingExtras) samePrices(other *PricingExtras) bool {
	if e == nil || other == nil {
		return e == nil && other == nil
	}
	return equalPrice(e.ExtraPrices.CacheWrite1h, other.ExtraPrices.CacheWrite1h) &&
		slices.EqualFunc(e.Tiers, other.Tiers, Tier.samePrices)
}

func (t Tier) samePrices(other Tier) bool {
	return t.ThresholdTokens == other.ThresholdTokens &&
		equalPrice(t.Prices.Input, other.Prices.Input) &&
		equalPrice(t.Prices.Output, other.Prices.Output) &&
		equalPrice(t.Prices.CacheRead, other.Prices.CacheRead) &&
		equalPrice(t.Prices.CacheWrite, other.Prices.CacheWrite) &&
		equalPrice(t.Prices.CacheWrite1h, other.Prices.CacheWrite1h)
}

func equalPrice(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
