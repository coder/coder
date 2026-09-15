// Package focus maps AI Gateway usage records into FOCUS v1.2-shaped rows for
// the experimental FOCUS export endpoint.
//
// This package is a read-only projection over aibridge_interceptions and
// aibridge_token_usages. It introduces no new source-of-truth table, and
// nothing here changes how usage is recorded or how cost is calculated.
package focus

import (
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

// TokenType is one of the token categories FOCUS fans a UsageRecord out into.
type TokenType string

const (
	TokenTypeInput      TokenType = "Input"
	TokenTypeOutput     TokenType = "Output"
	TokenTypeCacheRead  TokenType = "CacheRead"
	TokenTypeCacheWrite TokenType = "CacheWrite"
)

// Label returns the human-readable token type name used in ChargeDescription,
// e.g. "Input tokens".
func (t TokenType) Label() string {
	switch t {
	case TokenTypeInput:
		return "Input tokens"
	case TokenTypeOutput:
		return "Output tokens"
	case TokenTypeCacheRead:
		return "Cache read tokens"
	case TokenTypeCacheWrite:
		return "Cache write tokens"
	default:
		return string(t) + " tokens"
	}
}

// TokenUsage is one non-zero token-type entry within a UsageRecord. One
// UsageRecord fans out into one FOCUS Row per TokenUsage.
type TokenUsage struct {
	Type   TokenType
	Tokens int64
	// UnitPriceMicros is nil when unpriced, never zero-for-unknown.
	UnitPriceMicros *int64
}

// BudgetGroup is the resolved sub-account (budget group) a UsageRecord is
// attributed to. Distinct from budget.EffectiveGroup, which carries only a
// group ID and limit and has no Name field.
type BudgetGroup struct {
	ID   uuid.UUID
	Name string
}

// BillingAccount is the resolved account identity for one UsageRecord. It has
// no Coder analog today; both fields are derived by policy from Credential
// and, for BYOK, ProviderInstance and InitiatorID.
type BillingAccount struct {
	ID   string
	Name string
	Type string
}

// UsageRecord is one unit of AI Gateway usage with everything the export
// formats need already resolved. It deliberately holds no prompt, response,
// thinking, or tool-call payload, so no output format can leak one.
//
// At raw granularity, one UsageRecord corresponds to exactly one priced
// provider response (one aibridge_token_usages row). At a rolled-up
// granularity, one UsageRecord corresponds to a bucket of provider responses
// sharing every FOCUS dimension except quantity; RowCount then reports how
// many underlying responses were folded together, and the response-level
// identifiers (ResponseID, InterceptionID, SessionID) are empty whenever the
// bucket folded together more than one distinct value for that identifier,
// since no single value would be well-defined.
type UsageRecord struct {
	// TokenUsageID becomes ResourceId. At raw granularity this is
	// token_usages.id, unique per priced response. At a rolled-up
	// granularity there is no natural key, so this is a deterministic
	// synthetic UUID derived from the bucket's grouping key, stable for a
	// given billing period and mapping version.
	TokenUsageID uuid.UUID
	// ResponseID becomes x_ProviderResponseId. Empty when not uniquely
	// determined (rollup only).
	ResponseID string
	// InterceptionID becomes x_InterceptionId. Empty when not uniquely
	// determined (rollup only).
	InterceptionID uuid.UUID
	// SessionID becomes x_SessionId. Empty when not uniquely determined
	// (rollup only).
	SessionID  string
	ClientTool string  // interceptions.client, e.g. Mux
	ErrorType  *string // interceptions.error_type, nil when the interception succeeded (or, in rollup mode, when every folded response succeeded)

	StartedAt time.Time
	EndedAt   time.Time // already coalesced, never zero

	// BillingPeriodAnchor is the instant BillingPeriodStart/End is derived
	// from, per the AI Gateway to FOCUS Mapping table: token_usages.created_at
	// at raw granularity (i.e. the finest-real-timestamp proxy for "when this
	// priced response was recorded"), or the rollup bucket's start at a
	// rolled-up granularity. This is deliberately its own field rather than a
	// reuse of StartedAt or EndedAt:
	//   - StartedAt (interception.started_at) is when the request began, not
	//     when it was recorded/priced; using it can misattribute a request
	//     that starts in one calendar month but resolves in the next.
	//   - EndedAt is an exclusive bucket end at rollup granularity, so for the
	//     final bucket of any month it lands on the first instant of the
	//     following month, which is the wrong month for BillingPeriodStart/End.
	BillingPeriodAnchor time.Time

	InitiatorID       uuid.UUID
	InitiatorUsername string
	BudgetGroup       *BudgetGroup // nil when unresolved

	Model string
	// Vendor is the canonical, human-readable vendor label (ProviderName /
	// ServiceName / InvoiceIssuerName), resolved from ai_providers.type or the
	// wire-protocol fallback label.
	Vendor string
	// VendorTypeRaw is the short, machine-oriented vendor code used inside
	// SkuId/SkuPriceId: ai_providers.type's raw string when a provider row
	// resolved, else the wire protocol.
	VendorTypeRaw    string
	Publisher        string // derived from the model string
	WireProtocol     string // interceptions.provider, the upstream API format. Kept distinct from Vendor
	ProviderInstance string

	Credential     database.CredentialKind // centralized or byok. Independent of BillingAccount
	BillingAccount BillingAccount          // resolved by policy, not switched on Credential's value

	Usage []TokenUsage // one entry per token type with non-zero usage

	// RowCount is the number of raw aibridge_token_usages rows folded into
	// this UsageRecord. Always 1 at raw granularity.
	RowCount int64
}
