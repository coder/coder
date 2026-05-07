// aibridgepricesgen fetches model pricing from models.dev and writes a JSON
// seed file consumable by the AI Bridge cost-control loader. Output is sorted
// by (provider, model) so regenerations produce minimal diffs.
//
// Run via the gen/aibridge-prices Make target. Kept out of `make gen` because
// the output depends on live upstream data; refreshing prices should land in
// dedicated, reviewable commits rather than appearing as drift on unrelated
// gen runs.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"time"

	"golang.org/x/xerrors"
)

const (
	sourceURL    = "https://models.dev/api.json"
	fetchTimeout = 30 * time.Second
)

// supportedProviders lists the providers we ship prices for. Adding a
// provider here is enough to include it on the next regeneration.
var supportedProviders = []string{"anthropic", "openai"}

// upstreamProvider is the subset of a models.dev per-provider entry we read.
type upstreamProvider struct {
	Models map[string]upstreamModel `json:"models"`
}

type upstreamModel struct {
	Cost *upstreamCost `json:"cost"`
}

// Pointers distinguish "key absent" (nil) from "key present and zero" (0).
type upstreamCost struct {
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
}

// hasPricing reports whether the cost block has at least one populated price.
// Returns false for a nil receiver, so callers can pass m.Cost without a
// preceding nil check.
func (c *upstreamCost) hasPricing() bool {
	if c == nil {
		return false
	}
	return c.Input != nil || c.Output != nil ||
		c.CacheRead != nil || c.CacheWrite != nil
}

// priceRow is one ai_model_prices row in seed form. JSON tags match table
// column names so the loader can decode straight into INSERT params. Pointer
// fields preserve the distinction between "not populated by upstream" (null)
// and "explicitly zero" (0).
type priceRow struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	InputPrice      *int64 `json:"input_price"`
	OutputPrice     *int64 `json:"output_price"`
	CacheReadPrice  *int64 `json:"cache_read_price"`
	CacheWritePrice *int64 `json:"cache_write_price"`
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "aibridgepricesgen: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	upstream, err := fetch()
	if err != nil {
		return xerrors.Errorf("fetch %s: %w", sourceURL, err)
	}
	rows, err := convert(upstream, supportedProviders)
	if err != nil {
		return err
	}
	if err := validate(rows); err != nil {
		return err
	}
	if err := write(os.Stdout, rows); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stderr, "aibridgepricesgen: wrote %d prices for %d provider(s)\n", len(rows), len(supportedProviders))
	return nil
}

func fetch() (map[string]upstreamProvider, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, xerrors.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, xerrors.Errorf("read body: %w", err)
	}

	var data map[string]upstreamProvider
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, xerrors.Errorf("parse: %w", err)
	}
	return data, nil
}

// convert flattens the upstream map into table-shaped rows for the configured
// providers. If any configured provider is absent from the upstream payload,
// every missing provider is reported and the function returns an error so the
// caller doesn't ship an incomplete seed.
func convert(upstream map[string]upstreamProvider, providers []string) ([]priceRow, error) {
	var (
		rows    []priceRow
		missing []string
	)
	for _, providerID := range providers {
		provider, ok := upstream[providerID]
		if !ok {
			missing = append(missing, providerID)
			continue
		}
		for modelID, m := range provider.Models {
			if !m.Cost.hasPricing() {
				continue
			}
			rows = append(rows, priceRow{
				Provider:        providerID,
				Model:           modelID,
				InputPrice:      toMicros(m.Cost.Input),
				OutputPrice:     toMicros(m.Cost.Output),
				CacheReadPrice:  toMicros(m.Cost.CacheRead),
				CacheWritePrice: toMicros(m.Cost.CacheWrite),
			})
		}
	}
	if len(missing) > 0 {
		return nil, xerrors.Errorf("providers missing from upstream: %v", missing)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Provider != rows[j].Provider {
			return rows[i].Provider < rows[j].Provider
		}
		return rows[i].Model < rows[j].Model
	})
	return rows, nil
}

// validate checks invariants on the converted rows. Catches upstream
// changes that produce structurally valid but semantically broken seed
// data, e.g. a renamed `cost` key that leaves every row with all-null
// prices.
func validate(rows []priceRow) error {
	for _, r := range rows {
		if r.InputPrice != nil || r.OutputPrice != nil {
			return nil
		}
	}
	return xerrors.New("converted rows have no pricing data; upstream schema may have changed")
}

// toMicros scales a price into integer micro-units (1 unit = 1,000,000),
// rounding to avoid float-truncation errors. Returns nil for nil input, and
// for negative values, which are treated as missing.
func toMicros(price *float64) *int64 {
	if price == nil {
		return nil
	}
	if *price < 0 {
		_, _ = fmt.Fprintf(os.Stderr, "warning: negative price %f, treating as missing\n", *price)
		return nil
	}
	micros := int64(math.Round(*price * 1_000_000))
	return &micros
}

func write(w io.Writer, rows []priceRow) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rows); err != nil {
		return xerrors.Errorf("encode: %w", err)
	}
	return nil
}
