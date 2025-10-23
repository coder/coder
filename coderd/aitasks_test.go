package coderd_test

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentapisdk "github.com/coder/agentapi-sdk-go"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
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
													Slug:        "task-app",
													DisplayName: "Task App",
													Url:         opt.appURL,
												},
											},
										},
									},
								},
							},
							AiTasks: []*proto.AITask{
								{
									AppId: taskAppID.String(),
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

		t.Skip("TODO(mafredri): Remove, fixed down-stack!")

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
		assert.NotEmpty(t, got.WorkspaceStatus, "task status should not be empty")
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()

		var (
			client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			ctx        = testutil.Context(t, testutil.WaitLong)
			user       = coderdtest.CreateFirstUser(t, client)
			template   = createAITemplate(t, client, user)
			wantPrompt = "review my code"
			exp        = codersdk.NewExperimentalClient(client)
		)

		task, err := exp.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
			TemplateVersionID: template.ActiveVersionID,
			Input:             wantPrompt,
		})
		require.NoError(t, err)
		require.True(t, task.WorkspaceID.Valid)

		// Get the workspace and wait for it to be ready.
		ws, err := client.Workspace(ctx, task.WorkspaceID.UUID)
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)
		ws = coderdtest.MustWorkspace(t, client, task.WorkspaceID.UUID)
		// Assert invariant: the workspace has exactly one resource with one agent with one app.
		require.Len(t, ws.LatestBuild.Resources, 1)
		require.Len(t, ws.LatestBuild.Resources[0].Agents, 1)
		agentID := ws.LatestBuild.Resources[0].Agents[0].ID
		taskAppID := ws.LatestBuild.Resources[0].Agents[0].Apps[0].ID

		// Insert an app status for the workspace
		_, err = db.InsertWorkspaceAppStatus(dbauthz.AsSystemRestricted(ctx), database.InsertWorkspaceAppStatusParams{
			ID:          uuid.New(),
			WorkspaceID: task.WorkspaceID.UUID,
			CreatedAt:   dbtime.Now(),
			AgentID:     agentID,
			AppID:       taskAppID,
			State:       database.WorkspaceAppStatusStateComplete,
			Message:     "all done",
		})
		require.NoError(t, err)

		// Fetch the task by ID via experimental API and verify fields.
		updated, err := exp.TaskByID(ctx, task.ID)
		require.NoError(t, err)

		assert.Equal(t, task.ID, updated.ID, "task ID should match")
		assert.Equal(t, task.Name, updated.Name, "task name should match")
		assert.Equal(t, wantPrompt, updated.InitialPrompt, "task prompt should match the AI Prompt parameter")
		assert.Equal(t, task.WorkspaceID.UUID, updated.WorkspaceID.UUID, "workspace id should match")
		assert.Equal(t, ws.LatestBuild.BuildNumber, updated.WorkspaceBuildNumber, "workspace build number should match")
		assert.Equal(t, agentID, updated.WorkspaceAgentID.UUID, "workspace agent id should match")
		assert.Equal(t, taskAppID, updated.WorkspaceAppID.UUID, "workspace app id should match")
		assert.NotEmpty(t, updated.WorkspaceStatus, "task status should not be empty")

		// Stop the workspace
		coderdtest.MustTransitionWorkspace(t, client, task.WorkspaceID.UUID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

		// Verify that the previous status still remains
		updated, err = exp.TaskByID(ctx, task.ID)
		require.NoError(t, err)
		assert.NotNil(t, updated.CurrentState, "current state should not be nil")
		assert.Equal(t, "all done", updated.CurrentState.Message)
		assert.Equal(t, codersdk.TaskStateComplete, updated.CurrentState.State)

		// Start the workspace again
		coderdtest.MustTransitionWorkspace(t, client, task.WorkspaceID.UUID, codersdk.WorkspaceTransitionStop, codersdk.WorkspaceTransitionStart)

		// Verify that the status from the previous build is no longer present
		updated, err = exp.TaskByID(ctx, task.ID)
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
			require.True(t, task.WorkspaceID.Valid, "task should have a workspace ID")
			ws, err := client.Workspace(ctx, task.WorkspaceID.UUID)
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

			err = exp.DeleteTask(ctx, "me", task.ID)
			require.NoError(t, err, "delete task request should be accepted")

			// Poll until the workspace is deleted.
			for {
				dws, derr := client.DeletedWorkspace(ctx, task.WorkspaceID.UUID)
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
			require.True(t, task.WorkspaceID.Valid, "task should have a workspace ID")
			ws, err := client.Workspace(ctx, task.WorkspaceID.UUID)
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

			statusResponse := agentapisdk.StatusStable

			// Start a fake AgentAPI that accepts GET /status and POST /message.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/status" {
					w.Header().Set("Content-Type", "application/json")
					resp := agentapisdk.GetStatusResponse{
						Status: statusResponse,
					}
					respBytes, err := json.Marshal(resp)
					assert.NoError(t, err)
					w.WriteHeader(http.StatusOK)
					w.Write(respBytes)
					return
				}
				if r.Method == http.MethodPost && r.URL.Path == "/message" {
					w.Header().Set("Content-Type", "application/json")

					b, _ := io.ReadAll(r.Body)
					expectedReq := agentapisdk.PostMessageParams{
						Content: "Hello, Agent!",
						Type:    agentapisdk.MessageTypeUser,
					}
					expectedBytes, _ := json.Marshal(expectedReq)
					assert.Equal(t, string(expectedBytes), string(b), "expected message content")

					resp := agentapisdk.PostMessageResponse{Ok: true}
					respBytes, err := json.Marshal(resp)
					assert.NoError(t, err)
					w.WriteHeader(http.StatusOK)
					w.Write(respBytes)
					return
				}
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer srv.Close()

			// Create an AI-capable template whose sidebar app points to our fake AgentAPI.
			var (
				client, db     = coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
				ctx            = testutil.Context(t, testutil.WaitLong)
				owner          = coderdtest.CreateFirstUser(t, client)
				userClient, _  = coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
				agentAuthToken = uuid.NewString()
				template       = createAITemplate(t, client, owner, withAgentToken(agentAuthToken), withSidebarURL(srv.URL))
				exp            = codersdk.NewExperimentalClient(userClient)
			)

			task, err := exp.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
				TemplateVersionID: template.ActiveVersionID,
				Input:             "send me food",
			})
			require.NoError(t, err)
			require.True(t, task.WorkspaceID.Valid)

			// Get the workspace and wait for it to be ready.
			ws, err := userClient.Workspace(ctx, task.WorkspaceID.UUID)
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, ws.LatestBuild.ID)

			// Fetch the task by ID via experimental API and verify fields.
			task, err = exp.TaskByID(ctx, task.ID)
			require.NoError(t, err)
			require.NotZero(t, task.WorkspaceBuildNumber)
			require.True(t, task.WorkspaceAgentID.Valid)
			require.True(t, task.WorkspaceAppID.Valid)

			// Insert an app status for the workspace
			_, err = db.InsertWorkspaceAppStatus(dbauthz.AsSystemRestricted(ctx), database.InsertWorkspaceAppStatusParams{
				ID:          uuid.New(),
				WorkspaceID: task.WorkspaceID.UUID,
				CreatedAt:   dbtime.Now(),
				AgentID:     task.WorkspaceAgentID.UUID,
				AppID:       task.WorkspaceAppID.UUID,
				State:       database.WorkspaceAppStatusStateComplete,
				Message:     "all done",
			})
			require.NoError(t, err)

			// Start a fake agent so the workspace agent is connected before sending the message.
			agentClient := agentsdk.New(userClient.URL, agentsdk.WithFixedToken(agentAuthToken))
			_ = agenttest.New(t, userClient.URL, agentAuthToken, func(o *agent.Options) {
				o.Client = agentClient
			})
			coderdtest.NewWorkspaceAgentWaiter(t, userClient, ws.ID).WaitFor(coderdtest.AgentsReady)

			// Fetch the task by ID via experimental API and verify fields.
			task, err = exp.TaskByID(ctx, task.ID)
			require.NoError(t, err)

			// Make the sidebar app unhealthy initially.
			err = db.UpdateWorkspaceAppHealthByID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAppHealthByIDParams{
				ID:     task.WorkspaceAppID.UUID,
				Health: database.WorkspaceAppHealthUnhealthy,
			})
			require.NoError(t, err)

			err = exp.TaskSend(ctx, "me", task.ID, codersdk.TaskSendRequest{
				Input: "Hello, Agent!",
			})
			require.Error(t, err, "wanted error due to unhealthy sidebar app")

			// Make the sidebar app healthy.
			err = db.UpdateWorkspaceAppHealthByID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAppHealthByIDParams{
				ID:     task.WorkspaceAppID.UUID,
				Health: database.WorkspaceAppHealthHealthy,
			})
			require.NoError(t, err)

			statusResponse = agentapisdk.AgentStatus("bad")

			err = exp.TaskSend(ctx, "me", task.ID, codersdk.TaskSendRequest{
				Input: "Hello, Agent!",
			})
			require.Error(t, err, "wanted error due to bad status")

			statusResponse = agentapisdk.StatusStable

			//nolint:tparallel // Not intended to run in parallel.
			t.Run("SendOK", func(t *testing.T) {
				err = exp.TaskSend(ctx, "me", task.ID, codersdk.TaskSendRequest{
					Input: "Hello, Agent!",
				})
				require.NoError(t, err, "wanted no error due to healthy sidebar app and stable status")
			})

			//nolint:tparallel // Not intended to run in parallel.
			t.Run("MissingContent", func(t *testing.T) {
				err = exp.TaskSend(ctx, "me", task.ID, codersdk.TaskSendRequest{
					Input: "",
				})
				require.Error(t, err, "wanted error due to missing content")

				var sdkErr *codersdk.Error
				require.ErrorAs(t, err, &sdkErr)
				require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
			})
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
	})

	t.Run("Logs", func(t *testing.T) {
		t.Parallel()

		messageResponseData := agentapisdk.GetMessagesResponse{
			Messages: []agentapisdk.Message{
				{
					Id:      0,
					Content: "Welcome, user!",
					Role:    agentapisdk.RoleAgent,
					Time:    time.Date(2025, 9, 25, 10, 42, 48, 751774125, time.UTC),
				},
				{
					Id:      1,
					Content: "Hello, agent!",
					Role:    agentapisdk.RoleUser,
					Time:    time.Date(2025, 9, 25, 10, 46, 42, 880996296, time.UTC),
				},
				{
					Id:      2,
					Content: "What would you like to work on today?",
					Role:    agentapisdk.RoleAgent,
					Time:    time.Date(2025, 9, 25, 10, 46, 50, 747761102, time.UTC),
				},
			},
		}
		messageResponseBytes, err := json.Marshal(messageResponseData)
		require.NoError(t, err)
		messageResponse := string(messageResponseBytes)

		var shouldReturnError bool

		// Fake AgentAPI that returns a couple of messages or an error.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shouldReturnError {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, "boom")
				return
			}
			if r.Method == http.MethodGet && r.URL.Path == "/messages" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				io.WriteString(w, messageResponse)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		// Create an AI-capable template whose sidebar app points to our fake AgentAPI.
		var (
			client, db     = coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			ctx            = testutil.Context(t, testutil.WaitLong)
			owner          = coderdtest.CreateFirstUser(t, client)
			agentAuthToken = uuid.NewString()
			template       = createAITemplate(t, client, owner, withAgentToken(agentAuthToken), withSidebarURL(srv.URL))
			exp            = codersdk.NewExperimentalClient(client)
		)

		task, err := exp.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
			TemplateVersionID: template.ActiveVersionID,
			Input:             "show logs",
		})
		require.NoError(t, err)
		require.True(t, task.WorkspaceID.Valid)

		// Get the workspace and wait for it to be ready.
		ws, err := client.Workspace(ctx, task.WorkspaceID.UUID)
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		// Fetch the task by ID via experimental API and verify fields.
		task, err = exp.TaskByID(ctx, task.ID)
		require.NoError(t, err)
		require.NotZero(t, task.WorkspaceBuildNumber)
		require.True(t, task.WorkspaceAgentID.Valid)
		require.True(t, task.WorkspaceAppID.Valid)

		// Insert an app status for the workspace
		_, err = db.InsertWorkspaceAppStatus(dbauthz.AsSystemRestricted(ctx), database.InsertWorkspaceAppStatusParams{
			ID:          uuid.New(),
			WorkspaceID: task.WorkspaceID.UUID,
			CreatedAt:   dbtime.Now(),
			AgentID:     task.WorkspaceAgentID.UUID,
			AppID:       task.WorkspaceAppID.UUID,
			State:       database.WorkspaceAppStatusStateComplete,
			Message:     "all done",
		})
		require.NoError(t, err)

		// Start a fake agent so the workspace agent is connected before fetching logs.
		agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(agentAuthToken))
		_ = agenttest.New(t, client.URL, agentAuthToken, func(o *agent.Options) {
			o.Client = agentClient
		})
		coderdtest.NewWorkspaceAgentWaiter(t, client, ws.ID).WaitFor(coderdtest.AgentsReady)

		// Fetch the task by ID via experimental API and verify fields.
		task, err = exp.TaskByID(ctx, task.ID)
		require.NoError(t, err)

		//nolint:tparallel // Not intended to run in parallel.
		t.Run("OK", func(t *testing.T) {
			// Fetch logs.
			resp, err := exp.TaskLogs(ctx, "me", task.ID)
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

		//nolint:tparallel // Not intended to run in parallel.
		t.Run("UpstreamError", func(t *testing.T) {
			shouldReturnError = true
			t.Cleanup(func() { shouldReturnError = false })
			_, err := exp.TaskLogs(ctx, "me", task.ID)

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
					Parameters: []*proto.RichParameter{{Name: codersdk.AITaskPromptParameterName, Type: "string"}},
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
								Parameters: []*proto.RichParameter{{Name: codersdk.AITaskPromptParameterName, Type: "string"}},
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
		assert.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("TaskTableCreatedAndLinked", func(t *testing.T) {
		t.Parallel()

		var (
			ctx        = testutil.Context(t, testutil.WaitShort)
			taskPrompt = "Create a REST API"
		)

		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)

		// Create a template with AI task support to test the new task data model.
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionPlan: []*proto.Response{
				{Type: &proto.Response_Plan{Plan: &proto.PlanComplete{
					Parameters: []*proto.RichParameter{{Name: codersdk.AITaskPromptParameterName, Type: "string"}},
					HasAiTasks: true,
				}}},
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		expClient := codersdk.NewExperimentalClient(client)

		task, err := expClient.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
			TemplateVersionID: template.ActiveVersionID,
			Input:             taskPrompt,
		})
		require.NoError(t, err)
		require.True(t, task.WorkspaceID.Valid)

		ws, err := client.Workspace(ctx, task.WorkspaceID.UUID)
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		// Verify that the task was created in the tasks table with the correct
		// fields. This ensures the data model properly separates task records
		// from workspace records.
		dbCtx := dbauthz.AsSystemRestricted(ctx)
		dbTask, err := db.GetTaskByID(dbCtx, task.ID)
		require.NoError(t, err)
		assert.Equal(t, user.OrganizationID, dbTask.OrganizationID)
		assert.Equal(t, user.UserID, dbTask.OwnerID)
		assert.Equal(t, task.Name, dbTask.Name)
		assert.True(t, dbTask.WorkspaceID.Valid)
		assert.Equal(t, ws.ID, dbTask.WorkspaceID.UUID)
		assert.Equal(t, version.ID, dbTask.TemplateVersionID)
		assert.Equal(t, taskPrompt, dbTask.Prompt)
		assert.False(t, dbTask.DeletedAt.Valid)

		// Verify the bidirectional relationship works by looking up the task
		// via workspace ID.
		dbTaskByWs, err := db.GetTaskByWorkspaceID(dbCtx, ws.ID)
		require.NoError(t, err)
		assert.Equal(t, dbTask.ID, dbTaskByWs.ID)
	})

	t.Run("TaskWithCustomName", func(t *testing.T) {
		t.Parallel()

		var (
			ctx        = testutil.Context(t, testutil.WaitShort)
			taskPrompt = "Build a dashboard"
			taskName   = "my-custom-task"
		)

		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionPlan: []*proto.Response{
				{Type: &proto.Response_Plan{Plan: &proto.PlanComplete{
					Parameters: []*proto.RichParameter{{Name: codersdk.AITaskPromptParameterName, Type: "string"}},
					HasAiTasks: true,
				}}},
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		expClient := codersdk.NewExperimentalClient(client)

		task, err := expClient.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
			TemplateVersionID: template.ActiveVersionID,
			Input:             taskPrompt,
			Name:              taskName,
		})
		require.NoError(t, err)
		require.Equal(t, taskName, task.Name)

		// Verify the custom name is preserved in the database record.
		dbCtx := dbauthz.AsSystemRestricted(ctx)
		dbTask, err := db.GetTaskByID(dbCtx, task.ID)
		require.NoError(t, err)
		assert.Equal(t, taskName, dbTask.Name)
	})

	t.Run("MultipleTasksForSameUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionPlan: []*proto.Response{
				{Type: &proto.Response_Plan{Plan: &proto.PlanComplete{
					Parameters: []*proto.RichParameter{{Name: codersdk.AITaskPromptParameterName, Type: "string"}},
					HasAiTasks: true,
				}}},
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		expClient := codersdk.NewExperimentalClient(client)

		task1, err := expClient.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
			TemplateVersionID: template.ActiveVersionID,
			Input:             "First task",
			Name:              "task-1",
		})
		require.NoError(t, err)

		task2, err := expClient.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
			TemplateVersionID: template.ActiveVersionID,
			Input:             "Second task",
			Name:              "task-2",
		})
		require.NoError(t, err)

		// Verify both tasks are stored independently and can be listed together.
		dbCtx := dbauthz.AsSystemRestricted(ctx)
		tasks, err := db.ListTasks(dbCtx, database.ListTasksParams{
			OwnerID:        user.UserID,
			OrganizationID: uuid.Nil,
		})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(tasks), 2)

		taskIDs := make(map[uuid.UUID]bool)
		for _, task := range tasks {
			taskIDs[task.ID] = true
		}
		assert.True(t, taskIDs[task1.ID], "task1 should be in the list")
		assert.True(t, taskIDs[task2.ID], "task2 should be in the list")
	})

	t.Run("TaskLinkedToCorrectTemplateVersion", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)

		version1 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionPlan: []*proto.Response{
				{Type: &proto.Response_Plan{Plan: &proto.PlanComplete{
					Parameters: []*proto.RichParameter{{Name: codersdk.AITaskPromptParameterName, Type: "string"}},
					HasAiTasks: true,
				}}},
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version1.ID)

		version2 := coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionPlan: []*proto.Response{
				{Type: &proto.Response_Plan{Plan: &proto.PlanComplete{
					Parameters: []*proto.RichParameter{{Name: codersdk.AITaskPromptParameterName, Type: "string"}},
					HasAiTasks: true,
				}}},
			},
		}, template.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version2.ID)

		expClient := codersdk.NewExperimentalClient(client)

		// Create a task using version 2 to verify the template_version_id is
		// stored correctly.
		task, err := expClient.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
			TemplateVersionID: version2.ID,
			Input:             "Use version 2",
		})
		require.NoError(t, err)

		// Verify the task references the correct template version, not just the
		// active one.
		dbCtx := dbauthz.AsSystemRestricted(ctx)
		dbTask, err := db.GetTaskByID(dbCtx, task.ID)
		require.NoError(t, err)
		assert.Equal(t, version2.ID, dbTask.TemplateVersionID, "task should be linked to version 2")
	})
}

