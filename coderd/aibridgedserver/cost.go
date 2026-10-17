package aibridgedserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridge/budget"
	"github.com/coder/coder/v2/coderd/aibridge/prices/pricebook"
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

// unknownProviderType labels a metric whose provider did not resolve to a
// configured type.
const unknownProviderType = "unknown"

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
		{"cache_write_one_hour_input_tokens", in.GetCacheWriteOneHourInputTokens()},
		{"prompt_tokens", in.GetPromptTokens()},
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
	effectiveGroupID        uuid.NullUUID
	spendLimitMicros        sql.NullInt64
	limitSource             codersdk.AIBudgetLimitSource
	inputPriceMicros        sql.NullInt64
	outputPriceMicros       sql.NullInt64
	cacheReadPriceMicros    sql.NullInt64
	cacheWritePriceMicros   sql.NullInt64
	cacheWrite1hPriceMicros sql.NullInt64
	// priceTierThresholdTokens is the threshold of the long-context tier
	// whose prices applied, or NULL for the base prices.
	priceTierThresholdTokens sql.NullInt64
	costMicros               sql.NullInt64
	pricedModel              sql.NullString
}

// cacheWrite1h is part of cacheWrite, not an additional count.
type usageTokens struct {
	input, output, cacheRead, cacheWrite, cacheWrite1h, prompt int64
}

// The inserted row and its cost both read the counts through
// usageFromRequest, so they agree.
func usageFromRequest(in *proto.RecordTokenUsageRequest) usageTokens {
	u := usageTokens{
		input:      in.GetInputTokens(),
		output:     in.GetOutputTokens(),
		cacheRead:  in.GetCacheReadInputTokens(),
		cacheWrite: in.GetCacheWriteInputTokens(),
	}
	// A bad 1h count must not fail the check constraint and drop the row.
	u.cacheWrite1h = min(in.GetCacheWriteOneHourInputTokens(), u.cacheWrite)
	// A record carries at least its own input. An output-only record of a
	// stream carries the prompt size of its message in PromptTokens.
	u.prompt = max(in.GetPromptTokens(), u.input+u.cacheRead+u.cacheWrite)
	return u
}

// priceSet holds the per-million-token prices that apply to one usage
// record. A NULL price is treated as zero.
type priceSet struct {
	input, output, cacheRead, cacheWrite, cacheWrite1h sql.NullInt64
	// tierThreshold is the threshold of the applied tier, NULL for the base
	// prices.
	tierThreshold sql.NullInt64
}

// pricesForPrompt applies the highest tier below the prompt size, reading the
// tiers in their stored threshold order. An absent tier price uses the base
// price, and an absent 1h price uses the cache write price that applies.
func pricesForPrompt(row database.AIModelPrice, prompt int64) (priceSet, error) {
	set := priceSet{
		input:        row.InputPrice,
		output:       row.OutputPrice,
		cacheRead:    row.CacheReadPrice,
		cacheWrite:   row.CacheWritePrice,
		cacheWrite1h: row.CacheWrite1hPrice,
	}
	var tiers []pricebook.Tier
	if len(row.Tiers) > 0 {
		if err := json.Unmarshal(row.Tiers, &tiers); err != nil {
			return priceSet{}, xerrors.Errorf("decode tiers: %w", err)
		}
	}
	for _, t := range tiers {
		if prompt <= t.ThresholdTokens {
			break
		}
		set = priceSet{
			input:         priceOr(t.InputPrice, row.InputPrice),
			output:        priceOr(t.OutputPrice, row.OutputPrice),
			cacheRead:     priceOr(t.CacheReadPrice, row.CacheReadPrice),
			cacheWrite:    priceOr(t.CacheWritePrice, row.CacheWritePrice),
			cacheWrite1h:  priceOr(t.CacheWrite1hPrice, sql.NullInt64{}),
			tierThreshold: sql.NullInt64{Int64: t.ThresholdTokens, Valid: true},
		}
	}
	if !set.cacheWrite1h.Valid {
		set.cacheWrite1h = set.cacheWrite
	}
	return set, nil
}

func priceOr(price *int64, fallback sql.NullInt64) sql.NullInt64 {
	if price == nil {
		return fallback
	}
	return sql.NullInt64{Int64: *price, Valid: true}
}

