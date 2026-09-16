package codersdk_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestValidateOAuth2ScopeList(t *testing.T) {
	t.Parallel()

	names := func(n int) string {
		parts := make([]string, 0, n)
		for i := range n {
			parts = append(parts, fmt.Sprintf("s%d", i))
		}
		return strings.Join(parts, " ")
	}

	cases := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "empty", raw: ""},
		{name: "one", raw: "workspace:read"},
		{name: "unknown_names_pass", raw: "openid profile email"},
		{name: "at_name_limit", raw: names(codersdk.OAuth2ScopeListMaxNames)},
		{
			name:    "over_name_limit",
			raw:     names(codersdk.OAuth2ScopeListMaxNames + 1),
			wantErr: "at most 100 names",
		},
		{name: "at_byte_limit", raw: strings.Repeat("a", codersdk.OAuth2ScopeListMaxBytes)},
		{
			name:    "over_byte_limit",
			raw:     strings.Repeat("a", codersdk.OAuth2ScopeListMaxBytes+1),
			wantErr: "at most 4096 bytes",
		},
		{
			// The byte check runs first, so a long list reports the byte limit.
			name:    "long_and_many",
			raw:     strings.Repeat("a ", codersdk.OAuth2ScopeListMaxBytes),
			wantErr: "at most 4096 bytes",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := codersdk.ValidateOAuth2ScopeList(test.raw)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}
