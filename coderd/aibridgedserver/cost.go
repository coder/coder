package aibridgedserver

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridge/budget"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// maxAllowedTokenUsage bounds the token count an interception may report per
// category. A 1M-token context is the current frontier, so this leaves six
// orders of magnitude of headroom.
const maxAllowedTokenUsage int64 = 1_000_000_000_000

var (
	// tokensPerMillion is the divisor for prices, which are quoted per million
	// tokens.
	tokensPerMillion = decimal.NewFromInt(1_000_000)
	// maxCostMicros bounds one interception's cost at $10M.
	maxCostMicros = decimal.NewFromInt(10_000_000_000_000)
)

// errTokenUsageOutOfRange reports a token count outside [0, maxAllowedTokenUsage].
var errTokenUsageOutOfRange = xerrors.New("reported token usage is out of range")

// errCostOutOfRange reports a cost outside [0, maxCostMicros]. Real
// usage cannot reach it, so it means a wrong price row or implausible
// provider-reported token counts.
var errCostOutOfRange = xerrors.New("computed cost is out of range")

// validateTokenUsage rejects an interception whose reported token counts fall
// outside [0, maxAllowedTokenUsage].
func validateTokenUsage(in *proto.RecordTokenUsageRequest) error {
	for _, category := range []struct {
		name  string
		count int64
	}{
		{"input_tokens", in.GetInputTokens()},
		{"output_tokens", in.GetOutputTokens()},
		{"cache_read_input_tokens", in.GetCacheReadInputTokens()},
		{"cache_write_input_tokens", in.GetCacheWriteInputTokens()},
	} {
		if category.count < 0 || category.count > maxAllowedTokenUsage {
			return xerrors.Errorf("%s is %d, outside [0, %d]: %w",
				category.name, category.count, maxAllowedTokenUsage, errTokenUsageOutOfRange)
		}
	}
	return nil
}

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
// interception and computes its cost. Four independent conditions yield a NULL
// column rather than an error: an unresolved effective group (the user has no
// org membership), an interception whose provider name matches no configured
// provider, a model absent from the price table, and a cost outside the
// maxCostMicros range. A NULL cost means the cost is unknown.
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

	costMicros, err := computeCost(price,
		in.GetInputTokens(), in.GetOutputTokens(),
		in.GetCacheReadInputTokens(), in.GetCacheWriteInputTokens())
	if err != nil {
		// No trustworthy cost exists, so record it as unknown rather than
		// storing a figure derived from bad inputs.
		s.logger.Error(ctx, "cost out of range, recording token usage with NULL cost",
			slog.F("interception_id", intc.ID),
			slog.F("initiator_id", intc.InitiatorID),
			slog.F("provider", intc.Provider), slog.F("model", intc.Model),
			slog.F("input_tokens", in.GetInputTokens()),
			slog.F("output_tokens", in.GetOutputTokens()),
			slog.F("cache_read_input_tokens", in.GetCacheReadInputTokens()),
			slog.F("cache_write_input_tokens", in.GetCacheWriteInputTokens()),
			slog.Error(err))
		return result, nil
	}
	result.costMicros = sql.NullInt64{Int64: costMicros, Valid: true}
	return result, nil
}

// computeCost returns the cost of an interception in micro-units, snapshotting
// the per-token prices from the price table. Prices are expressed per million
// tokens; a NULL price column is treated as zero (e.g. providers that do not
// charge for cache writes).
func computeCost(price database.AIModelPrice, inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int64) (int64, error) {
	total := tokenCost(inputTokens, price.InputPrice).
		Add(tokenCost(outputTokens, price.OutputPrice)).
		Add(tokenCost(cacheReadTokens, price.CacheReadPrice)).
		Add(tokenCost(cacheWriteTokens, price.CacheWritePrice))

	if err := validateTotalCost(total); err != nil {
		return 0, err
	}
	return total.IntPart(), nil
}

// validateTotalCost rejects a computed cost outside [0, maxCostMicros].
//
// Rejecting the negative case early keeps it from reaching the
// cost_micros >= 0 check constraint, which would discard the whole record.
func validateTotalCost(total decimal.Decimal) error {
	if total.IsNegative() || total.GreaterThan(maxCostMicros) {
		return xerrors.Errorf("cost %s micro-units: %w", total.String(), errCostOutOfRange)
	}
	return nil
}

// tokenCost returns tokens * price / 1,000,000, treating a NULL price as zero.
//
// Each category is divided and truncated on its own, which makes a per-category breakdown
// recomputed from the snapshotted price columns add up to the stored cost.
func tokenCost(tokens int64, pricePerMillion sql.NullInt64) decimal.Decimal {
	if !pricePerMillion.Valid {
		return decimal.Zero
	}
	quotient, _ := decimal.NewFromInt(tokens).
		Mul(decimal.NewFromInt(pricePerMillion.Int64)).
		QuoRem(tokensPerMillion, 0)
	return quotient
}
