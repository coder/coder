package focus

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// FocusVersion and MappingVersion are surfaced both as response headers
// (Export manifest) and available to callers that want to stamp a manifest
// document later. MappingVersion changes whenever a column is renamed, a
// required-column's semantics change, or a cost calculation changes;
// changes that only add optional columns do not need to bump it.
const (
	FocusVersion = "1.2"
	// MappingVersion history:
	//   "1" -> "2": the M2/M3 fixes changed the emitted values of
	//   ResourceId (rollup synthetic ID hash key corrected to cover every
	//   GROUP BY dimension) and BillingPeriodStart/End (now derived from a
	//   record's start rather than its end) for rolled-up rows.
	//   "2" -> "3": every money/price column's decimal string width widened
	//   from 10 to 12 places (decimalString/decimalStringPtr), the exact
	//   precision floor of the underlying price_micros storage; values that
	//   fit in 10 places are unchanged, but any price below $0.0001/M now
	//   renders as a nonzero string instead of silently truncating to
	//   "0.0000000000".
	//   "3" -> "4": BillingPeriodStart/End is now derived from a dedicated
	//   UsageRecord.BillingPeriodAnchor (token_usages.created_at at raw
	//   grain, the rollup bucket's start at a rolled-up grain), matching the
	//   AI Gateway to FOCUS Mapping table's documented source column,
	//   instead of reusing StartedAt (the request's start). Changes the
	//   emitted BillingPeriodStart/End for the narrow edge case of a raw
	//   response whose request started in one calendar month but was
	//   recorded in the next.
	//   "4" -> "5": rollupGroupKeyExcludedFields (query.go) now also
	//   excludes InitiatorUsername, GroupName, and VendorDisplayName from a
	//   rolled-up row's synthetic ResourceId hash key, since each is a
	//   mutable display label functionally redundant with an ID already in
	//   the key. Renaming a user, group, or provider display name no longer
	//   changes that row's ResourceId.
	// Each of these is exactly the class of change this constant's own doc
	// comment says must bump it.
	MappingVersion = "5"
)

// Row is one FOCUS v1.2 export row: every required/conditional/recommended
// FOCUS column the mapping supports, plus Coder's x_ custom columns. Decimal
// money and price values are pre-formatted strings (StringFixed(12)) rather
// than a numeric type, since that is both the FOCUS-recommended
// representation and the shape parquet-go can write without a custom decimal
// converter.
//
// Field order here is also serialization order: Header (csv.go) and the
// parquet schema derived from these tags must stay in the same order the
// fields are declared in.
type Row struct {
	BilledCost          string    `parquet:"BilledCost"`
	BillingAccountID    string    `parquet:"BillingAccountId"`
	BillingAccountName  string    `parquet:"BillingAccountName"`
	BillingAccountType  string    `parquet:"BillingAccountType"`
	BillingCurrency     string    `parquet:"BillingCurrency"`
	BillingPeriodEnd    time.Time `parquet:"BillingPeriodEnd,timestamp(microsecond)"`
	BillingPeriodStart  time.Time `parquet:"BillingPeriodStart,timestamp(microsecond)"`
	ChargeCategory      string    `parquet:"ChargeCategory"`
	ChargeClass         *string   `parquet:"ChargeClass,optional"`
	ChargeDescription   string    `parquet:"ChargeDescription"`
	ChargeFrequency     string    `parquet:"ChargeFrequency"`
	ChargePeriodEnd     time.Time `parquet:"ChargePeriodEnd,timestamp(microsecond)"`
	ChargePeriodStart   time.Time `parquet:"ChargePeriodStart,timestamp(microsecond)"`
	ConsumedQuantity    int64     `parquet:"ConsumedQuantity"`
	ConsumedUnit        string    `parquet:"ConsumedUnit"`
	ContractedCost      string    `parquet:"ContractedCost"`
	ContractedUnitPrice *string   `parquet:"ContractedUnitPrice,optional"`
	EffectiveCost       string    `parquet:"EffectiveCost"`
	InvoiceIssuerName   string    `parquet:"InvoiceIssuerName"`
	ListCost            string    `parquet:"ListCost"`
	ListUnitPrice       *string   `parquet:"ListUnitPrice,optional"`
	PricingCategory     string    `parquet:"PricingCategory"`
	PricingQuantity     int64     `parquet:"PricingQuantity"`
	PricingUnit         string    `parquet:"PricingUnit"`
	ProviderName        string    `parquet:"ProviderName"`
	PublisherName       string    `parquet:"PublisherName"`
	ResourceID          *string   `parquet:"ResourceId,optional"`
	ResourceName        string    `parquet:"ResourceName"`
	ResourceType        string    `parquet:"ResourceType"`
	ServiceCategory     string    `parquet:"ServiceCategory"`
	ServiceName         string    `parquet:"ServiceName"`
	ServiceSubcategory  string    `parquet:"ServiceSubcategory"`
	SkuID               string    `parquet:"SkuId"`
	SkuPriceID          string    `parquet:"SkuPriceId"`
	SubAccountID        *string   `parquet:"SubAccountId,optional"`
	SubAccountName      *string   `parquet:"SubAccountName,optional"`
	SubAccountType      string    `parquet:"SubAccountType"`
	Tags                string    `parquet:"Tags"` // JSON object, scalar values only

	// Custom (x_) columns. Per the FOCUS spec's extension convention, every
	// custom column carries the x_ prefix.
	XProviderResponseID *string `parquet:"x_ProviderResponseId,optional"`
	XInterceptionID     *string `parquet:"x_InterceptionId,optional"`
	XSessionID          *string `parquet:"x_SessionId,optional"`
	XInitiatorID        string  `parquet:"x_InitiatorId"`
	XCredentialKind     string  `parquet:"x_CredentialKind"`
	XWireProtocol       string  `parquet:"x_WireProtocol"`
	XProviderInstance   string  `parquet:"x_ProviderInstance"`
	XClientTool         string  `parquet:"x_ClientTool"`
	XErrorType          *string `parquet:"x_ErrorType,optional"`
	XPricingStatus      string  `parquet:"x_PricingStatus"`
	XTokenType          string  `parquet:"x_TokenType"`
	// XRowCount is the number of raw aibridge_token_usages rows folded into
	// this row. Always 1 at raw granularity; >= 1 when the export was
	// requested at a rolled-up granularity.
	XRowCount int64 `parquet:"x_RowCount"`
}