// resolveTokenUsageCost resolves the effective group and per-token prices for an
// interception and computes its cost. Four independent conditions yield a NULL
// column rather than an error: an unresolved effective group (the user has no
// org membership), an interception whose provider name matches no configured
// provider, a requested and provider-reported model both absent from the price
// table, and a cost outside the maxCostMicros range. A NULL cost means the cost
// is unknown.
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
		s.logger.Warn(ctx, "no configured provider found for interception, recording token usage with NULL cost",
			slog.F("provider_name", intc.ProviderName),
			slog.F("model", intc.Model),
			slog.F("provider_model", in.GetProviderModel()))
		if s.metrics != nil {
			s.metrics.UnpricedTokenUsageRecords.WithLabelValues(intc.ProviderName, unknownProviderType, intc.Model, in.GetProviderModel()).Inc()
		}
		return result, nil
	case err != nil:
		return tokenUsageCost{}, xerrors.Errorf("get configured provider %q: %w", intc.ProviderName, err)
	}
	configuredType := string(provider.Type)

	// Snapshot the price for the requested model, falling back to the model
	// reported by the provider, and compute cost.
	price, err := s.store.GetAIModelPriceByProviderModel(ctx, database.GetAIModelPriceByProviderModelParams{
		Provider:      configuredType,
		Model:         intc.Model,
		ProviderModel: in.GetProviderModel(),
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Neither model is in the price table: record tokens but leave cost NULL.
		s.logger.Debug(ctx, "no price found for model, recording token usage with NULL cost",
			slog.F("provider", configuredType),
			slog.F("model", intc.Model),
			slog.F("provider_model", in.GetProviderModel()))
		if s.metrics != nil {
			s.metrics.UnpricedTokenUsageRecords.WithLabelValues(intc.ProviderName, configuredType, intc.Model, in.GetProviderModel()).Inc()
		}
		return result, nil
	case err != nil:
		return tokenUsageCost{}, xerrors.Errorf("look up model price for %s/%s: %w", configuredType, intc.Model, err)
	}

	// The row snapshots the prices that applied, so its per-category
	// breakdown still adds up to cost_micros.
	usage := usageFromRequest(in)
	prices, err := pricesForPrompt(price, usage.prompt)
	if err != nil {
		return tokenUsageCost{}, xerrors.Errorf("prices of %s/%s: %w", price.Provider, price.Model, err)
	}
	result.pricedModel = sql.NullString{String: price.Model, Valid: true}
	result.inputPriceMicros = prices.input
	result.outputPriceMicros = prices.output
	result.cacheReadPriceMicros = prices.cacheRead
	result.cacheWritePriceMicros = prices.cacheWrite
	result.cacheWrite1hPriceMicros = prices.cacheWrite1h
	result.priceTierThresholdTokens = prices.tierThreshold

	costMicros, err := computeCost(prices, usage)
	if err != nil {
		// No trustworthy cost exists, so record it as unknown rather than
		// storing a figure derived from bad inputs.
		s.logger.Error(ctx, "cost out of range, recording token usage with NULL cost",
			slog.F("interception_id", intc.ID),
			slog.F("initiator_id", intc.InitiatorID),
			slog.F("provider", intc.Provider),
			slog.F("model", intc.Model),
			slog.F("provider_model", in.GetProviderModel()),
			slog.F("priced_model", price.Model),
			slog.F("input_tokens", in.GetInputTokens()),
			slog.F("output_tokens", in.GetOutputTokens()),
			slog.F("cache_read_input_tokens", in.GetCacheReadInputTokens()),
			slog.F("cache_write_input_tokens", in.GetCacheWriteInputTokens()),
			slog.F("cache_write_one_hour_input_tokens", usage.cacheWrite1h),
			slog.F("prompt_tokens", usage.prompt),
			slog.Error(err))
		return result, nil
	}
	result.costMicros = sql.NullInt64{Int64: costMicros, Valid: true}
	return result, nil
}

// computeCost returns the cost in micro-units. Prices are per million tokens,
// and a NULL price counts as zero.
func computeCost(price priceSet, u usageTokens) (int64, error) {
	total := tokenCost(u.input, price.input).
		Add(tokenCost(u.output, price.output)).
		Add(tokenCost(u.cacheRead, price.cacheRead)).
		Add(tokenCost(u.cacheWrite-u.cacheWrite1h, price.cacheWrite)).
		Add(tokenCost(u.cacheWrite1h, price.cacheWrite1h))

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
