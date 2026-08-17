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
	organizationPath := "/api/experimental/organizations/" + secondOrg.ID.String() + "/mcp-servers/" + config.ID.String()
	frozenPath := "/api/experimental/mcp/servers/" + config.ID.String()

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
		})
	}
}