// MapUsageRecord fans a UsageRecord out into one Row per non-zero token type,
// per the AI Gateway to FOCUS Mapping table. deploymentID is api.AccessURL.Host.
func MapUsageRecord(rec UsageRecord, deploymentID string) []Row {
	billingAccount := resolveBillingAccount(rec.Credential, deploymentID, rec.ProviderInstance, rec.InitiatorID.String(), rec.InitiatorUsername, rec.Vendor)

	// BillingPeriodStart/End are derived from rec.BillingPeriodAnchor
	// (token_usages.created_at at raw grain, the rollup bucket's start at a
	// rolled-up grain), per the AI Gateway to FOCUS Mapping table, never from
	// StartedAt (the request's start, not when it was recorded/priced) or
	// EndedAt/bucket end: an exclusive bucket end lands on the first instant
	// of the next UTC calendar day, so for the final bucket of any month that
	// instant is already in the following month, and deriving the billing
	// period from it would misattribute that bucket's spend to the wrong
	// month on every export, every month (see MaxGranularitySeconds for the
	// related, larger-granularity version of this same trade-off).
	periodStart, periodEnd := billingPeriod(rec.BillingPeriodAnchor)

	tags := buildTags(rec.InitiatorUsername, rec.ClientTool)

	var subAccountID, subAccountName *string
	if rec.BudgetGroup != nil {
		id := rec.BudgetGroup.ID.String()
		name := rec.BudgetGroup.Name
		subAccountID, subAccountName = &id, &name
	}

	rows := make([]Row, 0, len(rec.Usage))
	for _, usage := range rec.Usage {
		price := unitPrice(usage.UnitPriceMicros)
		cost := lineCost(usage.Tokens, price)
		pricingStatus := "unpriced"
		if price != nil {
			pricingStatus = "priced"
		}

		sku := skuID(rec.VendorTypeRaw, rec.Model, usage.Type)

		row := Row{
			// BilledCost is intentionally always "0", not StringFixed(12)
			// like the other money columns and not the standard-FOCUS-null
			// "no billed cost recorded": Phase 1 has no invoiced/billed
			// figure distinct from EffectiveCost, and 0 is the deliberate,
			// standard placeholder for that until a real billed-cost
			// pipeline exists, not a stray zero measurement.
			BilledCost:          "0",
			BillingAccountID:    billingAccount.ID,
			BillingAccountName:  billingAccount.Name,
			BillingAccountType:  billingAccount.Type,
			BillingCurrency:     "USD",
			BillingPeriodStart:  periodStart,
			BillingPeriodEnd:    periodEnd,
			ChargeCategory:      "Usage",
			ChargeClass:         nil,
			ChargeDescription:   chargeDescription(rec.Model, usage.Type),
			ChargeFrequency:     "Usage-Based",
			ChargePeriodStart:   rec.StartedAt,
			ChargePeriodEnd:     rec.EndedAt,
			ConsumedQuantity:    usage.Tokens,
			ConsumedUnit:        "Tokens",
			ContractedCost:      decimalString(cost),
			ContractedUnitPrice: decimalStringPtr(price),
			EffectiveCost:       decimalString(cost),
			InvoiceIssuerName:   rec.Vendor,
			ListCost:            decimalString(cost),
			ListUnitPrice:       decimalStringPtr(price),
			PricingCategory:     "Standard",
			PricingQuantity:     usage.Tokens,
			PricingUnit:         "Tokens",
			ProviderName:        rec.Vendor,
			PublisherName:       rec.Publisher,
			ResourceID:          strPtr(rec.TokenUsageID.String()),
			ResourceName:        rec.Model,
			ResourceType:        "LLM Model",
			ServiceCategory:     "AI and Machine Learning",
			ServiceName:         rec.Vendor,
			ServiceSubcategory:  "Generative AI",
			SkuID:               sku,
			SkuPriceID:          skuPriceID(sku),
			SubAccountID:        subAccountID,
			SubAccountName:      subAccountName,
			SubAccountType:      "Coder Budget Group",
			Tags:                tags,

			XProviderResponseID: emptyToNil(rec.ResponseID),
			XInterceptionID:     uuidToNilablePtr(rec.InterceptionID),
			XSessionID:          emptyToNil(rec.SessionID),
			XInitiatorID:        rec.InitiatorID.String(),
			XCredentialKind:     string(rec.Credential),
			XWireProtocol:       rec.WireProtocol,
			XProviderInstance:   rec.ProviderInstance,
			XClientTool:         rec.ClientTool,
			XErrorType:          rec.ErrorType,
			XPricingStatus:      pricingStatus,
			XTokenType:          string(usage.Type),
			XRowCount:           rec.RowCount,
		}
		rows = append(rows, row)
	}
	return rows
}

