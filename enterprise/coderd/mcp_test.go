package coderd_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func createMCPServerConfigForOrganization(
	t testing.TB,
	client *codersdk.Client,
	organizationID uuid.UUID,
	slug string,
) codersdk.MCPServerConfig {
	t.Helper()

	config, err := client.CreateMCPServerConfig(
		testutil.Context(t, testutil.WaitLong),
		organizationID,
		codersdk.CreateMCPServerConfigRequest{
			DisplayName:   slug,
			Slug:          slug,
			Transport:     "streamable_http",
			URL:           "https://mcp.example.com/" + slug,
			AuthType:      "none",
			Availability:  "default_on",
			Enabled:       true,
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
		},
	)
	require.NoError(t, err)
	return config
}

func requireMCPServerConfigRequestStatus(
	t *testing.T,
	client *codersdk.Client,
	method string,
	path string,
	body any,
	wantStatus int,
) {
	t.Helper()

	res, err := client.Request(
		testutil.Context(t, testutil.WaitLong),
		method,
		path,
		body,
	)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, wantStatus, res.StatusCode)
}

func TestMCPServerConfigUpdateOnlyRoleReachesACLExcludedConfigs(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	owner, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureCustomRoles: 1,
			},
		},
	})

	enabled := createMCPServerConfigForOrganization(t, owner, firstUser.OrganizationID, "visible-mcp")
	disabled := createMCPServerConfigForOrganization(t, owner, firstUser.OrganizationID, "hidden-mcp")
	//nolint:gocritic // Owner access sets up the disabled fixture.
	_, err := owner.UpdateMCPServerConfig(ctx, firstUser.OrganizationID, disabled.ID,
		codersdk.UpdateMCPServerConfigRequest{Enabled: ptr.Ref(false)})
	require.NoError(t, err)
	for _, config := range []codersdk.MCPServerConfig{enabled, disabled} {
		//nolint:gocritic // Owner access removes the default ACL grant.
		err = owner.UpdateMCPServerConfigACL(ctx, firstUser.OrganizationID, config.ID, codersdk.UpdateMCPServerConfigACLRequest{
			GroupRoles: map[string]codersdk.MCPServerConfigRole{
				firstUser.OrganizationID.String(): codersdk.MCPServerConfigRoleDeleted,
			},
		})
		require.NoError(t, err)
	}

	//nolint:gocritic // Owner access isolates custom-role setup from the behavior under test.
	role, err := owner.CreateOrganizationRole(ctx, codersdk.Role{
		Name:           "mcp-update-only",
		OrganizationID: firstUser.OrganizationID.String(),
		OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
			codersdk.ResourceMCPServerConfig: {codersdk.ActionUpdate},
		}),
	})
	require.NoError(t, err)
	updateOnly, _ := coderdtest.CreateAnotherUser(t, owner, firstUser.OrganizationID,
		rbac.RoleIdentifier{Name: role.Name, OrganizationID: firstUser.OrganizationID})

	configs, err := updateOnly.MCPServerConfigs(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Len(t, configs, 2)
	for _, config := range configs {
		require.Empty(t, config.URL)
	}

	fetched, err := updateOnly.MCPServerConfigByID(ctx, firstUser.OrganizationID, disabled.ID)
	require.NoError(t, err)
	require.Equal(t, disabled.URL, fetched.URL)

	requireMCPServerConfigRequestStatus(t, updateOnly, http.MethodGet,
		"/api/experimental/organizations/"+firstUser.OrganizationID.String()+"/mcp-servers/"+disabled.ID.String()+"/oauth2/connect",
		nil, http.StatusNotFound)

	updatedName := "updated-hidden-mcp"
	updated, err := updateOnly.UpdateMCPServerConfig(ctx, firstUser.OrganizationID, disabled.ID,
		codersdk.UpdateMCPServerConfigRequest{DisplayName: &updatedName})
	require.NoError(t, err)
	require.Equal(t, updatedName, updated.DisplayName)
}

