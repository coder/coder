package coderd_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func TestDynamicParametersOwnerGroups(t *testing.T) {
	t.Parallel()

	cfg := coderdtest.DeploymentValues(t)
	cfg.Experiments = []string{string(codersdk.ExperimentDynamicParameters)}
	ownerClient := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, DeploymentValues: cfg})
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	templateAdmin, templateAdminUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())

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
	stream, err := templateAdmin.TemplateVersionDynamicParameters(ctx, templateAdminUser.ID, version.ID)
	require.NoError(t, err)
	defer stream.Close(websocket.StatusGoingAway)

	previews := stream.Chan()

	// Should automatically send a form state with all defaulted/empty values
	preview := testutil.RequireReceive(ctx, t, previews)
	require.Equal(t, -1, preview.ID)
	require.Empty(t, preview.Diagnostics)
	require.Equal(t, "group", preview.Parameters[0].Name)
	require.True(t, preview.Parameters[0].Value.Valid())
	require.Equal(t, "Everyone", preview.Parameters[0].Value.Value.AsString())

	// Send a new value, and see it reflected
	err = stream.Send(codersdk.DynamicParametersRequest{
		ID:     1,
		Inputs: map[string]string{"group": "Bloob"},
	})
	require.NoError(t, err)
	preview = testutil.RequireReceive(ctx, t, previews)
	require.Equal(t, 1, preview.ID)
	require.Empty(t, preview.Diagnostics)
	require.Equal(t, "group", preview.Parameters[0].Name)
	require.True(t, preview.Parameters[0].Value.Valid())
	require.Equal(t, "Bloob", preview.Parameters[0].Value.Value.AsString())

	// Back to default
	err = stream.Send(codersdk.DynamicParametersRequest{
		ID:     3,
		Inputs: map[string]string{},
	})
	require.NoError(t, err)
	preview = testutil.RequireReceive(ctx, t, previews)
	require.Equal(t, 3, preview.ID)
	require.Empty(t, preview.Diagnostics)
	require.Equal(t, "group", preview.Parameters[0].Name)
	require.True(t, preview.Parameters[0].Value.Valid())
	require.Equal(t, "Everyone", preview.Parameters[0].Value.Value.AsString())
}

func TestDynamicParametersOwnerSSHPublicKey(t *testing.T) {
	t.Parallel()

	cfg := coderdtest.DeploymentValues(t)
	cfg.Experiments = []string{string(codersdk.ExperimentDynamicParameters)}
	ownerClient := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, DeploymentValues: cfg})
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	templateAdmin, templateAdminUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())

	dynamicParametersTerraformSource, err := os.ReadFile("testdata/parameters/public_key/main.tf")
	require.NoError(t, err)
	dynamicParametersTerraformPlan, err := os.ReadFile("testdata/parameters/public_key/plan.json")
	require.NoError(t, err)
	sshKey, err := templateAdmin.GitSSHKey(t.Context(), "me")
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
	stream, err := templateAdmin.TemplateVersionDynamicParameters(ctx, templateAdminUser.ID, version.ID)
	require.NoError(t, err)
	defer stream.Close(websocket.StatusGoingAway)

	previews := stream.Chan()

	// Should automatically send a form state with all defaulted/empty values
	preview := testutil.RequireReceive(ctx, t, previews)
	require.Equal(t, -1, preview.ID)
	require.Empty(t, preview.Diagnostics)
	require.Equal(t, "public_key", preview.Parameters[0].Name)
	require.True(t, preview.Parameters[0].Value.Valid())
	require.Equal(t, sshKey.PublicKey, preview.Parameters[0].Value.Value.AsString())
}
