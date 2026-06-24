package database_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

// TestNameOrganizationPair_Value verifies that NameOrganizationPair serializes
// to a Postgres composite literal that Postgres can parse, even when the
// name contains characters that would otherwise produce a malformed record
// literal (parentheses, commas, double quotes, backslashes, whitespace).
//
// Valid Coder identifiers (alphanumeric with hyphens) must round-trip
// unchanged so existing rows continue to match.
func TestNameOrganizationPair_Value(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("e3b15ce2-0b6a-4934-a9aa-1d28613edb3e")
	suffix := "," + orgID.String() + ")"

	tests := []struct {
		name       string
		roleName   string
		wantPrefix string
	}{
		// Valid Coder identifiers must serialize without quoting so existing
		// rows continue to match byte-for-byte.
		{"Simple", "owner", "(owner"},
		{"Hyphen", "template-admin", "(template-admin"},
		{"OrgRole", "organization-admin", "(organization-admin"},
		{"WorkspaceCreationBan", "organization-workspace-creation-ban", "(organization-workspace-creation-ban"},
		{"Numeric", "role123", "(role123"},

		// Special characters must be quoted and, where required, escaped.
		{"Space", "with space", `("with space"`},
		{"Tab", "with\ttab", "(\"with\ttab\""},
		{"Newline", "with\nnewline", "(\"with\nnewline\""},
		{"Comma", "with,comma", `("with,comma"`},
		{"OpenParen", "with(paren", `("with(paren"`},
		{"CloseParen", "with)paren", `("with)paren"`},
		{"DoubleQuote", `with"quote`, `("with\"quote"`},
		{"Backslash", `with\backslash`, `("with\\backslash"`},
		{"BothEscaped", `mix "and" \slash`, `("mix \"and\" \\slash"`},

		// Empty and the literal "NULL" must be quoted to avoid Postgres
		// parsing them as a NULL field.
		{"Empty", "", `(""`},
		{"NullUppercase", "NULL", `("NULL"`},
		{"NullLowercase", "null", `("null"`},
		{"NullMixedCase", "Null", `("Null"`},

		// Regression: the exact value reported by the customer, which crashed
		// OAuth login with a malformed record literal error.
		{"CustomerReport", "Atlassian Developers (Bitbucket and Bamboo)", `("Atlassian Developers (Bitbucket and Bamboo)"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := database.NameOrganizationPair{
				Name:           tc.roleName,
				OrganizationID: orgID,
			}.Value()
			require.NoError(t, err)
			require.Equal(t, tc.wantPrefix+suffix, got)
		})
	}
}
