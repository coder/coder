package focus_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/aibridge/focus"
	"github.com/coder/coder/v2/coderd/database"
)

// TestMapUsageRecord_StreamingFanOutNoGroup mirrors the parent RFC's Example
// 1: one interception, two token_usages rows sharing a provider_response_id,
// no budget group (SubAccountId/Name null).
func TestMapUsageRecord_StreamingFanOutNoGroup(t *testing.T) {
	t.Parallel()

	inputPrice := int64(1_000_000)
	outputPrice := int64(5_000_000)
	cacheWritePrice := int64(1_250_000)

	rec := focus.UsageRecord{
		TokenUsageID:        uuid.MustParse("d1739bba-6711-4bc7-aa46-b9e32bfaa451"),
		ResponseID:          "msg_01Bk9iY843EAmrQYyYVgVZmP",
		InterceptionID:      uuid.MustParse("edeea219-190d-4494-b2f1-0f5746639178"),
		SessionID:           "edeea219-190d-4494-b2f1-0f5746639178",
		ClientTool:          "Mux",
		StartedAt:           time.Date(2026, 6, 19, 22, 58, 41, 374962000, time.UTC),
		EndedAt:             time.Date(2026, 6, 19, 22, 58, 42, 674767000, time.UTC),
		BillingPeriodAnchor: time.Date(2026, 6, 19, 22, 58, 42, 667943000, time.UTC), // token_usages.created_at
		InitiatorID:         uuid.MustParse("a26a1adc-48a9-4823-a58d-8e0b7e9c60f2"),
		InitiatorUsername:   "callum",
		BudgetGroup:         nil,
		Model:               "claude-haiku-4-5",
		Vendor:              "Anthropic",
		VendorTypeRaw:       "anthropic",
		Publisher:           "Anthropic",
		WireProtocol:        "anthropic",
		ProviderInstance:    "anthropic",
		Credential:          database.CredentialKindCentralized,
		BillingAccount: focus.BillingAccount{
			ID: "dev.coder.com", Name: "Coder Dogfood (dev.coder.com)", Type: "Coder Deployment",
		},
		Usage: []focus.TokenUsage{
			{Type: focus.TokenTypeInput, Tokens: 3, UnitPriceMicros: &inputPrice},
			{Type: focus.TokenTypeOutput, Tokens: 51, UnitPriceMicros: &outputPrice},
			{Type: focus.TokenTypeCacheWrite, Tokens: 9278, UnitPriceMicros: &cacheWritePrice},
		},
		RowCount: 1,
	}

	rows := focus.MapUsageRecord(rec, "dev.coder.com")
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}

	input := rows[0]
	if input.ResourceID == nil || *input.ResourceID != "d1739bba-6711-4bc7-aa46-b9e32bfaa451" {
		t.Errorf("ResourceID = %v", input.ResourceID)
	}
	if input.ChargeDescription != "claude-haiku-4-5 - Input tokens" {
		t.Errorf("ChargeDescription = %q", input.ChargeDescription)
	}
	if input.ConsumedQuantity != 3 || input.PricingQuantity != 3 {
		t.Errorf("quantity = %d/%d", input.ConsumedQuantity, input.PricingQuantity)
	}
	if input.ListUnitPrice == nil || *input.ListUnitPrice != "0.000001000000" {
		t.Errorf("ListUnitPrice = %v", input.ListUnitPrice)
	}
	if input.ListCost != "0.000003000000" {
		t.Errorf("ListCost = %q", input.ListCost)
	}
	if input.SkuID != "anthropic/claude-haiku-4-5/InputTokens" {
		t.Errorf("SkuID = %q", input.SkuID)
	}
	if input.SubAccountID != nil || input.SubAccountName != nil {
		t.Errorf("expected nil SubAccountID/Name, got %v/%v", input.SubAccountID, input.SubAccountName)
	}
	if input.Tags != `{"aibridge.client":"Mux","coder.username":"callum"}` {
		t.Errorf("Tags = %q", input.Tags)
	}
	if input.BillingPeriodStart != time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) {
		t.Errorf("BillingPeriodStart = %v", input.BillingPeriodStart)
	}
	if input.BillingPeriodEnd != time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) {
		t.Errorf("BillingPeriodEnd = %v", input.BillingPeriodEnd)
	}

	cacheWrite := rows[2]
	if cacheWrite.ListCost != "0.011597500000" {
		t.Errorf("cache write ListCost = %q, want 0.011597500000", cacheWrite.ListCost)
	}
}

