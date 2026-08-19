// Package prices seeds the ai_model_prices table from an embedded JSON
// price book at server startup.
package prices

import (
	"context"
	_ "embed"
	"sync"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge/prices/pricebook"
	"github.com/coder/coder/v2/coderd/database"
)

//go:embed data/prices.json
var seedJSON []byte

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
	rows, err := pricebook.Parse(data)
	if err != nil {
		return xerrors.Errorf("parse price seed: %w", err)
	}
	if len(rows) == 0 {
		return xerrors.New("price seed is empty")
	}
	return db.UpsertAIModelPrices(ctx, data)
}

// defaultPricedModels indexes the embedded price book by provider and model.
// Built on first use, since a deployment that never sets a price never needs
// it.
var defaultPricedModels = sync.OnceValue(func() map[pricebook.Key]struct{} {
	rows, err := pricebook.Parse(seedJSON)
	if err != nil {
		panic(xerrors.Errorf("parse embedded price seed: %w", err))
	}
	index := make(map[pricebook.Key]struct{}, len(rows))
	for _, row := range rows {
		index[row.Key()] = struct{}{}
	}
	return index
})

// IsDefaultPriced reports whether the embedded price book already carries a
// price for the model. Coder owns those prices and re-applies them on every
// startup, so an operator price set for one would not survive a restart.
func IsDefaultPriced(provider, model string) bool {
	_, ok := defaultPricedModels()[pricebook.Key{Provider: provider, Model: model}]
	return ok
}
