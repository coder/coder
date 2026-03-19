package coderd_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// mcpDeploymentValues returns deployment values with the agents
// experiment enabled, which is required by the MCP server config
// endpoints.
func mcpDeploymentValues(t testing.TB) *codersdk.DeploymentValues {
	t.Helper()

	values := coderdtest.DeploymentValues(t)
	values.Experiments = []string{string(codersdk.ExperimentAgents)}
	return values
}

// newMCPClient creates a test server with the agents experiment
// enabled and returns the admin client.
func newMCPClient(t testing.TB) *codersdk.Client {
	t.Helper()

	return coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: mcpDeploymentValues(t),
	})
}

// createMCPServerConfig is a helper that creates a minimal enabled
// MCP server config with auth_type=none.
func createMCPServerConfig(t testing.TB, client *codersdk.Client, slug string, enabled bool) codersdk.MCPServerConfig {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	config, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
		DisplayName:   "Test Server " + slug,
		Slug:          slug,
		Description:   "A test MCP server.",
		IconURL:       "https://example.com/icon.png",
		Transport:     "streamable_http",
		URL:           "https://mcp.example.com/" + slug,
		AuthType:      "none",
		Availability:  "default_on",
		Enabled:       enabled,
		ToolAllowList: []string{},
		ToolDenyList:  []string{},
	})
	require.NoError(t, err)
	return config
}

func TestMCPServerConfigsCRUD(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := newMCPClient(t)
	_ = coderdtest.CreateFirstUser(t, client)

	// Create a config with all fields populated including OAuth2
	// secrets so we can verify they are not leaked.
	created, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
		DisplayName:        "My MCP Server",
		Slug:               "my-mcp-server",
		Description:        "Integration test server.",
		IconURL:            "https://example.com/icon.png",
		Transport:          "streamable_http",
		URL:                "https://mcp.example.com/v1",
		AuthType:           "oauth2",
		OAuth2ClientID:     "client-id-123",
		OAuth2ClientSecret: "super-secret-value",
		OAuth2AuthURL:      "https://auth.example.com/authorize",
		OAuth2TokenURL:     "https://auth.example.com/token",
		OAuth2Scopes:       "read write",
		Availability:       "default_on",
		Enabled:            true,
		ToolAllowList:      []string{},
		ToolDenyList:       []string{},
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, created.ID)
	require.Equal(t, "My MCP Server", created.DisplayName)
	require.Equal(t, "my-mcp-server", created.Slug)
	require.Equal(t, "Integration test server.", created.Description)
	require.Equal(t, "streamable_http", created.Transport)
	require.Equal(t, "https://mcp.example.com/v1", created.URL)
	require.Equal(t, "oauth2", created.AuthType)
	require.Equal(t, "client-id-123", created.OAuth2ClientID)
	require.Equal(t, "default_on", created.Availability)
	require.True(t, created.Enabled)

	// Verify the secret is indicated but never returned.
	require.True(t, created.HasOAuth2Secret)

	// Verify the config appears in the list.
	configs, err := client.MCPServerConfigs(ctx)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	require.Equal(t, created.ID, configs[0].ID)
	require.True(t, configs[0].HasOAuth2Secret)

	// Update display name and availability.
	newName := "Renamed Server"
	newAvail := "force_on"
	updated, err := client.UpdateMCPServerConfig(ctx, created.ID, codersdk.UpdateMCPServerConfigRequest{
		DisplayName:  &newName,
		Availability: &newAvail,
	})
	require.NoError(t, err)
	require.Equal(t, "Renamed Server", updated.DisplayName)
	require.Equal(t, "force_on", updated.Availability)
	// Unchanged fields should remain the same.
	require.Equal(t, "my-mcp-server", updated.Slug)
	require.Equal(t, "oauth2", updated.AuthType)

	// Verify the update took effect through the list.
	configs, err = client.MCPServerConfigs(ctx)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	require.Equal(t, "Renamed Server", configs[0].DisplayName)
	require.Equal(t, "force_on", configs[0].Availability)

	// Delete it.
	err = client.DeleteMCPServerConfig(ctx, created.ID)
	require.NoError(t, err)

	// Verify it's gone.
	configs, err = client.MCPServerConfigs(ctx)
	require.NoError(t, err)
	require.Empty(t, configs)
}