func TestMCPServerConfigDeleteOnlyRoleReachesDisabled(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	owner, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureCustomRoles: 1,
			},
		},
	})

	enabled := createMCPServerConfigForOrganization(t, owner, firstUser.OrganizationID, "enabled-mcp")
	disabled := createMCPServerConfigForOrganization(t, owner, firstUser.OrganizationID, "disabled-mcp")
	//nolint:gocritic // Owner access sets up the disabled fixture.
	_, err := owner.UpdateMCPServerConfig(ctx, firstUser.OrganizationID, disabled.ID,
		codersdk.UpdateMCPServerConfigRequest{Enabled: ptr.Ref(false)})
	require.NoError(t, err)
	for _, config := range []codersdk.MCPServerConfig{enabled, disabled} {
		//nolint:gocritic // Owner access removes the default ACL grant.
		err = owner.UpdateMCPServerConfigACL(ctx, firstUser.OrganizationID, config.ID, codersdk.UpdateMCPServerConfigACLRequest{
			GroupRoles: map[string]codersdk.MCPServerConfigRole{
				firstUser.OrganizationID.String(): codersdk.MCPServerConfigRoleDeleted,
			},
		})
		require.NoError(t, err)
	}

	//nolint:gocritic // Owner access isolates custom-role setup from the behavior under test.
	role, err := owner.CreateOrganizationRole(ctx, codersdk.Role{
		Name:           "mcp-delete-only",
		OrganizationID: firstUser.OrganizationID.String(),
		OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
			codersdk.ResourceMCPServerConfig: {codersdk.ActionDelete},
		}),
	})
	require.NoError(t, err)
	deleteOnly, _ := coderdtest.CreateAnotherUser(t, owner, firstUser.OrganizationID,
		rbac.RoleIdentifier{Name: role.Name, OrganizationID: firstUser.OrganizationID})

	configs, err := deleteOnly.MCPServerConfigs(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Len(t, configs, 2)
	for _, config := range configs {
		require.Empty(t, config.URL)
	}

	fetched, err := deleteOnly.MCPServerConfigByID(ctx, firstUser.OrganizationID, disabled.ID)
	require.NoError(t, err)
	require.Equal(t, disabled.ID, fetched.ID)
	require.False(t, fetched.Enabled)
	require.Empty(t, fetched.URL)

	requireMCPServerConfigRequestStatus(t, deleteOnly, http.MethodGet,
		"/api/experimental/organizations/"+firstUser.OrganizationID.String()+"/mcp-servers/"+disabled.ID.String()+"/oauth2/connect",
		nil, http.StatusNotFound)

	err = deleteOnly.DeleteMCPServerConfig(ctx, firstUser.OrganizationID, disabled.ID)
	require.NoError(t, err)

	configs, err = deleteOnly.MCPServerConfigs(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	require.Equal(t, enabled.ID, configs[0].ID)
}

func TestMCPServerConfigShareOnlyRoleRoutes(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	owner, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureCustomRoles: 1,
			},
		},
	})
	//nolint:gocritic // Owner access creates a secret-bearing redaction fixture.
	config, err := owner.CreateMCPServerConfig(ctx, firstUser.OrganizationID, codersdk.CreateMCPServerConfigRequest{
		DisplayName:   "share-only-mcp",
		Slug:          "share-only-mcp",
		Transport:     "streamable_http",
		URL:           "https://mcp.example.com/share-only-mcp",
		AuthType:      "api_key",
		APIKeyHeader:  "X-Api-Key",
		APIKeyValue:   "share-only-secret",
		Availability:  "default_on",
		Enabled:       true,
		ToolAllowList: []string{},
		ToolDenyList:  []string{},
	})
	require.NoError(t, err)
	//nolint:gocritic // Owner access removes the default ACL grant.
	err = owner.UpdateMCPServerConfigACL(ctx, firstUser.OrganizationID, config.ID, codersdk.UpdateMCPServerConfigACLRequest{
		GroupRoles: map[string]codersdk.MCPServerConfigRole{
			firstUser.OrganizationID.String(): codersdk.MCPServerConfigRoleDeleted,
		},
	})
	require.NoError(t, err)

	//nolint:gocritic // Owner access isolates custom-role setup from the behavior under test.
	role, err := owner.CreateOrganizationRole(ctx, codersdk.Role{
		Name:           "mcp-share-only",
		OrganizationID: firstUser.OrganizationID.String(),
		OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
			codersdk.ResourceMCPServerConfig: {codersdk.ActionShare},
		}),
	})
	require.NoError(t, err)
	shareOnly, _ := coderdtest.CreateAnotherUser(t, owner, firstUser.OrganizationID,
		rbac.RoleIdentifier{Name: role.Name, OrganizationID: firstUser.OrganizationID})

	requireListed := func(enabled bool) {
		configs, err := shareOnly.MCPServerConfigs(ctx, firstUser.OrganizationID)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, config.ID, configs[0].ID)
		require.Equal(t, enabled, configs[0].Enabled)
		require.Equal(t, "api_key", configs[0].AuthType)
		require.True(t, configs[0].HasAPIKey)
		require.Empty(t, configs[0].URL)
		require.Empty(t, configs[0].Transport)
		require.Empty(t, configs[0].APIKeyHeader)
	}
	requireListed(true)

	//nolint:gocritic // Owner access disables the fixture before share-only listing is retested.
	_, err = owner.UpdateMCPServerConfig(ctx, firstUser.OrganizationID, config.ID,
		codersdk.UpdateMCPServerConfigRequest{Enabled: ptr.Ref(false)})
	require.NoError(t, err)
	requireListed(false)

	_, err = shareOnly.MCPServerConfigACL(ctx, firstUser.OrganizationID, config.ID)
	require.NoError(t, err)
	err = shareOnly.UpdateMCPServerConfigACL(ctx, firstUser.OrganizationID, config.ID, codersdk.UpdateMCPServerConfigACLRequest{})
	require.NoError(t, err)

	configPath := "/api/experimental/organizations/" + firstUser.OrganizationID.String() + "/mcp-servers/" + config.ID.String()
	requireMCPServerConfigRequestStatus(t, shareOnly, http.MethodGet, configPath, nil, http.StatusNotFound)
	requireMCPServerConfigRequestStatus(t, shareOnly, http.MethodGet, configPath+"/oauth2/connect", nil, http.StatusNotFound)
	requireMCPServerConfigRequestStatus(t, shareOnly, http.MethodPatch, configPath,
		codersdk.UpdateMCPServerConfigRequest{DisplayName: ptr.Ref("denied-update")}, http.StatusNotFound)
	requireMCPServerConfigRequestStatus(t, shareOnly, http.MethodDelete, configPath, nil, http.StatusNotFound)
}

