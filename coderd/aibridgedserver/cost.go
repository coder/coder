package aibridgedserver

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridge/budget"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// tokensPerMillion is the divisor for prices, which are quoted per million
// tokens.
const tokensPerMillion = 1_000_000

// tokenUsageCost holds the cost-attribution columns snapshotted onto a token
// usage record. A field left unset (Valid == false) is recorded as SQL NULL; a
// price or cost of 0 is recorded as 0, which is distinct from NULL.
type tokenUsageCost struct {
	effectiveGroupID      uuid.NullUUID
	spendLimitMicros      sql.NullInt64
	limitSource           codersdk.AIBudgetLimitSource
	inputPriceMicros      sql.NullInt64
	outputPriceMicros     sql.NullInt64
	cacheReadPriceMicros  sql.NullInt64
	cacheWritePriceMicros sql.NullInt64
	costMicros            sql.NullInt64
}

// resolveTokenUsageCost resolves the effective group and per-token prices for an
// interception and computes its cost. Three independent conditions yield a NULL
// column rather than an error: an unresolved effective group (the user has no
// org membership), an interception whose provider name matches no configured
// provider, and a model absent from the price table. The latter two leave prices
// and cost NULL (a NULL cost unambiguously means "model not priced").
// Any other error is returned.
func (s *Server) resolveTokenUsageCost(ctx context.Context, intc database.AIBridgeInterception, in *proto.RecordTokenUsageRequest) (tokenUsageCost, error) {
	var result tokenUsageCost

	// Resolve the effective group for attribution, independent of whether the
	// model is priced.
	effectiveGroup, ok, err := budget.ResolveUserEffectiveGroup(ctx, s.store, intc.InitiatorID, s.budgetPolicy)
	if err != nil {
		return tokenUsageCost{}, xerrors.Errorf("resolve effective AI group for user %q with policy %q: %w", intc.InitiatorID, s.budgetPolicy, err)
	}
	if !ok {
		// A user should always resolve to at least their Everyone group, so log
		// this unexpected case. Spend is still recorded, with a NULL group.
		s.logger.Warn(ctx, "no effective group for user, AI spend not attributed",
			slog.F("user_id", intc.InitiatorID))
	} else {
		result.effectiveGroupID = uuid.NullUUID{UUID: effectiveGroup.GroupID, Valid: true}
		// Limit is nil for the unlimited Everyone fallback; only a budgeted
		// group carries the spend limit and its source.
		if effectiveGroup.Limit != nil {
			result.spendLimitMicros = sql.NullInt64{Int64: effectiveGroup.Limit.SpendLimitMicros, Valid: true}
			result.limitSource = effectiveGroup.Limit.Source
		}
	}

	// The interception records one of three upstream wire formats. Prices are
	// keyed on the configured provider type, the provider actually serving the
	// request, resolved by provider name. Names are unique among live providers.
	provider, err := s.store.GetAIProviderByName(ctx, intc.ProviderName)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Only reachable if the provider was deleted mid-request.
		s.logger.Info(ctx, "no configured provider found for interception, recording token usage with NULL cost",
			slog.F("provider_name", intc.ProviderName), slog.F("model", intc.Model))
		if s.metrics != nil {
			s.metrics.UnpricedTokenUsageRecords.WithLabelValues(intc.ProviderName, intc.Model).Inc()
		}
		return result, nil
	case err != nil:
		return tokenUsageCost{}, xerrors.Errorf("get configured provider %q: %w", intc.ProviderName, err)
	}
	configuredType := string(provider.Type)

	// Snapshot the price for this (provider, model) and compute cost.
	price, err := s.store.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
		Provider: configuredType,
		Model:    intc.Model,
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Model not in the price table: record tokens but leave cost NULL.
		s.logger.Info(ctx, "no price found for model, recording token usage with NULL cost",
			slog.F("provider", configuredType), slog.F("model", intc.Model))
		if s.metrics != nil {
			s.metrics.UnpricedTokenUsageRecords.WithLabelValues(configuredType, intc.Model).Inc()
		}
		return result, nil
	case err != nil:
		return tokenUsageCost{}, xerrors.Errorf("look up model price for %s/%s: %w", configuredType, intc.Model, err)
	}

	result.inputPriceMicros = price.InputPrice
	result.outputPriceMicros = price.OutputPrice
	result.cacheReadPriceMicros = price.CacheReadPrice
	result.cacheWritePriceMicros = price.CacheWritePrice
	result.costMicros = sql.NullInt64{
		Int64: computeCost(price,
			in.GetInputTokens(), in.GetOutputTokens(),
			in.GetCacheReadInputTokens(), in.GetCacheWriteInputTokens()),
		Valid: true,
	}
	return result, nil
}

// computeCost returns the cost of an interception in micro-units, snapshotting
// the per-token prices from the price table. Prices are expressed per million
// tokens; a NULL price column is treated as zero (e.g. providers that do not
// charge for cache writes).
func computeCost(price database.AIModelPrice, inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int64) int64 {
	return tokenCost(inputTokens, price.InputPrice) +
		tokenCost(outputTokens, price.OutputPrice) +
		tokenCost(cacheReadTokens, price.CacheReadPrice) +
		tokenCost(cacheWriteTokens, price.CacheWritePrice)
}

// tokenCost returns tokens * price / 1,000,000, treating a NULL price as zero.
func tokenCost(tokens int64, pricePerMillion sql.NullInt64) int64 {
	if !pricePerMillion.Valid {
		return 0
	}
	return tokens * pricePerMillion.Int64 / tokensPerMillion
}
