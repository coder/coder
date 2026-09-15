package focus_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"

	"github.com/coder/coder/v2/coderd/aibridge/focus"
)

// TestParquetRoundTrip writes a small set of Rows, including nullable fields
// in both their present and absent states, and reads them back, confirming
// optional columns and pre-formatted decimal strings survive unchanged.
func TestParquetRoundTrip(t *testing.T) {
	t.Parallel()

	price := "0.0000010000"
	priced := focus.Row{
		BilledCost:          "0",
		BillingAccountID:    "dev.coder.com",
		BillingAccountName:  "Coder Deployment (dev.coder.com)",
		BillingAccountType:  "Coder Deployment",
		BillingCurrency:     "USD",
		BillingPeriodStart:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		BillingPeriodEnd:    time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		ChargeCategory:      "Usage",
		ChargeClass:         nil,
		ChargeDescription:   "claude-haiku-4-5 - Input tokens",
		ChargeFrequency:     "Usage-Based",
		ChargePeriodStart:   time.Date(2026, 6, 19, 22, 58, 41, 374962000, time.UTC),
		ChargePeriodEnd:     time.Date(2026, 6, 19, 22, 58, 42, 674767000, time.UTC),
		ConsumedQuantity:    3,
		ConsumedUnit:        "Tokens",
		ContractedCost:      "0.0000030000",
		ContractedUnitPrice: &price,
		EffectiveCost:       "0.0000030000",
		InvoiceIssuerName:   "Anthropic",
		ListCost:            "0.0000030000",
		ListUnitPrice:       &price,
		PricingCategory:     "Standard",
		PricingQuantity:     3,
		PricingUnit:         "Tokens",
		ProviderName:        "Anthropic",
		PublisherName:       "Anthropic",
		ResourceID:          ptr("d1739bba-6711-4bc7-aa46-b9e32bfaa451"),
		ResourceName:        "claude-haiku-4-5",
		ResourceType:        "LLM Model",
		ServiceCategory:     "AI and Machine Learning",
		ServiceName:         "Anthropic",
		ServiceSubcategory:  "Generative AI",
		SkuID:               "anthropic/claude-haiku-4-5/InputTokens",
		SkuPriceID:          "anthropic/claude-haiku-4-5/InputTokens/unversioned",
		SubAccountID:        nil,
		SubAccountName:      nil,
		SubAccountType:      "Coder Budget Group",
		Tags:                `{"aibridge.client":"Mux","coder.username":"callum"}`,
		XInitiatorID:        "a26a1adc-48a9-4823-a58d-8e0b7e9c60f2",
		XCredentialKind:     "centralized",
		XWireProtocol:       "anthropic",
		XProviderInstance:   "anthropic",
		XClientTool:         "Mux",
		XPricingStatus:      "priced",
		XTokenType:          "Input",
		XRowCount:           1,
	}

	unpriced := priced
	unpriced.ContractedUnitPrice = nil
	unpriced.ListUnitPrice = nil
	unpriced.ResourceID = nil
	unpriced.SubAccountID = ptr("3ea6ae38-ff77-446a-ad06-fb1ba0c9e1b1")
	unpriced.SubAccountName = ptr("Vibes")
	unpriced.XPricingStatus = "unpriced"

	rows := []focus.Row{priced, unpriced}

	var buf bytes.Buffer
	if err := focus.WriteParquet(&buf, rows); err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	got, err := parquet.Read[focus.Row](bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("parquet.Read: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}

	if got[0].ContractedUnitPrice == nil || *got[0].ContractedUnitPrice != price {
		t.Errorf("row 0 ContractedUnitPrice = %v, want %s", got[0].ContractedUnitPrice, price)
	}
	if got[0].ResourceID == nil || *got[0].ResourceID != *priced.ResourceID {
		t.Errorf("row 0 ResourceID = %v", got[0].ResourceID)
	}
	if got[0].SubAccountID != nil {
		t.Errorf("row 0 SubAccountID = %v, want nil", got[0].SubAccountID)
	}

	if got[1].ContractedUnitPrice != nil {
		t.Errorf("row 1 ContractedUnitPrice = %v, want nil (unpriced)", got[1].ContractedUnitPrice)
	}
	if got[1].ResourceID != nil {
		t.Errorf("row 1 ResourceID = %v, want nil", got[1].ResourceID)
	}
	if got[1].SubAccountID == nil || *got[1].SubAccountID != "3ea6ae38-ff77-446a-ad06-fb1ba0c9e1b1" {
		t.Errorf("row 1 SubAccountID = %v", got[1].SubAccountID)
	}
	if !got[1].ChargePeriodStart.Equal(unpriced.ChargePeriodStart) {
		t.Errorf("row 1 ChargePeriodStart = %v, want %v", got[1].ChargePeriodStart, unpriced.ChargePeriodStart)
	}
}
