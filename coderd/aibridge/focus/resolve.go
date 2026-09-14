package focus

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/coder/coder/v2/coderd/database"
)

// vendorLabels maps a configured ai_providers.type to the canonical,
// human-readable vendor label used for ProviderName, ServiceName, and
// InvoiceIssuerName: the entity that actually gives access to the model,
// never a constant "Coder" identifier. openai-compat has no canonical label
// of its own and always falls back to the provider instance's display_name,
// or, when no provider row is available at all, to a title-cased wire
// protocol string (see VendorLabel).
var vendorLabels = map[database.AIProviderType]string{
	database.AIProviderTypeAnthropic:  "Anthropic",
	database.AIProviderTypeOpenai:     "OpenAI",
	database.AIProviderTypeAzure:      "Azure",
	database.AIProviderTypeBedrock:    "Bedrock",
	database.AIProviderTypeGoogle:     "Google",
	database.AIProviderTypeOpenrouter: "OpenRouter",
	database.AIProviderTypeVercel:     "Vercel",
	database.AIProviderTypeCopilot:    "GitHub Copilot",
}

// unversionedCatalogSuffix is appended to SkuPriceId in place of a real
// pricing-catalog version stamp, since the embedded catalog has no
// version/git-SHA today. See the Phase One Draft, SkuId / SkuPriceId.
const unversionedCatalogSuffix = "unversioned"

// vendorResolution is the outcome of resolving a row's ai_providers join.
type vendorResolution struct {
	// Label is the canonical vendor label (ProviderName / ServiceName /
	// InvoiceIssuerName).
	Label string
	// Publisher is the model's publisher, which may differ from Label for a
	// reseller vendor (e.g. Vercel serving a DeepSeek model).
	Publisher string
}

// resolveVendor resolves the canonical vendor label and publisher for a row.
// vendorType/displayName are nil when the provider join found no live
// ai_providers row (a soft-deleted or never-configured provider name); in
// that case wireProtocol is title-cased as a last-resort label.
func resolveVendor(vendorType *database.AIProviderType, displayName *string, wireProtocol, model string) vendorResolution {
	var label string
	switch {
	case vendorType == nil:
		// No live provider row. Fall back to the raw wire-protocol string,
		// title-cased. Expected mainly for a provider name an operator deleted
		// with no live replacement (Open Questions Q1, parent RFC), or an
		// interception recorded under a provider name that was never
		// registered in ai_providers. Since coderd no longer seeds ai_providers
		// from deployment env config (removed in #29053; providers are
		// configured exclusively through the admin API/UI now), this can occur
		// for any provider name that was never explicitly created there, not
		// just a narrow historical window.
		label = titleCase(wireProtocol)
	case *vendorType == database.AIProviderTypeOpenaiCompat:
		// openai-compat has no canonical vendor label; fall back to the
		// instance's admin-set display_name, else the same last-resort label.
		if displayName != nil && strings.TrimSpace(*displayName) != "" {
			label = *displayName
		} else {
			label = titleCase(wireProtocol)
		}
	default:
		if l, ok := vendorLabels[*vendorType]; ok {
			label = l
		} else {
			// Unknown type value (e.g. a future addition this map hasn't
			// caught up with yet). Fall back rather than emit an empty label.
			label = titleCase(string(*vendorType))
		}
	}

	publisher := resolvePublisher(vendorType, label, model)
	return vendorResolution{Label: label, Publisher: publisher}
}

// resolvePublisher parses the model string for vendors whose model strings
// are namespaced, and otherwise reuses the vendor label: on a direct,
// non-reseller instance the publisher and the vendor are the same entity.
func resolvePublisher(vendorType *database.AIProviderType, vendorLabel, model string) string {
	if vendorType == nil {
		return vendorLabel
	}
	switch *vendorType {
	case database.AIProviderTypeBedrock:
		if prefix, _, ok := strings.Cut(model, "."); ok {
			return prefix
		}
	case database.AIProviderTypeOpenrouter, database.AIProviderTypeVercel:
		if prefix, _, ok := strings.Cut(model, "/"); ok {
			return prefix
		}
	}
	return vendorLabel
}

