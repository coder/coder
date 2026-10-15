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
// NOTE: the batch SQL upsert extracts these fields from the raw JSON by name,
// so the tags here and that query must stay in sync. TestSeedFromBytes/
// RowTagsMatchSQLColumns in the parent package guards that.
type Row struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	InputPrice      *int64 `json:"input_price"`
	OutputPrice     *int64 `json:"output_price"`
	CacheReadPrice  *int64 `json:"cache_read_price"`
	CacheWritePrice *int64 `json:"cache_write_price"`
	// CacheWrite1hPrice is nil when a 1h write costs the cache write price.
	CacheWrite1hPrice *int64 `json:"cache_write_1h_price"`
	// Tiers are sorted by ThresholdTokens. omitempty keeps rows without
	// tiers unchanged in the generated artifact.
	Tiers []Tier `json:"tiers,omitempty"`
}

// Tier holds the prices for a request whose prompt is over ThresholdTokens
// tokens. A nil price uses the price of the Row, but a nil CacheWrite1hPrice
// uses the cache write price that applies to the tier.
type Tier struct {
	ThresholdTokens   int64  `json:"threshold_tokens"`
	InputPrice        *int64 `json:"input_price"`
	OutputPrice       *int64 `json:"output_price"`
	CacheReadPrice    *int64 `json:"cache_read_price"`
	CacheWritePrice   *int64 `json:"cache_write_price"`
	CacheWrite1hPrice *int64 `json:"cache_write_1h_price"`
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
		equalPrice(r.CacheWrite1hPrice, other.CacheWrite1hPrice) &&
		slices.EqualFunc(r.Tiers, other.Tiers, Tier.samePrices)
}

func (t Tier) samePrices(other Tier) bool {
	return t.ThresholdTokens == other.ThresholdTokens &&
		equalPrice(t.InputPrice, other.InputPrice) &&
		equalPrice(t.OutputPrice, other.OutputPrice) &&
		equalPrice(t.CacheReadPrice, other.CacheReadPrice) &&
		equalPrice(t.CacheWritePrice, other.CacheWritePrice) &&
		equalPrice(t.CacheWrite1hPrice, other.CacheWrite1hPrice)
}

func equalPrice(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
