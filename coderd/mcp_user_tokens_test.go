package coderd_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMCPServerUserAPIKeyTokens(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	aliceClient, alice := coderdtest.CreateAnotherUser(t, client.Client, orgID)
	bobClient, bob := coderdtest.CreateAnotherUser(t, client.Client, orgID)

	// The config stores how tokens are sent, never a shared token.
	config, err := client.CreateMCPServerConfig(ctx, orgID, codersdk.CreateMCPServerConfigRequest{
		DisplayName:  "GitHub",
		Slug:         "github",
		Transport:    "streamable_http",
		URL:          "https://mcp.example.com/github",
		AuthType:     codersdk.MCPServerAuthTypeUserAPIKey,
		APIKeyValue:  "shared-secret",
		Availability: "default_off",
		Enabled:      true,
	})
	require.NoError(t, err)
	require.Equal(t, "Authorization", config.APIKeyHeader)
	require.Equal(t, "Bearer ", config.APIKeyPrefix)
	require.False(t, config.HasAPIKey)
	require.False(t, config.AuthConnected)

	badHeader := "Bad Header"
	_, err = client.UpdateMCPServerConfig(ctx, orgID, config.ID, codersdk.UpdateMCPServerConfigRequest{APIKeyHeader: &badHeader})
	require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())

	// Every user stores their own token.
	status, err := aliceClient.SetMCPServerUserToken(ctx, config.ID, codersdk.UpdateMCPServerUserTokenRequest{Token: " alice-token "})
	require.NoError(t, err)
	require.True(t, status.HasToken)
	require.NotNil(t, status.UpdatedAt)
	_, err = bobClient.SetMCPServerUserToken(ctx, config.ID, codersdk.UpdateMCPServerUserTokenRequest{Token: "bob-token"})
	require.NoError(t, err)
	_, err = aliceClient.SetMCPServerUserToken(ctx, config.ID, codersdk.UpdateMCPServerUserTokenRequest{Token: "line\nbreak"})
	require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())

	//nolint:gocritic // Reads the stored tokens directly.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	for user, want := range map[uuid.UUID]string{alice.ID: "alice-token", bob.ID: "bob-token"} {
		tokens, err := db.GetMCPServerUserTokensByUserID(sysCtx, user)
		require.NoError(t, err)
		require.Len(t, tokens, 1)
		require.Equal(t, want, tokens[0].AccessToken)
	}
	adminTokens, err := db.GetMCPServerUserTokensByUserID(sysCtx, firstUser.UserID)
	require.NoError(t, err)
	require.Empty(t, adminTokens)

	// Tokens are reported as connected, never returned.
	configs, err := aliceClient.MCPServerConfigs(ctx, orgID)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	require.True(t, configs[0].AuthConnected)
	require.Equal(t, "Authorization", configs[0].APIKeyHeader)
	raw, err := json.Marshal(configs)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "alice-token")
	adminView, err := client.MCPServerConfigByID(ctx, orgID, config.ID)
	require.NoError(t, err)
	require.False(t, adminView.AuthConnected, "the admin has no token of their own")

	// Removing a token affects only its owner.
	require.NoError(t, aliceClient.DeleteMCPServerUserToken(ctx, config.ID))
	fetched, err := aliceClient.MCPServerConfigByID(ctx, orgID, config.ID)
	require.NoError(t, err)
	require.False(t, fetched.AuthConnected)
	fetched, err = bobClient.MCPServerConfigByID(ctx, orgID, config.ID)
	require.NoError(t, err)
	require.True(t, fetched.AuthConnected)

	// Servers that do not use personal tokens, and servers the user cannot
	// read, refuse them.
	plain := createMCPServerConfig(t, client.Client, orgID, "plain", true)
	_, err = aliceClient.SetMCPServerUserToken(ctx, plain.ID, codersdk.UpdateMCPServerUserTokenRequest{Token: "x"})
	require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())
	require.NoError(t, client.UpdateMCPServerConfigACL(ctx, orgID, config.ID, codersdk.UpdateMCPServerConfigACLRequest{
		GroupRoles: map[string]codersdk.MCPServerConfigRole{orgID.String(): codersdk.MCPServerConfigRoleDeleted},
	}))
	_, err = aliceClient.SetMCPServerUserToken(ctx, config.ID, codersdk.UpdateMCPServerUserTokenRequest{Token: "x"})
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())

	// Changing where tokens are sent removes them.
	newURL := "https://other.example.com/github"
	_, err = client.UpdateMCPServerConfig(ctx, orgID, config.ID, codersdk.UpdateMCPServerConfigRequest{URL: &newURL})
	require.NoError(t, err)
	bobTokens, err := db.GetMCPServerUserTokensByUserID(sysCtx, bob.ID)
	require.NoError(t, err)
	require.Empty(t, bobTokens)
}

