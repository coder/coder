package coderd_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestAITasksPrompts(t *testing.T) {
	t.Parallel()

	t.Run("EmptyBuildIDs", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{})
		_ = coderdtest.CreateFirstUser(t, client)
		experimentalClient := codersdk.NewExperimentalClient(client)

		ctx := testutil.Context(t, testutil.WaitShort)

		// Test with empty build IDs
		prompts, err := experimentalClient.AITaskPrompts(ctx, []uuid.UUID{})
		require.NoError(t, err)
		require.Empty(t, prompts.Prompts)
	})

	t.Run("MultipleBuilds", func(t *testing.T) {
		t.Parallel()

		if !dbtestutil.WillUsePostgres() {
			t.Skip("This test checks RBAC, which is not supported in the in-memory database")
		}

		adminClient := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		first := coderdtest.CreateFirstUser(t, adminClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, first.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)

		// Create a template with parameters
		version := coderdtest.CreateTemplateVersion(t, adminClient, first.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionPlan: []*proto.Response{{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
						Parameters: []*proto.RichParameter{
							{
								Name:         "param1",
								Type:         "string",
								DefaultValue: "default1",
							},
							{
								Name:         codersdk.AITaskPromptParameterName,
								Type:         "string",
								DefaultValue: "default2",
							},
						},
					},
				},
			}},
			ProvisionApply: echo.ApplyComplete,
		})
		template := coderdtest.CreateTemplate(t, adminClient, first.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, adminClient, version.ID)

		// Create two workspaces with different parameters
		workspace1 := coderdtest.CreateWorkspace(t, memberClient, template.ID, func(request *codersdk.CreateWorkspaceRequest) {
			request.RichParameterValues = []codersdk.WorkspaceBuildParameter{
				{Name: "param1", Value: "value1a"},
				{Name: codersdk.AITaskPromptParameterName, Value: "value2a"},
			}
		})
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, memberClient, workspace1.LatestBuild.ID)

		workspace2 := coderdtest.CreateWorkspace(t, memberClient, template.ID, func(request *codersdk.CreateWorkspaceRequest) {
			request.RichParameterValues = []codersdk.WorkspaceBuildParameter{
				{Name: "param1", Value: "value1b"},
				{Name: codersdk.AITaskPromptParameterName, Value: "value2b"},
			}
		})
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, memberClient, workspace2.LatestBuild.ID)

		workspace3 := coderdtest.CreateWorkspace(t, adminClient, template.ID, func(request *codersdk.CreateWorkspaceRequest) {
			request.RichParameterValues = []codersdk.WorkspaceBuildParameter{
				{Name: "param1", Value: "value1c"},
				{Name: codersdk.AITaskPromptParameterName, Value: "value2c"},
			}
		})
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, adminClient, workspace3.LatestBuild.ID)
		allBuildIDs := []uuid.UUID{workspace1.LatestBuild.ID, workspace2.LatestBuild.ID, workspace3.LatestBuild.ID}

		experimentalMemberClient := codersdk.NewExperimentalClient(memberClient)
		// Test parameters endpoint as member
		prompts, err := experimentalMemberClient.AITaskPrompts(ctx, allBuildIDs)
		require.NoError(t, err)
		// we expect 2 prompts because the member client does not have access to workspace3
		// since it was created by the admin client
		require.Len(t, prompts.Prompts, 2)

		// Check workspace1 parameters
		build1Prompt := prompts.Prompts[workspace1.LatestBuild.ID.String()]
		require.Equal(t, "value2a", build1Prompt)

		// Check workspace2 parameters
		build2Prompt := prompts.Prompts[workspace2.LatestBuild.ID.String()]
		require.Equal(t, "value2b", build2Prompt)

		experimentalAdminClient := codersdk.NewExperimentalClient(adminClient)
		// Test parameters endpoint as admin
		// we expect 3 prompts because the admin client has access to all workspaces
		prompts, err = experimentalAdminClient.AITaskPrompts(ctx, allBuildIDs)
		require.NoError(t, err)
		require.Len(t, prompts.Prompts, 3)

		// Check workspace3 parameters
		build3Prompt := prompts.Prompts[workspace3.LatestBuild.ID.String()]
		require.Equal(t, "value2c", build3Prompt)
	})

	t.Run("NonExistentBuildIDs", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{})
		_ = coderdtest.CreateFirstUser(t, client)

		ctx := testutil.Context(t, testutil.WaitShort)

		// Test with non-existent build IDs
		nonExistentID := uuid.New()
		experimentalClient := codersdk.NewExperimentalClient(client)
		prompts, err := experimentalClient.AITaskPrompts(ctx, []uuid.UUID{nonExistentID})
		require.NoError(t, err)
		require.Empty(t, prompts.Prompts)
	})
}
