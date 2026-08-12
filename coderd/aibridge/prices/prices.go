// Package prices seeds the ai_model_prices table from an embedded JSON
// price book at server startup.
package prices

import (
	"context"
	_ "embed"
	"encoding/json"
	"sync"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

//go:embed data/prices.json
var seedJSON []byte

// Pointer fields preserve the distinction between "not populated by upstream"
// (null) and "explicitly zero" (0). Used only for Go-side type validation in
// parseSeed; the upsert reads the raw JSON bytes via the batch SQL query.
//
// NOTE: the JSON contract for the price seed lives in three places that must
// stay in sync: the corresponding struct in the price generator, the column
// extraction in the batch SQL upsert, and the tags here.
type seedRow struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	InputPrice      *int64 `json:"input_price"`
	OutputPrice     *int64 `json:"output_price"`
	CacheReadPrice  *int64 `json:"cache_read_price"`
	CacheWritePrice *int64 `json:"cache_write_price"`
}

// Seed applies the embedded price seed to ai_model_prices table, replacing the
// price columns of any existing (provider, model) row and inserting new ones.
// Rows already in the table that no longer appear in the seed are left
// untouched, so historical entries persist across upstream model deprecations.
func Seed(ctx context.Context, db database.Store) error {
	return SeedFromBytes(ctx, db, seedJSON)
}

// SeedFromBytes applies an arbitrary JSON seed. Most callers should use Seed,
// which applies the seed embedded in this binary; SeedFromBytes is exposed
// for tests that need to inject a deterministic seed.
func SeedFromBytes(ctx context.Context, db database.Store, data []byte) error {
	rows, err := parseSeed(data)
	if err != nil {
		return xerrors.Errorf("parse price seed: %w", err)
	}
	if len(rows) == 0 {
		return xerrors.New("price seed is empty")
	}
	return db.UpsertAIModelPrices(ctx, data)
}

func parseSeed(data []byte) ([]seedRow, error) {
	var rows []seedRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// modelKey identifies a priced model.
type modelKey struct {
	provider string
	model    string
}

// defaultPricedModels indexes the embedded price book by provider and model.
// Built on first use, since a deployment that never sets a price never needs
// it.
var defaultPricedModels = sync.OnceValue(func() map[modelKey]struct{} {
	rows, err := parseSeed(seedJSON)
	if err != nil {
		panic(xerrors.Errorf("parse embedded price seed: %w", err))
	}
	index := make(map[modelKey]struct{}, len(rows))
	for _, row := range rows {
		index[modelKey{provider: row.Provider, model: row.Model}] = struct{}{}
	}
	return index
})

// IsDefaultPriced reports whether the embedded price book already carries a
// price for the model. Coder owns those prices and re-applies them on every
// startup, so an operator price set for one would not survive a restart.
func IsDefaultPriced(provider, model string) bool {
	_, ok := defaultPricedModels()[modelKey{provider: provider, model: model}]
	return ok
}
