package coderd_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisioner/terraform"
	provProto "github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func TestDynamicParametersOwnerSSHPublicKey(t *testing.T) {
	t.Parallel()

	cfg := coderdtest.DeploymentValues(t)
	cfg.Experiments = []string{string(codersdk.ExperimentDynamicParameters)}
	ownerClient := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, DeploymentValues: cfg})
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	templateAdmin, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())

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
	stream, err := templateAdmin.TemplateVersionDynamicParameters(ctx, version.ID)
	require.NoError(t, err)
	defer stream.Close(websocket.StatusGoingAway)

	previews := stream.Chan()

	// Should automatically send a form state with all defaulted/empty values
	preview := testutil.RequireReceive(ctx, t, previews)
	require.Equal(t, -1, preview.ID)
	require.Empty(t, preview.Diagnostics)
	require.Equal(t, "public_key", preview.Parameters[0].Name)
	require.True(t, preview.Parameters[0].Value.Valid)
	require.Equal(t, sshKey.PublicKey, preview.Parameters[0].Value.Value)
}

func TestDynamicParametersWithTerraformValues(t *testing.T) {
	t.Parallel()

	t.Run("OK_Modules", func(t *testing.T) {
		t.Parallel()

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
		require.True(t, preview.Parameters[0].Value.Valid)
		require.Equal(t, "CL", preview.Parameters[0].Value.Value)
	})

	// OldProvisioners use the static parameters in the dynamic param flow
	t.Run("OldProvisioner", func(t *testing.T) {
		t.Parallel()

		const defaultValue = "PS"
		setup := setupDynamicParamsTest(t, setupDynamicParamsTestParams{
			provisionerDaemonVersion: "1.4",
			mainTF:                   nil,
			modulesArchive:           nil,
			plan:                     nil,
			static: []*proto.RichParameter{
				{
					Name:         "jetbrains_ide",
					Type:         "string",
					DefaultValue: defaultValue,
					Icon:         "",
					Options: []*proto.RichParameterOption{
						{
							Name:        "PHPStorm",
							Description: "",
							Value:       defaultValue,
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
		require.Contains(t, preview.Diagnostics[0].Summary, "required metadata to support dynamic parameters")
		require.Len(t, preview.Parameters, 1)
		require.Equal(t, "jetbrains_ide", preview.Parameters[0].Name)
		require.True(t, preview.Parameters[0].Value.Valid)
		require.Equal(t, defaultValue, preview.Parameters[0].Value.Value)

		// Test some inputs
		for _, exp := range []string{defaultValue, "GO", "Invalid", defaultValue} {
			inputs := map[string]string{}
			if exp != defaultValue {
				// Let the default value be the default without being explicitly set
				inputs["jetbrains_ide"] = exp
			}
			err := stream.Send(codersdk.DynamicParametersRequest{
				ID:     1,
				Inputs: inputs,
			})
			require.NoError(t, err)

			preview := testutil.RequireReceive(ctx, t, previews)
			diagCount := len(preview.Diagnostics)
			require.Equal(t, 1, diagCount)
			require.Contains(t, preview.Diagnostics[0].Summary, "required metadata to support dynamic parameters")

			require.Len(t, preview.Parameters, 1)
			if exp == "Invalid" { // Try an invalid option
				require.Len(t, preview.Parameters[0].Diagnostics, 1)
			} else {
				require.Len(t, preview.Parameters[0].Diagnostics, 0)
			}
			require.Equal(t, "jetbrains_ide", preview.Parameters[0].Name)
			require.True(t, preview.Parameters[0].Value.Valid)
			require.Equal(t, exp, preview.Parameters[0].Value.Value)
		}
	})

	t.Run("FileError", func(t *testing.T) {
		// Verify files close even if the websocket terminates from an error
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		dynamicParametersTerraformSource, err := os.ReadFile("testdata/parameters/modules/main.tf")
		require.NoError(t, err)

		modulesArchive, err := terraform.GetModulesArchive(os.DirFS("testdata/parameters/modules"))
		require.NoError(t, err)

		setup := setupDynamicParamsTest(t, setupDynamicParamsTestParams{
			db:                       &dbRejectGitSSHKey{Store: db},
			ps:                       ps,
			provisionerDaemonVersion: provProto.CurrentVersion.String(),
			mainTF:                   dynamicParametersTerraformSource,
			modulesArchive:           modulesArchive,
			expectWebsocketError:     true,
		})
		// This is checked in setupDynamicParamsTest. Just doing this in the
		// test to make it obvious what this test is doing.
		require.Zero(t, setup.api.FileCache.Count())
	})

	t.Run("RebuildParameters", func(t *testing.T) {
		t.Parallel()

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

		ctx := testutil.Context(t, testutil.WaitMedium)
		stream := setup.stream
		previews := stream.Chan()

		// Should see the output of the module represented
		preview := testutil.RequireReceive(ctx, t, previews)
		require.Equal(t, -1, preview.ID)
		require.Empty(t, preview.Diagnostics)

		require.Len(t, preview.Parameters, 1)
		require.Equal(t, "jetbrains_ide", preview.Parameters[0].Name)
		require.True(t, preview.Parameters[0].Value.Valid)
		require.Equal(t, "CL", preview.Parameters[0].Value.Value)
		_ = stream.Close(websocket.StatusGoingAway)

		wrk := coderdtest.CreateWorkspace(t, setup.client, setup.template.ID, func(request *codersdk.CreateWorkspaceRequest) {
			request.RichParameterValues = []codersdk.WorkspaceBuildParameter{
				{
					Name:  preview.Parameters[0].Name,
					Value: "GO",
				},
			}
		})
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, setup.client, wrk.LatestBuild.ID)

		params, err := setup.client.WorkspaceBuildParameters(ctx, wrk.LatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, params, 1)
		require.Equal(t, "jetbrains_ide", params[0].Name)
		require.Equal(t, "GO", params[0].Value)

		// A helper function to assert params
		doTransition := func(t *testing.T, trans codersdk.WorkspaceTransition) {
			t.Helper()

			fooVal := coderdtest.RandomUsername(t)
			bld, err := setup.client.CreateWorkspaceBuild(ctx, wrk.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID: setup.template.ActiveVersionID,
				Transition:        trans,
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					// No validation, so this should work as is.
					// Overwrite the value on each transition
					{Name: "foo", Value: fooVal},
				},
				EnableDynamicParameters: ptr.Ref(true),
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, setup.client, wrk.LatestBuild.ID)

			latestParams, err := setup.client.WorkspaceBuildParameters(ctx, bld.ID)
			require.NoError(t, err)
			require.ElementsMatch(t, latestParams, []codersdk.WorkspaceBuildParameter{
				{Name: "jetbrains_ide", Value: "GO"},
				{Name: "foo", Value: fooVal},
			})
		}

		// Restart the workspace, then delete. Asserting params on all builds.
		doTransition(t, codersdk.WorkspaceTransitionStop)
		doTransition(t, codersdk.WorkspaceTransitionStart)
		doTransition(t, codersdk.WorkspaceTransitionDelete)
	})

	t.Run("BadOwner", func(t *testing.T) {
		t.Parallel()

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

		err = stream.Send(codersdk.DynamicParametersRequest{
			ID: 1,
			Inputs: map[string]string{
				"jetbrains_ide": "GO",
			},
			OwnerID: uuid.New(),
		})
		require.NoError(t, err)

		preview = testutil.RequireReceive(ctx, t, previews)
		require.Equal(t, 1, preview.ID)
		require.Len(t, preview.Diagnostics, 1)
		require.Equal(t, preview.Diagnostics[0].Extra.Code, "owner_not_found")
	})
}

type setupDynamicParamsTestParams struct {
	db                       database.Store
	ps                       pubsub.Pubsub
	provisionerDaemonVersion string
	mainTF                   []byte
	modulesArchive           []byte
	plan                     []byte

	static               []*proto.RichParameter
	expectWebsocketError bool
}

type dynamicParamsTest struct {
	client   *codersdk.Client
	api      *coderd.API
	stream   *wsjson.Stream[codersdk.DynamicParametersResponse, codersdk.DynamicParametersRequest]
	template codersdk.Template
}

func setupDynamicParamsTest(t *testing.T, args setupDynamicParamsTestParams) dynamicParamsTest {
	cfg := coderdtest.DeploymentValues(t)
	cfg.Experiments = []string{string(codersdk.ExperimentDynamicParameters)}
	ownerClient, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database:                 args.db,
		Pubsub:                   args.ps,
		IncludeProvisionerDaemon: true,
		ProvisionerDaemonVersion: args.provisionerDaemonVersion,
		DeploymentValues:         cfg,
	})

	owner := coderdtest.CreateFirstUser(t, ownerClient)
	templateAdmin, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())

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
	tpl := coderdtest.CreateTemplate(t, templateAdmin, owner.OrganizationID, version.ID)

	ctx := testutil.Context(t, testutil.WaitShort)
	stream, err := templateAdmin.TemplateVersionDynamicParameters(ctx, version.ID)
	if args.expectWebsocketError {
		require.Errorf(t, err, "expected error forming websocket")
	} else {
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		if stream != nil {
			_ = stream.Close(websocket.StatusGoingAway)
		}
		// Cache should always have 0 files when the only stream is closed
		require.Eventually(t, func() bool {
			return api.FileCache.Count() == 0
		}, testutil.WaitShort/5, testutil.IntervalMedium)
	})

	return dynamicParamsTest{
		client:   ownerClient,
		api:      api,
		stream:   stream,
		template: tpl,
	}
}

// dbRejectGitSSHKey is a cheeky way to force an error to occur in a place
// that is generally impossible to force an error.
type dbRejectGitSSHKey struct {
	database.Store
}

func (*dbRejectGitSSHKey) GetGitSSHKey(_ context.Context, _ uuid.UUID) (database.GitSSHKey, error) {
	return database.GitSSHKey{}, xerrors.New("forcing a fake error")
}
