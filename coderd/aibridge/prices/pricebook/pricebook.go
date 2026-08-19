// Package pricebook defines the serialized AI Gateway model price book.
package pricebook

import "encoding/json"

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