// TestMapUsageRecord_OpenAICompatUnpriced mirrors Example 2: three token
// types, all unpriced, with a budget group present.
func TestMapUsageRecord_OpenAICompatUnpriced(t *testing.T) {
	t.Parallel()

	rec := focus.UsageRecord{
		TokenUsageID:        uuid.MustParse("12c19969-e2a4-4965-a2be-34063313fbcb"),
		ResponseID:          "chatcmpl-724dccfa3f9944619005441f2f21db47",
		InterceptionID:      uuid.MustParse("1d152081-a0ce-479a-b1a4-b6705a23988f"),
		SessionID:           "1d152081-a0ce-479a-b1a4-b6705a23988f",
		ClientTool:          "Unknown",
		StartedAt:           time.Date(2026, 8, 12, 11, 37, 59, 868000000, time.UTC),
		EndedAt:             time.Date(2026, 8, 12, 11, 38, 0, 473000000, time.UTC),
		BillingPeriodAnchor: time.Date(2026, 8, 12, 11, 38, 0, 473382000, time.UTC), // token_usages.created_at
		InitiatorID:         uuid.MustParse("a49e1179-f7a9-4bb9-996c-5ad441f9f2fd"),
		InitiatorUsername:   "ibetitsmike",
		BudgetGroup: &focus.BudgetGroup{
			ID:   uuid.MustParse("3ea6ae38-ff77-446a-ad06-fb1ba0c9e1b1"),
			Name: "Vibes",
		},
		Model:            "poolside/laguna-xs-2.1",
		Vendor:           "Poolside",
		VendorTypeRaw:    "openai-compat",
		Publisher:        "Poolside",
		WireProtocol:     "openai",
		ProviderInstance: "openaicompa-poolside",
		Credential:       database.CredentialKindCentralized,
		Usage: []focus.TokenUsage{
			{Type: focus.TokenTypeInput, Tokens: 118},
			{Type: focus.TokenTypeOutput, Tokens: 2},
			{Type: focus.TokenTypeCacheRead, Tokens: 16},
		},
		RowCount: 1,
	}

	rows := focus.MapUsageRecord(rec, "dev.coder.com")
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	for _, row := range rows {
		if row.XPricingStatus != "unpriced" {
			t.Errorf("x_PricingStatus = %q, want unpriced", row.XPricingStatus)
		}
		if row.ListUnitPrice != nil {
			t.Errorf("ListUnitPrice = %v, want nil", row.ListUnitPrice)
		}
		if row.ListCost != "0.000000000000" {
			t.Errorf("ListCost = %q, want 0", row.ListCost)
		}
	}
	if got := *rows[0].SubAccountID; got != "3ea6ae38-ff77-446a-ad06-fb1ba0c9e1b1" {
		t.Errorf("SubAccountID = %q", got)
	}
	if got := *rows[0].SubAccountName; got != "Vibes" {
		t.Errorf("SubAccountName = %q", got)
	}
}

// TestMapUsageRecord_BYOK mirrors Example 3: BYOK BillingAccount identity.
func TestMapUsageRecord_BYOK(t *testing.T) {
	t.Parallel()

	inputPrice := int64(5_000_000)
	outputPrice := int64(25_000_000)

	rec := focus.UsageRecord{
		TokenUsageID:        uuid.MustParse("0e929f6f-a216-41e1-a1a8-c46045b1cb59"),
		StartedAt:           time.Date(2026, 8, 12, 16, 51, 20, 674414000, time.UTC),
		EndedAt:             time.Date(2026, 8, 12, 16, 51, 22, 556077000, time.UTC),
		BillingPeriodAnchor: time.Date(2026, 8, 12, 16, 51, 22, 556134000, time.UTC), // token_usages.created_at
		InitiatorID:         uuid.MustParse("a49e1179-f7a9-4bb9-996c-5ad441f9f2fd"),
		InitiatorUsername:   "ibetitsmike",
		Model:               "claude-opus-4-8",
		Vendor:              "Anthropic",
		VendorTypeRaw:       "anthropic",
		Publisher:           "Anthropic",
		WireProtocol:        "anthropic",
		ProviderInstance:    "agents-anthropic",
		Credential:          database.CredentialKindByok,
		Usage: []focus.TokenUsage{
			{Type: focus.TokenTypeInput, Tokens: 2131, UnitPriceMicros: &inputPrice},
			{Type: focus.TokenTypeOutput, Tokens: 40, UnitPriceMicros: &outputPrice},
		},
		RowCount: 1,
	}

	rows := focus.MapUsageRecord(rec, "dev.coder.com")
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	want := "byok:agents-anthropic:a49e1179-f7a9-4bb9-996c-5ad441f9f2fd"
	if rows[0].BillingAccountID != want {
		t.Errorf("BillingAccountID = %q, want %q", rows[0].BillingAccountID, want)
	}
	if rows[0].BillingAccountType != "User Provider Account" {
		t.Errorf("BillingAccountType = %q", rows[0].BillingAccountType)
	}
	if rows[0].ListCost != "0.010655000000" {
		t.Errorf("ListCost = %q, want 0.010655000000", rows[0].ListCost)
	}
}

