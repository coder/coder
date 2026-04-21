package coderd_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	require.False(t, created.AllowInPlanMode)

	// Verify the secret is indicated but never returned.
	require.True(t, created.HasOAuth2Secret)

	// Verify the config appears in the list and direct get responses.
	configs, err := client.MCPServerConfigs(ctx)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	require.Equal(t, created.ID, configs[0].ID)
	require.True(t, configs[0].HasOAuth2Secret)
	require.False(t, configs[0].AllowInPlanMode)

	fetched, err := client.MCPServerConfigByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
	require.False(t, fetched.AllowInPlanMode)

	// Update display name, availability, and allow_in_plan_mode.
	newName := "Renamed Server"
	newAvail := "force_on"
	allowInPlanMode := true
	updated, err := client.UpdateMCPServerConfig(ctx, created.ID, codersdk.UpdateMCPServerConfigRequest{
		DisplayName:     &newName,
		Availability:    &newAvail,
		AllowInPlanMode: &allowInPlanMode,
	})
	require.NoError(t, err)
	require.Equal(t, "Renamed Server", updated.DisplayName)
	require.Equal(t, "force_on", updated.Availability)
	require.True(t, updated.AllowInPlanMode)
	// Unchanged fields should remain the same.
	require.Equal(t, "my-mcp-server", updated.Slug)
	require.Equal(t, "oauth2", updated.AuthType)

	// Verify the update took effect through the list and direct get.
	configs, err = client.MCPServerConfigs(ctx)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	require.Equal(t, "Renamed Server", configs[0].DisplayName)
	require.Equal(t, "force_on", configs[0].Availability)
	require.True(t, configs[0].AllowInPlanMode)

	fetched, err = client.MCPServerConfigByID(ctx, created.ID)
	require.NoError(t, err)
	require.True(t, fetched.AllowInPlanMode)

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

