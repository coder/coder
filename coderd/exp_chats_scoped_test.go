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

// TestCoderAgentsScopedTokens runs the API calls that our CI jobs make with
// tokens that hold only the scopes each job needs.
func TestCoderAgentsScopedTokens(t *testing.T) {
	t.Parallel()

	admin := newChatClient(t)
	first := coderdtest.CreateFirstUser(t, admin.Client)
	orgID := first.OrganizationID
	model := createChatModel(t, admin)
	mcpServer := createMCPServerConfig(t, admin.Client, orgID, "existing", true)

	scoped := func(t *testing.T, scopes ...codersdk.APIKeyScope) *codersdk.Client {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitShort)
		token, err := admin.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{Scopes: scopes})
		require.NoError(t, err)
		return codersdk.New(admin.URL, codersdk.WithSessionToken(token.Key))
	}

	// The coderd Terraform provider makes these calls to plan the Coder Agents
	// resources of a deployment.
	plan := func(ctx context.Context, t *testing.T, client *codersdk.Client) {
		t.Helper()
		_, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		_, err = client.OrganizationByName(ctx, codersdk.DefaultOrganization)
		require.NoError(t, err)
		// A list drops the rows that the scopes do not cover instead of failing.
		members, err := client.OrganizationMembers(ctx, orgID)
		require.NoError(t, err)
		require.NotEmpty(t, members)
		_, err = client.AIProvider(ctx, model.AIProviderID.String())
		require.NoError(t, err)
		_, err = client.ChatModels(ctx, orgID)
		require.NoError(t, err)
		_, err = client.ChatModel(ctx, orgID, model.ID)
		require.NoError(t, err)
		servers, err := client.MCPServerConfigs(ctx, orgID)
		require.NoError(t, err)
		require.NotEmpty(t, servers)
		_, err = client.MCPServerConfigByID(ctx, orgID, mcpServer.ID)
		require.NoError(t, err)
		_, err = client.GetChatSystemPrompt(ctx)
		require.NoError(t, err)
	}

	t.Run("TerraformPlan", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		plan(ctx, t, scoped(t,
			codersdk.APIKeyScopeUserRead,
			codersdk.APIKeyScopeOrganizationRead,
			codersdk.APIKeyScopeOrganizationMemberRead,
			codersdk.APIKeyScopeAssignRoleRead,
			codersdk.APIKeyScopeAiProviderRead,
			codersdk.APIKeyScopeChatModelConfigRead,
			codersdk.APIKeyScopeMcpServerConfigRead,
			codersdk.APIKeyScopeDeploymentConfigUpdate,
		))
	})

	t.Run("TerraformApply", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := scoped(t,
			codersdk.APIKeyScopeUserRead,
			codersdk.APIKeyScopeOrganizationRead,
			codersdk.APIKeyScopeOrganizationMemberRead,
			codersdk.APIKeyScopeAssignRoleRead,
			codersdk.APIKeyScopeAiProviderAll,
			codersdk.APIKeyScopeChatModelConfigAll,
			codersdk.APIKeyScopeMcpServerConfigAll,
			codersdk.APIKeyScopeDeploymentConfigUpdate,
		)
		plan(ctx, t, client)

		name := "applied-" + uuid.NewString()
		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderType(coderdtest.TestChatProviderOpenAICompat),
			Name:    name,
			BaseURL: "https://example.com/v1",
			Enabled: true,
			APIKeys: []string{coderdtest.TestChatProviderAPIKey},
		})
		require.NoError(t, err)
		_, err = client.UpdateAIProvider(ctx, provider.ID.String(), codersdk.UpdateAIProviderRequest{DisplayName: &name})
		require.NoError(t, err)

		contextLimit := int64(4096)
		applied, err := client.CreateChatModel(ctx, orgID, codersdk.CreateChatModelRequest{
			AIProviderID: &provider.ID,
			Model:        coderdtest.TestChatModelOpenAICompat,
			ContextLimit: &contextLimit,
		})
		require.NoError(t, err)
		_, err = client.UpdateChatModel(ctx, orgID, applied.ID, codersdk.UpdateChatModelRequest{DisplayName: name})
		require.NoError(t, err)
		require.NoError(t, client.DeleteChatModel(ctx, orgID, applied.ID))
		require.NoError(t, client.DeleteAIProvider(ctx, provider.ID.String()))

		server := createMCPServerConfig(t, client, orgID, "applied", true)
		_, err = client.UpdateMCPServerConfig(ctx, orgID, server.ID, codersdk.UpdateMCPServerConfigRequest{DisplayName: &name})
		require.NoError(t, err)
		require.NoError(t, client.DeleteMCPServerConfig(ctx, orgID, server.ID))

		require.NoError(t, client.UpdateChatSystemPrompt(ctx, codersdk.UpdateChatSystemPromptRequest{SystemPrompt: name}))
	})

	// triage-via-chat-api.yaml starts a chat and polls it.
	t.Run("ChatTriage", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := scoped(t,
			codersdk.APIKeyScopeOrganizationRead,
			codersdk.APIKeyScopeChatCreate,
			codersdk.APIKeyScopeChatRead,
			codersdk.APIKeyScopeChatUpdate,
			codersdk.APIKeyScopeChatModelConfigRead,
		)

		orgs, err := client.Organizations(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, orgs)
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: orgID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "triage this issue"}},
		})
		require.NoError(t, err)
		_, err = client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
	})
}
