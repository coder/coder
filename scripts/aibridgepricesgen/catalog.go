package main

import (
	"cmp"
	_ "embed"
	"encoding/json"
	"io"

	"golang.org/x/xerrors"
)

// curationJSON is the checked-in editorial curation input for the frontend
// known-models catalog. Entry order within each provider controls suggestion
// order in the UI. Everything factual (display name, limits, pricing) is
// joined from models.dev at generation time; the curation file only carries
// editorial choices: which models to suggest, aliases, reasoning defaults,
// and overrides.
//
//go:embed curation.json
var curationJSON []byte

// curatedModel is one entry in curation.json.
type curatedModel struct {
	ModelIdentifier string   `json:"modelIdentifier"`
	Aliases         []string `json:"aliases"`
	// DisplayName overrides the upstream `name` when set. Needed where
	// upstream naming does not match what we want to show (for example
	// "Claude Haiku 4.5 (latest)").
	DisplayName string `json:"displayName"`
	// ReasoningEffort is editorial, not from models.dev. Mutually
	// exclusive with ThinkingBudgetTokens.
	ReasoningEffort string `json:"reasoningEffort"`
	// ThinkingBudgetTokens is Anthropic-only, for models that do not
	// support adaptive thinking and use the legacy
	// `thinking.budget_tokens` API instead.
	ThinkingBudgetTokens int `json:"thinkingBudgetTokens"`
}

// catalogEntry matches the frontend KnownModel shape (knownModels/types.ts).
// Costs are flat USD per million tokens, straight from models.dev; tiered
// pricing such as context_over_200k is intentionally omitted.
type catalogEntry struct {
	Provider             string   `json:"provider"`
	ModelIdentifier      string   `json:"modelIdentifier"`
	DisplayName          string   `json:"displayName"`
	Aliases              []string `json:"aliases"`
	ContextLimit         *int64   `json:"contextLimit,omitempty"`
	MaxOutputTokens      *int64   `json:"maxOutputTokens,omitempty"`
	ReasoningEffort      string   `json:"reasoningEffort,omitempty"`
	ThinkingBudgetTokens int      `json:"thinkingBudgetTokens,omitempty"`
	InputCost            *float64 `json:"inputCost,omitempty"`
	OutputCost           *float64 `json:"outputCost,omitempty"`
	CacheReadCost        *float64 `json:"cacheReadCost,omitempty"`
	CacheWriteCost       *float64 `json:"cacheWriteCost,omitempty"`
}

// validReasoningEfforts are the values accepted for curatedModel.ReasoningEffort.
var validReasoningEfforts = map[string]bool{"low": true, "medium": true, "high": true}

// buildCatalog joins the curation file with the upstream models.dev payload
// and returns provider-keyed ordered entry lists.
func buildCatalog(upstream map[string]upstreamProvider, curation map[string][]curatedModel) (map[string][]catalogEntry, error) {
	out := make(map[string][]catalogEntry, len(curation))
	for providerID, curated := range curation {
		provider, ok := upstream[providerID]
		if !ok {
			return nil, xerrors.Errorf("provider %q missing from upstream", providerID)
		}
		seenIdentifiers := make(map[string]bool, len(curated))
		seenAliases := make(map[string]bool)
		entries := make([]catalogEntry, 0, len(curated))
		for _, c := range curated {
			if c.ModelIdentifier == "" {
				return nil, xerrors.Errorf("provider %q: entry with empty modelIdentifier", providerID)
			}
			if seenIdentifiers[c.ModelIdentifier] {
				return nil, xerrors.Errorf("provider %q: duplicate modelIdentifier %q", providerID, c.ModelIdentifier)
			}
			seenIdentifiers[c.ModelIdentifier] = true
			if c.ReasoningEffort != "" && !validReasoningEfforts[c.ReasoningEffort] {
				return nil, xerrors.Errorf(`%s/%s: reasoningEffort %q is not one of "low", "medium", "high"`, providerID, c.ModelIdentifier, c.ReasoningEffort)
			}
			if c.ThinkingBudgetTokens < 0 {
				return nil, xerrors.Errorf("%s/%s: thinkingBudgetTokens %d is negative", providerID, c.ModelIdentifier, c.ThinkingBudgetTokens)
			}
			if c.ReasoningEffort != "" && c.ThinkingBudgetTokens != 0 {
				return nil, xerrors.Errorf("%s/%s: reasoningEffort and thinkingBudgetTokens are mutually exclusive", providerID, c.ModelIdentifier)
			}
			for _, alias := range c.Aliases {
				if alias == "" {
					return nil, xerrors.Errorf("%s/%s: empty-string alias", providerID, c.ModelIdentifier)
				}
				if seenAliases[alias] {
					return nil, xerrors.Errorf("%s/%s: alias %q declared more than once in provider", providerID, c.ModelIdentifier, alias)
				}
				seenAliases[alias] = true
			}
			m, ok := provider.Models[c.ModelIdentifier]
			if !ok {
				return nil, xerrors.Errorf("%s/%s: model missing from upstream (patch it in via overrides.jq if intentional)", providerID, c.ModelIdentifier)
			}
			if !m.Cost.hasPricing() {
				return nil, xerrors.Errorf("%s/%s: upstream model has no pricing data", providerID, c.ModelIdentifier)
			}
			if m.Limit.Context == nil || m.Limit.Output == nil {
				return nil, xerrors.Errorf("%s/%s: upstream model missing limit.context or limit.output", providerID, c.ModelIdentifier)
			}
			displayName := cmp.Or(c.DisplayName, m.Name)
			if displayName == "" {
				return nil, xerrors.Errorf("%s/%s: no displayName override and upstream name is empty", providerID, c.ModelIdentifier)
			}
			aliases := c.Aliases
			if aliases == nil {
				aliases = []string{}
			}
			entries = append(entries, catalogEntry{
				Provider:             providerID,
				ModelIdentifier:      c.ModelIdentifier,
				DisplayName:          displayName,
				Aliases:              aliases,
				ContextLimit:         m.Limit.Context,
				MaxOutputTokens:      m.Limit.Output,
				ReasoningEffort:      c.ReasoningEffort,
				ThinkingBudgetTokens: c.ThinkingBudgetTokens,
				InputCost:            m.Cost.Input,
				OutputCost:           m.Cost.Output,
				CacheReadCost:        m.Cost.CacheRead,
				CacheWriteCost:       m.Cost.CacheWrite,
			})
		}
		// An alias resolving to a canonical identifier would make exact-alias
		// lookup and canonical-id lookup disagree.
		for alias := range seenAliases {
			if seenIdentifiers[alias] {
				return nil, xerrors.Errorf("alias %q duplicates a modelIdentifier in provider %q", alias, providerID)
			}
		}
		out[providerID] = entries
	}
	return out, nil
}

func writeCatalog(w io.Writer, catalog map[string][]catalogEntry) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(catalog); err != nil {
		return xerrors.Errorf("encode: %w", err)
	}
	return nil
}
