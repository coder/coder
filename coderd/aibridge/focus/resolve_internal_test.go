package focus

import (
	"testing"

	"github.com/coder/coder/v2/coderd/database"
)

// These are white-box tests of resolveVendor, resolveBillingAccount,
// unitPrice, lineCost, skuID, and skuPriceID: unexported helpers with no
// callers outside this package (row.go and query.go), kept unexported
// specifically so they are not part of the package's public API. Exercising
// them directly requires being in package focus rather than focus_test; see
// escapecsv_internal_test.go for the same trade-off made for escapeCSVCell.

//nolint:testpackage // exercises the unexported resolveVendor directly.
func TestResolveVendor(t *testing.T) {
	t.Parallel()

	anthropic := database.AIProviderTypeAnthropic
	bedrock := database.AIProviderTypeBedrock
	openrouter := database.AIProviderTypeOpenrouter
	vercel := database.AIProviderTypeVercel
	openaiCompat := database.AIProviderTypeOpenaiCompat
	copilot := database.AIProviderTypeCopilot

	cases := []struct {
		name          string
		vendorType    *database.AIProviderType
		displayName   *string
		wireProtocol  string
		model         string
		wantLabel     string
		wantPublisher string
	}{
		{
			name:          "direct anthropic",
			vendorType:    &anthropic,
			wireProtocol:  "anthropic",
			model:         "claude-haiku-4-5",
			wantLabel:     "Anthropic",
			wantPublisher: "Anthropic",
		},
		{
			name:          "bedrock publisher split on dot",
			vendorType:    &bedrock,
			wireProtocol:  "anthropic",
			model:         "anthropic.claude-3-sonnet",
			wantLabel:     "Bedrock",
			wantPublisher: "anthropic",
		},
		{
			name:          "openrouter publisher split on slash",
			vendorType:    &openrouter,
			wireProtocol:  "openai",
			model:         "deepseek/deepseek-v4",
			wantLabel:     "OpenRouter",
			wantPublisher: "deepseek",
		},
		{
			name:          "vercel reseller: provider diverges from publisher",
			vendorType:    &vercel,
			wireProtocol:  "openai",
			model:         "deepseek/deepseek-v4-flash-0731",
			wantLabel:     "Vercel",
			wantPublisher: "deepseek",
		},
		{
			name:          "openai-compat falls back to display name",
			vendorType:    &openaiCompat,
			displayName:   strPtr("Poolside"),
			wireProtocol:  "openai",
			model:         "poolside/laguna-xs-2.1",
			wantLabel:     "Poolside",
			wantPublisher: "Poolside",
		},
		{
			name:          "openai-compat with no display name falls back to wire protocol",
			vendorType:    &openaiCompat,
			wireProtocol:  "openai",
			model:         "some-model",
			wantLabel:     "Openai",
			wantPublisher: "Openai",
		},
		{
			name:          "copilot canonical label",
			vendorType:    &copilot,
			wireProtocol:  "copilot",
			model:         "gpt-4o",
			wantLabel:     "GitHub Copilot",
			wantPublisher: "GitHub Copilot",
		},
		{
			name:          "no provider row falls back to title-cased wire protocol",
			vendorType:    nil,
			wireProtocol:  "copilot",
			model:         "gpt-4o",
			wantLabel:     "Copilot",
			wantPublisher: "Copilot",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := resolveVendor(tc.vendorType, tc.displayName, tc.wireProtocol, tc.model)
			if got.Label != tc.wantLabel {
				t.Errorf("Label = %q, want %q", got.Label, tc.wantLabel)
			}
			if got.Publisher != tc.wantPublisher {
				t.Errorf("Publisher = %q, want %q", got.Publisher, tc.wantPublisher)
			}
		})
	}
}

