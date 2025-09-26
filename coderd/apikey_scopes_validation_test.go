package coderd_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
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

	fetchToken := func(tokenName string) codersdk.APIKeyWithOwner {
		t.Helper()
		tokens, err := client.Tokens(ctx, codersdk.Me, codersdk.TokensFilter{})
		require.NoError(t, err)
		for _, token := range tokens {
			if token.TokenName == tokenName {
				return token
			}
		}
		names := make([]string, 0, len(tokens))
		for _, token := range tokens {
			names = append(names, token.TokenName)
		}
		t.Fatalf("token %q not found, available tokens: %v", tokenName, names)
		return codersdk.APIKeyWithOwner{}
	}

	// Invalid resource type should be rejected.
	_, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead},
		AllowList: []codersdk.APIAllowListTarget{
			{Type: codersdk.RBACResource("unknown"), ID: uuid.New().String()},
		},
	})
	require.Error(t, err)

	// Valid typed allow list should succeed.
	typedTarget := codersdk.AllowResourceTarget(codersdk.ResourceWorkspace, uuid.New())
	typedTokenName := "workspace-target"
	resp, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Scopes:    []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead},
		TokenName: typedTokenName,
		AllowList: []codersdk.APIAllowListTarget{typedTarget},
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Key)
	token := fetchToken(typedTokenName)
	require.Len(t, token.AllowList, 1)
	require.Equal(t, typedTarget.String(), token.AllowList[0].String())

	// Wildcard resource allow list should succeed.
	workspaceWildcard := codersdk.AllowTypeTarget(codersdk.ResourceWorkspace)
	workspaceWildcardName := "workspace-wildcard"
	resp, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Scopes:    []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead},
		TokenName: workspaceWildcardName,
		AllowList: []codersdk.APIAllowListTarget{workspaceWildcard},
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Key)
	token = fetchToken(workspaceWildcardName)
	require.Len(t, token.AllowList, 1)
	require.Equal(t, workspaceWildcard.String(), token.AllowList[0].String(), "typed wildcard preserves resource type wildcard")

	// Full wildcard allow list should succeed.
	fullWildcard := codersdk.AllowAllTarget()
	fullWildcardName := "wildcard-all"
	resp, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Scopes:    []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead},
		TokenName: fullWildcardName,
		AllowList: []codersdk.APIAllowListTarget{fullWildcard},
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Key)
	token = fetchToken(fullWildcardName)
	require.Len(t, token.AllowList, 1)
	require.Equal(t, fullWildcard.String(), token.AllowList[0].String())
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