func TestChatProjectMCPServerTokens(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	editorClient, editor := coderdtest.CreateAnotherUser(t, client.Client, orgID)
	editorExp := codersdk.NewExperimentalClient(editorClient)
	strangerClient, _ := coderdtest.CreateAnotherUser(t, client.Client, orgID)
	stranger := codersdk.NewExperimentalClient(strangerClient)

	keyed, err := client.CreateMCPServerConfig(ctx, orgID, codersdk.CreateMCPServerConfigRequest{
		DisplayName:  "Slack",
		Slug:         "slack",
		Transport:    "streamable_http",
		URL:          "https://mcp.example.com/slack",
		AuthType:     codersdk.MCPServerAuthTypeUserAPIKey,
		Availability: "default_off",
		Enabled:      true,
	})
	require.NoError(t, err)
	plain := createMCPServerConfig(t, client.Client, orgID, "docs", true)
	unattached, err := client.CreateMCPServerConfig(ctx, orgID, codersdk.CreateMCPServerConfigRequest{
		DisplayName:  "Other",
		Slug:         "other",
		Transport:    "streamable_http",
		URL:          "https://mcp.example.com/other",
		AuthType:     codersdk.MCPServerAuthTypeUserAPIKey,
		Availability: "default_off",
		Enabled:      true,
	})
	require.NoError(t, err)

	project := createChatProject(t, client, orgID, "Slack Bot")
	servers := []uuid.UUID{keyed.ID, plain.ID}
	_, err = client.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{MCPServerIDs: &servers})
	require.NoError(t, err)
	require.NoError(t, client.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACLRequest{
		UserRoles: map[string]codersdk.ChatProjectRole{editor.ID.String(): codersdk.ChatProjectRoleEdit},
	}))

	list, err := client.ChatProjectMCPServerTokens(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	byID := map[uuid.UUID]codersdk.ChatProjectMCPServerToken{}
	for _, entry := range list {
		byID[entry.MCPServerConfigID] = entry
	}
	require.True(t, byID[keyed.ID].TokenRequired)
	require.False(t, byID[keyed.ID].HasToken)
	require.False(t, byID[plain.ID].TokenRequired)

	// Only the agent's owner sets the agent's token.
	_, err = editorExp.SetChatProjectMCPServerToken(ctx, orgID, project.ID, keyed.ID, codersdk.UpdateMCPServerUserTokenRequest{Token: "xoxb-editor"})
	require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())
	_, err = stranger.SetChatProjectMCPServerToken(ctx, orgID, project.ID, keyed.ID, codersdk.UpdateMCPServerUserTokenRequest{Token: "xoxb-stranger"})
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	_, err = client.SetChatProjectMCPServerToken(ctx, orgID, project.ID, unattached.ID, codersdk.UpdateMCPServerUserTokenRequest{Token: "xoxb-1"})
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	_, err = client.SetChatProjectMCPServerToken(ctx, orgID, project.ID, plain.ID, codersdk.UpdateMCPServerUserTokenRequest{Token: "xoxb-1"})
	require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())

	status, err := client.SetChatProjectMCPServerToken(ctx, orgID, project.ID, keyed.ID, codersdk.UpdateMCPServerUserTokenRequest{Token: "xoxb-agent"})
	require.NoError(t, err)
	require.True(t, status.HasToken)

	// The token belongs to the agent's user, not the owner who set it.
	//nolint:gocritic // Reads the stored tokens directly.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	agentTokens, err := db.GetMCPServerUserTokensByUserID(sysCtx, project.AgentUser.ID)
	require.NoError(t, err)
	require.Len(t, agentTokens, 1)
	require.Equal(t, "xoxb-agent", agentTokens[0].AccessToken)
	ownerTokens, err := db.GetMCPServerUserTokensByUserID(sysCtx, firstUser.UserID)
	require.NoError(t, err)
	require.Empty(t, ownerTokens)

	// People the agent is shared with see whether a token is set, never the
	// token.
	list, err = editorExp.ChatProjectMCPServerTokens(ctx, orgID, project.ID)
	require.NoError(t, err)
	raw, err := json.Marshal(list)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "xoxb-agent")
	for _, entry := range list {
		if entry.MCPServerConfigID == keyed.ID {
			require.True(t, entry.HasToken)
			require.NotNil(t, entry.TokenUpdatedAt)
		}
	}

	require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t,
		editorExp.DeleteChatProjectMCPServerToken(ctx, orgID, project.ID, keyed.ID)).StatusCode())
	require.NoError(t, client.DeleteChatProjectMCPServerToken(ctx, orgID, project.ID, keyed.ID))
	agentTokens, err = db.GetMCPServerUserTokensByUserID(sysCtx, project.AgentUser.ID)
	require.NoError(t, err)
	require.Empty(t, agentTokens)
}
