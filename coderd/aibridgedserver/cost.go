package aibridgedserver

import (
	"context"
	"database/sql"
	"errors"
	"math"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridge/budget"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

var (
	// tokensPerMillion is the divisor for prices, which are quoted per million
	// tokens.
	tokensPerMillion = decimal.NewFromInt(1_000_000)
	// maxCostMicros is the largest cost the database column can hold.
	maxCostMicros = decimal.NewFromInt(math.MaxInt64)
)

// errCostOutOfRange reports a cost that cannot be stored in the database column.
// Real usage cannot reach it, so it means a wrong price row or implausible
// provider-reported token counts.
var errCostOutOfRange = xerrors.New("computed cost is out of range")

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
// interception and computes its cost. Three conditions yield a NULL column
// rather than an error: an unresolved effective group (the user has no org
// membership), a model absent from the price table, and a cost outside the
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

	// Snapshot the price for this (provider, model) and compute cost.
	price, err := s.store.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
		Provider: intc.Provider,
		Model:    intc.Model,
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Model not in the price table: record tokens but leave cost NULL.
		s.logger.Debug(ctx, "no price found for model, recording token usage with NULL cost",
			slog.F("provider", intc.Provider), slog.F("model", intc.Model))
		if s.metrics != nil {
			s.metrics.UnpricedTokenUsageRecords.WithLabelValues(intc.Provider, intc.Model).Inc()
		}
		return result, nil
	case err != nil:
		return tokenUsageCost{}, xerrors.Errorf("look up model price for %s/%s: %w", intc.Provider, intc.Model, err)
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
		// storing a figure derived from bad inputs. The token counts are logged
		// because the range error alone does not say which input was wrong.
		s.logger.Error(ctx, "cost out of range, recording token usage with NULL cost",
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

	// Rejecting the negative case here keeps it from reaching the
	// cost_micros >= 0 check constraint, which would discard the whole record.
	if total.IsNegative() || total.GreaterThan(maxCostMicros) {
		return 0, xerrors.Errorf("cost %s micro-units: %w", total.String(), errCostOutOfRange)
	}
	return total.IntPart(), nil
}

// tokenCost returns tokens * price / 1,000,000, treating a NULL price as zero.
func tokenCost(tokens int64, pricePerMillion sql.NullInt64) decimal.Decimal {
	if !pricePerMillion.Valid {
		return decimal.Zero
	}
	return decimal.NewFromInt(tokens).
		Mul(decimal.NewFromInt(pricePerMillion.Int64)).
		Div(tokensPerMillion).
		Truncate(0)
}