func TestMCPServerConfigReadOnlyRoleCanConnect(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	owner, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureCustomRoles: 1,
			},
		},
	})
	//nolint:gocritic // Owner access creates the fixture before read-only authorization is tested.
	config, err := owner.CreateMCPServerConfig(ctx, firstUser.OrganizationID, codersdk.CreateMCPServerConfigRequest{
		DisplayName:    "read-only-connect",
		Slug:           "read-only-connect",
		Transport:      "streamable_http",
		URL:            "https://mcp.example.com/read-only-connect",
		AuthType:       "oauth2",
		OAuth2ClientID: "read-only-client",
		OAuth2AuthURL:  "https://auth.example.com/authorize",
		OAuth2TokenURL: "https://auth.example.com/token",
		Availability:   "default_on",
		Enabled:        true,
		ToolAllowList:  []string{},
		ToolDenyList:   []string{},
	})
	require.NoError(t, err)
	//nolint:gocritic // Owner access removes the default ACL grant.
	err = owner.UpdateMCPServerConfigACL(ctx, firstUser.OrganizationID, config.ID, codersdk.UpdateMCPServerConfigACLRequest{
		GroupRoles: map[string]codersdk.MCPServerConfigRole{
			firstUser.OrganizationID.String(): codersdk.MCPServerConfigRoleDeleted,
		},
	})
	require.NoError(t, err)

	//nolint:gocritic // Owner access isolates custom-role setup from the behavior under test.
	role, err := owner.CreateOrganizationRole(ctx, codersdk.Role{
		Name:           "mcp-read-only",
		OrganizationID: firstUser.OrganizationID.String(),
		OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
			codersdk.ResourceMCPServerConfig: {codersdk.ActionRead},
		}),
	})
	require.NoError(t, err)
	readOnly, _ := coderdtest.CreateAnotherUser(t, owner, firstUser.OrganizationID,
		rbac.RoleIdentifier{Name: role.Name, OrganizationID: firstUser.OrganizationID})
	readOnly.HTTPClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	requireMCPServerConfigRequestStatus(t, readOnly, http.MethodGet,
		"/api/experimental/organizations/"+firstUser.OrganizationID.String()+"/mcp-servers/"+config.ID.String()+"/oauth2/connect",
		nil, http.StatusTemporaryRedirect)
}

