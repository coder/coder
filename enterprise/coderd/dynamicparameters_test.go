package coderd_test

import (
	_ "embed"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

// TestDynamicParameterTemplate uses a template with some dynamic elements, and
// tests the parameters, values, etc are all as expected.
func TestDynamicParameterTemplate(t *testing.T) {
	t.Parallel()

	owner, _, api, first := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{IncludeProvisionerDaemon: true},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		},
	})

	orgID := first.OrganizationID

	_, userData := coderdtest.CreateAnotherUser(t, owner, orgID)
	templateAdmin, templateAdminData := coderdtest.CreateAnotherUser(t, owner, orgID, rbac.ScopedRoleOrgTemplateAdmin(orgID))
	userAdmin, userAdminData := coderdtest.CreateAnotherUser(t, owner, orgID, rbac.ScopedRoleOrgUserAdmin(orgID))
	_, auditorData := coderdtest.CreateAnotherUser(t, owner, orgID, rbac.ScopedRoleOrgAuditor(orgID))

	coderdtest.CreateGroup(t, owner, orgID, "developer", auditorData, userData)
	coderdtest.CreateGroup(t, owner, orgID, "admin", templateAdminData, userAdminData)
	coderdtest.CreateGroup(t, owner, orgID, "auditor", auditorData, templateAdminData, userAdminData)

	dynamicParametersTerraformSource, err := os.ReadFile("testdata/parameters/dynamic/main.tf")
	require.NoError(t, err)

	_, version := coderdtest.DynamicParameterTemplate(t, templateAdmin, orgID, coderdtest.DynamicParameterTemplateParams{
		MainTF:         string(dynamicParametersTerraformSource),
		Plan:           nil,
		ModulesArchive: nil,
		StaticParams:   nil,
	})

	var _ = userAdmin

	ctx := testutil.Context(t, testutil.WaitLong)

	stream, err := templateAdmin.TemplateVersionDynamicParameters(ctx, userData.ID.String(), version.ID)
	require.NoError(t, err)
	defer func() {
		_ = stream.Close(websocket.StatusNormalClosure)

		// Wait until the cache ends up empty. This verifies the cache does not
		// leak any files.
		require.Eventually(t, func() bool {
			return api.AGPL.FileCache.Count() == 0
		}, testutil.WaitShort, testutil.IntervalFast, "file cache should be empty after the test")
	}()

	// Initial response
	preview, pop := coderdtest.SynchronousStream(stream)
	init := pop()
	require.Len(t, init.Diagnostics, 0, "no top level diags")
	coderdtest.AssertParameter(t, "isAdmin", init.Parameters).
		Exists().Value("false")
	coderdtest.AssertParameter(t, "adminonly", init.Parameters).
		NotExists()
	coderdtest.AssertParameter(t, "groups", init.Parameters).
		Exists().Options(database.EveryoneGroup, "developer")

	// Switch to an admin
	resp, err := preview(codersdk.DynamicParametersRequest{
		ID: 1,
		Inputs: map[string]string{
			"colors": `["red"]`,
			"thing":  "apple",
		},
		OwnerID: userAdminData.ID,
	})
	require.NoError(t, err)
	require.Equal(t, resp.ID, 1)
	require.Len(t, resp.Diagnostics, 0, "no top level diags")

	coderdtest.AssertParameter(t, "isAdmin", resp.Parameters).
		Exists().Value("true")
	coderdtest.AssertParameter(t, "adminonly", resp.Parameters).
		Exists()
	coderdtest.AssertParameter(t, "groups", resp.Parameters).
		Exists().Options(database.EveryoneGroup, "admin", "auditor")
	coderdtest.AssertParameter(t, "colors", resp.Parameters).
		Exists().Value(`["red"]`)
	coderdtest.AssertParameter(t, "thing", resp.Parameters).
		Exists().Value("apple").Options("apple", "ruby")
	coderdtest.AssertParameter(t, "cool", resp.Parameters).
		NotExists()

	// Try some other colors
	resp, err = preview(codersdk.DynamicParametersRequest{
		ID: 2,
		Inputs: map[string]string{
			"colors": `["yellow", "blue"]`,
			"thing":  "banana",
		},
		OwnerID: userAdminData.ID,
	})
	require.NoError(t, err)
	require.Equal(t, resp.ID, 2)
	require.Len(t, resp.Diagnostics, 0, "no top level diags")

	coderdtest.AssertParameter(t, "cool", resp.Parameters).
		Exists()
	coderdtest.AssertParameter(t, "isAdmin", resp.Parameters).
		Exists().Value("true")
	coderdtest.AssertParameter(t, "colors", resp.Parameters).
		Exists().Value(`["yellow", "blue"]`)
	coderdtest.AssertParameter(t, "thing", resp.Parameters).
		Exists().Value("banana").Options("banana", "ocean", "sky")
}