func TestTasksNotification(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                 string
		latestAppStatuses    []codersdk.WorkspaceAppStatusState
		newAppStatus         codersdk.WorkspaceAppStatusState
		isAITask             bool
		isNotificationSent   bool
		notificationTemplate uuid.UUID
		taskPrompt           string
	}{
		// Should not send a notification when the agent app is not an AI task.
		{
			name:               "NoAITask",
			latestAppStatuses:  nil,
			newAppStatus:       codersdk.WorkspaceAppStatusStateWorking,
			isAITask:           false,
			isNotificationSent: false,
			taskPrompt:         "NoAITask",
		},
		// Should not send a notification when the new app status is neither 'Working' nor 'Idle'.
		{
			name:               "NonNotifiedState",
			latestAppStatuses:  nil,
			newAppStatus:       codersdk.WorkspaceAppStatusStateComplete,
			isAITask:           true,
			isNotificationSent: false,
			taskPrompt:         "NonNotifiedState",
		},
		// Should not send a notification when the new app status equals the latest status (Working).
		{
			name:               "NonNotifiedTransition",
			latestAppStatuses:  []codersdk.WorkspaceAppStatusState{codersdk.WorkspaceAppStatusStateWorking},
			newAppStatus:       codersdk.WorkspaceAppStatusStateWorking,
			isAITask:           true,
			isNotificationSent: false,
			taskPrompt:         "NonNotifiedTransition",
		},
		// Should NOT send TemplateTaskWorking when the AI task's FIRST status is 'Working' (obvious state).
		{
			name:                 "TemplateTaskWorking",
			latestAppStatuses:    nil,
			newAppStatus:         codersdk.WorkspaceAppStatusStateWorking,
			isAITask:             true,
			isNotificationSent:   false,
			notificationTemplate: notifications.TemplateTaskWorking,
			taskPrompt:           "TemplateTaskWorking",
		},
		// Should send TemplateTaskIdle when the AI task's FIRST status is 'Idle' (task completed immediately).
		{
			name:                 "InitialTemplateTaskIdle",
			latestAppStatuses:    nil,
			newAppStatus:         codersdk.WorkspaceAppStatusStateIdle,
			isAITask:             true,
			isNotificationSent:   true,
			notificationTemplate: notifications.TemplateTaskIdle,
			taskPrompt:           "InitialTemplateTaskIdle",
		},
		// Should send TemplateTaskWorking when the AI task transitions to 'Working' from 'Idle'.
		{
			name: "TemplateTaskWorkingFromIdle",
			latestAppStatuses: []codersdk.WorkspaceAppStatusState{
				codersdk.WorkspaceAppStatusStateWorking,
				codersdk.WorkspaceAppStatusStateIdle,
			}, // latest
			newAppStatus:         codersdk.WorkspaceAppStatusStateWorking,
			isAITask:             true,
			isNotificationSent:   true,
			notificationTemplate: notifications.TemplateTaskWorking,
			taskPrompt:           "TemplateTaskWorkingFromIdle",
		},
		// Should send TemplateTaskIdle when the AI task transitions to 'Idle'.
		{
			name:                 "TemplateTaskIdle",
			latestAppStatuses:    []codersdk.WorkspaceAppStatusState{codersdk.WorkspaceAppStatusStateWorking},
			newAppStatus:         codersdk.WorkspaceAppStatusStateIdle,
			isAITask:             true,
			isNotificationSent:   true,
			notificationTemplate: notifications.TemplateTaskIdle,
			taskPrompt:           "TemplateTaskIdle",
		},
		// Long task prompts should be truncated to 160 characters.
		{
			name:                 "LongTaskPrompt",
			latestAppStatuses:    []codersdk.WorkspaceAppStatusState{codersdk.WorkspaceAppStatusStateWorking},
			newAppStatus:         codersdk.WorkspaceAppStatusStateIdle,
			isAITask:             true,
			isNotificationSent:   true,
			notificationTemplate: notifications.TemplateTaskIdle,
			taskPrompt:           "This is a very long task prompt that should be truncated to 160 characters. Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.",
		},
		// Should send TemplateTaskCompleted when the AI task transitions to 'Complete'.
		{
			name:                 "TemplateTaskCompleted",
			latestAppStatuses:    []codersdk.WorkspaceAppStatusState{codersdk.WorkspaceAppStatusStateWorking},
			newAppStatus:         codersdk.WorkspaceAppStatusStateComplete,
			isAITask:             true,
			isNotificationSent:   true,
			notificationTemplate: notifications.TemplateTaskCompleted,
			taskPrompt:           "TemplateTaskCompleted",
		},
		// Should send TemplateTaskFailed when the AI task transitions to 'Failure'.
		{
			name:                 "TemplateTaskFailed",
			latestAppStatuses:    []codersdk.WorkspaceAppStatusState{codersdk.WorkspaceAppStatusStateWorking},
			newAppStatus:         codersdk.WorkspaceAppStatusStateFailure,
			isAITask:             true,
			isNotificationSent:   true,
			notificationTemplate: notifications.TemplateTaskFailed,
			taskPrompt:           "TemplateTaskFailed",
		},
		// Should send TemplateTaskCompleted when the AI task transitions from 'Idle' to 'Complete'.
		{
			name:                 "TemplateTaskCompletedFromIdle",
			latestAppStatuses:    []codersdk.WorkspaceAppStatusState{codersdk.WorkspaceAppStatusStateIdle},
			newAppStatus:         codersdk.WorkspaceAppStatusStateComplete,
			isAITask:             true,
			isNotificationSent:   true,
			notificationTemplate: notifications.TemplateTaskCompleted,
			taskPrompt:           "TemplateTaskCompletedFromIdle",
		},
		// Should send TemplateTaskFailed when the AI task transitions from 'Idle' to 'Failure'.
		{
			name:                 "TemplateTaskFailedFromIdle",
			latestAppStatuses:    []codersdk.WorkspaceAppStatusState{codersdk.WorkspaceAppStatusStateIdle},
			newAppStatus:         codersdk.WorkspaceAppStatusStateFailure,
			isAITask:             true,
			isNotificationSent:   true,
			notificationTemplate: notifications.TemplateTaskFailed,
			taskPrompt:           "TemplateTaskFailedFromIdle",
		},
		// Should NOT send notification when transitioning from 'Complete' to 'Complete' (no change).
		{
			name:               "NoNotificationCompleteToComplete",
			latestAppStatuses:  []codersdk.WorkspaceAppStatusState{codersdk.WorkspaceAppStatusStateComplete},
			newAppStatus:       codersdk.WorkspaceAppStatusStateComplete,
			isAITask:           true,
			isNotificationSent: false,
			taskPrompt:         "NoNotificationCompleteToComplete",
		},
		// Should NOT send notification when transitioning from 'Failure' to 'Failure' (no change).
		{
			name:               "NoNotificationFailureToFailure",
			latestAppStatuses:  []codersdk.WorkspaceAppStatusState{codersdk.WorkspaceAppStatusStateFailure},
			newAppStatus:       codersdk.WorkspaceAppStatusStateFailure,
			isAITask:           true,
			isNotificationSent: false,
			taskPrompt:         "NoNotificationFailureToFailure",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			notifyEnq := &notificationstest.FakeEnqueuer{}
			client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
				DeploymentValues:      coderdtest.DeploymentValues(t),
				NotificationsEnqueuer: notifyEnq,
			})

			// Given: a member user
			ownerUser := coderdtest.CreateFirstUser(t, client)
			client, memberUser := coderdtest.CreateAnotherUser(t, client, ownerUser.OrganizationID)

			// Given: a workspace build with an agent containing an App
			workspaceAgentAppID := uuid.New()
			workspaceBuildID := uuid.New()
			workspaceBuildSeed := database.WorkspaceBuild{
				ID: workspaceBuildID,
			}
			if tc.isAITask {
				workspaceBuildSeed = database.WorkspaceBuild{
					ID: workspaceBuildID,
					// AI Task configuration
					HasAITask:          sql.NullBool{Bool: true, Valid: true},
					AITaskSidebarAppID: uuid.NullUUID{UUID: workspaceAgentAppID, Valid: true},
				}
			}
			workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OrganizationID: ownerUser.OrganizationID,
				OwnerID:        memberUser.ID,
			}).Seed(workspaceBuildSeed).Params(database.WorkspaceBuildParameter{
				WorkspaceBuildID: workspaceBuildID,
				Name:             codersdk.AITaskPromptParameterName,
				Value:            tc.taskPrompt,
			}).WithAgent(func(agent []*proto.Agent) []*proto.Agent {
				agent[0].Apps = []*proto.App{{
					Id:   workspaceAgentAppID.String(),
					Slug: "ccw",
				}}
				return agent
			}).Do()

			// Given: the workspace agent app has previous statuses
			agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(workspaceBuild.AgentToken))
			if len(tc.latestAppStatuses) > 0 {
				workspace := coderdtest.MustWorkspace(t, client, workspaceBuild.Workspace.ID)
				for _, appStatus := range tc.latestAppStatuses {
					dbgen.WorkspaceAppStatus(t, db, database.WorkspaceAppStatus{
						WorkspaceID: workspaceBuild.Workspace.ID,
						AgentID:     workspace.LatestBuild.Resources[0].Agents[0].ID,
						AppID:       workspaceAgentAppID,
						State:       database.WorkspaceAppStatusState(appStatus),
					})
				}
			}

			// When: the agent updates the app status
			err := agentClient.PatchAppStatus(ctx, agentsdk.PatchAppStatus{
				AppSlug: "ccw",
				Message: "testing",
				URI:     "https://example.com",
				State:   tc.newAppStatus,
			})
			require.NoError(t, err)

			// Then: The workspace app status transitions successfully
			workspace, err := client.Workspace(ctx, workspaceBuild.Workspace.ID)
			require.NoError(t, err)
			workspaceAgent, err := client.WorkspaceAgent(ctx, workspace.LatestBuild.Resources[0].Agents[0].ID)
			require.NoError(t, err)
			require.Len(t, workspaceAgent.Apps, 1)
			require.GreaterOrEqual(t, len(workspaceAgent.Apps[0].Statuses), 1)
			latestStatusIndex := len(workspaceAgent.Apps[0].Statuses) - 1
			require.Equal(t, tc.newAppStatus, workspaceAgent.Apps[0].Statuses[latestStatusIndex].State)

			if tc.isNotificationSent {
				// Then: A notification is sent to the workspace owner (memberUser)
				sent := notifyEnq.Sent(notificationstest.WithTemplateID(tc.notificationTemplate))
				require.Len(t, sent, 1)
				require.Equal(t, memberUser.ID, sent[0].UserID)
				require.Len(t, sent[0].Labels, 2)
				// NOTE: len(string) is the number of bytes in the string, not the number of runes.
				require.LessOrEqual(t, utf8.RuneCountInString(sent[0].Labels["task"]), 160)
				if len(tc.taskPrompt) > 160 {
					require.Contains(t, tc.taskPrompt, strings.TrimSuffix(sent[0].Labels["task"], "…"))
				} else {
					require.Equal(t, tc.taskPrompt, sent[0].Labels["task"])
				}
				require.Equal(t, workspace.Name, sent[0].Labels["workspace"])
			} else {
				// Then: No notification is sent
				sentWorking := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateTaskWorking))
				sentIdle := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateTaskIdle))
				require.Len(t, sentWorking, 0)
				require.Len(t, sentIdle, 0)
			}
		})
	}
}
