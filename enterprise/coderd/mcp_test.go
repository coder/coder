package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
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
	configID uuid.UUID,
	pathSuffix string,
	body any,
	wantStatus int,
) {
	t.Helper()

	res, err := client.Request(
		testutil.Context(t, testutil.WaitLong),
		method,
		"/api/experimental/mcp-servers/"+configID.String()+pathSuffix,
		body,
	)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, wantStatus, res.StatusCode)
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

	//nolint:gocritic // The owner verifies organization-scoped collection behavior.
	firstConfigs, err := client.MCPServerConfigs(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Len(t, firstConfigs, 1)
	require.Equal(t, firstConfig.ID, firstConfigs[0].ID)
	require.Equal(t, firstUser.OrganizationID, firstConfigs[0].OrganizationID)

	secondConfigs, err := client.MCPServerConfigs(ctx, secondOrg.ID)
	require.NoError(t, err)
	require.Len(t, secondConfigs, 1)
	require.Equal(t, secondConfig.ID, secondConfigs[0].ID)
	require.Equal(t, secondOrg.ID, secondConfigs[0].OrganizationID)
}

func TestMCPServerConfigItemCrossOrganizationConcealment(t *testing.T) {
	t.Parallel()

	client, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	secondOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})
	otherClient, _ := coderdtest.CreateAnotherUser(t, client, secondOrg.ID)
	config := createMCPServerConfigForOrganization(t, client, firstUser.OrganizationID, "private-org-one-mcp")

	for _, test := range []struct {
		name       string
		method     string
		pathSuffix string
		body       any
	}{
		{name: "Get", method: http.MethodGet},
		{name: "Patch", method: http.MethodPatch, body: codersdk.UpdateMCPServerConfigRequest{DisplayName: ptr.Ref("cross-org")}},
		{name: "Delete", method: http.MethodDelete},
		{name: "GetACL", method: http.MethodGet, pathSuffix: "/acl"},
		{name: "PatchACL", method: http.MethodPatch, pathSuffix: "/acl", body: codersdk.UpdateMCPServerConfigACL{}},
		{name: "OAuthConnect", method: http.MethodGet, pathSuffix: "/oauth2/connect"},
		{name: "OAuthCallback", method: http.MethodGet, pathSuffix: "/oauth2/callback"},
		{name: "OAuthDisconnect", method: http.MethodDelete, pathSuffix: "/oauth2/disconnect"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			requireMCPServerConfigRequestStatus(t, otherClient, test.method, config.ID, test.pathSuffix, test.body, http.StatusNotFound)
		})
	}
}

func TestMCPServerConfigACL(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		},
	})
	config := createMCPServerConfigForOrganization(t, client, firstUser.OrganizationID, "acl-mcp")
	userClient, user := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
	groupMemberClient, groupMember := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
	group := coderdtest.CreateGroup(t, client, firstUser.OrganizationID, "mcp-readers", groupMember)

	//nolint:gocritic // The owner configures ACLs under test.
	err := client.UpdateMCPServerConfigACL(ctx, config.ID, codersdk.UpdateMCPServerConfigACL{
		GroupRoles: map[string]codersdk.MCPServerConfigRole{
			firstUser.OrganizationID.String(): codersdk.MCPServerConfigRoleDeleted,
		},
	})
	require.NoError(t, err)

	for _, reader := range []*codersdk.Client{userClient, groupMemberClient} {
		requireMCPServerConfigRequestStatus(t, reader, http.MethodGet, config.ID, "", nil, http.StatusNotFound)
		requireMCPServerConfigRequestStatus(t, reader, http.MethodGet, config.ID, "/acl", nil, http.StatusNotFound)
	}

	err = client.UpdateMCPServerConfigACL(ctx, config.ID, codersdk.UpdateMCPServerConfigACL{
		UserRoles: map[string]codersdk.MCPServerConfigRole{
			user.ID.String(): codersdk.MCPServerConfigRoleRead,
		},
		GroupRoles: map[string]codersdk.MCPServerConfigRole{
			group.ID.String(): codersdk.MCPServerConfigRoleRead,
		},
	})
	require.NoError(t, err)

	//nolint:gocritic // The owner verifies the complete ACL response.
	acl, err := client.GetMCPServerConfigACL(ctx, config.ID)
	require.NoError(t, err)
	require.Len(t, acl.Users, 1)
	require.Equal(t, user.ID, acl.Users[0].ID)
	require.Equal(t, codersdk.MCPServerConfigRoleRead, acl.Users[0].Role)
	require.Len(t, acl.Groups, 1)
	require.Equal(t, group.ID, acl.Groups[0].ID)
	require.Equal(t, codersdk.MCPServerConfigRoleRead, acl.Groups[0].Role)

	for _, reader := range []*codersdk.Client{userClient, groupMemberClient} {
		configs, err := reader.MCPServerConfigs(ctx, firstUser.OrganizationID)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, config.ID, configs[0].ID)
		requireMCPServerConfigRequestStatus(t, reader, http.MethodGet, config.ID, "", nil, http.StatusOK)
		requireMCPServerConfigRequestStatus(t, reader, http.MethodGet, config.ID, "/acl", nil, http.StatusOK)
		requireMCPServerConfigRequestStatus(t, reader, http.MethodPatch, config.ID, "", codersdk.UpdateMCPServerConfigRequest{DisplayName: ptr.Ref("reader")}, http.StatusNotFound)
		requireMCPServerConfigRequestStatus(t, reader, http.MethodDelete, config.ID, "", nil, http.StatusNotFound)
		requireMCPServerConfigRequestStatus(t, reader, http.MethodPatch, config.ID, "/acl", codersdk.UpdateMCPServerConfigACL{}, http.StatusNotFound)
	}

	err = client.UpdateMCPServerConfigACL(ctx, config.ID, codersdk.UpdateMCPServerConfigACL{
		UserRoles: map[string]codersdk.MCPServerConfigRole{
			user.ID.String(): codersdk.MCPServerConfigRole("write"),
		},
	})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
}

func TestMCPServerConfigACLRequiresTemplateRBAC(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, firstUser := coderdenttest.New(t, nil)
	config := createMCPServerConfigForOrganization(t, client, firstUser.OrganizationID, "unlicensed-acl-mcp")

	//nolint:gocritic // The owner verifies the entitlement gate.
	err := client.UpdateMCPServerConfigACL(ctx, config.ID, codersdk.UpdateMCPServerConfigACL{})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
}
