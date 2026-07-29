// Package modelprices converts a models.dev pricing snapshot into the
// wire-format rows used by the AI Gateway cost-control loader and the
// experimental `coder exp model-prices import` command.
//
// The upstream JSON shape is provider → model → cost, where cost fields are
// float64 USD-per-million-token prices. Transform flattens that into a
// sorted slice of PriceRow, converting each price to int64 micro-units
// (1 unit = 1,000,000) via rounding. Pointer fields preserve the
// distinction between "not populated by upstream" (nil) and "explicitly
// zero" (0). A model is skipped only when none of its cost fields are
// populated (all nil).
package modelprices

import (
	"encoding/json"
	"math"
	"sort"

	"golang.org/x/xerrors"
)

// UpstreamProvider is the subset of a models.dev per-provider entry we read.
type UpstreamProvider struct {
	Models map[string]UpstreamModel `json:"models"`
}

// UpstreamModel is a single model entry within a provider.
type UpstreamModel struct {
	Name  string        `json:"name"`
	Limit UpstreamLimit `json:"limit"`
	Cost  *UpstreamCost `json:"cost"`
}

// Pointer fields in UpstreamLimit and UpstreamCost distinguish "key absent"
// (nil) from "key present and zero" (0).
type UpstreamLimit struct {
	Context *int64 `json:"context"`
	Output  *int64 `json:"output"`
}

type UpstreamCost struct {
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
}

// HasPricing reports whether the cost block has at least one populated price.
// Returns false for a nil receiver, so callers can pass m.Cost without a
// preceding nil check.
func (c *UpstreamCost) HasPricing() bool {
	if c == nil {
		return false
	}
	return c.Input != nil || c.Output != nil ||
		c.CacheRead != nil || c.CacheWrite != nil
}

// PriceRow is one wire-format row of the cost-control price seed.
//
// Pointer fields preserve the distinction between "not populated by upstream"
// (null) and "explicitly zero" (0).
type PriceRow struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	InputPrice      *int64 `json:"input_price"`
	OutputPrice     *int64 `json:"output_price"`
	CacheReadPrice  *int64 `json:"cache_read_price"`
	CacheWritePrice *int64 `json:"cache_write_price"`
}

// Transform parses a models.dev api.json snapshot, flattens the
// provider → model → cost tree into rows, converts float USD prices to int64
// micro-units, skips models with no populated cost fields, and sorts the
// result by (provider, model). It processes ALL providers; filtering to a
// subset is the caller's responsibility.
func Transform(data []byte) ([]PriceRow, error) {
	var upstream map[string]UpstreamProvider
	if err := json.Unmarshal(data, &upstream); err != nil {
		return nil, xerrors.Errorf("parse models.dev JSON: %w", err)
	}

	var rows []PriceRow
	for providerID, provider := range upstream {
		for modelID, m := range provider.Models {
			if !m.Cost.HasPricing() {
				continue
			}
			rows = append(rows, PriceRow{
				Provider:        providerID,
				Model:           modelID,
				InputPrice:      toMicros(m.Cost.Input),
				OutputPrice:     toMicros(m.Cost.Output),
				CacheReadPrice:  toMicros(m.Cost.CacheRead),
				CacheWritePrice: toMicros(m.Cost.CacheWrite),
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Provider != rows[j].Provider {
			return rows[i].Provider < rows[j].Provider
		}
		return rows[i].Model < rows[j].Model
	})
	return rows, nil
}

// toMicros scales a price into integer micro-units (1 unit = 1,000,000),
// rounding to avoid float-truncation errors. Returns nil for nil input, and
// for negative values, which are treated as missing.
func toMicros(price *float64) *int64 {
	if price == nil {
		return nil
	}
	if *price < 0 {
		return nil
	}
	micros := int64(math.Round(*price * 1_000_000))
	return &micros
}
