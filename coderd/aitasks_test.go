package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/util/slice"
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

func TestTasks(t *testing.T) {
	t.Parallel()

	createAITemplate := func(t *testing.T, client *coderdtest.Client, user coderdtest.User) codersdk.Template {
		t.Helper()

		// Create a template version that supports AI tasks with the AI Prompt parameter.
		taskAppID := uuid.New()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionPlan: []*proto.Response{
				{
					Type: &proto.Response_Plan{
						Plan: &proto.PlanComplete{
							Parameters: []*proto.RichParameter{{Name: codersdk.AITaskPromptParameterName, Type: "string"}},
							HasAiTasks: true,
						},
					},
				},
			},
			ProvisionApply: []*proto.Response{
				{
					Type: &proto.Response_Apply{
						Apply: &proto.ApplyComplete{
							Resources: []*proto.Resource{
								{
									Name: "example",
									Type: "aws_instance",
									Agents: []*proto.Agent{
										{
											Id:   uuid.NewString(),
											Name: "example",
											Apps: []*proto.App{
												{
													Id:          taskAppID.String(),
													Slug:        "task-sidebar",
													DisplayName: "Task Sidebar",
												},
											},
										},
									},
								},
							},
							AiTasks: []*proto.AITask{
								{
									SidebarApp: &proto.AITaskSidebarApp{
										Id: taskAppID.String(),
									},
								},
							},
						},
					},
				},
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		return template
	}

	t.Run("List", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		template := createAITemplate(t, client, user)

		// Create a workspace (task) with a specific prompt.
		wantPrompt := "build me a web app"
		workspace := coderdtest.CreateWorkspace(t, client, template.ID, func(req *codersdk.CreateWorkspaceRequest) {
			req.RichParameterValues = []codersdk.WorkspaceBuildParameter{
				{Name: codersdk.AITaskPromptParameterName, Value: wantPrompt},
			}
		})
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// List tasks via experimental API and verify the prompt and status mapping.
		exp := codersdk.NewExperimentalClient(client)
		tasks, err := exp.Tasks(ctx, &codersdk.TasksFilter{Owner: codersdk.Me})
		require.NoError(t, err)

		got, ok := slice.Find(tasks, func(task codersdk.Task) bool { return task.ID == workspace.ID })
		require.True(t, ok, "task should be found in the list")
		assert.Equal(t, wantPrompt, got.InitialPrompt, "task prompt should match the AI Prompt parameter")
		assert.Equal(t, workspace.Name, got.Name, "task name should map from workspace name")
		assert.Equal(t, workspace.ID, got.WorkspaceID.UUID, "workspace id should match")
		// Status should be populated via app status or workspace status mapping.
		assert.NotEmpty(t, got.Status, "task status should not be empty")
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		template := createAITemplate(t, client, user)

		// Create a workspace (task) with a specific prompt.
		wantPrompt := "review my code"
		workspace := coderdtest.CreateWorkspace(t, client, template.ID, func(req *codersdk.CreateWorkspaceRequest) {
			req.RichParameterValues = []codersdk.WorkspaceBuildParameter{
				{Name: codersdk.AITaskPromptParameterName, Value: wantPrompt},
			}
		})
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// Fetch the task by ID via experimental API and verify fields.
		exp := codersdk.NewExperimentalClient(client)
		task, err := exp.TaskByID(ctx, workspace.ID)
		require.NoError(t, err)

		assert.Equal(t, workspace.ID, task.ID, "task ID should match workspace ID")
		assert.Equal(t, workspace.Name, task.Name, "task name should map from workspace name")
		assert.Equal(t, wantPrompt, task.InitialPrompt, "task prompt should match the AI Prompt parameter")
		assert.Equal(t, workspace.ID, task.WorkspaceID.UUID, "workspace id should match")
		assert.NotEmpty(t, task.Status, "task status should not be empty")
	})
}

func TestTasksCreate(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx = testutil.Context(t, testutil.WaitShort)

			taskPrompt = "Some task prompt"
		)

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)

		// Given: A template with an "AI Prompt" parameter
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionPlan: []*proto.Response{
				{Type: &proto.Response_Plan{Plan: &proto.PlanComplete{
					Parameters: []*proto.RichParameter{{Name: "AI Prompt", Type: "string"}},
					HasAiTasks: true,
				}}},
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		expClient := codersdk.NewExperimentalClient(client)

		// When: We attempt to create a Task.
		workspace, err := expClient.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
			TemplateVersionID: template.ActiveVersionID,
			Prompt:            taskPrompt,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// Then: We expect a workspace to have been created.
		assert.NotEmpty(t, workspace.Name)
		assert.Equal(t, template.ID, workspace.TemplateID)

		// And: We expect it to have the "AI Prompt" parameter correctly set.
		parameters, err := client.WorkspaceBuildParameters(ctx, workspace.LatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, parameters, 1)
		assert.Equal(t, codersdk.AITaskPromptParameterName, parameters[0].Name)
		assert.Equal(t, taskPrompt, parameters[0].Value)
	})

	t.Run("FailsOnNonTaskTemplate", func(t *testing.T) {
		t.Parallel()

		var (
			ctx = testutil.Context(t, testutil.WaitShort)

			taskPrompt = "Some task prompt"
		)

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)

		// Given: A template without an "AI Prompt" parameter
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		expClient := codersdk.NewExperimentalClient(client)

		// When: We attempt to create a Task.
		_, err := expClient.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
			TemplateVersionID: template.ActiveVersionID,
			Prompt:            taskPrompt,
		})

		// Then: We expect it to fail.
		var sdkErr *codersdk.Error
		require.Error(t, err)
		require.ErrorAsf(t, err, &sdkErr, "error should be of type *codersdk.Error")
		assert.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("FailsOnInvalidTemplate", func(t *testing.T) {
		t.Parallel()

		var (
			ctx = testutil.Context(t, testutil.WaitShort)

			taskPrompt = "Some task prompt"
		)

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)

		// Given: A template
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		_ = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		expClient := codersdk.NewExperimentalClient(client)

		// When: We attempt to create a Task with an invalid template version ID.
		_, err := expClient.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
			TemplateVersionID: uuid.New(),
			Prompt:            taskPrompt,
		})

		// Then: We expect it to fail.
		var sdkErr *codersdk.Error
		require.Error(t, err)
		require.ErrorAsf(t, err, &sdkErr, "error should be of type *codersdk.Error")
		assert.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}