func TestMCPServerConfigCollectionOrganizationIsolation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	secondOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})
	firstConfig := createMCPServerConfigForOrganization(t, client, firstUser.OrganizationID, "org-one-mcp")
	secondConfig := createMCPServerConfigForOrganization(t, client, secondOrg.ID, "org-two-mcp")

	//nolint:gocritic // Site owner access is the behavior under test.
	firstConfigs, err := client.MCPServerConfigs(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Len(t, firstConfigs, 1)
	require.Equal(t, firstConfig.ID, firstConfigs[0].ID)
	require.Equal(t, firstUser.OrganizationID, firstConfigs[0].OrganizationID)

	//nolint:gocritic // Site owner access is the behavior under test.
	secondConfigs, err := client.MCPServerConfigs(ctx, secondOrg.ID)
	require.NoError(t, err)
	require.Len(t, secondConfigs, 1)
	require.Equal(t, secondConfig.ID, secondConfigs[0].ID)
	require.Equal(t, secondOrg.ID, secondConfigs[0].OrganizationID)
}

func TestMCPServerConfigItemCrossOrganizationConcealment(t *testing.T) {
	t.Parallel()

	mAudit := audit.NewMock()
	client, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			Auditor: mAudit,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	secondOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})
	otherClient, _ := coderdtest.CreateAnotherUser(t, client, secondOrg.ID)
	config := createMCPServerConfigForOrganization(t, client, firstUser.OrganizationID, "private-org-one-mcp")
	organizationPath := "/api/experimental/organizations/" + secondOrg.ID.String() + "/mcp-servers/" + config.ID.String()
	frozenPath := "/api/experimental/mcp/servers/" + config.ID.String()
	mAudit.ResetLogs()

	for _, test := range []struct {
		name       string
		method     string
		path       string
		body       any
		wantStatus int
	}{
		{name: "Get", method: http.MethodGet, path: organizationPath},
		{name: "Patch", method: http.MethodPatch, path: organizationPath, body: codersdk.UpdateMCPServerConfigRequest{DisplayName: ptr.Ref("cross-org")}},
		{name: "Delete", method: http.MethodDelete, path: organizationPath},
		{name: "OAuthConnect", method: http.MethodGet, path: organizationPath + "/oauth2/connect"},
		{name: "OAuthCallback", method: http.MethodGet, path: frozenPath + "/oauth2/callback"},
		// Disconnect returns 200 for every caller without a token,
		// including nonexistent config IDs, so the response does not
		// reveal whether the config exists.
		{name: "OAuthDisconnect", method: http.MethodDelete, path: frozenPath + "/oauth2/disconnect", wantStatus: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			wantStatus := test.wantStatus
			if wantStatus == 0 {
				wantStatus = http.StatusNotFound
			}
			requireMCPServerConfigRequestStatus(t, otherClient, test.method, test.path, test.body, wantStatus)

			// Read-gated routes 404 in the param middleware before any
			// handler runs, and disconnect does not audit, so nothing
			// is audited.
			for _, log := range mAudit.AuditLogs() {
				require.NotEqual(t, database.ResourceTypeMCPServerConfig, log.ResourceType)
			}
		})
	}

	// Compare raw responses because SDK decoding can hide body differences
	// that reveal whether the config exists.
	t.Run("OAuthDisconnectBodyMatchesNonexistent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		rawDisconnect := func(id uuid.UUID) (int, string) {
			res, err := otherClient.Request(ctx, http.MethodDelete,
				"/api/experimental/mcp/servers/"+id.String()+"/oauth2/disconnect", nil)
			require.NoError(t, err)
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			return res.StatusCode, string(body)
		}

		hiddenStatus, hiddenBody := rawDisconnect(config.ID)
		missingStatus, missingBody := rawDisconnect(uuid.New())
		require.Equal(t, missingStatus, hiddenStatus)
		require.Equal(t, missingBody, hiddenBody)

		var disconnect codersdk.MCPServerOAuth2DisconnectResponse
		require.NoError(t, json.Unmarshal([]byte(hiddenBody), &disconnect))
		require.False(t, disconnect.TokenRevoked)
		require.Empty(t, disconnect.TokenRevocationError)
	})
}

