package coderd_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func TestDynamicParametersOwnerGroups(t *testing.T) {
	t.Parallel()

	ownerClient, owner := coderdenttest.New(t,
		&coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
			Options: &coderdtest.Options{IncludeProvisionerDaemon: true},
		},
	)
	templateAdmin, templateAdminUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgTemplateAdmin(owner.OrganizationID))
	_, noGroupUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	// Create the group to be asserted
	group := coderdtest.CreateGroup(t, ownerClient, owner.OrganizationID, "bloob", templateAdminUser)

	dynamicParametersTerraformSource, err := os.ReadFile("testdata/parameters/groups/main.tf")
	require.NoError(t, err)
	dynamicParametersTerraformPlan, err := os.ReadFile("testdata/parameters/groups/plan.json")
	require.NoError(t, err)

	files := echo.WithExtraFiles(map[string][]byte{
		"main.tf": dynamicParametersTerraformSource,
	})
	files.ProvisionPlan = []*proto.Response{{
		Type: &proto.Response_Plan{
			Plan: &proto.PlanComplete{
				Plan: dynamicParametersTerraformPlan,
			},
		},
	}}

	version := coderdtest.CreateTemplateVersion(t, templateAdmin, owner.OrganizationID, files)
	coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdmin, version.ID)
	_ = coderdtest.CreateTemplate(t, templateAdmin, owner.OrganizationID, version.ID)

	ctx := testutil.Context(t, testutil.WaitShort)

	// First check with a no group admin user, that they do not see the extra group
	// Use the admin client, as the user might not have access to the template.
	// Also checking that the admin can see the form for the other user.
	noGroupStream, err := templateAdmin.TemplateVersionDynamicParameters(ctx, noGroupUser.ID.String(), version.ID)
	require.NoError(t, err)
	defer noGroupStream.Close(websocket.StatusGoingAway)
	noGroupPreviews := noGroupStream.Chan()
	noGroupPreview := testutil.RequireReceive(ctx, t, noGroupPreviews)
	require.Equal(t, -1, noGroupPreview.ID)
	require.Empty(t, noGroupPreview.Diagnostics)
	require.Equal(t, "group", noGroupPreview.Parameters[0].Name)
	require.Equal(t, database.EveryoneGroup, noGroupPreview.Parameters[0].Value.Value)
	require.Equal(t, 1, len(noGroupPreview.Parameters[0].Options)) // Only 1 group
	noGroupStream.Close(websocket.StatusGoingAway)

	// Now try with a user with more than 1 group
	stream, err := templateAdmin.TemplateVersionDynamicParameters(ctx, codersdk.Me, version.ID)
	require.NoError(t, err)
	defer stream.Close(websocket.StatusGoingAway)

	previews, pop := coderdtest.SynchronousStream(stream)

	// Should automatically send a form state with all defaulted/empty values
	preview := pop()
	require.Equal(t, -1, preview.ID)
	require.Empty(t, preview.Diagnostics)
	require.Equal(t, "group", preview.Parameters[0].Name)
	require.True(t, preview.Parameters[0].Value.Valid)
	require.Equal(t, database.EveryoneGroup, preview.Parameters[0].Value.Value)

	// Send a new value, and see it reflected
	preview, err = previews(codersdk.DynamicParametersRequest{
		ID:     1,
		Inputs: map[string]string{"group": group.Name},
	})
	require.NoError(t, err)
	require.Equal(t, 1, preview.ID)
	require.Empty(t, preview.Diagnostics)
	require.Equal(t, "group", preview.Parameters[0].Name)
	require.True(t, preview.Parameters[0].Value.Valid)
	require.Equal(t, group.Name, preview.Parameters[0].Value.Value)

	// Back to default
	preview, err = previews(codersdk.DynamicParametersRequest{
		ID:     3,
		Inputs: map[string]string{},
	})
	require.NoError(t, err)
	require.Equal(t, 3, preview.ID)
	require.Empty(t, preview.Diagnostics)
	require.Equal(t, "group", preview.Parameters[0].Name)
	require.True(t, preview.Parameters[0].Value.Valid)
	require.Equal(t, database.EveryoneGroup, preview.Parameters[0].Value.Value)
}