func TestMCPServerConfigsNonAdmin(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newMCPClient(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

	// Admin creates two configs: one enabled, one disabled.
	_ = createMCPServerConfig(t, adminClient, "enabled-server", true)
	_ = createMCPServerConfig(t, adminClient, "disabled-server", false)

	// Admin sees both.
	adminConfigs, err := adminClient.MCPServerConfigs(ctx)
	require.NoError(t, err)
	require.Len(t, adminConfigs, 2)

	// Regular user sees only the enabled one.
	memberConfigs, err := memberClient.MCPServerConfigs(ctx)
	require.NoError(t, err)
	require.Len(t, memberConfigs, 1)
	require.Equal(t, "enabled-server", memberConfigs[0].Slug)
}

// TestMCPServerConfigsSecretsNeverLeaked is a load-bearing test that
// ensures secret fields (OAuth2 client secret, API key value, custom
// headers) are never present in API responses for any caller. If this
// test fails, it means a code change accidentally started exposing
// secrets. See: https://github.com/coder/coder/pull/23227#discussion_r2959461109
func TestMCPServerConfigsSecretsNeverLeaked(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newMCPClient(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

	// Create a config with ALL secret fields populated.
	created, err := adminClient.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
		DisplayName:        "Secrets Test",
		Slug:               "secrets-test",
		Transport:          "streamable_http",
		URL:                "https://mcp.example.com/secrets",
		AuthType:           "oauth2",
		OAuth2ClientID:     "client-id-secret-test",
		OAuth2ClientSecret: "THIS-IS-A-SECRET-VALUE",
		OAuth2AuthURL:      "https://auth.example.com/authorize",
		OAuth2TokenURL:     "https://auth.example.com/token",
		OAuth2Scopes:       "read write",
		APIKeyHeader:       "X-Api-Key",
		APIKeyValue:        "THIS-IS-A-SECRET-API-KEY",
		CustomHeaders:      map[string]string{"X-Custom": "THIS-IS-A-SECRET-HEADER"},
		Availability:       "default_on",
		Enabled:            true,
		ToolAllowList:      []string{},
		ToolDenyList:       []string{},
	})
	require.NoError(t, err)

	// The sentinel values we must never see in any JSON response.
	secrets := []string{
		"THIS-IS-A-SECRET-VALUE",
		"THIS-IS-A-SECRET-API-KEY",
		"THIS-IS-A-SECRET-HEADER",
	}

	assertNoSecrets := func(t *testing.T, label string, v interface{}) {
		t.Helper()
		data, err := json.Marshal(v)
		require.NoError(t, err)
		jsonStr := string(data)
		for _, secret := range secrets {
			assert.False(t, strings.Contains(jsonStr, secret),
				"%s: JSON response contains secret %q", label, secret)
		}
	}

	// Verify the create response doesn't leak secrets.
	assertNoSecrets(t, "admin create response", created)

	// Verify boolean indicators are set correctly.
	require.True(t, created.HasOAuth2Secret, "HasOAuth2Secret should be true")
	require.True(t, created.HasAPIKey, "HasAPIKey should be true")
	require.True(t, created.HasCustomHeaders, "HasCustomHeaders should be true")

	// Admin list endpoint.
	adminConfigs, err := adminClient.MCPServerConfigs(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, adminConfigs)
	for _, cfg := range adminConfigs {
		assertNoSecrets(t, "admin list", cfg)
	}

	// Admin get-by-ID endpoint.
	adminSingle, err := adminClient.MCPServerConfigByID(ctx, created.ID)
	require.NoError(t, err)
	assertNoSecrets(t, "admin get-by-id", adminSingle)

	// Non-admin list endpoint.
	memberConfigs, err := memberClient.MCPServerConfigs(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, memberConfigs)
	for _, cfg := range memberConfigs {
		assertNoSecrets(t, "member list", cfg)
		// Non-admin should also not see admin-only fields.
		assert.Empty(t, cfg.OAuth2ClientID, "member should not see OAuth2ClientID")
		assert.Empty(t, cfg.OAuth2AuthURL, "member should not see OAuth2AuthURL")
		assert.Empty(t, cfg.OAuth2TokenURL, "member should not see OAuth2TokenURL")
		assert.Empty(t, cfg.APIKeyHeader, "member should not see APIKeyHeader")
		assert.Empty(t, cfg.OAuth2Scopes, "member should not see OAuth2Scopes")
		assert.Empty(t, cfg.URL, "member should not see URL")
		assert.Empty(t, cfg.Transport, "member should not see Transport")
	}

	// Non-admin get-by-ID endpoint.
	memberSingle, err := memberClient.MCPServerConfigByID(ctx, created.ID)
	require.NoError(t, err)
	assertNoSecrets(t, "member get-by-id", memberSingle)
	assert.Empty(t, memberSingle.OAuth2ClientID, "member should not see OAuth2ClientID")
	assert.Empty(t, memberSingle.OAuth2AuthURL, "member should not see OAuth2AuthURL")
	assert.Empty(t, memberSingle.OAuth2TokenURL, "member should not see OAuth2TokenURL")
	assert.Empty(t, memberSingle.OAuth2Scopes, "member should not see OAuth2Scopes")
	assert.Empty(t, memberSingle.APIKeyHeader, "member should not see APIKeyHeader")
	assert.Empty(t, memberSingle.URL, "member should not see URL")
	assert.Empty(t, memberSingle.Transport, "member should not see Transport")
}