// titleCase upper-cases the first letter of s. Used only for the
// no-provider-row fallback label, where the input is already a short,
// lowercase wire-protocol token (e.g. "anthropic", "openai", "copilot").
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// resolveBillingAccount resolves BillingAccountId/Name/Type, branching on
// credential kind, per the Phase One Draft's BillingAccount section.
// deploymentID is api.AccessURL.Host; vendorLabel is the resolved vendor
// label for the row's provider instance.
func resolveBillingAccount(credential database.CredentialKind, deploymentID, providerInstance, initiatorID, initiatorUsername, vendorLabel string) BillingAccount {
	switch credential {
	case database.CredentialKindByok:
		return BillingAccount{
			ID:   fmt.Sprintf("byok:%s:%s", providerInstance, initiatorID),
			Name: fmt.Sprintf("%s (BYOK, %s)", initiatorUsername, vendorLabel),
			Type: "User Provider Account",
		}
	default:
		return BillingAccount{
			ID:   deploymentID,
			Name: fmt.Sprintf("Coder Deployment (%s)", deploymentID),
			Type: "Coder Deployment",
		}
	}
}

var microDivisor = decimal.NewFromInt(1_000_000)

// unitPrice converts a snapshotted *_price_micros value (micro-USD per
// million tokens) into an exact decimal USD-per-token unit price. Returns nil
// when priceMicros is nil (unpriced), never zero-for-unknown.
func unitPrice(priceMicros *int64) *decimal.Decimal {
	if priceMicros == nil {
		return nil
	}
	v := decimal.NewFromInt(*priceMicros).Div(microDivisor).Div(microDivisor)
	return &v
}

// lineCost returns the exact decimal cost of quantity tokens at price,
// or zero when price is nil (unpriced).
//
// This is the authoritative per-line-item cost for the FOCUS export: unlike
// coderd/aibridgedserver's tokenCost/computeCost (which truncates each token
// category to whole micro-USD before summing, so that a per-category
// breakdown recomputed from the persisted cost_micros integer adds up
// exactly), LineCost keeps full decimal precision and is never truncated.
// The two are expected to diverge by a small, systematic sub-micro-USD delta
// per row; a consumer reconciling the FOCUS export against the existing
// /export endpoint's cost_micros column should expect that and treat FOCUS's
// ContractedCost/EffectiveCost/ListCost as the more precise figure.
//
// Separately: when aibridgedserver rejected a total cost as out of range
// (see validateTotalCost), it persists token_usages.cost_micros as NULL and
// the aggregated /export endpoint's cost_micros for that period is
// correspondingly reduced. The FOCUS export has no equivalent "unknown"
// state for LineCost: it recomputes cost from the same snapshotted
// per-token prices regardless of whether the original write considered the
// total trustworthy, so it can emit a cost for a token-usage row that the
// rest of the system recorded as unknown. This is a known limitation, not
// yet reconciled in Phase 1.
func lineCost(quantity int64, price *decimal.Decimal) decimal.Decimal {
	if price == nil {
		return decimal.Zero
	}
	return decimal.NewFromInt(quantity).Mul(*price)
}

// skuID composes the FOCUS SkuId for one token type: <vendorType>/<model>/<TokenType>Tokens.
// vendorType is the raw ai_providers.type string (or the wire protocol, when
// no provider row resolved) rather than the display label, matching the
// Phase One Draft.
func skuID(vendorTypeOrWireProtocol, model string, tokenType TokenType) string {
	return fmt.Sprintf("%s/%s/%sTokens", vendorTypeOrWireProtocol, model, tokenType)
}

// skuPriceID appends the (currently unversioned) pricing-catalog version
// suffix to a SkuId. See unversionedCatalogSuffix.
func skuPriceID(sku string) string {
	return sku + "/" + unversionedCatalogSuffix
}

// chargeDescription builds the FOCUS ChargeDescription for one token type:
// "<model> - <TokenType label>". No join and no vendor label: model is
// already on aibridge_interceptions, and using it alone keeps the format
// identical across every provider.
func chargeDescription(model string, tokenType TokenType) string {
	return fmt.Sprintf("%s - %s", model, tokenType.Label())
}
