// aibridgepricesgen converts a models.dev api.json snapshot into generated
// artifacts, selected by -format:
//
//   - "prices": a JSON seed file consumable by the AI Gateway cost-control
//     loader, sorted by (provider, model) so regenerations produce minimal
//     diffs.
//   - "catalog": the frontend known-models JSON, joining the snapshot with
//     the editorial curation in curation.json and preserving its entry order.
//
// Run via the gen/aibridge-prices Make target, which fetches and patches the
// snapshot (_gen/models-dev.json). Kept out of `make gen` because the output
// depends on live upstream data; refreshing prices should land in dedicated,
// reviewable commits rather than appearing as drift on unrelated gen runs.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge/prices/pricebook"
	"github.com/coder/coder/v2/coderd/aibridge/prices/providers"
)

// upstreamProvider is the subset of a models.dev per-provider entry we read.
type upstreamProvider struct {
	Models map[string]upstreamModel `json:"models"`
}

type upstreamModel struct {
	Name  string        `json:"name"`
	Limit upstreamLimit `json:"limit"`
	Cost  *upstreamCost `json:"cost"`
}

// Pointer fields in upstreamLimit and upstreamCost distinguish "key absent"
// (nil) from "key present and zero" (0).
type upstreamLimit struct {
	Context *int64 `json:"context"`
	Output  *int64 `json:"output"`
}

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

func main() {
	format := flag.String("format", "prices", `output format: "prices" (cost-control seed) or "catalog" (frontend known-models JSON)`)
	upstreamPath := flag.String("upstream", "", "path to a models.dev api.json snapshot (required)")
	flag.Parse()
	if err := run(*format, *upstreamPath); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "aibridgepricesgen: %v\n", err)
		os.Exit(1)
	}
}

func run(format, upstreamPath string) error {
	// Validate flags before touching the filesystem so a typo fails fast.
	switch format {
	case "prices", "catalog":
	default:
		return xerrors.Errorf(`unknown -format %q (want "prices" or "catalog")`, format)
	}
	if upstreamPath == "" {
		return xerrors.New("-upstream is required; run via `make gen/aibridge-prices`, which fetches and patches the snapshot")
	}

	upstream, err := readUpstream(upstreamPath)
	if err != nil {
		return xerrors.Errorf("read %s: %w", upstreamPath, err)
	}
	if format == "catalog" {
		return runCatalog(upstream)
	}
	return runPrices(upstream)
}

// readUpstream loads a models.dev api.json snapshot from disk, typically the
// Makefile's _gen/models-dev.json (fetched once and patched by overrides.jq).
func readUpstream(path string) (map[string]upstreamProvider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var upstream map[string]upstreamProvider
	if err := json.Unmarshal(data, &upstream); err != nil {
		return nil, xerrors.Errorf("parse: %w", err)
	}
	return upstream, nil
}

func runPrices(upstream map[string]upstreamProvider) error {
	rows, err := convert(upstream, providers.SupportedStrings())
	if err != nil {
		return err
	}
	if err := validate(rows); err != nil {
		return err
	}
	if err := pricebook.Write(os.Stdout, rows); err != nil {
		return xerrors.Errorf("encode: %w", err)
	}
	_, _ = fmt.Fprintf(os.Stderr, "aibridgepricesgen: wrote %d prices for %d provider(s)\n", len(rows), len(providers.Supported))
	return nil
}

func runCatalog(upstream map[string]upstreamProvider) error {
	var curation map[string][]curatedModel
	if err := json.Unmarshal(curationJSON, &curation); err != nil {
		return xerrors.Errorf("parse embedded curation.json: %w", err)
	}
	catalog, err := buildCatalog(upstream, curation)
	if err != nil {
		return err
	}
	if err := writeCatalog(os.Stdout, catalog); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stderr, "aibridgepricesgen: wrote catalog for %d provider(s)\n", len(catalog))
	return nil
}

// convert flattens the upstream map into table-shaped rows for the configured
// providers. If any configured provider is absent from the upstream payload,
// every missing provider is reported and the function returns an error so the
// caller doesn't ship an incomplete seed.
func convert(upstream map[string]upstreamProvider, providerIDs []string) ([]pricebook.Row, error) {
	var (
		rows    []pricebook.Row
		missing []string
	)
	for _, providerID := range providerIDs {
		provider, ok := upstream[providerID]
		if !ok || len(provider.Models) == 0 {
			missing = append(missing, providerID)
			continue
		}
		for modelID, m := range provider.Models {
			if !m.Cost.hasPricing() {
				continue
			}
			rows = append(rows, pricebook.Row{
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
		return nil, xerrors.Errorf("providers missing or empty in upstream: %v", missing)
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
func validate(rows []pricebook.Row) error {
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