// TestMapUsageRecord_ResellerDivergence mirrors Example 4: ProviderName
// (Vercel) diverges from PublisherName (DeepSeek).
func TestMapUsageRecord_ResellerDivergence(t *testing.T) {
	t.Parallel()

	inputPrice := int64(130_000)

	rec := focus.UsageRecord{
		TokenUsageID:        uuid.MustParse("8e0e030b-c6cb-4d1a-8402-94c2fcc1c439"),
		StartedAt:           time.Now(),
		EndedAt:             time.Now(),
		BillingPeriodAnchor: time.Now(),
		InitiatorID:         uuid.New(),
		Model:               "deepseek/deepseek-v4-flash-0731",
		Vendor:              "Vercel",
		VendorTypeRaw:       "vercel",
		Publisher:           "DeepSeek",
		WireProtocol:        "openai",
		ProviderInstance:    "agents-vercel",
		Credential:          database.CredentialKindCentralized,
		Usage: []focus.TokenUsage{
			{Type: focus.TokenTypeInput, Tokens: 815, UnitPriceMicros: &inputPrice},
		},
		RowCount: 1,
	}

	rows := focus.MapUsageRecord(rec, "dev.coder.com")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	row := rows[0]
	if row.ProviderName != "Vercel" {
		t.Errorf("ProviderName = %q, want Vercel", row.ProviderName)
	}
	if row.PublisherName != "DeepSeek" {
		t.Errorf("PublisherName = %q, want DeepSeek", row.PublisherName)
	}
	if row.InvoiceIssuerName != "Vercel" {
		t.Errorf("InvoiceIssuerName = %q, want Vercel (the vendor still issues the invoice)", row.InvoiceIssuerName)
	}
	if row.SkuID != "vercel/deepseek/deepseek-v4-flash-0731/InputTokens" {
		t.Errorf("SkuID = %q", row.SkuID)
	}
}

// TestMapUsageRecord_RollupOmitsAmbiguousIdentifiers exercises a synthetic
// rolled-up UsageRecord (as loadRollupUsageRecords would produce for a bucket
// spanning more than one response), confirming the response-level
// identifiers are omitted and x_RowCount reflects the fold.
func TestMapUsageRecord_RollupOmitsAmbiguousIdentifiers(t *testing.T) {
	t.Parallel()

	rec := focus.UsageRecord{
		TokenUsageID:        uuid.New(), // synthetic
		ResponseID:          "",         // ambiguous: omitted
		InterceptionID:      uuid.Nil,   // ambiguous: omitted
		SessionID:           "",
		ClientTool:          "Coder Agents",
		StartedAt:           time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC),
		EndedAt:             time.Date(2026, 8, 12, 17, 0, 0, 0, time.UTC),
		BillingPeriodAnchor: time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC), // bucket start
		InitiatorID:         uuid.New(),
		InitiatorUsername:   "callum",
		Model:               "claude-haiku-4-5",
		Vendor:              "Anthropic",
		VendorTypeRaw:       "anthropic",
		Publisher:           "Anthropic",
		WireProtocol:        "anthropic",
		ProviderInstance:    "anthropic",
		Credential:          database.CredentialKindCentralized,
		Usage: []focus.TokenUsage{
			{Type: focus.TokenTypeInput, Tokens: 500},
		},
		RowCount: 7,
	}

	rows := focus.MapUsageRecord(rec, "dev.coder.com")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	row := rows[0]
	if row.XProviderResponseID != nil {
		t.Errorf("x_ProviderResponseId = %v, want nil", row.XProviderResponseID)
	}
	if row.XInterceptionID != nil {
		t.Errorf("x_InterceptionId = %v, want nil", row.XInterceptionID)
	}
	if row.XSessionID != nil {
		t.Errorf("x_SessionId = %v, want nil", row.XSessionID)
	}
	if row.XRowCount != 7 {
		t.Errorf("x_RowCount = %d, want 7", row.XRowCount)
	}
	if row.ChargePeriodStart != rec.StartedAt || row.ChargePeriodEnd != rec.EndedAt {
		t.Errorf("charge period = %v..%v, want bucket boundaries", row.ChargePeriodStart, row.ChargePeriodEnd)
	}
}
