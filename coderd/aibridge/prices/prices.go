// Package prices loads the embedded AI Bridge model price seed into the
// ai_model_prices table at server startup.
package prices

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

//go:embed data/prices.json
var seedJSON []byte

// seedRow mirrors the JSON shape produced by scripts/aibridgepricesgen.
// Pointer fields preserve the distinction between "not populated by upstream"
// (null) and "explicitly zero" (0) so we can write SQL NULL where appropriate.
type seedRow struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	InputPrice      *int64 `json:"input_price"`
	OutputPrice     *int64 `json:"output_price"`
	CacheReadPrice  *int64 `json:"cache_read_price"`
	CacheWritePrice *int64 `json:"cache_write_price"`
}

// Load applies the embedded price seed to ai_model_prices, replacing the
// price columns of any existing (provider, model) row and inserting new ones.
// Rows already in the table that no longer appear in the seed are left
// untouched, so historical entries persist across upstream model deprecations.
//
// The whole batch runs inside a single transaction: either every row lands or
// none do, so a failure mid-load can't leave the table half-updated.
func Load(ctx context.Context, db database.Store) error {
	return LoadFromBytes(ctx, db, seedJSON)
}

// LoadFromBytes applies an arbitrary JSON seed. Most callers should use Load,
// which applies the seed embedded in this binary; LoadFromBytes is exposed
// for tests that need to inject a deterministic seed.
func LoadFromBytes(ctx context.Context, db database.Store, data []byte) error {
	rows, err := parseSeed(data)
	if err != nil {
		return xerrors.Errorf("parse price seed: %w", err)
	}
	if len(rows) == 0 {
		return xerrors.New("price seed is empty")
	}

	return db.InTx(func(tx database.Store) error {
		for _, r := range rows {
			err := tx.UpsertAIModelPrice(ctx, database.UpsertAIModelPriceParams{
				Provider:        r.Provider,
				Model:           r.Model,
				InputPrice:      nullInt64(r.InputPrice),
				OutputPrice:     nullInt64(r.OutputPrice),
				CacheReadPrice:  nullInt64(r.CacheReadPrice),
				CacheWritePrice: nullInt64(r.CacheWritePrice),
			})
			if err != nil {
				return xerrors.Errorf("upsert %s/%s: %w", r.Provider, r.Model, err)
			}
		}
		return nil
	}, nil)
}

func parseSeed(data []byte) ([]seedRow, error) {
	var rows []seedRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func nullInt64(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}
