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
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"strings"

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
	// Tiers are long-context prices. context_over_200k repeats the first
	// tier under a wrong name for 272K models, so the generator ignores it.
	Tiers []upstreamTier `json:"tiers"`
}

// upstreamTier holds the prices for a request whose prompt is over Tier.Size
// tokens.
type upstreamTier struct {
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
	Tier       struct {
		Type string `json:"type"`
		Size int64  `json:"size"`
	} `json:"tier"`
}

// claude1hMultiplier is the 1h cache write price as a multiple of input.
// Anthropic and Bedrock bill a 1h write at 2x input. models.dev has no field
// for it.
const claude1hMultiplier = 2

// messagesProviders use the Messages path, the only path that reports the
// 1h part of cache writes.
var messagesProviders = []string{"anthropic", "bedrock", "copilot"}

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
			tiers, err := convertTiers(providerID, modelID, m.Cost.Tiers)
			if err != nil {
				return nil, xerrors.Errorf("convert tiers of %s/%s: %w", providerID, modelID, err)
			}
			rows = append(rows, pricebook.Row{
				Provider:          providerID,
				Model:             modelID,
				InputPrice:        toMicros(m.Cost.Input),
				OutputPrice:       toMicros(m.Cost.Output),
				CacheReadPrice:    toMicros(m.Cost.CacheRead),
				CacheWritePrice:   toMicros(m.Cost.CacheWrite),
				CacheWrite1hPrice: claude1hPrice(providerID, modelID, toMicros(m.Cost.Input)),
				Tiers:             tiers,
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

// claude1hPrice returns the 1h cache write price for a Claude row on a
// provider that uses the Messages path, and nil for other rows.
func claude1hPrice(providerID, modelID string, input *int64) *int64 {
	if input == nil || !strings.Contains(modelID, "claude") {
		return nil
	}
	if !slices.Contains(messagesProviders, providerID) {
		return nil
	}
	price := *input * claude1hMultiplier
	return &price
}

// convertTiers converts the long-context tiers of one model, sorted by
// threshold. It fails on a tier type other than "context" or a size that is
// not positive, since either would price requests wrongly.
func convertTiers(providerID, modelID string, upstream []upstreamTier) ([]pricebook.Tier, error) {
	if len(upstream) == 0 {
		return nil, nil
	}
	tiers := make([]pricebook.Tier, 0, len(upstream))
	for _, t := range upstream {
		if t.Tier.Type != "context" {
			return nil, xerrors.Errorf("unknown tier type %q", t.Tier.Type)
		}
		if t.Tier.Size <= 0 {
			return nil, xerrors.Errorf("tier size %d is not positive", t.Tier.Size)
		}
		input := toMicros(t.Input)
		tiers = append(tiers, pricebook.Tier{
			ThresholdTokens:   t.Tier.Size,
			InputPrice:        input,
			OutputPrice:       toMicros(t.Output),
			CacheReadPrice:    toMicros(t.CacheRead),
			CacheWritePrice:   toMicros(t.CacheWrite),
			CacheWrite1hPrice: claude1hPrice(providerID, modelID, input),
		})
	}
	slices.SortFunc(tiers, func(a, b pricebook.Tier) int {
		return cmp.Compare(a.ThresholdTokens, b.ThresholdTokens)
	})
	for i := 1; i < len(tiers); i++ {
		if tiers[i].ThresholdTokens == tiers[i-1].ThresholdTokens {
			return nil, xerrors.Errorf("duplicate tier size %d", tiers[i].ThresholdTokens)
		}
	}
	return tiers, nil
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
