// Package pricebook defines the serialized AI Gateway model price book.
package pricebook

import (
	"encoding/json"
	"io"
)

// Row is one provider and model entry in the generated price book. Pointer
// fields preserve the distinction between a missing price and an explicit zero.
//
// NOTE: the batch SQL upsert extracts these fields from the raw JSON by name,
// so the tags here and that query must stay in sync.
type Row struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	InputPrice      *int64 `json:"input_price"`
	OutputPrice     *int64 `json:"output_price"`
	CacheReadPrice  *int64 `json:"cache_read_price"`
	CacheWritePrice *int64 `json:"cache_write_price"`
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
		equalPrice(r.CacheWritePrice, other.CacheWritePrice)
}

func equalPrice(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