func TestMCPServerConfigsOAuth2AutoDiscovery(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		// Stand up a mock auth server that serves RFC 8414 metadata and
		// a RFC 7591 dynamic client registration endpoint.
		authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-authorization-server":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "` + r.Host + `",
					"authorization_endpoint": "` + "http://" + r.Host + `/authorize",
					"token_endpoint": "` + "http://" + r.Host + `/token",
					"registration_endpoint": "` + "http://" + r.Host + `/register",
					"response_types_supported": ["code"],
					"scopes_supported": ["read", "write"]
				}`))
			case "/register":
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"client_id": "auto-discovered-client-id",
					"client_secret": "auto-discovered-client-secret"
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(authServer.Close)

		// Stand up a mock MCP server that serves RFC 9728 Protected
		// Resource Metadata at the path-aware well-known URL.
		// The URL used for the config ends with /v1/mcp, so the
		// path-aware metadata URL is
		// /.well-known/oauth-protected-resource/v1/mcp.
		mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-protected-resource/v1/mcp":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"resource": "` + "http://" + r.Host + `",
					"authorization_servers": ["` + authServer.URL + `"]
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(mcpServer.Close)

		client := newMCPClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		// Create config with auth_type=oauth2 but no OAuth2 fields —
		// the server should auto-discover them.
		created, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:   "Auto-Discovery Server",
			Slug:          "auto-discovery",
			Transport:     "streamable_http",
			URL:           mcpServer.URL + "/v1/mcp",
			AuthType:      "oauth2",
			Availability:  "default_on",
			Enabled:       true,
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
		})
		require.NoError(t, err)
		require.Equal(t, "auto-discovered-client-id", created.OAuth2ClientID)
		require.True(t, created.HasOAuth2Secret)
		require.Equal(t, authServer.URL+"/authorize", created.OAuth2AuthURL)
		require.Equal(t, authServer.URL+"/token", created.OAuth2TokenURL)
		require.Equal(t, "read write", created.OAuth2Scopes)
	})

	// Verify that when both path-aware and root-level protected
	// resource metadata are available, the path-aware URL takes
	// priority. Each points to a different auth server so we can
	// distinguish which one was actually used.
	t.Run("PathAwareTakesPriority", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		// Auth server that returns "path-scope" as the supported
		// scope.
		pathAuthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-authorization-server":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "` + "http://" + r.Host + `",
					"authorization_endpoint": "` + "http://" + r.Host + `/authorize",
					"token_endpoint": "` + "http://" + r.Host + `/token",
					"registration_endpoint": "` + "http://" + r.Host + `/register",
					"response_types_supported": ["code"],
					"scopes_supported": ["path-scope"]
				}`))
			case "/register":
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"client_id": "path-client-id",
					"client_secret": "path-client-secret"
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(pathAuthServer.Close)

		// Auth server that returns "root-scope" as the supported
		// scope.
		rootAuthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-authorization-server":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "` + "http://" + r.Host + `",
					"authorization_endpoint": "` + "http://" + r.Host + `/authorize",
					"token_endpoint": "` + "http://" + r.Host + `/token",
					"registration_endpoint": "` + "http://" + r.Host + `/register",
					"response_types_supported": ["code"],
					"scopes_supported": ["root-scope"]
				}`))
			case "/register":
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"client_id": "root-client-id",
					"client_secret": "root-client-secret"
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(rootAuthServer.Close)

		// MCP server serves different protected resource metadata at
		// path-aware vs root URLs, each pointing to a different auth
		// server.
		mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-protected-resource/v1/mcp":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"resource": "` + "http://" + r.Host + `/v1/mcp",
					"authorization_servers": ["` + pathAuthServer.URL + `"]
				}`))
			case "/.well-known/oauth-protected-resource":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"resource": "` + "http://" + r.Host + `",
					"authorization_servers": ["` + rootAuthServer.URL + `"]
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(mcpServer.Close)

		client := newMCPClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		created, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:   "Priority Test",
			Slug:          "priority-test",
			Transport:     "streamable_http",
			URL:           mcpServer.URL + "/v1/mcp",
			AuthType:      "oauth2",
			Availability:  "default_on",
			Enabled:       true,
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
		})
		require.NoError(t, err)
		// The path-aware auth server returns "path-scope", the root
		// auth server returns "root-scope". If path-aware takes
		// priority, we get "path-scope".
		require.Equal(t, "path-client-id", created.OAuth2ClientID)
		require.Equal(t, "path-scope", created.OAuth2Scopes)
	})

	// Verify discovery works when the protected resource metadata
	// is only available at the root-level well-known URL (no path
	// component). This covers servers that don't use path-aware
	// metadata.
	t.Run("RootLevelFallback", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-authorization-server":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "` + r.Host + `",
					"authorization_endpoint": "` + "http://" + r.Host + `/authorize",
					"token_endpoint": "` + "http://" + r.Host + `/token",
					"registration_endpoint": "` + "http://" + r.Host + `/register",
					"response_types_supported": ["code"],
					"scopes_supported": ["all"]
				}`))
			case "/register":
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"client_id": "root-client-id",
					"client_secret": "root-client-secret"
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(authServer.Close)

		// MCP server only serves metadata at the root well-known
		// URL, NOT at the path-aware location.
		mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-protected-resource":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"resource": "` + "http://" + r.Host + `",
					"authorization_servers": ["` + authServer.URL + `"]
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(mcpServer.Close)

		client := newMCPClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		created, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:   "Root Fallback Server",
			Slug:          "root-fallback",
			Transport:     "streamable_http",
			URL:           mcpServer.URL + "/v1/mcp",
			AuthType:      "oauth2",
			Availability:  "default_on",
			Enabled:       true,
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
		})
		require.NoError(t, err)
		require.Equal(t, "root-client-id", created.OAuth2ClientID)
		require.True(t, created.HasOAuth2Secret)
		require.Equal(t, authServer.URL+"/authorize", created.OAuth2AuthURL)
		require.Equal(t, authServer.URL+"/token", created.OAuth2TokenURL)
		require.Equal(t, "all", created.OAuth2Scopes)
	})

	// Verify that when the authorization server issuer URL has a
	// path component (e.g. https://github.com/login/oauth), the
	// discovery uses the path-aware metadata URL per RFC 8414 §3.1.
	t.Run("PathAwareAuthServerMetadata", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		// Auth server that serves metadata at the path-aware URL.
		// The issuer URL is http://host/login/oauth, so the
		// metadata URL should be
		// /.well-known/oauth-authorization-server/login/oauth.
		authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-authorization-server/login/oauth":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "` + "http://" + r.Host + `/login/oauth",
					"authorization_endpoint": "` + "http://" + r.Host + `/login/oauth/authorize",
					"token_endpoint": "` + "http://" + r.Host + `/login/oauth/token",
					"registration_endpoint": "` + "http://" + r.Host + `/register",
					"response_types_supported": ["code"],
					"scopes_supported": ["repo", "read:org"]
				}`))
			case "/register":
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"client_id": "path-aware-client-id"
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(authServer.Close)

		// MCP server that points to an auth server with a path
		// in its issuer URL (like GitHub's /login/oauth).
		mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-protected-resource/mcp":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"resource": "` + "http://" + r.Host + `/mcp",
					"authorization_servers": ["` + authServer.URL + `/login/oauth"]
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(mcpServer.Close)

		client := newMCPClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		created, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:   "Path-Aware Auth",
			Slug:          "path-aware-auth",
			Transport:     "streamable_http",
			URL:           mcpServer.URL + "/mcp",
			AuthType:      "oauth2",
			Availability:  "default_on",
			Enabled:       true,
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
		})
		require.NoError(t, err)
		require.Equal(t, "path-aware-client-id", created.OAuth2ClientID)
		require.Equal(t, authServer.URL+"/login/oauth/authorize", created.OAuth2AuthURL)
		require.Equal(t, authServer.URL+"/login/oauth/token", created.OAuth2TokenURL)
		require.Equal(t, "repo read:org", created.OAuth2Scopes)
	})

	// Regression test: verify that during dynamic client registration
	// the redirect_uris sent to the authorization server contain the
	// real config UUID, NOT the literal string "{id}".  Before the
	// fix, the callback URL was built before the config row existed,
	// so it contained "{id}" literally, which caused "redirect URIs
	// not approved" errors when the user later tried to connect.
	t.Run("RedirectURIContainsRealConfigID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		// Buffered channel so the handler never blocks.
		registeredRedirectURI := make(chan string, 1)

		// Stand up a mock auth server that captures the redirect_uris
		// from the RFC 7591 Dynamic Client Registration request.
		authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-authorization-server":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "` + "http://" + r.Host + `",
					"authorization_endpoint": "` + "http://" + r.Host + `/authorize",
					"token_endpoint": "` + "http://" + r.Host + `/token",
					"registration_endpoint": "` + "http://" + r.Host + `/register",
					"response_types_supported": ["code"],
					"scopes_supported": ["read", "write"]
				}`))
			case "/register":
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				// Decode the registration body and capture redirect_uris.
				var body map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad json", http.StatusBadRequest)
					return
				}
				if uris, ok := body["redirect_uris"].([]interface{}); ok && len(uris) > 0 {
					if uri, ok := uris[0].(string); ok {
						registeredRedirectURI <- uri
					}
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"client_id": "test-client-id",
					"client_secret": "test-client-secret"
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(authServer.Close)

		// Stand up a mock MCP server that returns RFC 9728 Protected
		// Resource Metadata pointing to the auth server.
		mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-protected-resource/v1/mcp",
				"/.well-known/oauth-protected-resource":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"resource": "` + "http://" + r.Host + `",
					"authorization_servers": ["` + authServer.URL + `"]
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(mcpServer.Close)

		client := newMCPClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		// Create config with auth_type=oauth2 but no OAuth2 fields to
		// trigger auto-discovery and dynamic client registration.
		created, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:   "Redirect URI Test",
			Slug:          "redirect-uri-test",
			Transport:     "streamable_http",
			URL:           mcpServer.URL + "/v1/mcp",
			AuthType:      "oauth2",
			Availability:  "default_on",
			Enabled:       true,
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
		})
		require.NoError(t, err)
		require.Equal(t, "test-client-id", created.OAuth2ClientID)
		require.True(t, created.HasOAuth2Secret)

		// The registration request has already completed by the time
		// CreateMCPServerConfig returns, so the URI is in the channel.
		var redirectURI string
		select {
		case redirectURI = <-registeredRedirectURI:
		case <-ctx.Done():
			t.Fatal("timed out waiting for registration redirect URI")
		}

		// Core assertion: the redirect URI must NOT contain the
		// literal placeholder "{id}".  Before the fix the callback
		// URL was built before the database insert, so it had
		// "{id}" where the UUID should be.
		require.NotContains(t, redirectURI, "{id}",
			"redirect URI sent during registration must not contain the literal \"{id}\" placeholder")

		// Verify the redirect URI contains the real config UUID that
		// was assigned by the database.
		require.Contains(t, redirectURI, created.ID.String(),
			"redirect URI should contain the actual config UUID")

		// Sanity-check the full path structure.
		require.Contains(t, redirectURI,
			"/api/experimental/mcp/servers/"+created.ID.String()+"/oauth2/callback",
			"redirect URI should have the expected callback path")

		// Double-check that the ID segment is a valid UUID (not some
		// other placeholder or malformed value).
		pathParts := strings.Split(redirectURI, "/")
		var foundUUID bool
		for _, part := range pathParts {
			if _, err := uuid.Parse(part); err == nil {
				foundUUID = true
				require.Equal(t, created.ID.String(), part,
					"UUID in redirect URI path should match created config ID")
				break
			}
		}
		require.True(t, foundUUID,
			"redirect URI path should contain a valid UUID segment")
	})

	t.Run("PartialOAuth2FieldsRejected", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newMCPClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		// Provide client_id but omit auth_url and token_url.
		_, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:    "Partial Fields",
			Slug:           "partial-oauth2",
			Transport:      "streamable_http",
			URL:            "https://mcp.example.com/partial",
			AuthType:       "oauth2",
			OAuth2ClientID: "only-client-id",
			Availability:   "default_on",
			Enabled:        true,
			ToolAllowList:  []string{},
			ToolDenyList:   []string{},
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "automatic discovery")
	})

	t.Run("DiscoveryFailure", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		// MCP server that returns 404 for the well-known endpoint and
		// a non-401 status for the root — discovery has nothing to latch
		// onto.
		mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}))
		t.Cleanup(mcpServer.Close)

		client := newMCPClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:   "Will Fail",
			Slug:          "discovery-fail",
			Transport:     "streamable_http",
			URL:           mcpServer.URL + "/v1/mcp",
			AuthType:      "oauth2",
			Availability:  "default_on",
			Enabled:       true,
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "auto-discovery failed")
	})

	t.Run("ManualConfigStillWorks", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newMCPClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		// Providing all three OAuth2 fields bypasses discovery entirely.
		created, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:    "Manual Config",
			Slug:           "manual-oauth2",
			Transport:      "streamable_http",
			URL:            "https://mcp.example.com/manual",
			AuthType:       "oauth2",
			OAuth2ClientID: "manual-client-id",
			OAuth2AuthURL:  "https://auth.example.com/authorize",
			OAuth2TokenURL: "https://auth.example.com/token",
			Availability:   "default_on",
			Enabled:        true,
			ToolAllowList:  []string{},
			ToolDenyList:   []string{},
		})
		require.NoError(t, err)
		require.Equal(t, "manual-client-id", created.OAuth2ClientID)
		require.Equal(t, "https://auth.example.com/authorize", created.OAuth2AuthURL)
		require.Equal(t, "https://auth.example.com/token", created.OAuth2TokenURL)
	})
}

// nolint:bodyclose
func TestMCPServerOAuth2PKCE(t *testing.T) {
	t.Parallel()

	t.Run("ConnectSetsPKCEParams", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newMCPClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		// Create an OAuth2 MCP server config.
		created, err := adminClient.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:    "PKCE Test",
			Slug:           "pkce-test",
			Transport:      "streamable_http",
			URL:            "https://mcp.example.com/pkce",
			AuthType:       "oauth2",
			OAuth2ClientID: "test-client",
			OAuth2AuthURL:  "https://auth.example.com/authorize",
			OAuth2TokenURL: "https://auth.example.com/token",
			Availability:   "default_on",
			Enabled:        true,
			ToolAllowList:  []string{},
			ToolDenyList:   []string{},
		})
		require.NoError(t, err)

		// Prevent the HTTP client from following redirects so we
		// can inspect the response headers and cookies directly.
		memberClient.HTTPClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}

		connectURL, err := memberClient.URL.Parse(
			"/api/experimental/mcp/servers/" + created.ID.String() + "/oauth2/connect",
		)
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, "GET", connectURL.String(), nil)
		require.NoError(t, err)
		req.AddCookie(&http.Cookie{
			Name:  codersdk.SessionTokenCookie,
			Value: memberClient.SessionToken(),
		})

		res, err := memberClient.HTTPClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()

		require.Equal(t, http.StatusTemporaryRedirect, res.StatusCode)

		// The redirect URL must contain PKCE query parameters.
		location, err := res.Location()
		require.NoError(t, err)
		query := location.Query()
		require.Equal(t, "S256", query.Get("code_challenge_method"),
			"connect redirect must include code_challenge_method=S256")
		require.NotEmpty(t, query.Get("code_challenge"),
			"connect redirect must include a code_challenge")

		// A verifier cookie must be set.
		var verifierCookie *http.Cookie
		for _, c := range res.Cookies() {
			if c.Name == "mcp_oauth2_verifier_"+created.ID.String() {
				verifierCookie = c
				break
			}
		}
		require.NotNil(t, verifierCookie, "response must set a PKCE verifier cookie")
		require.NotEmpty(t, verifierCookie.Value)

		// Verify the code_challenge matches SHA256(verifier).
		h := sha256.Sum256([]byte(verifierCookie.Value))
		expectedChallenge := base64.RawURLEncoding.EncodeToString(h[:])
		require.Equal(t, expectedChallenge, query.Get("code_challenge"),
			"code_challenge must equal base64url(SHA256(verifier))")
	})

	t.Run("CallbackSendsVerifier", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		// Track the code_verifier received by the mock token endpoint.
		receivedVerifier := make(chan string, 1)

		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/token" && r.Method == http.MethodPost {
				if err := r.ParseForm(); err == nil {
					receivedVerifier <- r.FormValue("code_verifier")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"access_token": "test-access-token",
					"token_type": "Bearer",
					"expires_in": 3600,
					"refresh_token": "test-refresh-token"
				}`))
				return
			}
			http.NotFound(w, r)
		}))
		t.Cleanup(tokenServer.Close)

		adminClient := newMCPClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		created, err := adminClient.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:    "PKCE Callback Test",
			Slug:           "pkce-callback",
			Transport:      "streamable_http",
			URL:            "https://mcp.example.com/pkce-cb",
			AuthType:       "oauth2",
			OAuth2ClientID: "test-client",
			OAuth2AuthURL:  "https://auth.example.com/authorize",
			OAuth2TokenURL: tokenServer.URL + "/token",
			Availability:   "default_on",
			Enabled:        true,
			ToolAllowList:  []string{},
			ToolDenyList:   []string{},
		})
		require.NoError(t, err)

		memberClient.HTTPClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}

		// Simulate the callback with a known state and verifier.
		state := "test-state-value"
		verifier := "test-verifier-value-that-is-at-least-43-chars-long-for-pkce-spec"

		callbackURL, err := memberClient.URL.Parse(
			"/api/experimental/mcp/servers/" + created.ID.String() + "/oauth2/callback",
		)
		require.NoError(t, err)
		q := callbackURL.Query()
		q.Set("code", "test-auth-code")
		q.Set("state", state)
		callbackURL.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, "GET", callbackURL.String(), nil)
		require.NoError(t, err)
		req.AddCookie(&http.Cookie{
			Name:  codersdk.SessionTokenCookie,
			Value: memberClient.SessionToken(),
		})
		req.AddCookie(&http.Cookie{
			Name:  "mcp_oauth2_state_" + created.ID.String(),
			Value: state,
		})
		req.AddCookie(&http.Cookie{
			Name:  "mcp_oauth2_verifier_" + created.ID.String(),
			Value: verifier,
		})

		res, err := memberClient.HTTPClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()

		require.Equal(t, http.StatusOK, res.StatusCode,
			"callback should succeed when given valid state, verifier, and code")

		// Verify the mock token endpoint received the code_verifier.
		var gotVerifier string
		select {
		case gotVerifier = <-receivedVerifier:
		case <-ctx.Done():
			t.Fatal("timed out waiting for token exchange")
		}
		require.Equal(t, verifier, gotVerifier,
			"token exchange must send the PKCE code_verifier")

		// Verify the verifier cookie is cleared in the response.
		for _, c := range res.Cookies() {
			if c.Name == "mcp_oauth2_verifier_"+created.ID.String() {
				require.Equal(t, -1, c.MaxAge,
					"verifier cookie must be cleared after callback")
			}
		}
	})

	t.Run("CallbackWithoutVerifierStillWorks", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		// Token endpoint that does not require a code_verifier.
		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/token" && r.Method == http.MethodPost {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"access_token": "no-pkce-token",
					"token_type": "Bearer"
				}`))
				return
			}
			http.NotFound(w, r)
		}))
		t.Cleanup(tokenServer.Close)

		adminClient := newMCPClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		created, err := adminClient.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:    "No PKCE Callback",
			Slug:           "no-pkce-callback",
			Transport:      "streamable_http",
			URL:            "https://mcp.example.com/no-pkce",
			AuthType:       "oauth2",
			OAuth2ClientID: "test-client",
			OAuth2AuthURL:  "https://auth.example.com/authorize",
			OAuth2TokenURL: tokenServer.URL + "/token",
			Availability:   "default_on",
			Enabled:        true,
			ToolAllowList:  []string{},
			ToolDenyList:   []string{},
		})
		require.NoError(t, err)

		memberClient.HTTPClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}

		// Call the callback without a verifier cookie to verify
		// backwards compatibility with providers that don't use PKCE.
		state := "test-state-no-pkce"
		callbackURL, err := memberClient.URL.Parse(
			"/api/experimental/mcp/servers/" + created.ID.String() + "/oauth2/callback",
		)
		require.NoError(t, err)
		q := callbackURL.Query()
		q.Set("code", "test-auth-code")
		q.Set("state", state)
		callbackURL.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, "GET", callbackURL.String(), nil)
		require.NoError(t, err)
		req.AddCookie(&http.Cookie{
			Name:  codersdk.SessionTokenCookie,
			Value: memberClient.SessionToken(),
		})
		req.AddCookie(&http.Cookie{
			Name:  "mcp_oauth2_state_" + created.ID.String(),
			Value: state,
		})
		// Deliberately omit the verifier cookie.

		res, err := memberClient.HTTPClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()

		require.Equal(t, http.StatusOK, res.StatusCode,
			"callback without verifier cookie should still succeed")
	})
}

func TestChatWithMCPServerIDs(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := newMCPClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client)

	expClient := codersdk.NewExperimentalClient(client)

	// Create the chat model config required for creating a chat.
	_ = createChatModelConfigForMCP(t, expClient)

	// Create an enabled MCP server config.
	mcpConfig := createMCPServerConfig(t, client, "chat-mcp-server", true)

	// Create a chat referencing the MCP server.
	chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
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
	fetched, err := expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Contains(t, fetched.MCPServerIDs, mcpConfig.ID)
}

// createChatModelConfigForMCP sets up a chat provider and model
// config so that CreateChat succeeds. This mirrors the helper in
// chats_test.go but is defined here to avoid coupling.
func createChatModelConfigForMCP(t testing.TB, client *codersdk.ExperimentalClient) codersdk.ChatModelConfig {
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

func TestMCPOAuth2DiscoveryEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("EmptyAuthorizationServers", func(t *testing.T) {
		t.Parallel()

		// When the path-aware PRM returns an empty
		// authorization_servers array, discovery should fall
		// back to the root-level PRM.
		t.Run("RootFallback", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)

			authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/.well-known/oauth-authorization-server":
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{
						"issuer": "` + "http://" + r.Host + `",
						"authorization_endpoint": "` + "http://" + r.Host + `/authorize",
						"token_endpoint": "` + "http://" + r.Host + `/token",
						"registration_endpoint": "` + "http://" + r.Host + `/register",
						"response_types_supported": ["code"],
						"scopes_supported": ["fallback-scope"]
					}`))
				case "/register":
					if r.Method != http.MethodPost {
						http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{
						"client_id": "fallback-client-id",
						"client_secret": "fallback-client-secret"
					}`))
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(authServer.Close)

			mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/.well-known/oauth-protected-resource/v1/mcp":
					// Path-aware: empty authorization_servers.
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{
						"resource": "` + "http://" + r.Host + `/v1/mcp",
						"authorization_servers": []
					}`))
				case "/.well-known/oauth-protected-resource":
					// Root: valid authorization_servers.
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{
						"resource": "` + "http://" + r.Host + `",
						"authorization_servers": ["` + authServer.URL + `"]
					}`))
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(mcpServer.Close)

			client := newMCPClient(t)
			_ = coderdtest.CreateFirstUser(t, client)

			created, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
				DisplayName:   "Empty Auth Servers Fallback",
				Slug:          "empty-as-fallback",
				Transport:     "streamable_http",
				URL:           mcpServer.URL + "/v1/mcp",
				AuthType:      "oauth2",
				Availability:  "default_on",
				Enabled:       true,
				ToolAllowList: []string{},
				ToolDenyList:  []string{},
			})
			require.NoError(t, err)
			require.Equal(t, "fallback-client-id", created.OAuth2ClientID)
			require.Equal(t, authServer.URL+"/authorize", created.OAuth2AuthURL)
			require.Equal(t, authServer.URL+"/token", created.OAuth2TokenURL)
			require.Equal(t, "fallback-scope", created.OAuth2Scopes)
		})

		// When both path-aware and root PRM return empty
		// authorization_servers, discovery should fail.
		t.Run("BothEmpty", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)

			mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/.well-known/oauth-protected-resource/v1/mcp",
					"/.well-known/oauth-protected-resource":
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{
						"resource": "` + "http://" + r.Host + `",
						"authorization_servers": []
					}`))
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(mcpServer.Close)

			client := newMCPClient(t)
			_ = coderdtest.CreateFirstUser(t, client)

			_, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
				DisplayName:   "Both Empty",
				Slug:          "both-empty-as",
				Transport:     "streamable_http",
				URL:           mcpServer.URL + "/v1/mcp",
				AuthType:      "oauth2",
				Availability:  "default_on",
				Enabled:       true,
				ToolAllowList: []string{},
				ToolDenyList:  []string{},
			})
			require.Error(t, err)
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
			require.Contains(t, sdkErr.Message, "auto-discovery failed")
		})
	})

	// When the path-aware PRM returns malformed JSON,
	// discovery should fall back to the root-level PRM.
	t.Run("MalformedJSONFromDiscovery", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-authorization-server":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "` + "http://" + r.Host + `",
					"authorization_endpoint": "` + "http://" + r.Host + `/authorize",
					"token_endpoint": "` + "http://" + r.Host + `/token",
					"registration_endpoint": "` + "http://" + r.Host + `/register",
					"response_types_supported": ["code"],
					"scopes_supported": ["json-fallback"]
				}`))
			case "/register":
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"client_id": "json-fallback-client",
					"client_secret": "json-fallback-secret"
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(authServer.Close)

		mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-protected-resource/v1/mcp":
				// Return valid HTTP 200 but invalid JSON.
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`not json`))
			case "/.well-known/oauth-protected-resource":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"resource": "` + "http://" + r.Host + `",
					"authorization_servers": ["` + authServer.URL + `"]
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(mcpServer.Close)

		client := newMCPClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		created, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:   "Malformed JSON Fallback",
			Slug:          "malformed-json",
			Transport:     "streamable_http",
			URL:           mcpServer.URL + "/v1/mcp",
			AuthType:      "oauth2",
			Availability:  "default_on",
			Enabled:       true,
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
		})
		require.NoError(t, err)
		require.Equal(t, "json-fallback-client", created.OAuth2ClientID)
		require.Equal(t, authServer.URL+"/authorize", created.OAuth2AuthURL)
		require.Equal(t, authServer.URL+"/token", created.OAuth2TokenURL)
		require.Equal(t, "json-fallback", created.OAuth2Scopes)
	})

	// When the path-aware auth server metadata is missing required
	// endpoints, discovery should fall back to the root-level
	// metadata URL.
	t.Run("AuthServerMetadataMissingEndpoints", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		// Auth server that returns incomplete metadata at the
		// path-aware URL but complete metadata at the root URL.
		authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-authorization-server/auth":
				// Path-aware: missing required endpoints.
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "` + "http://" + r.Host + `/auth"
				}`))
			case "/.well-known/oauth-authorization-server":
				// Root-level: complete metadata.
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "` + "http://" + r.Host + `",
					"authorization_endpoint": "` + "http://" + r.Host + `/authorize",
					"token_endpoint": "` + "http://" + r.Host + `/token",
					"registration_endpoint": "` + "http://" + r.Host + `/register",
					"response_types_supported": ["code"],
					"scopes_supported": ["endpoint-fallback"]
				}`))
			case "/register":
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"client_id": "endpoint-fallback-client",
					"client_secret": "endpoint-fallback-secret"
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(authServer.Close)

		// PRM points to auth server with a path (/auth) so that
		// discoverAuthServerMetadata tries the path-aware URL first.
		mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-protected-resource/v1/mcp":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"resource": "` + "http://" + r.Host + `/v1/mcp",
					"authorization_servers": ["` + authServer.URL + `/auth"]
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(mcpServer.Close)

		client := newMCPClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		created, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:   "Missing Endpoints Fallback",
			Slug:          "missing-endpoints",
			Transport:     "streamable_http",
			URL:           mcpServer.URL + "/v1/mcp",
			AuthType:      "oauth2",
			Availability:  "default_on",
			Enabled:       true,
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
		})
		require.NoError(t, err)
		require.Equal(t, "endpoint-fallback-client", created.OAuth2ClientID)
		require.Equal(t, authServer.URL+"/authorize", created.OAuth2AuthURL)
		require.Equal(t, authServer.URL+"/token", created.OAuth2TokenURL)
		require.Equal(t, "endpoint-fallback", created.OAuth2Scopes)
	})

	// When both RFC 8414 metadata URLs (path-aware and root) fail,
	// discovery should fall back to the OIDC well-known URL.
	// The auth server issuer has a path (/login/oauth) so the
	// OIDC URL is {issuer}/.well-known/openid-configuration =
	// /login/oauth/.well-known/openid-configuration.
	t.Run("OIDCFallback", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/login/oauth/.well-known/openid-configuration":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "` + "http://" + r.Host + `/login/oauth",
					"authorization_endpoint": "` + "http://" + r.Host + `/login/oauth/authorize",
					"token_endpoint": "` + "http://" + r.Host + `/login/oauth/token",
					"registration_endpoint": "` + "http://" + r.Host + `/register",
					"response_types_supported": ["code"],
					"scopes_supported": ["oidc-scope"]
				}`))
			case "/register":
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"client_id": "oidc-client-id",
					"client_secret": "oidc-client-secret"
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(authServer.Close)

		// PRM points to auth server with a path (/login/oauth)
		// so that RFC 8414 URLs are tried first and fail.
		mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-protected-resource/v1/mcp":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"resource": "` + "http://" + r.Host + `/v1/mcp",
					"authorization_servers": ["` + authServer.URL + `/login/oauth"]
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(mcpServer.Close)

		client := newMCPClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		created, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:   "OIDC Fallback",
			Slug:          "oidc-fallback",
			Transport:     "streamable_http",
			URL:           mcpServer.URL + "/v1/mcp",
			AuthType:      "oauth2",
			Availability:  "default_on",
			Enabled:       true,
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
		})
		require.NoError(t, err)
		require.Equal(t, "oidc-client-id", created.OAuth2ClientID)
		require.Equal(t, authServer.URL+"/login/oauth/authorize", created.OAuth2AuthURL)
		require.Equal(t, authServer.URL+"/login/oauth/token", created.OAuth2TokenURL)
		require.Equal(t, "oidc-scope", created.OAuth2Scopes)
	})

	// When the registration endpoint returns a response
	// without a client_id, the entire discovery flow should
	// fail.
	t.Run("RegistrationMissingClientID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-authorization-server":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "` + "http://" + r.Host + `",
					"authorization_endpoint": "` + "http://" + r.Host + `/authorize",
					"token_endpoint": "` + "http://" + r.Host + `/token",
					"registration_endpoint": "` + "http://" + r.Host + `/register",
					"response_types_supported": ["code"]
				}`))
			case "/register":
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				// Return response with client_secret but no
				// client_id.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"client_secret": "secret-without-id"
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(authServer.Close)

		mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-protected-resource/v1/mcp":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"resource": "` + "http://" + r.Host + `/v1/mcp",
					"authorization_servers": ["` + authServer.URL + `"]
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(mcpServer.Close)

		client := newMCPClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		_, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:   "Missing Client ID",
			Slug:          "missing-client-id",
			Transport:     "streamable_http",
			URL:           mcpServer.URL + "/v1/mcp",
			AuthType:      "oauth2",
			Availability:  "default_on",
			Enabled:       true,
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "auto-discovery failed")
	})

	// Regression test for the exact scenario that motivated the PR:
	// an MCP server URL with a trailing slash (like
	// https://api.githubcopilot.com/mcp/).
	t.Run("TrailingSlashURL", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-authorization-server":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "` + "http://" + r.Host + `",
					"authorization_endpoint": "` + "http://" + r.Host + `/authorize",
					"token_endpoint": "` + "http://" + r.Host + `/token",
					"registration_endpoint": "` + "http://" + r.Host + `/register",
					"response_types_supported": ["code"],
					"scopes_supported": ["read"]
				}`))
			case "/register":
				if r.Method != http.MethodPost {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"client_id": "trailing-slash-client",
					"client_secret": "trailing-slash-secret"
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(authServer.Close)

		// Serve protected resource metadata at the path-aware URL
		// WITH the trailing slash: /.well-known/oauth-protected-resource/mcp/
		mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-protected-resource/mcp/":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"resource": "` + "http://" + r.Host + `/mcp/",
					"authorization_servers": ["` + authServer.URL + `"]
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(mcpServer.Close)

		client := newMCPClient(t)
		_ = coderdtest.CreateFirstUser(t, client)

		// URL has a trailing slash, matching the GitHub Copilot URL
		// pattern: https://api.githubcopilot.com/mcp/
		created, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
			DisplayName:   "Trailing Slash",
			Slug:          "trailing-slash",
			Transport:     "streamable_http",
			URL:           mcpServer.URL + "/mcp/",
			AuthType:      "oauth2",
			Availability:  "default_on",
			Enabled:       true,
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
		})
		require.NoError(t, err)
		require.Equal(t, "trailing-slash-client", created.OAuth2ClientID)
		require.True(t, created.HasOAuth2Secret)
	})
}
