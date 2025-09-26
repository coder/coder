package coderd_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/x/wildcard"
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
		{name: "AllowsLegacyScopes2", scope: "all", wantErr: false},
		{name: "AllowsCanonicalSpecialScope", scope: "coder:all", wantErr: false},
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

			// Fetch and verify the stored scopes match expectation.
			keys, err := client.Tokens(ctx, codersdk.Me, codersdk.TokensFilter{})
			require.NoError(t, err)
			require.Len(t, keys, 1)

			// Normalize legacy singular scopes to canonical coder:* values.
			expected := tc.scope
			switch tc.scope {
			case codersdk.APIKeyScopeAll:
				expected = codersdk.APIKeyScopeCoderAll
			case codersdk.APIKeyScopeApplicationConnect:
				expected = codersdk.APIKeyScopeCoderApplicationConnect
			}

			require.Contains(t, keys[0].Scopes, expected)
		})
	}
}

func TestTokenCreation_AllowListValidation(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitShort)
	defer cancel()

	// Invalid resource type should be rejected.
	_, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead},
		AllowList: []codersdk.APIAllowListTarget{
			{Type: wildcard.Of(codersdk.RBACResource("unknown")), ID: wildcard.Of(uuid.New())},
		},
	})
	require.Error(t, err)

	// Valid typed allow list should succeed.
	resp, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Scopes:    []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead},
		AllowList: []codersdk.APIAllowListTarget{codersdk.AllowResourceTarget(codersdk.ResourceWorkspace, uuid.New())},
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Key)
}

func TestTokenCreationAllowsElevatedScopes(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	admin := coderdtest.New(t, nil)
	first := coderdtest.CreateFirstUser(t, admin)

	limitedClient, _ := coderdtest.CreateAnotherUser(t, admin, first.OrganizationID)

	resp, err := limitedClient.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeCoderWorkspacesDelete},
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Key)

	resp, err = limitedClient.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeCoderApikeysManageSelf},
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Key)
}