func TestMCPServerConfigsOAuth2CallbackTokenBinding(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db, firstUser := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	secondOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})
	memberClient, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID, rbac.ScopedRoleOrgMember(secondOrg.ID))

	newTokenServer := func(accessToken string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w,
				`{"access_token":%q,"token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-%s"}`,
				accessToken, accessToken,
			)
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	createOAuthConfig := func(organizationID uuid.UUID, tokenURL string) codersdk.MCPServerConfig {
		t.Helper()
		config, err := client.CreateMCPServerConfig(ctx, organizationID, codersdk.CreateMCPServerConfigRequest{
			DisplayName:    "Callback Binding",
			Slug:           "callback-binding",
			Transport:      "streamable_http",
			URL:            "https://mcp.example.com/callback-binding",
			AuthType:       "oauth2",
			OAuth2ClientID: "client-" + organizationID.String(),
			OAuth2AuthURL:  "https://auth.example.com/authorize",
			OAuth2TokenURL: tokenURL,
			Availability:   "default_on",
			Enabled:        true,
			ToolAllowList:  []string{},
			ToolDenyList:   []string{},
		})
		require.NoError(t, err)
		return config
	}
	completeCallback := func(config codersdk.MCPServerConfig) {
		t.Helper()
		state := "state-" + config.ID.String()
		callbackURL, err := memberClient.URL.Parse(
			"/api/experimental/mcp/servers/" + config.ID.String() + "/oauth2/callback",
		)
		require.NoError(t, err)
		query := callbackURL.Query()
		query.Set("code", "auth-code-"+config.ID.String())
		query.Set("state", state)
		callbackURL.RawQuery = query.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, callbackURL.String(), nil)
		require.NoError(t, err)
		req.AddCookie(&http.Cookie{Name: codersdk.SessionTokenCookie, Value: memberClient.SessionToken()})
		req.AddCookie(&http.Cookie{Name: "mcp_oauth2_state_" + config.ID.String(), Value: state})
		res, err := memberClient.HTTPClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	}
	tokenRow := func(configID uuid.UUID) database.MCPServerUserToken {
		t.Helper()
		//nolint:gocritic // Verifying persisted state requires system access.
		row, err := db.GetMCPServerUserToken(dbauthz.AsSystemRestricted(ctx), database.GetMCPServerUserTokenParams{
			MCPServerConfigID: configID,
			UserID:            member.ID,
		})
		require.NoError(t, err)
		return row
	}

	// The same slug in both organizations proves tokens bind to the config
	// ID, not the slug.
	firstConfig := createOAuthConfig(firstUser.OrganizationID, newTokenServer("org-one-access-token").URL)
	secondConfig := createOAuthConfig(secondOrg.ID, newTokenServer("org-two-access-token").URL)

	completeCallback(firstConfig)
	firstToken := tokenRow(firstConfig.ID)
	require.Equal(t, "org-one-access-token", firstToken.AccessToken)

	completeCallback(secondConfig)
	firstToken = tokenRow(firstConfig.ID)
	secondToken := tokenRow(secondConfig.ID)
	require.Equal(t, "org-one-access-token", firstToken.AccessToken)
	require.Equal(t, "org-two-access-token", secondToken.AccessToken)
	require.NotEqual(t, firstToken.ID, secondToken.ID)
}