func TestMCPServerConfigsAuthConnected(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newMCPClient(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

	// Create an oauth2 server config (enabled).
	created, err := adminClient.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
		DisplayName:    "OAuth Server",
		Slug:           "oauth-server",
		Transport:      "streamable_http",
		URL:            "https://mcp.example.com/oauth",
		AuthType:       "oauth2",
		OAuth2ClientID: "cid",
		OAuth2AuthURL:  "https://auth.example.com/authorize",
		OAuth2TokenURL: "https://auth.example.com/token",
		Availability:   "default_on",
		Enabled:        true,
		ToolAllowList:  []string{},
		ToolDenyList:   []string{},
	})
	require.NoError(t, err)

	// Regular user lists configs — auth_connected should be false
	// because no token has been stored.
	memberConfigs, err := memberClient.MCPServerConfigs(ctx)
	require.NoError(t, err)
	require.Len(t, memberConfigs, 1)
	require.Equal(t, created.ID, memberConfigs[0].ID)
	require.False(t, memberConfigs[0].AuthConnected)

	// Also create a non-oauth server. It should report
	// auth_connected=true because no auth is needed.
	_ = createMCPServerConfig(t, adminClient, "no-auth-server", true)
	memberConfigs, err = memberClient.MCPServerConfigs(ctx)
	require.NoError(t, err)
	require.Len(t, memberConfigs, 2)
	for _, cfg := range memberConfigs {
		if cfg.AuthType == "none" {
			require.True(t, cfg.AuthConnected)
		} else {
			require.False(t, cfg.AuthConnected)
		}
	}
}

func TestMCPServerConfigsAvailability(t *testing.T) {
	t.Parallel()

	client := newMCPClient(t)
	_ = coderdtest.CreateFirstUser(t, client)

	validValues := []string{"force_on", "default_on", "default_off"}
	for _, av := range validValues {
		av := av
		t.Run(av, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			created, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
				DisplayName:   "Server " + av,
				Slug:          "server-" + av,
				Transport:     "streamable_http",
				URL:           "https://mcp.example.com/" + av,
				AuthType:      "none",
				Availability:  av,
				Enabled:       true,
				ToolAllowList: []string{},
				ToolDenyList:  []string{},
			})
			require.NoError(t, err)
			require.Equal(t, av, created.Availability)
		})
	}

	t.Run("InvalidAvailability", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:   "Bad Availability",
			Slug:          "bad-avail",
			Transport:     "streamable_http",
			URL:           "https://mcp.example.com/bad",
			AuthType:      "none",
			Availability:  "always_on",
			Enabled:       true,
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})
}

func TestMCPServerConfigsUniqueSlug(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := newMCPClient(t)
	_ = coderdtest.CreateFirstUser(t, client)

	_, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
		DisplayName:   "First",
		Slug:          "test-server",
		Transport:     "streamable_http",
		URL:           "https://mcp.example.com/first",
		AuthType:      "none",
		Availability:  "default_off",
		Enabled:       true,
		ToolAllowList: []string{},
		ToolDenyList:  []string{},
	})
	require.NoError(t, err)

	// Attempt to create another config with the same slug.
	_, err = client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
		DisplayName:   "Second",
		Slug:          "test-server",
		Transport:     "streamable_http",
		URL:           "https://mcp.example.com/second",
		AuthType:      "none",
		Availability:  "default_off",
		Enabled:       true,
		ToolAllowList: []string{},
		ToolDenyList:  []string{},
	})
	require.Error(t, err)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
}

func TestMCPServerConfigsOAuth2Disconnect(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newMCPClient(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

	created, err := adminClient.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
		DisplayName:    "OAuth Disconnect Test",
		Slug:           "oauth-disconnect",
		Transport:      "streamable_http",
		URL:            "https://mcp.example.com/oauth-disc",
		AuthType:       "oauth2",
		OAuth2ClientID: "cid",
		OAuth2AuthURL:  "https://auth.example.com/authorize",
		OAuth2TokenURL: "https://auth.example.com/token",
		Availability:   "default_on",
		Enabled:        true,
		ToolAllowList:  []string{},
		ToolDenyList:   []string{},
	})
	require.NoError(t, err)

	// Disconnect should succeed even when no token exists (idempotent).
	err = memberClient.MCPServerOAuth2Disconnect(ctx, created.ID)
	require.NoError(t, err)
}

func TestChatWithMCPServerIDs(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := newMCPClient(t)
	_ = coderdtest.CreateFirstUser(t, client)

	// Create the chat model config required for creating a chat.
	_ = createChatModelConfigForMCP(t, client)

	// Create an enabled MCP server config.
	mcpConfig := createMCPServerConfig(t, client, "chat-mcp-server", true)

	// Create a chat referencing the MCP server.
	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello with mcp server",
			},
		},
		MCPServerIDs: []uuid.UUID{mcpConfig.ID},
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, chat.ID)
	require.Contains(t, chat.MCPServerIDs, mcpConfig.ID)

	// Fetch the chat and verify the MCP server IDs persist.
	fetched, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Contains(t, fetched.MCPServerIDs, mcpConfig.ID)
}

// createChatModelConfigForMCP sets up a chat provider and model
// config so that CreateChat succeeds. This mirrors the helper in
// chats_test.go but is defined here to avoid coupling.
func createChatModelConfigForMCP(t testing.TB, client *codersdk.Client) codersdk.ChatModelConfig {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai",
		APIKey:   "test-api-key",
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	modelConfig, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)
	return modelConfig
}