// billingPeriod truncates t to its UTC calendar month start/end.
func billingPeriod(t time.Time) (start, end time.Time) {
	t = t.UTC()
	start = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	end = start.AddDate(0, 1, 0)
	return start, end
}

// buildTags returns the FOCUS Tags JSON object: exactly the username and
// client dimensions today. Provider-defined keys keep the coder./aibridge.
// prefixes; workspace/team/project keys are omitted entirely rather than
// emitted with a null value, since no resolution mechanism for them exists
// (see the parent RFC's Attribution gaps).
func buildTags(username, clientTool string) string {
	tags := map[string]string{
		"coder.username":  username,
		"aibridge.client": clientTool,
	}
	b, err := json.Marshal(tags)
	if err != nil {
		// map[string]string can never fail to marshal.
		panic(fmt.Sprintf("marshal FOCUS tags: %v", err))
	}
	return string(b)
}

func decimalStringPtr(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.StringFixed(12)
	return &s
}

func decimalString(d decimal.Decimal) string {
	return d.StringFixed(12)
}

func strPtr(s string) *string {
	return &s
}

// emptyToNil maps an empty string to nil. Used for x_ProviderResponseId and
// x_SessionId, whose NULL therefore has two distinct causes conflated into
// one encoding: a rollup bucket folding more than one distinct underlying
// value (see loadRollupUsageRecords in query.go), or a raw, single-response
// record whose underlying provider_response_id/session_id was itself empty.
// dto.go documents NULL here as meaning "ambiguous", which is only true in
// the first case; low-impact today since both cases legitimately have no
// single well-defined value to report, but a future consumer that expects
// NULL to mean specifically "rolled up" would be misled by the second case.
func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// zeroUUID is the string form of the uuid.Nil zero value, used to detect an
// unset uuid.UUID (interception ID, in rollup rows where it wasn't uniquely
// determined) without importing google/uuid here just for a comparison.
const zeroUUID = "00000000-0000-0000-0000-000000000000"

func uuidToNilablePtr(id fmt.Stringer) *string {
	s := id.String()
	if s == "" || s == zeroUUID {
		return nil
	}
	return &s
}
