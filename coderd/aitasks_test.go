package coderd_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
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

	type aiTemplateOpts struct {
		appURL    string
		authToken string
	}

	type aiTemplateOpt func(*aiTemplateOpts)

	withSidebarURL := func(url string) aiTemplateOpt { return func(o *aiTemplateOpts) { o.appURL = url } }
	withAgentToken := func(token string) aiTemplateOpt { return func(o *aiTemplateOpts) { o.authToken = token } }

	createAITemplate := func(t *testing.T, client *codersdk.Client, user codersdk.CreateFirstUserResponse, opts ...aiTemplateOpt) codersdk.Template {
		t.Helper()

		opt := aiTemplateOpts{
			authToken: uuid.New().String(),
		}
		for _, o := range opts {
			o(&opt)
		}

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
											Auth: &proto.Agent_Token{
												Token: opt.authToken,
											},
											Apps: []*proto.App{
												{
													Id:          taskAppID.String(),
													Slug:        "task-sidebar",
													DisplayName: "Task Sidebar",
													Url:         opt.appURL,
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

		var (
			client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			ctx        = testutil.Context(t, testutil.WaitLong)
			user       = coderdtest.CreateFirstUser(t, client)
			template   = createAITemplate(t, client, user)
			// Create a workspace (task) with a specific prompt.
			wantPrompt = "review my code"
			workspace  = coderdtest.CreateWorkspace(t, client, template.ID, func(req *codersdk.CreateWorkspaceRequest) {
				req.RichParameterValues = []codersdk.WorkspaceBuildParameter{
					{Name: codersdk.AITaskPromptParameterName, Value: wantPrompt},
				}
			})
		)

		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		ws := coderdtest.MustWorkspace(t, client, workspace.ID)
		// Assert invariant: the workspace has exactly one resource with one agent with one app.
		require.Len(t, ws.LatestBuild.Resources, 1)
		require.Len(t, ws.LatestBuild.Resources[0].Agents, 1)
		agentID := ws.LatestBuild.Resources[0].Agents[0].ID
		taskAppID := ws.LatestBuild.Resources[0].Agents[0].Apps[0].ID

		// Insert an app status for the workspace
		_, err := db.InsertWorkspaceAppStatus(dbauthz.AsSystemRestricted(ctx), database.InsertWorkspaceAppStatusParams{
			ID:          uuid.New(),
			WorkspaceID: workspace.ID,
			CreatedAt:   dbtime.Now(),
			AgentID:     agentID,
			AppID:       taskAppID,
			State:       database.WorkspaceAppStatusStateComplete,
			Message:     "all done",
		})
		require.NoError(t, err)

		// Fetch the task by ID via experimental API and verify fields.
		exp := codersdk.NewExperimentalClient(client)
		task, err := exp.TaskByID(ctx, workspace.ID)
		require.NoError(t, err)

		assert.Equal(t, workspace.ID, task.ID, "task ID should match workspace ID")
		assert.Equal(t, workspace.Name, task.Name, "task name should map from workspace name")
		assert.Equal(t, wantPrompt, task.InitialPrompt, "task prompt should match the AI Prompt parameter")
		assert.Equal(t, workspace.ID, task.WorkspaceID.UUID, "workspace id should match")
		assert.NotEmpty(t, task.Status, "task status should not be empty")

		// Stop the workspace
		coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

		// Verify that the previous status still remains
		updated, err := exp.TaskByID(ctx, workspace.ID)
		require.NoError(t, err)
		assert.NotNil(t, updated.CurrentState, "current state should not be nil")
		assert.Equal(t, "all done", updated.CurrentState.Message)
		assert.Equal(t, codersdk.TaskStateComplete, updated.CurrentState.State)

		// Start the workspace again
		coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStop, codersdk.WorkspaceTransitionStart)

		// Verify that the status from the previous build is no longer present
		updated, err = exp.TaskByID(ctx, workspace.ID)
		require.NoError(t, err)
		assert.Nil(t, updated.CurrentState, "current state should be nil")
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			template := createAITemplate(t, client, user)

			ctx := testutil.Context(t, testutil.WaitLong)

			exp := codersdk.NewExperimentalClient(client)
			task, err := exp.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
				TemplateVersionID: template.ActiveVersionID,
				Input:             "delete me",
			})
			require.NoError(t, err)
			ws, err := client.Workspace(ctx, task.ID)
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

			err = exp.DeleteTask(ctx, "me", task.ID)
			require.NoError(t, err, "delete task request should be accepted")

			// Poll until the workspace is deleted.
			for {
				dws, derr := client.DeletedWorkspace(ctx, task.ID)
				if derr == nil && dws.LatestBuild.Status == codersdk.WorkspaceStatusDeleted {
					break
				}
				if ctx.Err() != nil {
					require.NoError(t, derr, "expected to fetch deleted workspace before deadline")
					require.Equal(t, codersdk.WorkspaceStatusDeleted, dws.LatestBuild.Status, "workspace should be deleted before deadline")
					break
				}
				time.Sleep(testutil.IntervalMedium)
			}
		})

		t.Run("NotFound", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			_ = coderdtest.CreateFirstUser(t, client)

			ctx := testutil.Context(t, testutil.WaitShort)

			exp := codersdk.NewExperimentalClient(client)
			err := exp.DeleteTask(ctx, "me", uuid.New())

			var sdkErr *codersdk.Error
			require.Error(t, err, "expected an error for non-existent task")
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, 404, sdkErr.StatusCode())
		})

		t.Run("NotTaskWorkspace", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)

			ctx := testutil.Context(t, testutil.WaitShort)

			// Create a template without AI tasks support and a workspace from it.
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			ws := coderdtest.CreateWorkspace(t, client, template.ID)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

			exp := codersdk.NewExperimentalClient(client)
			err := exp.DeleteTask(ctx, "me", ws.ID)

			var sdkErr *codersdk.Error
			require.Error(t, err, "expected an error for non-task workspace delete via tasks endpoint")
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, 404, sdkErr.StatusCode())
		})

		t.Run("UnauthorizedUserCannotDeleteOthersTask", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)

			// Owner's AI-capable template and workspace (task).
			template := createAITemplate(t, client, owner)

			ctx := testutil.Context(t, testutil.WaitShort)

			exp := codersdk.NewExperimentalClient(client)
			task, err := exp.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
				TemplateVersionID: template.ActiveVersionID,
				Input:             "delete me not",
			})
			require.NoError(t, err)
			ws, err := client.Workspace(ctx, task.ID)
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

			// Another regular org member without elevated permissions.
			otherClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
			expOther := codersdk.NewExperimentalClient(otherClient)

			// Attempt to delete the owner's task as a non-owner without permissions.
			err = expOther.DeleteTask(ctx, "me", task.ID)

			var authErr *codersdk.Error
			require.Error(t, err, "expected an authorization error when deleting another user's task")
			require.ErrorAs(t, err, &authErr)
			// Accept either 403 or 404 depending on authz behavior.
			if authErr.StatusCode() != 403 && authErr.StatusCode() != 404 {
				t.Fatalf("unexpected status code: %d (expected 403 or 404)", authErr.StatusCode())
			}
		})
	})

	t.Run("Send", func(t *testing.T) {
		t.Parallel()

		t.Run("IntegrationOK", func(t *testing.T) {
			t.Parallel()

			client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			userClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

			createStatusResponse := func(status string) string {
				return `
					{
						"$schema": "http://localhost:3284/schemas/StatusResponseBody.json",
						"status": "` + status + `"
					}
				`
			}
			statusResponse := createStatusResponse("stable")

			// Start a fake AgentAPI that accepts GET /status and POST /message.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/status" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = fmt.Fprint(w, statusResponse)
					return
				}
				if r.Method == http.MethodPost && r.URL.Path == "/message" {
					w.Header().Set("Content-Type", "application/json")

					b, _ := io.ReadAll(r.Body)
					assert.Equal(t, `{"content":"Hello, Agent!","type":"user"}`, string(b), "expected message content")

					w.WriteHeader(http.StatusOK)
					io.WriteString(w, `{"ok": true}`)
					return
				}
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer srv.Close()

			// Create an AI-capable template whose sidebar app points to our fake AgentAPI.
			authToken := uuid.NewString()
			template := createAITemplate(t, client, owner, withSidebarURL(srv.URL), withAgentToken(authToken))

			// Create a workspace (task) from the AI-capable template.
			ws := coderdtest.CreateWorkspace(t, userClient, template.ID, func(req *codersdk.CreateWorkspaceRequest) {
				req.RichParameterValues = []codersdk.WorkspaceBuildParameter{
					{Name: codersdk.AITaskPromptParameterName, Value: "send a message"},
				}
			})
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

			// Start a fake agent so the workspace agent is connected before sending the message.
			agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(authToken))
			_ = agenttest.New(t, client.URL, authToken, func(o *agent.Options) {
				o.Client = agentClient
			})
			coderdtest.NewWorkspaceAgentWaiter(t, client, ws.ID).WaitFor(coderdtest.AgentsReady)

			ctx := testutil.Context(t, testutil.WaitMedium)

			// Lookup the sidebar app ID.
			w, err := client.Workspace(ctx, ws.ID)
			require.NoError(t, err)
			var sidebarAppID uuid.UUID
			for _, res := range w.LatestBuild.Resources {
				for _, ag := range res.Agents {
					for _, app := range ag.Apps {
						if app.Slug == "task-sidebar" {
							sidebarAppID = app.ID
						}
					}
				}
			}
			require.NotEqual(t, uuid.Nil, sidebarAppID)

			// Make the sidebar app unhealthy initially.
			err = api.Database.UpdateWorkspaceAppHealthByID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAppHealthByIDParams{
				ID:     sidebarAppID,
				Health: database.WorkspaceAppHealthUnhealthy,
			})
			require.NoError(t, err)

			exp := codersdk.NewExperimentalClient(userClient)
			err = exp.TaskSend(ctx, "me", ws.ID, codersdk.TaskSendRequest{
				Input: "Hello, Agent!",
			})
			require.Error(t, err, "wanted error due to unhealthy sidebar app")

			// Make the sidebar app healthy.
			err = api.Database.UpdateWorkspaceAppHealthByID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAppHealthByIDParams{
				ID:     sidebarAppID,
				Health: database.WorkspaceAppHealthHealthy,
			})
			require.NoError(t, err)

			statusResponse = createStatusResponse("bad")

			err = exp.TaskSend(ctx, "me", ws.ID, codersdk.TaskSendRequest{
				Input: "Hello, Agent!",
			})
			require.Error(t, err, "wanted error due to bad status")

			statusResponse = createStatusResponse("stable")

			// Send task input to the tasks sidebar app and expect 204.e
			err = exp.TaskSend(ctx, "me", ws.ID, codersdk.TaskSendRequest{
				Input: "Hello, Agent!",
			})
			require.NoError(t, err, "wanted no error due to healthy sidebar app and stable status")
		})

		t.Run("MissingContent", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			ctx := testutil.Context(t, testutil.WaitLong)

			template := createAITemplate(t, client, user)

			// Create a workspace (task).
			ws := coderdtest.CreateWorkspace(t, client, template.ID, func(req *codersdk.CreateWorkspaceRequest) {
				req.RichParameterValues = []codersdk.WorkspaceBuildParameter{
					{Name: codersdk.AITaskPromptParameterName, Value: "do work"},
				}
			})
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

			exp := codersdk.NewExperimentalClient(client)
			err := exp.TaskSend(ctx, "me", ws.ID, codersdk.TaskSendRequest{
				Input: "",
			})

			var sdkErr *codersdk.Error
			require.Error(t, err)
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		})

		t.Run("TaskNotFound", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			_ = coderdtest.CreateFirstUser(t, client)
			ctx := testutil.Context(t, testutil.WaitShort)

			exp := codersdk.NewExperimentalClient(client)
			err := exp.TaskSend(ctx, "me", uuid.New(), codersdk.TaskSendRequest{
				Input: "hi",
			})

			var sdkErr *codersdk.Error
			require.Error(t, err)
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
		})

		t.Run("NotATask", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			ctx := testutil.Context(t, testutil.WaitShort)

			// Create a template without AI tasks.
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

			ws := coderdtest.CreateWorkspace(t, client, template.ID)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

			exp := codersdk.NewExperimentalClient(client)
			err := exp.TaskSend(ctx, "me", ws.ID, codersdk.TaskSendRequest{
				Input: "hello",
			})

			var sdkErr *codersdk.Error
			require.Error(t, err)
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		})
	})

	t.Run("Logs", func(t *testing.T) {
		t.Parallel()

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			ctx := testutil.Context(t, testutil.WaitLong)

			messageResponse := `
				{
					"$schema": "http://localhost:3284/schemas/MessagesResponseBody.json",
					"messages": [
						{
							"id": 0,
							"content": "Welcome, user!",
							"role": "agent",
							"time": "2025-09-25T10:42:48.751774125Z"
						},
						{
							"id": 1,
							"content": "Hello, agent!",
							"role": "user",
							"time": "2025-09-25T10:46:42.880996296Z"
						},
						{
							"id": 2,
							"content": "What would you like to work on today?",
							"role": "agent",
							"time": "2025-09-25T10:46:50.747761102Z"
						}
					]
				}
			`

			// Fake AgentAPI that returns a couple of messages.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/messages" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					io.WriteString(w, messageResponse)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			t.Cleanup(srv.Close)

			// Template pointing sidebar app to our fake AgentAPI.
			authToken := uuid.NewString()
			template := createAITemplate(t, client, owner, withSidebarURL(srv.URL), withAgentToken(authToken))

			// Create task workspace.
			ws := coderdtest.CreateWorkspace(t, client, template.ID, func(req *codersdk.CreateWorkspaceRequest) {
				req.RichParameterValues = []codersdk.WorkspaceBuildParameter{
					{Name: codersdk.AITaskPromptParameterName, Value: "show logs"},
				}
			})
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

			// Start a fake agent.
			agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(authToken))
			_ = agenttest.New(t, client.URL, authToken, func(o *agent.Options) {
				o.Client = agentClient
			})
			coderdtest.NewWorkspaceAgentWaiter(t, client, ws.ID).WaitFor(coderdtest.AgentsReady)

			// Omit sidebar app health as undefined is OK.

			// Fetch logs.
			exp := codersdk.NewExperimentalClient(client)
			resp, err := exp.TaskLogs(ctx, "me", ws.ID)
			require.NoError(t, err)
			require.Len(t, resp.Logs, 3)
			assert.Equal(t, 0, resp.Logs[0].ID)
			assert.Equal(t, codersdk.TaskLogTypeOutput, resp.Logs[0].Type)
			assert.Equal(t, "Welcome, user!", resp.Logs[0].Content)

			assert.Equal(t, 1, resp.Logs[1].ID)
			assert.Equal(t, codersdk.TaskLogTypeInput, resp.Logs[1].Type)
			assert.Equal(t, "Hello, agent!", resp.Logs[1].Content)

			assert.Equal(t, 2, resp.Logs[2].ID)
			assert.Equal(t, codersdk.TaskLogTypeOutput, resp.Logs[2].Type)
			assert.Equal(t, "What would you like to work on today?", resp.Logs[2].Content)
		})

		t.Run("UpstreamError", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			ctx := testutil.Context(t, testutil.WaitShort)

			// Fake AgentAPI that returns 500 for messages.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, "boom")
			}))
			t.Cleanup(srv.Close)

			authToken := uuid.NewString()
			template := createAITemplate(t, client, owner, withSidebarURL(srv.URL), withAgentToken(authToken))
			ws := coderdtest.CreateWorkspace(t, client, template.ID, func(req *codersdk.CreateWorkspaceRequest) {
				req.RichParameterValues = []codersdk.WorkspaceBuildParameter{
					{Name: codersdk.AITaskPromptParameterName, Value: "show logs"},
				}
			})
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

			// Start fake agent.
			agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(authToken))
			_ = agenttest.New(t, client.URL, authToken, func(o *agent.Options) {
				o.Client = agentClient
			})
			coderdtest.NewWorkspaceAgentWaiter(t, client, ws.ID).WaitFor(coderdtest.AgentsReady)

			exp := codersdk.NewExperimentalClient(client)
			_, err := exp.TaskLogs(ctx, "me", ws.ID)

			var sdkErr *codersdk.Error
			require.Error(t, err)
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, http.StatusBadGateway, sdkErr.StatusCode())
		})
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
		task, err := expClient.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
			TemplateVersionID: template.ActiveVersionID,
			Input:             taskPrompt,
		})
		require.NoError(t, err)
		require.True(t, task.WorkspaceID.Valid)

		ws, err := client.Workspace(ctx, task.WorkspaceID.UUID)
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		// Then: We expect a workspace to have been created.
		assert.NotEmpty(t, task.Name)
		assert.Equal(t, template.ID, task.TemplateID)

		// And: We expect it to have the "AI Prompt" parameter correctly set.
		parameters, err := client.WorkspaceBuildParameters(ctx, ws.LatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, parameters, 1)
		assert.Equal(t, codersdk.AITaskPromptParameterName, parameters[0].Name)
		assert.Equal(t, taskPrompt, parameters[0].Value)
	})

	t.Run("CustomNames", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name               string
			taskName           string
			expectFallbackName bool
			expectError        string
		}{
			{
				name:     "ValidName",
				taskName: "a-valid-task-name",
			},
			{
				name:        "NotValidName",
				taskName:    "this is not a valid task name",
				expectError: "Unable to create a Task with the provided name.",
			},
			{
				name:               "NoNameProvided",
				taskName:           "",
				expectFallbackName: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				var (
					ctx       = testutil.Context(t, testutil.WaitShort)
					client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
					expClient = codersdk.NewExperimentalClient(client)
					user      = coderdtest.CreateFirstUser(t, client)
					version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
						Parse:          echo.ParseComplete,
						ProvisionApply: echo.ApplyComplete,
						ProvisionPlan: []*proto.Response{
							{Type: &proto.Response_Plan{Plan: &proto.PlanComplete{
								Parameters: []*proto.RichParameter{{Name: "AI Prompt", Type: "string"}},
								HasAiTasks: true,
							}}},
						},
					})
					template = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
				)

				coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

				// When: We attempt to create a Task.
				task, err := expClient.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
					TemplateVersionID: template.ActiveVersionID,
					Input:             "Some prompt",
					Name:              tt.taskName,
				})
				if tt.expectError == "" {
					require.NoError(t, err)
					require.True(t, task.WorkspaceID.Valid)

					// Then: We expect the correct name to have been picked.
					err = codersdk.NameValid(task.Name)
					require.NoError(t, err, "Generated task name should be valid")

					require.NotEmpty(t, task.Name)
					if !tt.expectFallbackName {
						require.Equal(t, tt.taskName, task.Name)
					}
				} else {
					require.ErrorContains(t, err, tt.expectError)
				}
			})
		}
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
			Input:             taskPrompt,
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
			Input:             taskPrompt,
		})

		// Then: We expect it to fail.
		var sdkErr *codersdk.Error
		require.Error(t, err)
		require.ErrorAsf(t, err, &sdkErr, "error should be of type *codersdk.Error")
		assert.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}
