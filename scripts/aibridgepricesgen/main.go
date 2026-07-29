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
	"io"
	"os"
	"slices"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/modelprices"
)

// supportedProviders lists the providers we ship prices for. Adding a
// provider here is enough to include it on the next regeneration.
var supportedProviders = []string{"anthropic", "openai"}

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

	data, err := os.ReadFile(upstreamPath)
	if err != nil {
		return xerrors.Errorf("read %s: %w", upstreamPath, err)
	}
	var upstream map[string]modelprices.UpstreamProvider
	if err := json.Unmarshal(data, &upstream); err != nil {
		return xerrors.Errorf("parse: %w", err)
	}
	if format == "catalog" {
		return runCatalog(upstream)
	}
	return runPrices(data, upstream)
}

// runPrices transforms the full upstream snapshot (all providers) and then
// filters down to supportedProviders, so the shared Transform stays
// provider-agnostic and the generator owns the shipping decision.
func runPrices(data []byte, upstream map[string]modelprices.UpstreamProvider) error {
	rows, err := runPricesRows(data, upstream)
	if err != nil {
		return err
	}
	if err := write(os.Stdout, rows); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stderr, "aibridgepricesgen: wrote %d prices for %d provider(s)\n", len(rows), len(supportedProviders))
	return nil
}

// runPricesRows is the testable core of runPrices: it transforms, filters to
// supportedProviders, rejects missing providers, and validates. It does not
// write.
func runPricesRows(data []byte, upstream map[string]modelprices.UpstreamProvider) ([]modelprices.PriceRow, error) {
	warnNegativePrices(upstream)
	rows, err := modelprices.Transform(data)
	if err != nil {
		return nil, err
	}
	// Every supported provider must appear in the upstream payload and have
	// at least one priced model, otherwise we'd ship an incomplete seed.
	missing := missingProviders(upstream, supportedProviders)
	if len(missing) > 0 {
		return nil, xerrors.Errorf("providers missing or empty in upstream: %v", missing)
	}
	rows = filterProviders(rows, supportedProviders)
	if err := validate(rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func runCatalog(upstream map[string]modelprices.UpstreamProvider) error {
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

// missingProviders returns the supported provider IDs that are absent from
// the upstream payload or have no models.
func missingProviders(upstream map[string]modelprices.UpstreamProvider, providers []string) []string {
	var missing []string
	for _, providerID := range providers {
		provider, ok := upstream[providerID]
		if !ok || len(provider.Models) == 0 {
			missing = append(missing, providerID)
		}
	}
	return missing
}

// filterProviders keeps only rows whose provider is in the allowlist,
// preserving the (already sorted) row order.
func filterProviders(rows []modelprices.PriceRow, providers []string) []modelprices.PriceRow {
	out := make([]modelprices.PriceRow, 0, len(rows))
	for _, r := range rows {
		if slices.Contains(providers, r.Provider) {
			out = append(out, r)
		}
	}
	return out
}

// validate checks invariants on the converted rows. Catches upstream
// changes that produce structurally valid but semantically broken seed
// data, e.g. a renamed `cost` key that leaves every row with all-null
// prices.
func validate(rows []modelprices.PriceRow) error {
	for _, r := range rows {
		if r.InputPrice != nil || r.OutputPrice != nil {
			return nil
		}
	}
	return xerrors.New("converted rows have no pricing data; upstream schema may have changed")
}

// warnNegativePrices prints a stderr warning for every negative upstream cost
// field. modelprices.Transform silently drops negatives (treating them as
// missing); this restores the visibility the generator's old toMicros provided.
func warnNegativePrices(upstream map[string]modelprices.UpstreamProvider) {
	for providerID, provider := range upstream {
		for modelID, m := range provider.Models {
			if m.Cost == nil {
				continue
			}
			for _, pc := range []struct {
				name  string
				value *float64
			}{
				{"input", m.Cost.Input},
				{"output", m.Cost.Output},
				{"cache_read", m.Cost.CacheRead},
				{"cache_write", m.Cost.CacheWrite},
			} {
				if pc.value != nil && *pc.value < 0 {
					_, _ = fmt.Fprintf(os.Stderr, "warning: negative %s price %f for %s/%s, treating as missing\n", pc.name, *pc.value, providerID, modelID)
				}
			}
		}
	}
}

func write(w io.Writer, rows []modelprices.PriceRow) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rows); err != nil {
		return xerrors.Errorf("encode: %w", err)
	}
	return nil
}
