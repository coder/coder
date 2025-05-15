package coderd_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisioner/terraform"
	provProto "github.com/coder/coder/v2/provisionerd/proto"
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

func TestDynamicParametersWithTerraformValues(t *testing.T) {
	t.Parallel()

	t.Run("OK_Modules", func(t *testing.T) {
		dynamicParametersTerraformSource, err := os.ReadFile("testdata/parameters/modules/main.tf")
		require.NoError(t, err)

		modulesArchive, err := terraform.GetModulesArchive(os.DirFS("testdata/parameters/modules"))
		require.NoError(t, err)

		setup := setupDynamicParamsTest(t, setupDynamicParamsTestParams{
			provisionerDaemonVersion: provProto.CurrentVersion.String(),
			mainTF:                   dynamicParametersTerraformSource,
			modulesArchive:           modulesArchive,
			plan:                     nil,
			static:                   nil,
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		stream := setup.stream
		previews := stream.Chan()

		// Should see the output of the module represented
		preview := testutil.RequireReceive(ctx, t, previews)
		require.Equal(t, -1, preview.ID)
		require.Empty(t, preview.Diagnostics)

		require.Len(t, preview.Parameters, 1)
		require.Equal(t, "jetbrains_ide", preview.Parameters[0].Name)
		require.True(t, preview.Parameters[0].Value.Valid())
		require.Equal(t, "CL", preview.Parameters[0].Value.AsString())
	})

	// OldProvisioners use the static parameters in the dynamic param flow
	t.Run("OldProvisioner", func(t *testing.T) {
		setup := setupDynamicParamsTest(t, setupDynamicParamsTestParams{
			provisionerDaemonVersion: "1.4",
			mainTF:                   nil,
			modulesArchive:           nil,
			plan:                     nil,
			static: []*proto.RichParameter{
				{
					Name:         "jetbrains_ide",
					Type:         "string",
					DefaultValue: "PS",
					Icon:         "",
					Options: []*proto.RichParameterOption{
						{
							Name:        "PHPStorm",
							Description: "",
							Value:       "PS",
							Icon:        "",
						},
						{
							Name:        "Golang",
							Description: "",
							Value:       "GO",
							Icon:        "",
						},
					},
					ValidationRegex: "[PG][SO]",
					ValidationError: "Regex check",
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		stream := setup.stream
		previews := stream.Chan()

		// Assert the initial state
		preview := testutil.RequireReceive(ctx, t, previews)
		diagCount := len(preview.Diagnostics)
		require.Equal(t, 1, diagCount)
		require.Contains(t, preview.Diagnostics[0].Summary, "classic creation flow")
		require.Len(t, preview.Parameters, 1)
		require.Equal(t, "jetbrains_ide", preview.Parameters[0].Name)
		require.True(t, preview.Parameters[0].Value.Valid())
		require.Equal(t, "PS", preview.Parameters[0].Value.AsString())

		// Test some inputs
		for _, exp := range []string{"PS", "GO", "Invalid"} {
			err := stream.Send(codersdk.DynamicParametersRequest{
				ID: 1,
				Inputs: map[string]string{
					"jetbrains_ide": exp,
				},
			})
			require.NoError(t, err)

			preview := testutil.RequireReceive(ctx, t, previews)
			diagCount := len(preview.Diagnostics)
			require.Equal(t, 1, diagCount)
			require.Contains(t, preview.Diagnostics[0].Summary, "classic creation flow")

			require.Len(t, preview.Parameters, 1)
			if exp == "Invalid" { // Try an invalid option
				require.Len(t, preview.Parameters[0].Diagnostics, 1)
			} else {
				require.Len(t, preview.Parameters[0].Diagnostics, 0)
			}
			require.Equal(t, "jetbrains_ide", preview.Parameters[0].Name)
			require.True(t, preview.Parameters[0].Value.Valid())
			require.Equal(t, exp, preview.Parameters[0].Value.AsString())
		}

	})
}

type setupDynamicParamsTestParams struct {
	provisionerDaemonVersion string
	mainTF                   []byte
	modulesArchive           []byte
	plan                     []byte

	static []*proto.RichParameter
}

type dynamicParamsTest struct {
	client *codersdk.Client
	api    *coderd.API
	stream *wsjson.Stream[codersdk.DynamicParametersResponse, codersdk.DynamicParametersRequest]
}

func setupDynamicParamsTest(t *testing.T, args setupDynamicParamsTestParams) dynamicParamsTest {
	cfg := coderdtest.DeploymentValues(t)
	cfg.Experiments = []string{string(codersdk.ExperimentDynamicParameters)}
	ownerClient, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		ProvisionerDaemonVersion: args.provisionerDaemonVersion,
		DeploymentValues:         cfg,
	})

	owner := coderdtest.CreateFirstUser(t, ownerClient)
	templateAdmin, templateAdminUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())

	files := echo.WithExtraFiles(map[string][]byte{
		"main.tf": args.mainTF,
	})
	files.ProvisionPlan = []*proto.Response{{
		Type: &proto.Response_Plan{
			Plan: &proto.PlanComplete{
				Plan:        args.plan,
				ModuleFiles: args.modulesArchive,
				Parameters:  args.static,
			},
		},
	}}

	version := coderdtest.CreateTemplateVersion(t, templateAdmin, owner.OrganizationID, files)
	coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdmin, version.ID)
	_ = coderdtest.CreateTemplate(t, templateAdmin, owner.OrganizationID, version.ID)

	ctx := testutil.Context(t, testutil.WaitShort)
	stream, err := templateAdmin.TemplateVersionDynamicParameters(ctx, templateAdminUser.ID, version.ID)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = stream.Close(websocket.StatusGoingAway)
		// Cache should always have 0 files when the only stream is closed
		require.Eventually(t, func() bool {
			return api.FileCache.Count() == 0
		}, testutil.WaitShort/5, testutil.IntervalMedium)
	})

	return dynamicParamsTest{
		client: ownerClient,
		stream: stream,
		api:    api,
	}
}
