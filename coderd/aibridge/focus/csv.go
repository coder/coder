package focus

import (
	"encoding/csv"
	"io"
	"strconv"
	"strings"
)

// Header is the FOCUS export's CSV column order, matching Row's field order.
var Header = []string{
	"BilledCost", "BillingAccountId", "BillingAccountName", "BillingAccountType",
	"BillingCurrency", "BillingPeriodEnd", "BillingPeriodStart",
	"ChargeCategory", "ChargeClass", "ChargeDescription", "ChargeFrequency",
	"ChargePeriodEnd", "ChargePeriodStart",
	"ConsumedQuantity", "ConsumedUnit",
	"ContractedCost", "ContractedUnitPrice", "EffectiveCost", "InvoiceIssuerName",
	"ListCost", "ListUnitPrice",
	"PricingCategory", "PricingQuantity", "PricingUnit",
	"ProviderName", "PublisherName",
	"ResourceId", "ResourceName", "ResourceType",
	"ServiceCategory", "ServiceName", "ServiceSubcategory",
	"SkuId", "SkuPriceId",
	"SubAccountId", "SubAccountName", "SubAccountType",
	"Tags",
	"x_ProviderResponseId", "x_InterceptionId", "x_SessionId", "x_InitiatorId",
	"x_CredentialKind", "x_WireProtocol", "x_ProviderInstance", "x_ClientTool",
	"x_ErrorType", "x_PricingStatus", "x_TokenType", "x_RowCount",
}

// csvFormulaPrefixes are the leading characters a spreadsheet treats as the
// start of a formula rather than text. Duplicated from
// enterprise/coderd/aibridge.go's unexported escapeCSVCell/csvFormulaPrefixes
// rather than imported: the original lives in the enterprise package, and
// this AGPL package shouldn't depend on it anyway, per the same
// license-boundary principle that keeps the feature gate entirely in
// enterprise/coderd/aibridge.go.
const csvFormulaPrefixes = "=+-@\t\r"

// escapeCSVCell prefixes a leading formula character with a single quote,
// which spreadsheets strip on display, so the value renders as its original
// text instead of being evaluated as a formula.
func escapeCSVCell(value string) string {
	if value == "" || !strings.ContainsRune(csvFormulaPrefixes, rune(value[0])) {
		return value
	}
	return "'" + value
}

// escapeCSVCellPtr applies escapeCSVCell to a possibly-nil pointer, returning
// "" for nil.
func escapeCSVCellPtr(value *string) string {
	if value == nil {
		return ""
	}
	return escapeCSVCell(*value)
}

// csvCells returns r's values in Header order for one CSV row.
//
// Escaped (spreadsheet formula-injection risk): SubAccountName (a synced IdP
// group name has no server-side character validation), ChargeDescription and
// ResourceName (both embed interceptions.model, already treated as untrusted
// by the existing AI spend export's own escaping), BillingAccountName (embeds
// a username; low risk since usernames are normalized, escaped anyway for
// defense in depth), ProviderName/PublisherName/ServiceName/InvoiceIssuerName
// (safe when resolved from the canonical vendor-label map, but the
// openai-compat fallback is admin-set free text with no validation), and
// x_ProviderResponseId (populated verbatim from the upstream provider's own
// API response, which Coder never validates).
//
// Not escaped: x_ProviderInstance (ai_providers.name has a DB CHECK
// constraint matching a safe pattern) and x_ClientTool (always one of a fixed
// enum of values, never raw user-agent text).
func csvCells(r Row) []string {
	return []string{
		r.BilledCost, r.BillingAccountID, escapeCSVCell(r.BillingAccountName), r.BillingAccountType,
		r.BillingCurrency, r.BillingPeriodEnd.UTC().Format(rfc3339Micro), r.BillingPeriodStart.UTC().Format(rfc3339Micro),
		r.ChargeCategory, ptrOrEmpty(r.ChargeClass), escapeCSVCell(r.ChargeDescription), r.ChargeFrequency,
		r.ChargePeriodEnd.UTC().Format(rfc3339Micro), r.ChargePeriodStart.UTC().Format(rfc3339Micro),
		strconv.FormatInt(r.ConsumedQuantity, 10), r.ConsumedUnit,
		r.ContractedCost, ptrOrEmpty(r.ContractedUnitPrice), r.EffectiveCost, escapeCSVCell(r.InvoiceIssuerName),
		r.ListCost, ptrOrEmpty(r.ListUnitPrice),
		r.PricingCategory, strconv.FormatInt(r.PricingQuantity, 10), r.PricingUnit,
		escapeCSVCell(r.ProviderName), escapeCSVCell(r.PublisherName),
		ptrOrEmpty(r.ResourceID), escapeCSVCell(r.ResourceName), r.ResourceType,
		r.ServiceCategory, escapeCSVCell(r.ServiceName), r.ServiceSubcategory,
		r.SkuID, r.SkuPriceID,
		ptrOrEmpty(r.SubAccountID), escapeCSVCellPtr(r.SubAccountName), r.SubAccountType,
		r.Tags,
		escapeCSVCellPtr(r.XProviderResponseID), ptrOrEmpty(r.XInterceptionID), ptrOrEmpty(r.XSessionID), r.XInitiatorID,
		r.XCredentialKind, r.XWireProtocol, r.XProviderInstance, r.XClientTool,
		ptrOrEmpty(r.XErrorType), r.XPricingStatus, r.XTokenType, strconv.FormatInt(r.XRowCount, 10),
	}
}

const rfc3339Micro = "2006-01-02T15:04:05.999999Z07:00"

func ptrOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// WriteCSV writes rows as a FOCUS CSV document, header first, to w.
// Formula-injection escaping happens only here, applied to a cell just
// before it's written, never on the Row struct itself: WriteParquet and the
// Row values it serializes are never touched, since formula injection is a
// spreadsheet-import risk specific to CSV, and Parquet consumers read typed
// binary data, not evaluated formulas.
func WriteCSV(w io.Writer, rows []Row) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(Header); err != nil {
		return err
	}
	for _, row := range rows {
		if err := cw.Write(csvCells(row)); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