//nolint:testpackage // exercises the unexported resolveBillingAccount directly.
func TestResolveBillingAccount(t *testing.T) {
	t.Parallel()

	t.Run("centralized", func(t *testing.T) {
		t.Parallel()
		got := resolveBillingAccount(database.CredentialKindCentralized, "dev.coder.com", "anthropic", "user-id", "callum", "Anthropic")
		if got.ID != "dev.coder.com" {
			t.Errorf("ID = %q", got.ID)
		}
		if got.Type != "Coder Deployment" {
			t.Errorf("Type = %q", got.Type)
		}
	})

	t.Run("byok", func(t *testing.T) {
		t.Parallel()
		got := resolveBillingAccount(database.CredentialKindByok, "dev.coder.com", "agents-anthropic", "a49e1179-f7a9-4bb9-996c-5ad441f9f2fd", "ibetitsmike", "Anthropic")
		wantID := "byok:agents-anthropic:a49e1179-f7a9-4bb9-996c-5ad441f9f2fd"
		if got.ID != wantID {
			t.Errorf("ID = %q, want %q", got.ID, wantID)
		}
		wantName := "ibetitsmike (BYOK, Anthropic)"
		if got.Name != wantName {
			t.Errorf("Name = %q, want %q", got.Name, wantName)
		}
		if got.Type != "User Provider Account" {
			t.Errorf("Type = %q", got.Type)
		}
	})
}

//nolint:testpackage // exercises the unexported unitPrice/lineCost directly.
func TestUnitPriceAndLineCost(t *testing.T) {
	t.Parallel()

	if unitPrice(nil) != nil {
		t.Fatal("expected nil unit price for nil priceMicros")
	}

	priceMicros := int64(5_000_000)
	price := unitPrice(&priceMicros)
	if price == nil {
		t.Fatal("expected non-nil unit price")
	}
	// Asserted at 12 places (row.go's decimalString/decimalStringPtr format
	// width): price_micros is USD-per-token to exactly 12 decimal places
	// (micro-USD per million tokens = 1e-12 USD per token per unit), which
	// is the real precision floor of the underlying storage, not an
	// arbitrary width.
	if got, want := price.StringFixed(12), "0.000005000000"; got != want {
		t.Errorf("unit price = %s, want %s", got, want)
	}

	cost := lineCost(2131, price)
	if got, want := cost.StringFixed(12), "0.010655000000"; got != want {
		t.Errorf("cost = %s, want %s", got, want)
	}

	// Unpriced: zero cost, never a NULL/zero-for-unknown unit price.
	unpriced := lineCost(100, nil)
	if got, want := unpriced.StringFixed(12), "0.000000000000"; got != want {
		t.Errorf("unpriced cost = %s, want %s", got, want)
	}

	// Regression test for the decimal-widening fix: at the old 10-place
	// width, any price below $0.0001/M silently truncated to
	// "0.0000000000", indistinguishable from exactly free. price_micros
	// values below 100 (i.e. below $0.0001/M) are exactly the values that
	// need the 11th/12th decimal place to render as nonzero.
	tinyPriceMicros := int64(1) // $0.000001/M
	tinyPrice := unitPrice(&tinyPriceMicros)
	if tinyPrice == nil {
		t.Fatal("expected non-nil unit price")
	}
	if got, want := tinyPrice.StringFixed(12), "0.000000000001"; got != want {
		t.Errorf("tiny unit price = %s, want %s (must not silently render as free)", got, want)
	}
}

//nolint:testpackage // exercises the unexported skuID/skuPriceID directly.
func TestSkuID(t *testing.T) {
	t.Parallel()

	sku := skuID("anthropic", "claude-haiku-4-5", TokenTypeInput)
	if want := "anthropic/claude-haiku-4-5/InputTokens"; sku != want {
		t.Errorf("SkuID = %q, want %q", sku, want)
	}
	skuPrice := skuPriceID(sku)
	if want := "anthropic/claude-haiku-4-5/InputTokens/unversioned"; skuPrice != want {
		t.Errorf("SkuPriceID = %q, want %q", skuPrice, want)
	}
}
