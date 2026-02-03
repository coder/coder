package coderd_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentapisdk "github.com/coder/agentapi-sdk-go"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

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

		// Create a template version that supports AI tasks.
		taskAppID := uuid.New()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionGraph: []*proto.Response{
				{
					Type: &proto.Response_Graph{
						Graph: &proto.GraphComplete{
							HasAiTasks: true,
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

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		template := createAITemplate(t, client, user)

		// Create a task with a specific prompt using the new data model.
		wantPrompt := "build me a web app"
		task, err := client.CreateTask(ctx, codersdk.Me, codersdk.CreateTaskRequest{
			TemplateVersionID: template.ActiveVersionID,
			Input:             wantPrompt,
		})
		require.NoError(t, err)
		require.True(t, task.WorkspaceID.Valid, "task should have a workspace ID")

		// Wait for the workspace to be built.
		workspace, err := client.Workspace(ctx, task.WorkspaceID.UUID)
		require.NoError(t, err)
		if assert.True(t, workspace.TaskID.Valid, "task id should be set on workspace") {
			assert.Equal(t, task.ID, workspace.TaskID.UUID, "workspace task id should match")
		}
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// List tasks via experimental API and verify the prompt and status mapping.
		tasks, err := client.Tasks(ctx, &codersdk.TasksFilter{Owner: codersdk.Me})
		require.NoError(t, err)

		got, ok := slice.Find(tasks, func(t codersdk.Task) bool { return t.ID == task.ID })
		require.True(t, ok, "task should be found in the list")
		assert.Equal(t, wantPrompt, got.InitialPrompt, "task prompt should match the input")
		assert.Equal(t, task.WorkspaceID.UUID, got.WorkspaceID.UUID, "workspace id should match")
		assert.Equal(t, task.WorkspaceName, got.WorkspaceName, "workspace name should match")
		// Status should be populated via the tasks_with_status view.
		assert.NotEmpty(t, got.Status, "task status should not be empty")
		assert.NotEmpty(t, got.WorkspaceStatus, "workspace status should not be empty")
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()

		var (
			client, db     = coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			ctx            = testutil.Context(t, testutil.WaitLong)
			user           = coderdtest.CreateFirstUser(t, client)
			anotherUser, _ = coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
			template       = createAITemplate(t, client, user)
			wantPrompt     = "review my code"
		)

		task, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
			TemplateVersionID: template.ActiveVersionID,
			Input:             wantPrompt,
		})
		require.NoError(t, err)
		require.True(t, task.WorkspaceID.Valid)

		// Get the workspace and wait for it to be ready.
		ws, err := client.Workspace(ctx, task.WorkspaceID.UUID)
		require.NoError(t, err)
		if assert.True(t, ws.TaskID.Valid, "task id should be set on workspace") {
			assert.Equal(t, task.ID, ws.TaskID.UUID, "workspace task id should match")
		}
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
		updated, err := client.TaskByID(ctx, task.ID)
		require.NoError(t, err)

		assert.Equal(t, task.ID, updated.ID, "task ID should match")
		assert.Equal(t, task.Name, updated.Name, "task name should match")
		assert.Equal(t, wantPrompt, updated.InitialPrompt, "task prompt should match the input")
		assert.Equal(t, task.WorkspaceID.UUID, updated.WorkspaceID.UUID, "workspace id should match")
		assert.Equal(t, task.WorkspaceName, updated.WorkspaceName, "workspace name should match")
		assert.Equal(t, ws.LatestBuild.BuildNumber, updated.WorkspaceBuildNumber, "workspace build number should match")
		assert.Equal(t, agentID, updated.WorkspaceAgentID.UUID, "workspace agent id should match")
		assert.Equal(t, taskAppID, updated.WorkspaceAppID.UUID, "workspace app id should match")
		assert.NotEmpty(t, updated.WorkspaceStatus, "task status should not be empty")

		// Fetch the task by name and verify the same result
		byName, err := client.TaskByOwnerAndName(ctx, codersdk.Me, task.Name)
		require.NoError(t, err)
		require.Equal(t, byName, updated)

		// Another member user should not be able to fetch the task
		_, err = anotherUser.TaskByID(ctx, task.ID)
		require.Error(t, err, "fetching task should fail by ID for another member user")
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
		// Also test by name
		_, err = anotherUser.TaskByOwnerAndName(ctx, task.OwnerName, task.Name)
		require.Error(t, err, "fetching task should fail by name for another member user")
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())

		// Stop the workspace
		coderdtest.MustTransitionWorkspace(t, client, task.WorkspaceID.UUID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)

		// Verify that the previous status still remains
		updated, err = client.TaskByID(ctx, task.ID)
		require.NoError(t, err)
		assert.NotNil(t, updated.CurrentState, "current state should not be nil")
		assert.Equal(t, "all done", updated.CurrentState.Message)
		assert.Equal(t, codersdk.TaskStateComplete, updated.CurrentState.State)
		previousCurrentState := updated.CurrentState

		// Start the workspace again
		coderdtest.MustTransitionWorkspace(t, client, task.WorkspaceID.UUID, codersdk.WorkspaceTransitionStop, codersdk.WorkspaceTransitionStart)

		// Verify that the status from the previous build has been cleared
		// and replaced by the agent initialization status.
		updated, err = client.TaskByID(ctx, task.ID)
		require.NoError(t, err)
		assert.NotEqual(t, previousCurrentState, updated.CurrentState)
		assert.Equal(t, codersdk.TaskStateWorking, updated.CurrentState.State)
		assert.NotEqual(t, "all done", updated.CurrentState.Message)
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			template := createAITemplate(t, client, user)

			ctx := testutil.Context(t, testutil.WaitLong)

			task, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
				TemplateVersionID: template.ActiveVersionID,
				Input:             "delete me",
			})
			require.NoError(t, err)
			require.True(t, task.WorkspaceID.Valid, "task should have a workspace ID")
			ws, err := client.Workspace(ctx, task.WorkspaceID.UUID)
			require.NoError(t, err)
			if assert.True(t, ws.TaskID.Valid, "task id should be set on workspace") {
				assert.Equal(t, task.ID, ws.TaskID.UUID, "workspace task id should match")
			}
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

			err = client.DeleteTask(ctx, "me", task.ID)
			require.NoError(t, err, "delete task request should be accepted")

			// Poll until the workspace is deleted.
			testutil.Eventually(ctx, t, func(ctx context.Context) (done bool) {
				dws, derr := client.DeletedWorkspace(ctx, task.WorkspaceID.UUID)
				if !assert.NoError(t, derr, "expected to fetch deleted workspace before deadline") {
					return false
				}
				t.Logf("workspace latest_build status: %q", dws.LatestBuild.Status)
				return dws.LatestBuild.Status == codersdk.WorkspaceStatusDeleted
			}, testutil.IntervalMedium, "workspace should be deleted before deadline")
		})

		t.Run("NotFound", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			_ = coderdtest.CreateFirstUser(t, client)

			ctx := testutil.Context(t, testutil.WaitShort)

			err := client.DeleteTask(ctx, "me", uuid.New())

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
			if assert.False(t, ws.TaskID.Valid, "task id should not be set on non-task workspace") {
				assert.Zero(t, ws.TaskID, "non-task workspace task id should be empty")
			}
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

			err := client.DeleteTask(ctx, "me", ws.ID)

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

			task, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
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

			// Attempt to delete the owner's task as a non-owner without permissions.
			err = otherClient.DeleteTask(ctx, "me", task.ID)

			var authErr *codersdk.Error
			require.Error(t, err, "expected an authorization error when deleting another user's task")
			require.ErrorAs(t, err, &authErr)
			// Accept either 403 or 404 depending on authz behavior.
			if authErr.StatusCode() != 403 && authErr.StatusCode() != 404 {
				t.Fatalf("unexpected status code: %d (expected 403 or 404)", authErr.StatusCode())
			}
		})

		t.Run("DeletedWorkspace", func(t *testing.T) {
			t.Parallel()

			client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			template := createAITemplate(t, client, user)
			ctx := testutil.Context(t, testutil.WaitLong)
			task, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
				TemplateVersionID: template.ActiveVersionID,
				Input:             "delete me",
			})
			require.NoError(t, err)
			require.True(t, task.WorkspaceID.Valid, "task should have a workspace ID")
			ws, err := client.Workspace(ctx, task.WorkspaceID.UUID)
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

			// Mark the workspace as deleted directly in the database, bypassing provisionerd.
			require.NoError(t, db.UpdateWorkspaceDeletedByID(dbauthz.AsProvisionerd(ctx), database.UpdateWorkspaceDeletedByIDParams{
				ID:      ws.ID,
				Deleted: true,
			}))
			// We should still be able to fetch the task if its workspace was deleted.
			// Provisionerdserver will attempt delete the related task when deleting a workspace.
			// This test ensures that we can still handle the case where, for some reason, the
			// task has not been marked as deleted, but the workspace has.
			task, err = client.TaskByID(ctx, task.ID)
			require.NoError(t, err, "fetching a task should still work if its related workspace is deleted")
			err = client.DeleteTask(ctx, task.OwnerID.String(), task.ID)
			require.NoError(t, err, "should be possible to delete a task with no workspace")
		})

		t.Run("DeletingTaskWorkspaceDeletesTask", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			template := createAITemplate(t, client, user)

			ctx := testutil.Context(t, testutil.WaitLong)

			task, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
				TemplateVersionID: template.ActiveVersionID,
				Input:             "delete me",
			})
			require.NoError(t, err)
			require.True(t, task.WorkspaceID.Valid, "task should have a workspace ID")
			ws, err := client.Workspace(ctx, task.WorkspaceID.UUID)
			require.NoError(t, err)
			if assert.True(t, ws.TaskID.Valid, "task id should be set on workspace") {
				assert.Equal(t, task.ID, ws.TaskID.UUID, "workspace task id should match")
			}
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

			// When; the task workspace is deleted
			coderdtest.MustTransitionWorkspace(t, client, ws.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionDelete)
			// Then: the task associated with the workspace is also deleted
			_, err = client.TaskByID(ctx, task.ID)
			require.Error(t, err, "expected an error fetching the task")
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr, "expected a codersdk.Error")
			require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
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
			)

			task, err := userClient.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
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
			task, err = client.TaskByID(ctx, task.ID)
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
			task, err = client.TaskByID(ctx, task.ID)
			require.NoError(t, err)

			// Make the sidebar app unhealthy initially.
			err = db.UpdateWorkspaceAppHealthByID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAppHealthByIDParams{
				ID:     task.WorkspaceAppID.UUID,
				Health: database.WorkspaceAppHealthUnhealthy,
			})
			require.NoError(t, err)

			err = client.TaskSend(ctx, "me", task.ID, codersdk.TaskSendRequest{
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

			err = client.TaskSend(ctx, "me", task.ID, codersdk.TaskSendRequest{
				Input: "Hello, Agent!",
			})
			require.Error(t, err, "wanted error due to bad status")

			statusResponse = agentapisdk.StatusStable

			//nolint:tparallel // Not intended to run in parallel.
			t.Run("SendOK", func(t *testing.T) {
				err = client.TaskSend(ctx, "me", task.ID, codersdk.TaskSendRequest{
					Input: "Hello, Agent!",
				})
				require.NoError(t, err, "wanted no error due to healthy sidebar app and stable status")
			})

			//nolint:tparallel // Not intended to run in parallel.
			t.Run("MissingContent", func(t *testing.T) {
				err = client.TaskSend(ctx, "me", task.ID, codersdk.TaskSendRequest{
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

			err := client.TaskSend(ctx, "me", uuid.New(), codersdk.TaskSendRequest{
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
					Time:    time.Date(2025, 9, 25, 10, 42, 48, 0, time.UTC),
				},
				{
					Id:      1,
					Content: "Hello, agent!",
					Role:    agentapisdk.RoleUser,
					Time:    time.Date(2025, 9, 25, 10, 46, 42, 0, time.UTC),
				},
				{
					Id:      2,
					Content: "What would you like to work on today?",
					Role:    agentapisdk.RoleAgent,
					Time:    time.Date(2025, 9, 25, 10, 46, 50, 0, time.UTC),
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
		)

		task, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
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
		task, err = client.TaskByIdentifier(ctx, task.ID.String())
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
		task, err = client.TaskByID(ctx, task.ID)
		require.NoError(t, err)

		//nolint:tparallel // Not intended to run in parallel.
		t.Run("OK", func(t *testing.T) {
			// Fetch logs.
			resp, err := client.TaskLogs(ctx, "me", task.ID)
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
			_, err := client.TaskLogs(ctx, "me", task.ID)

			var sdkErr *codersdk.Error
			require.Error(t, err)
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, http.StatusBadGateway, sdkErr.StatusCode())
		})
	})

	t.Run("LogsWithSnapshot", func(t *testing.T) {
		t.Parallel()

		ownerClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)

		ownerUser, err := ownerClient.User(testutil.Context(t, testutil.WaitMedium), owner.UserID.String())
		require.NoError(t, err)
		ownerSubject := coderdtest.AuthzUserSubject(ownerUser)

		// Create a regular user to test snapshot access.
		client, user := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		// Helper to create a task in the desired state.
		createTaskInState := func(ctx context.Context, t *testing.T, status database.TaskStatus) uuid.UUID {
			ctx = dbauthz.As(ctx, ownerSubject)

			builder := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OrganizationID: owner.OrganizationID,
				OwnerID:        user.ID,
			}).
				WithTask(database.TaskTable{
					OrganizationID: owner.OrganizationID,
					OwnerID:        user.ID,
				}, nil)

			switch status {
			case database.TaskStatusPending:
				builder = builder.Pending()
			case database.TaskStatusInitializing:
				builder = builder.Starting()
			case database.TaskStatusPaused:
				builder = builder.Seed(database.WorkspaceBuild{
					Transition: database.WorkspaceTransitionStop,
				})
			case database.TaskStatusError:
				// For error state, create a completed build then manipulate app health.
			default:
				require.Fail(t, "unsupported task status in test helper", "status: %s", status)
			}

			resp := builder.Do()
			taskID := resp.Task.ID

			// Post-process by manipulating agent and app state.
			if status == database.TaskStatusError {
				// First, set agent to ready state so agent_status returns 'active'.
				// This ensures the cascade reaches app_status.
				err := db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
					ID:             resp.Agents[0].ID,
					LifecycleState: database.WorkspaceAgentLifecycleStateReady,
				})
				require.NoError(t, err)

				// Then set workspace app health to unhealthy to trigger error state.
				apps, err := db.GetWorkspaceAppsByAgentID(ctx, resp.Agents[0].ID)
				require.NoError(t, err)
				require.Len(t, apps, 1, "expected exactly one app for task")

				err = db.UpdateWorkspaceAppHealthByID(ctx, database.UpdateWorkspaceAppHealthByIDParams{
					ID:     apps[0].ID,
					Health: database.WorkspaceAppHealthUnhealthy,
				})
				require.NoError(t, err)
			}

			return taskID
		}

		// Prepare snapshot data used across tests.
		snapshotMessages := []agentapisdk.Message{
			{
				Id:      0,
				Content: "First message",
				Role:    agentapisdk.RoleAgent,
				Time:    time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			},
			{
				Id:      1,
				Content: "Second message",
				Role:    agentapisdk.RoleUser,
				Time:    time.Date(2025, 1, 1, 10, 1, 0, 0, time.UTC),
			},
		}

		snapshotData := agentapisdk.GetMessagesResponse{
			Messages: snapshotMessages,
		}

		envelope := coderd.TaskLogSnapshotEnvelope{
			Format: "agentapi",
			Data:   snapshotData,
		}

		snapshotJSON, err := json.Marshal(envelope)
		require.NoError(t, err)

		snapshotTime := time.Date(2025, 1, 1, 10, 5, 0, 0, time.UTC)

		// Helper to verify snapshot logs content.
		verifySnapshotLogs := func(t *testing.T, got codersdk.TaskLogsResponse) {
			t.Helper()
			want := codersdk.TaskLogsResponse{
				Snapshot:   true,
				SnapshotAt: &snapshotTime,
				Logs: []codersdk.TaskLogEntry{
					{
						ID:      0,
						Type:    codersdk.TaskLogTypeOutput,
						Content: "First message",
						Time:    snapshotMessages[0].Time,
					},
					{
						ID:      1,
						Type:    codersdk.TaskLogTypeInput,
						Content: "Second message",
						Time:    snapshotMessages[1].Time,
					},
				},
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("got bad response (-want +got):\n%s", diff)
			}
		}

		t.Run("PendingTaskReturnsSnapshot", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			taskID := createTaskInState(ctx, t, database.TaskStatusPending)

			err := db.UpsertTaskSnapshot(dbauthz.As(ctx, ownerSubject), database.UpsertTaskSnapshotParams{
				TaskID:               taskID,
				LogSnapshot:          json.RawMessage(snapshotJSON),
				LogSnapshotCreatedAt: snapshotTime,
			})
			require.NoError(t, err, "upserting task snapshot")

			logsResp, err := client.TaskLogs(ctx, "me", taskID)
			require.NoError(t, err, "fetching task logs")
			verifySnapshotLogs(t, logsResp)
		})

		t.Run("InitializingTaskReturnsSnapshot", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			taskID := createTaskInState(ctx, t, database.TaskStatusInitializing)

			err := db.UpsertTaskSnapshot(dbauthz.As(ctx, ownerSubject), database.UpsertTaskSnapshotParams{
				TaskID:               taskID,
				LogSnapshot:          json.RawMessage(snapshotJSON),
				LogSnapshotCreatedAt: snapshotTime,
			})
			require.NoError(t, err, "upserting task snapshot")

			logsResp, err := client.TaskLogs(ctx, "me", taskID)
			require.NoError(t, err, "fetching task logs")
			verifySnapshotLogs(t, logsResp)
		})

		t.Run("PausedTaskReturnsSnapshot", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			taskID := createTaskInState(ctx, t, database.TaskStatusPaused)

			err := db.UpsertTaskSnapshot(dbauthz.As(ctx, ownerSubject), database.UpsertTaskSnapshotParams{
				TaskID:               taskID,
				LogSnapshot:          json.RawMessage(snapshotJSON),
				LogSnapshotCreatedAt: snapshotTime,
			})
			require.NoError(t, err, "upserting task snapshot")

			logsResp, err := client.TaskLogs(ctx, "me", taskID)
			require.NoError(t, err, "fetching task logs")
			verifySnapshotLogs(t, logsResp)
		})

		t.Run("NoSnapshotReturnsEmpty", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			taskID := createTaskInState(ctx, t, database.TaskStatusPending)

			logsResp, err := client.TaskLogs(ctx, "me", taskID)
			require.NoError(t, err)

			assert.True(t, logsResp.Snapshot)
			assert.Nil(t, logsResp.SnapshotAt)
			assert.Len(t, logsResp.Logs, 0)
		})

		t.Run("InvalidSnapshotFormat", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			taskID := createTaskInState(ctx, t, database.TaskStatusPending)

			invalidEnvelope := coderd.TaskLogSnapshotEnvelope{
				Format: "unknown-format",
				Data:   map[string]any{},
			}
			invalidJSON, err := json.Marshal(invalidEnvelope)
			require.NoError(t, err)

			err = db.UpsertTaskSnapshot(dbauthz.As(ctx, ownerSubject), database.UpsertTaskSnapshotParams{
				TaskID:               taskID,
				LogSnapshot:          json.RawMessage(invalidJSON),
				LogSnapshotCreatedAt: snapshotTime,
			})
			require.NoError(t, err)

			_, err = client.TaskLogs(ctx, "me", taskID)
			require.Error(t, err)

			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			assert.Equal(t, http.StatusInternalServerError, sdkErr.StatusCode())
			assert.Contains(t, sdkErr.Message, "Unsupported task snapshot format")
		})

		t.Run("MalformedSnapshotData", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			taskID := createTaskInState(ctx, t, database.TaskStatusPending)

			err := db.UpsertTaskSnapshot(dbauthz.As(ctx, ownerSubject), database.UpsertTaskSnapshotParams{
				TaskID:               taskID,
				LogSnapshot:          json.RawMessage(`{"format":"agentapi","data":"not an object"}`),
				LogSnapshotCreatedAt: snapshotTime,
			})
			require.NoError(t, err)

			_, err = client.TaskLogs(ctx, "me", taskID)
			require.Error(t, err)

			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			assert.Equal(t, http.StatusInternalServerError, sdkErr.StatusCode())
		})

		t.Run("ErrorStateReturnsError", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			taskID := createTaskInState(ctx, t, database.TaskStatusError)

			_, err := client.TaskLogs(ctx, "me", taskID)
			require.Error(t, err)

			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			assert.Equal(t, http.StatusConflict, sdkErr.StatusCode())
			assert.Contains(t, sdkErr.Message, "Cannot fetch logs for task in current state")
			assert.Contains(t, sdkErr.Detail, "error")
		})
	})

	t.Run("UpdateInput", func(t *testing.T) {
		tests := []struct {
			name               string
			disableProvisioner bool
			transition         database.WorkspaceTransition
			cancelTransition   bool
			deleteTask         bool
			taskInput          string
			wantStatus         codersdk.TaskStatus
			wantErr            string
			wantErrStatusCode  int
		}{
			{
				name: "TaskStatusInitializing",
				// We want to disable the provisioner so that the task
				// never gets provisioned (ensuring it stays in Initializing).
				disableProvisioner: true,
				taskInput:          "Valid prompt",
				wantStatus:         codersdk.TaskStatusInitializing,
				wantErr:            "Unable to update",
				wantErrStatusCode:  http.StatusConflict,
			},
			{
				name:       "TaskStatusPaused",
				transition: database.WorkspaceTransitionStop,
				taskInput:  "Valid prompt",
				wantStatus: codersdk.TaskStatusPaused,
			},
			{
				name:              "TaskStatusError",
				transition:        database.WorkspaceTransitionStart,
				cancelTransition:  true,
				taskInput:         "Valid prompt",
				wantStatus:        codersdk.TaskStatusError,
				wantErr:           "Unable to update",
				wantErrStatusCode: http.StatusConflict,
			},
			{
				name:       "EmptyPrompt",
				transition: database.WorkspaceTransitionStop,
				// We want to ensure an empty prompt is rejected.
				taskInput:         "",
				wantStatus:        codersdk.TaskStatusPaused,
				wantErr:           "Task input is required.",
				wantErrStatusCode: http.StatusBadRequest,
			},
			{
				name:              "TaskDeleted",
				transition:        database.WorkspaceTransitionStop,
				deleteTask:        true,
				taskInput:         "Valid prompt",
				wantErr:           httpapi.ResourceNotFoundResponse.Message,
				wantErrStatusCode: http.StatusNotFound,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				client, provisioner := coderdtest.NewWithProvisionerCloser(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
				user := coderdtest.CreateFirstUser(t, client)
				ctx := testutil.Context(t, testutil.WaitLong)

				template := createAITemplate(t, client, user)

				if tt.disableProvisioner {
					provisioner.Close()
				}

				// Given: We create a task
				task, err := client.CreateTask(ctx, codersdk.Me, codersdk.CreateTaskRequest{
					TemplateVersionID: template.ActiveVersionID,
					Input:             "initial prompt",
				})
				require.NoError(t, err)
				require.True(t, task.WorkspaceID.Valid, "task should have a workspace ID")

				if !tt.disableProvisioner {
					// Given: The Task is running
					workspace, err := client.Workspace(ctx, task.WorkspaceID.UUID)
					require.NoError(t, err)
					coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

					// If we're going to cancel the transition, we want to close the provisioner
					// to stop the job completing before we can cancel it.
					if tt.cancelTransition {
						provisioner.Close()
					}

					// Given: We transition the task's workspace
					build := coderdtest.CreateWorkspaceBuild(t, client, workspace, tt.transition)
					if tt.cancelTransition {
						// Given: We cancel the workspace build
						err := client.CancelWorkspaceBuild(ctx, build.ID, codersdk.CancelWorkspaceBuildParams{})
						require.NoError(t, err)

						coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

						// Then: We expect it to be canceled
						build, err = client.WorkspaceBuild(ctx, build.ID)
						require.NoError(t, err)
						require.Equal(t, codersdk.WorkspaceStatusCanceled, build.Status)
					} else {
						coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)
					}
				}

				if tt.deleteTask {
					err = client.DeleteTask(ctx, codersdk.Me, task.ID)
					require.NoError(t, err)
				} else {
					// Given: Task has expected status
					task, err = client.TaskByID(ctx, task.ID)
					require.NoError(t, err)
					require.Equal(t, tt.wantStatus, task.Status)
				}

				// When: We attempt to update the task input
				err = client.UpdateTaskInput(ctx, task.OwnerName, task.ID, codersdk.UpdateTaskInputRequest{
					Input: tt.taskInput,
				})
				if tt.wantErr != "" {
					require.ErrorContains(t, err, tt.wantErr)

					if tt.wantErrStatusCode != 0 {
						var apiErr *codersdk.Error
						require.ErrorAs(t, err, &apiErr)
						require.Equal(t, tt.wantErrStatusCode, apiErr.StatusCode())
					}

					if !tt.deleteTask {
						// Then: We expect the input to **not** be updated
						task, err = client.TaskByID(ctx, task.ID)
						require.NoError(t, err)
						require.NotEqual(t, tt.taskInput, task.InitialPrompt)
					}
				} else {
					require.NoError(t, err)

					if !tt.deleteTask {
						// Then: We expect the input to be updated
						task, err = client.TaskByID(ctx, task.ID)
						require.NoError(t, err)
						require.Equal(t, tt.taskInput, task.InitialPrompt)
					}
				}
			})
		}

		t.Run("NonExistentTask", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			ctx := testutil.Context(t, testutil.WaitShort)

			// Attempt to update prompt for non-existent task
			err := client.UpdateTaskInput(ctx, user.UserID.String(), uuid.New(), codersdk.UpdateTaskInputRequest{
				Input: "Should fail",
			})
			require.Error(t, err)
			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
		})

		t.Run("UnauthorizedUser", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			anotherUser, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
			ctx := testutil.Context(t, testutil.WaitLong)

			template := createAITemplate(t, client, user)

			// Create a task as the first user
			task, err := client.CreateTask(ctx, codersdk.Me, codersdk.CreateTaskRequest{
				TemplateVersionID: template.ActiveVersionID,
				Input:             "initial prompt",
			})
			require.NoError(t, err)
			require.True(t, task.WorkspaceID.Valid)

			// Wait for workspace to complete
			workspace, err := client.Workspace(ctx, task.WorkspaceID.UUID)
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

			// Stop the workspace
			build := coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStop)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

			// Attempt to update prompt as another user should fail with 404 Not Found
			err = anotherUser.UpdateTaskInput(ctx, task.OwnerName, task.ID, codersdk.UpdateTaskInputRequest{
				Input: "Should fail - unauthorized",
			})
			require.Error(t, err)
			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
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

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionGraph: []*proto.Response{
				{Type: &proto.Response_Graph{Graph: &proto.GraphComplete{
					HasAiTasks: true,
				}}},
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		task, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
			TemplateVersionID: template.ActiveVersionID,
			Input:             taskPrompt,
		})
		require.NoError(t, err)
		require.True(t, task.WorkspaceID.Valid)

		ws, err := client.Workspace(ctx, task.WorkspaceID.UUID)
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		assert.NotEmpty(t, task.Name)
		assert.Equal(t, template.ID, task.TemplateID)

		parameters, err := client.WorkspaceBuildParameters(ctx, ws.LatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, parameters, 0)
	})

	t.Run("CustomNames", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name                      string
			taskName                  string
			taskDisplayName           string
			expectFallbackName        bool
			expectFallbackDisplayName bool
			expectError               string
		}{
			{
				name:                      "ValidName",
				taskName:                  "a-valid-task-name",
				expectFallbackDisplayName: true,
			},
			{
				name:        "NotValidName",
				taskName:    "this is not a valid task name",
				expectError: "Unable to create a Task with the provided name.",
			},
			{
				name:               "NoNameProvided",
				taskName:           "",
				taskDisplayName:    "A valid task display name",
				expectFallbackName: true,
			},
			{
				name:               "ValidDisplayName",
				taskDisplayName:    "A valid task display name",
				expectFallbackName: true,
			},
			{
				name:            "NotValidDisplayName",
				taskDisplayName: "This is a task display name with a length greater than 64 characters.",
				expectError:     "Display name must be 64 characters or less.",
			},
			{
				name:                      "NoDisplayNameProvided",
				taskName:                  "a-valid-task-name",
				taskDisplayName:           "",
				expectFallbackDisplayName: true,
			},
			{
				name:            "ValidNameAndDisplayName",
				taskName:        "a-valid-task-name",
				taskDisplayName: "A valid task display name",
			},
			{
				name:                      "NoNameAndDisplayNameProvided",
				taskName:                  "",
				taskDisplayName:           "",
				expectFallbackName:        true,
				expectFallbackDisplayName: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				var (
					ctx     = testutil.Context(t, testutil.WaitShort)
					client  = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
					user    = coderdtest.CreateFirstUser(t, client)
					version = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
						Parse:          echo.ParseComplete,
						ProvisionApply: echo.ApplyComplete,
						ProvisionGraph: []*proto.Response{
							{Type: &proto.Response_Graph{Graph: &proto.GraphComplete{
								HasAiTasks: true,
							}}},
						},
					})
					template = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
				)

				coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

				// When: We attempt to create a Task.
				task, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
					TemplateVersionID: template.ActiveVersionID,
					Input:             "Some prompt",
					Name:              tt.taskName,
					DisplayName:       tt.taskDisplayName,
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

					// Then: We expect the correct display name to have been picked.
					require.NotEmpty(t, task.DisplayName)
					if !tt.expectFallbackDisplayName {
						require.Equal(t, tt.taskDisplayName, task.DisplayName)
					}
				} else {
					var apiErr *codersdk.Error
					require.ErrorAs(t, err, &apiErr)
					require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
					require.Equal(t, apiErr.Message, tt.expectError)
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

		// Given: A template without AI task support (no coder_ai_task resource)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		// When: We attempt to create a Task.
		_, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
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

		// When: We attempt to create a Task with an invalid template version ID.
		_, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
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
			ProvisionGraph: []*proto.Response{
				{Type: &proto.Response_Graph{Graph: &proto.GraphComplete{
					HasAiTasks: true,
				}}},
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		task, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
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
			ProvisionGraph: []*proto.Response{
				{Type: &proto.Response_Graph{Graph: &proto.GraphComplete{
					HasAiTasks: true,
				}}},
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		task, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
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
			ProvisionGraph: []*proto.Response{
				{Type: &proto.Response_Graph{Graph: &proto.GraphComplete{
					HasAiTasks: true,
				}}},
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		task1, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
			TemplateVersionID: template.ActiveVersionID,
			Input:             "First task",
			Name:              "task-1",
		})
		require.NoError(t, err)

		task2, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
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
			ProvisionGraph: []*proto.Response{
				{Type: &proto.Response_Graph{Graph: &proto.GraphComplete{
					HasAiTasks: true,
				}}},
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version1.ID)

		version2 := coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionGraph: []*proto.Response{
				{Type: &proto.Response_Graph{Graph: &proto.GraphComplete{
					HasAiTasks: true,
				}}},
			},
		}, template.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version2.ID)

		// Create a task using version 2 to verify the template_version_id is
		// stored correctly.
		task, err := client.CreateTask(ctx, "me", codersdk.CreateTaskRequest{
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
		agentLifecycle       database.WorkspaceAgentLifecycleState
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
			agentLifecycle:       database.WorkspaceAgentLifecycleStateReady,
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
			agentLifecycle:       database.WorkspaceAgentLifecycleStateReady,
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
			agentLifecycle:       database.WorkspaceAgentLifecycleStateReady,
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
			agentLifecycle:       database.WorkspaceAgentLifecycleStateReady,
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
			agentLifecycle:       database.WorkspaceAgentLifecycleStateReady,
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
			agentLifecycle:       database.WorkspaceAgentLifecycleStateReady,
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
			agentLifecycle:       database.WorkspaceAgentLifecycleStateReady,
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
			agentLifecycle:       database.WorkspaceAgentLifecycleStateReady,
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
		// Should NOT send notification when agent is in 'starting' lifecycle state (agent startup).
		{
			name:               "AgentStarting_NoNotification",
			latestAppStatuses:  nil,
			newAppStatus:       codersdk.WorkspaceAppStatusStateIdle,
			isAITask:           true,
			isNotificationSent: false,
			taskPrompt:         "AgentStarting_NoNotification",
			agentLifecycle:     database.WorkspaceAgentLifecycleStateStarting,
		},
		// Should NOT send notification when agent is in 'created' lifecycle state (agent not started).
		{
			name:               "AgentCreated_NoNotification",
			latestAppStatuses:  []codersdk.WorkspaceAppStatusState{codersdk.WorkspaceAppStatusStateWorking},
			newAppStatus:       codersdk.WorkspaceAppStatusStateIdle,
			isAITask:           true,
			isNotificationSent: false,
			taskPrompt:         "AgentCreated_NoNotification",
			agentLifecycle:     database.WorkspaceAgentLifecycleStateCreated,
		},
		// Should send notification when agent is in 'ready' lifecycle state (agent fully started).
		{
			name:                 "AgentReady_SendNotification",
			latestAppStatuses:    []codersdk.WorkspaceAppStatusState{codersdk.WorkspaceAppStatusStateWorking},
			newAppStatus:         codersdk.WorkspaceAppStatusStateIdle,
			isAITask:             true,
			isNotificationSent:   true,
			notificationTemplate: notifications.TemplateTaskIdle,
			taskPrompt:           "AgentReady_SendNotification",
			agentLifecycle:       database.WorkspaceAgentLifecycleStateReady,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			clock := quartz.NewMock(t)

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
			workspaceBuilder := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OrganizationID: ownerUser.OrganizationID,
				OwnerID:        memberUser.ID,
			}).Seed(database.WorkspaceBuild{
				ID: workspaceBuildID,
			})
			if tc.isAITask {
				workspaceBuilder = workspaceBuilder.
					WithTask(database.TaskTable{
						Prompt: tc.taskPrompt,
					}, &proto.App{
						Id:   workspaceAgentAppID.String(),
						Slug: "ccw",
					})
			} else {
				workspaceBuilder = workspaceBuilder.
					WithAgent(func(agent []*proto.Agent) []*proto.Agent {
						agent[0].Apps = []*proto.App{{
							Id:   workspaceAgentAppID.String(),
							Slug: "ccw",
						}}
						return agent
					})
			}
			workspaceBuild := workspaceBuilder.Do()

			ctx := testutil.Context(t, testutil.WaitShort)

			// Given: set the agent lifecycle state if specified
			if tc.agentLifecycle != "" {
				workspace := coderdtest.MustWorkspace(t, client, workspaceBuild.Workspace.ID)
				agentID := workspace.LatestBuild.Resources[0].Agents[0].ID

				var (
					startedAt sql.NullTime
					readyAt   sql.NullTime
				)
				if tc.agentLifecycle == database.WorkspaceAgentLifecycleStateReady {
					startedAt = sql.NullTime{Time: clock.Now(), Valid: true}
					readyAt = sql.NullTime{Time: clock.Now(), Valid: true}
				} else if tc.agentLifecycle == database.WorkspaceAgentLifecycleStateStarting {
					startedAt = sql.NullTime{Time: clock.Now(), Valid: true}
				}

				// nolint:gocritic // This is a system restricted operation for test setup.
				err := db.UpdateWorkspaceAgentLifecycleStateByID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAgentLifecycleStateByIDParams{
					ID:             agentID,
					LifecycleState: tc.agentLifecycle,
					StartedAt:      startedAt,
					ReadyAt:        readyAt,
				})
				require.NoError(t, err)
			}

			// Given: the workspace agent app has previous statuses
			agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(workspaceBuild.AgentToken))
			if len(tc.latestAppStatuses) > 0 {
				workspace := coderdtest.MustWorkspace(t, client, workspaceBuild.Workspace.ID)

				for _, appStatus := range tc.latestAppStatuses {
					clock.Advance(time.Second)

					dbgen.WorkspaceAppStatus(t, db, database.WorkspaceAppStatus{
						CreatedAt:   clock.Now(),
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
			// Statuses are ordered by created_at DESC, so the first element is the latest.
			require.Equal(t, tc.newAppStatus, workspaceAgent.Apps[0].Statuses[0].State)

			if tc.isNotificationSent {
				// Then: A notification is sent to the workspace owner (memberUser)
				sent := notifyEnq.Sent(notificationstest.WithTemplateID(tc.notificationTemplate))
				require.Len(t, sent, 1)
				require.Equal(t, memberUser.ID, sent[0].UserID)
				require.Len(t, sent[0].Labels, 2)
				require.Equal(t, workspaceBuild.Task.Name, sent[0].Labels["task"])
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

func TestPostWorkspaceAgentTaskSnapshot(t *testing.T) {
	t.Parallel()

	// Shared coderd with mock clock for all tests.
	clock := quartz.NewMock(t)
	ownerClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
		Clock: clock,
	})
	owner := coderdtest.CreateFirstUser(t, ownerClient)

	createTaskWorkspace := func(t *testing.T, agentToken string) (taskID uuid.UUID, workspaceID uuid.UUID) {
		t.Helper()
		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: owner.OrganizationID,
			OwnerID:        owner.UserID,
		}).WithTask(database.TaskTable{
			Prompt: "test prompt",
		}, &proto.App{
			Slug: "task-app",
			Url:  "http://localhost:8080",
		}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Auth = &proto.Agent_Token{Token: agentToken}
			return agents
		}).Do()
		return workspaceBuild.Task.ID, workspaceBuild.Workspace.ID
	}

	makePayload := func(t *testing.T, content string) []byte {
		t.Helper()
		data := agentapisdk.GetMessagesResponse{
			Messages: []agentapisdk.Message{
				{Id: 0, Role: "agent", Content: content, Time: time.Now()},
			},
		}
		b, err := json.Marshal(data)
		require.NoError(t, err)
		return b
	}

	makeRequest := func(t *testing.T, taskID uuid.UUID, agentToken string, payload []byte, format string) *http.Response {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitShort)

		url := ownerClient.URL.JoinPath("/api/v2/workspaceagents/me/tasks", taskID.String(), "log-snapshot").String()
		if format != "" {
			url += "?format=" + format
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set(codersdk.SessionTokenHeader, agentToken)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return res
	}

	unmarshalSnapshot := func(t *testing.T, snapshotJSON json.RawMessage) agentapisdk.GetMessagesResponse {
		t.Helper()
		// Pre-populate Data with the correct type so json.Unmarshal decodes
		// directly into it instead of creating a map[string]any.
		envelope := coderd.TaskLogSnapshotEnvelope{
			Data: &agentapisdk.GetMessagesResponse{},
		}
		err := json.Unmarshal(snapshotJSON, &envelope)
		require.NoError(t, err)
		require.Equal(t, "agentapi", envelope.Format)

		return *envelope.Data.(*agentapisdk.GetMessagesResponse)
	}

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		agentToken := uuid.NewString()
		taskID, _ := createTaskWorkspace(t, agentToken)
		ctx := testutil.Context(t, testutil.WaitShort)

		res := makeRequest(t, taskID, agentToken, makePayload(t, "test"), "agentapi")
		defer res.Body.Close()
		require.Equal(t, http.StatusNoContent, res.StatusCode)

		snapshot, err := db.GetTaskSnapshot(dbauthz.AsSystemRestricted(ctx), taskID)
		require.NoError(t, err)

		data := unmarshalSnapshot(t, snapshot.LogSnapshot)
		require.Len(t, data.Messages, 1)
		require.Equal(t, "test", data.Messages[0].Content)
	})

	//nolint:paralleltest // Not parallel, advances shared clock.
	t.Run("Overwrite", func(t *testing.T) {
		agentToken := uuid.NewString()
		taskID, _ := createTaskWorkspace(t, agentToken)
		ctx := testutil.Context(t, testutil.WaitShort)

		// First snapshot.
		res1 := makeRequest(t, taskID, agentToken, makePayload(t, "first"), "agentapi")
		res1.Body.Close()
		require.Equal(t, http.StatusNoContent, res1.StatusCode)

		snapshot1, err := db.GetTaskSnapshot(dbauthz.AsSystemRestricted(ctx), taskID)
		require.NoError(t, err)
		firstTime := snapshot1.LogSnapshotCreatedAt

		// Advance clock to ensure timestamp differs.
		clock.Advance(time.Second)

		// Second snapshot.
		res2 := makeRequest(t, taskID, agentToken, makePayload(t, "second"), "agentapi")
		res2.Body.Close()
		require.Equal(t, http.StatusNoContent, res2.StatusCode)

		snapshot2, err := db.GetTaskSnapshot(dbauthz.AsSystemRestricted(ctx), taskID)
		require.NoError(t, err)
		require.True(t, snapshot2.LogSnapshotCreatedAt.After(firstTime))

		// Verify data was overwritten.
		data := unmarshalSnapshot(t, snapshot2.LogSnapshot)
		require.Len(t, data.Messages, 1)
		require.Equal(t, "second", data.Messages[0].Content)
	})

	t.Run("MissingFormat", func(t *testing.T) {
		t.Parallel()
		agentToken := uuid.NewString()
		taskID, _ := createTaskWorkspace(t, agentToken)

		res := makeRequest(t, taskID, agentToken, makePayload(t, "test"), "")
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)

		var errResp codersdk.Response
		json.NewDecoder(res.Body).Decode(&errResp)
		require.Contains(t, errResp.Message, "Invalid query parameters")
		require.Len(t, errResp.Validations, 1)
		require.Equal(t, "format", errResp.Validations[0].Field)
		require.Contains(t, errResp.Validations[0].Detail, "required and cannot be empty")
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		t.Parallel()
		agentToken := uuid.NewString()
		taskID, _ := createTaskWorkspace(t, agentToken)

		res := makeRequest(t, taskID, agentToken, makePayload(t, "test"), "unknown")
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)

		var errResp codersdk.Response
		json.NewDecoder(res.Body).Decode(&errResp)
		require.Contains(t, errResp.Message, "Invalid format parameter")
	})

	t.Run("PayloadTooLarge", func(t *testing.T) {
		t.Parallel()
		agentToken := uuid.NewString()
		taskID, _ := createTaskWorkspace(t, agentToken)

		largeContent := strings.Repeat("x", 65*1024)
		payload := makePayload(t, largeContent)

		res := makeRequest(t, taskID, agentToken, payload, "agentapi")
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
		res.Body.Close()
	})

	t.Run("InvalidTaskID", func(t *testing.T) {
		t.Parallel()
		agentToken := uuid.NewString()
		createTaskWorkspace(t, agentToken)
		ctx := testutil.Context(t, testutil.WaitShort)

		url := ownerClient.URL.JoinPath("/api/v2/workspaceagents/me/tasks", "not-a-uuid", "log-snapshot").String() + "?format=agentapi"
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(makePayload(t, "test")))
		req.Header.Set(codersdk.SessionTokenHeader, agentToken)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)

		var errResp codersdk.Response
		json.NewDecoder(res.Body).Decode(&errResp)
		require.Contains(t, errResp.Message, "Invalid task ID format")
	})

	t.Run("TaskNotFound", func(t *testing.T) {
		t.Parallel()
		agentToken := uuid.NewString()
		createTaskWorkspace(t, agentToken)

		res := makeRequest(t, uuid.New(), agentToken, makePayload(t, "test"), "agentapi")
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("WrongWorkspace", func(t *testing.T) {
		t.Parallel()
		agent1Token := uuid.NewString()
		agent2Token := uuid.NewString()
		taskID1, _ := createTaskWorkspace(t, agent1Token)
		taskID2, _ := createTaskWorkspace(t, agent2Token)

		// Try to POST snapshot for task2 using agent1's token.
		res := makeRequest(t, taskID2, agent1Token, makePayload(t, "test"), "agentapi")
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)

		// Verify we CAN post for our own task.
		res2 := makeRequest(t, taskID1, agent1Token, makePayload(t, "test"), "agentapi")
		defer res2.Body.Close()
		require.Equal(t, http.StatusNoContent, res2.StatusCode)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()
		agentToken := uuid.NewString()
		taskID, _ := createTaskWorkspace(t, agentToken)

		res := makeRequest(t, taskID, "", makePayload(t, "test"), "agentapi")
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		t.Parallel()
		agentToken := uuid.NewString()
		taskID, _ := createTaskWorkspace(t, agentToken)

		res := makeRequest(t, taskID, agentToken, []byte("{invalid json"), "agentapi")
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)

		var errResp codersdk.Response
		json.NewDecoder(res.Body).Decode(&errResp)
		require.Contains(t, errResp.Message, "Failed to decode request payload")
	})

	t.Run("InvalidAgentAPIPayload", func(t *testing.T) {
		t.Parallel()
		agentToken := uuid.NewString()
		taskID, _ := createTaskWorkspace(t, agentToken)

		// Missing required "messages" field.
		res := makeRequest(t, taskID, agentToken, []byte(`{"truncated":false,"total_count":0}`), "agentapi")
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)

		var errResp codersdk.Response
		json.NewDecoder(res.Body).Decode(&errResp)
		require.Contains(t, errResp.Message, "Invalid agentapi payload structure")
	})

	t.Run("DeletedTask", func(t *testing.T) {
		t.Parallel()
		agentToken := uuid.NewString()
		taskID, _ := createTaskWorkspace(t, agentToken)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Delete the task.
		err := ownerClient.DeleteTask(ctx, owner.UserID.String(), taskID)
		require.NoError(t, err)

		res := makeRequest(t, taskID, agentToken, makePayload(t, "test"), "agentapi")
		defer res.Body.Close()
		// Agent token becomes invalid after task deletion.
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})
}
