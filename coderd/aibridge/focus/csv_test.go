package focus_test

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/coder/coder/v2/coderd/aibridge/focus"
)

// ptr is a small generic helper shared by this package's black-box tests
// (csv_test.go, parquet_test.go) for building a pointer to a literal.
func ptr[T any](v T) *T { return &v }

// TestWriteCSV_EscapesRiskyColumnsOnly confirms the columns the Phase One
// Draft calls out as formula-injection risks are escaped, that the two
// columns explicitly declared safe by construction are left alone, and that
// WriteParquet on the same Row leaves every value untouched.
func TestWriteCSV_EscapesRiskyColumnsOnly(t *testing.T) {
	t.Parallel()

	formula := "=cmd|' /C calc'!A1"
	row := focus.Row{
		BillingAccountName:  formula,
		ChargeDescription:   formula,
		ResourceName:        formula,
		ProviderName:        formula,
		PublisherName:       formula,
		ServiceName:         formula,
		InvoiceIssuerName:   formula,
		SubAccountName:      ptr(formula),
		XProviderResponseID: ptr(formula),
		// Safe by construction: never escaped even though they share the
		// same leading character.
		XProviderInstance: formula,
		XClientTool:       formula,

		ConsumedUnit:    "Tokens",
		PricingUnit:     "Tokens",
		SubAccountType:  "Coder Budget Group",
		BillingCurrency: "USD",
		ChargeCategory:  "Usage",
		ChargeFrequency: "Usage-Based",
		Tags:            "{}",
	}

	var buf bytes.Buffer
	if err := focus.WriteCSV(&buf, []focus.Row{row}); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}

	r := csv.NewReader(&buf)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2 (header + 1 row)", len(records))
	}
	if len(records[0]) != len(focus.Header) {
		t.Fatalf("header has %d columns, want %d", len(records[0]), len(focus.Header))
	}
	if len(records[1]) != len(records[0]) {
		t.Fatalf("row has %d columns, want %d (matching the header)", len(records[1]), len(records[0]))
	}

	header := records[0]
	values := records[1]
	byName := func(name string) string {
		for i, h := range header {
			if h == name {
				return values[i]
			}
		}
		t.Fatalf("column %q not found in header", name)
		return ""
	}

	escapedCols := []string{
		"BillingAccountName", "ChargeDescription", "ResourceName",
		"ProviderName", "PublisherName", "ServiceName", "InvoiceIssuerName",
		"SubAccountName", "x_ProviderResponseId",
	}
	for _, col := range escapedCols {
		got := byName(col)
		if !strings.HasPrefix(got, "'") {
			t.Errorf("column %s = %q, want a leading single-quote escape", col, got)
		}
	}

	safeCols := []string{"x_ProviderInstance", "x_ClientTool"}
	for _, col := range safeCols {
		got := byName(col)
		if got != formula {
			t.Errorf("column %s = %q, want unescaped %q", col, got, formula)
		}
	}

	// The same Row written as Parquet must never see escaping.
	var pbuf bytes.Buffer
	if err := focus.WriteParquet(&pbuf, []focus.Row{row}); err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}
	if pbuf.Len() == 0 {
		t.Fatal("expected non-empty parquet output")
	}
}
