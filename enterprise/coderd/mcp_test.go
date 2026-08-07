package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
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
	// Drop the audited setup create so the subtests can assert the
	// denied requests below record nothing.
	mAudit.ResetLogs()

	for _, test := range []struct {
		name       string
		method     string
		pathSuffix string
		body       any
	}{
		{name: "Get", method: http.MethodGet},
		{name: "Patch", method: http.MethodPatch, body: codersdk.UpdateMCPServerConfigRequest{DisplayName: ptr.Ref("cross-org")}},
		{name: "Delete", method: http.MethodDelete},
		{name: "OAuthConnect", method: http.MethodGet, pathSuffix: "/oauth2/connect"},
		{name: "OAuthCallback", method: http.MethodGet, pathSuffix: "/oauth2/callback"},
		{name: "OAuthDisconnect", method: http.MethodDelete, pathSuffix: "/oauth2/disconnect"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			requireMCPServerConfigRequestStatus(t, otherClient, test.method, config.ID, test.pathSuffix, test.body, http.StatusNotFound)

			// The read-denied 404 comes from the param middleware before
			// any mutation handler runs, so no audit entry is recorded
			// for the concealed config.
			for _, log := range mAudit.AuditLogs() {
				require.NotEqual(t, database.ResourceTypeMCPServerConfig, log.ResourceType)
			}
		})
	}
}
