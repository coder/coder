package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestTokenCreation_ScopeValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		scope   codersdk.APIKeyScope
		wantErr bool
	}{
		{name: "AllowsPublicLowLevelScope", scope: "workspace:read", wantErr: false},
		{name: "RejectsInternalOnlyScope", scope: "debug_info:read", wantErr: true},
		{name: "AllowsLegacyScopes", scope: "application_connect", wantErr: false},
		{name: "AllowsCanonicalSpecialScope", scope: "all", wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, nil)
			_ = coderdtest.CreateFirstUser(t, client)

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitShort)
			defer cancel()

			resp, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{Scope: tc.scope})
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, resp.Key)
		})
	}
}
