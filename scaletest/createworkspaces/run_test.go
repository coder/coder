package createworkspaces_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/scaletest/agentconn"
	"github.com/coder/coder/v2/scaletest/createworkspaces"
	"github.com/coder/coder/v2/scaletest/reconnectingpty"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
	"github.com/coder/coder/v2/testutil"
)

func Test_Runner(t *testing.T) {
	t.Parallel()

	if testutil.RaceEnabled() {
		t.Skip("Race detector enabled, skipping time-sensitive test.")
	}

	testParameters := []*proto.RichParameter{
		{
			Name:         "foo",
			DefaultValue: "baz",
		},
	}
	testParameterValues := []codersdk.WorkspaceBuildParameter{
		{
			Name:  "foo",
			Value: "baz",
		},
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)

		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionPlan: []*proto.Response{
				{
					Type: &proto.Response_Plan{
						Plan: &proto.PlanComplete{
							Parameters: testParameters,
						},
					},
				},
			},
			ProvisionApply: []*proto.Response{
				{
					Type: &proto.Response_Log{
						Log: &proto.Log{
							Level:  proto.LogLevel_INFO,
							Output: "hello from logs",
						},
					},
				},
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
											Name: "agent",
											Auth: &proto.Agent_Token{
												Token: authToken,
											},
											Apps: []*proto.App{},
										},
									},
								},
							},
						},
					},
				},
			},
		})

		version = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx := testutil.Context(t, testutil.WaitLong)

		closerCh := goEventuallyStartFakeAgent(ctx, t, client, authToken)

		const (
			username = "scaletest-user"
			email    = "scaletest@test.coder.com"
		)
		runner := createworkspaces.NewRunner(client, createworkspaces.Config{
			User: createworkspaces.UserConfig{
				OrganizationID: user.OrganizationID,
				Username:       username,
				Email:          email,
			},
			Workspace: workspacebuild.Config{
				OrganizationID: user.OrganizationID,
				Request: codersdk.CreateWorkspaceRequest{
					TemplateID:          template.ID,
					RichParameterValues: testParameterValues,
				},
			},
			ReconnectingPTY: &reconnectingpty.Config{
				Init: workspacesdk.AgentReconnectingPTYInit{
					Height:  24,
					Width:   80,
					Command: "echo hello",
				},
				Timeout: httpapi.Duration(testutil.WaitLong),
			},
			AgentConn: &agentconn.Config{
				ConnectionMode: agentconn.ConnectionModeDerp,
				HoldDuration:   0,
			},
		})

		logs := bytes.NewBuffer(nil)
		err := runner.Run(ctx, "1", logs)
		logsStr := logs.String()
		t.Log("Runner logs:\n\n" + logsStr)
		require.NoError(t, err)

		// Wait for the workspace agent to start.
		closer := <-closerCh
		t.Cleanup(func() { _ = closer.Close() })

		// Ensure a user and workspace were created.
		users, err := client.Users(ctx, codersdk.UsersRequest{})
		require.NoError(t, err)
		require.Len(t, users.Users, 2) // 1 user already exists
		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)

		// Ensure the correct build parameters were used.
		buildParams, err := client.WorkspaceBuildParameters(ctx, workspaces.Workspaces[0].LatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, buildParams, 1)
		require.Equal(t, testParameterValues[0].Name, buildParams[0].Name)
		require.Equal(t, testParameterValues[0].Value, buildParams[0].Value)

		// Look for strings in the logs.
		require.Contains(t, logsStr, "Generating user password...")
		require.Contains(t, logsStr, "Creating user:")
		require.Contains(t, logsStr, "Org ID:   "+user.OrganizationID.String())
		require.Contains(t, logsStr, "Username: "+username)
		require.Contains(t, logsStr, "Email:    "+email)
		require.Contains(t, logsStr, "Logging in as new user...")
		require.Contains(t, logsStr, "Creating workspace...")
		require.Contains(t, logsStr, `"agent" is connected`)
		require.Contains(t, logsStr, "Opening reconnecting PTY connection to agent")
		require.Contains(t, logsStr, "Opening connection to workspace agent")

		cleanupLogs := bytes.NewBuffer(nil)
		err = runner.Cleanup(ctx, "1", cleanupLogs)
		require.NoError(t, err)
		cleanupLogsStr := cleanupLogs.String()
		require.Contains(t, cleanupLogsStr, "deleting workspace")
		require.NotContains(t, cleanupLogsStr, "canceling workspace build") // The build should have already completed.
		require.Contains(t, cleanupLogsStr, "Build succeeded!")

		// Ensure the user and workspace were deleted.
		users, err = client.Users(ctx, codersdk.UsersRequest{})
		require.NoError(t, err)
		require.Len(t, users.Users, 1) // 1 user already exists
		workspaces, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 0)
	})

	t.Run("CleanupPendingBuild", func(t *testing.T) {
		t.Parallel()

		// need to include our own logger because the provisioner (rightly) drops error logs when we shut down the
		// test with a build in progress.
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			Logger:                   &logger,
		})
		user := coderdtest.CreateFirstUser(t, client)

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionPlan: []*proto.Response{
				{
					Type: &proto.Response_Plan{
						Plan: &proto.PlanComplete{
							Parameters: testParameters,
						},
					},
				},
			},
			ProvisionApply: []*proto.Response{
				{
					Type: &proto.Response_Log{Log: &proto.Log{}}, // This provisioner job will never complete.
				},
			},
		})

		version = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(request *codersdk.CreateTemplateRequest) {
			request.AllowUserCancelWorkspaceJobs = ptr.Ref(true)
		})

		const (
			username = "scaletest-user"
			email    = "scaletest@test.coder.com"
		)
		runner := createworkspaces.NewRunner(client, createworkspaces.Config{
			User: createworkspaces.UserConfig{
				OrganizationID: user.OrganizationID,
				Username:       username,
				Email:          email,
			},
			Workspace: workspacebuild.Config{
				OrganizationID: user.OrganizationID,
				Request: codersdk.CreateWorkspaceRequest{
					TemplateID:          template.ID,
					RichParameterValues: testParameterValues,
				},
			},
		})

		runnerCtx, runnerCancel := context.WithTimeout(context.Background(), testutil.WaitLong)

		done := make(chan struct{})
		logs := bytes.NewBuffer(nil)
		go func() {
			err := runner.Run(runnerCtx, "1", logs)
			logsStr := logs.String()
			t.Log("Runner logs:\n\n" + logsStr)
			require.ErrorIs(t, err, context.Canceled)
			close(done)
		}()

		// Wait for the workspace build job to be picked up.
		checkJobStartedCtx := testutil.Context(t, testutil.WaitLong)
		jobCh := make(chan codersdk.ProvisionerJob, 1)
		require.Eventually(t, func() bool {
			workspaces, err := client.Workspaces(checkJobStartedCtx, codersdk.WorkspaceFilter{})
			if err != nil {
				return false
			}
			if len(workspaces.Workspaces) == 0 {
				return false
			}

			ws := workspaces.Workspaces[0]
			t.Logf("checking build: %s | %s | %s", ws.ID, ws.LatestBuild.Transition, ws.LatestBuild.Job.Status)
			// There should be only one build at present.
			if ws.LatestBuild.Transition != codersdk.WorkspaceTransitionStart {
				t.Errorf("expected build transition %s, got %s", codersdk.WorkspaceTransitionStart, ws.LatestBuild.Transition)
				return false
			}

			if ws.LatestBuild.Job.Status != codersdk.ProvisionerJobRunning {
				return false
			}
			jobCh <- ws.LatestBuild.Job
			return true
		}, testutil.WaitLong, testutil.IntervalSlow)

		t.Log("canceling scaletest workspace creation")
		runnerCancel()
		<-done
		t.Log("canceled scaletest workspace creation")
		// Ensure we have a job to interrogate
		runningJob := testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, jobCh)
		require.NotZero(t, runningJob.ID)

		// When we run the cleanup, it should be canceled
		cleanupLogs := bytes.NewBuffer(nil)
		// Reset ctx to avoid timeouts.
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		done = make(chan struct{})
		go func() {
			// This will return an error as the "delete" operation will never complete.
			_ = runner.Cleanup(cleanupCtx, "1", cleanupLogs)
			close(done)
		}()

		// Ensure the job has been marked as canceled
		checkJobCanceledCtx := testutil.Context(t, testutil.WaitLong)
		require.Eventually(t, func() bool {
			pj, err := client.OrganizationProvisionerJob(checkJobCanceledCtx, runningJob.OrganizationID, runningJob.ID)
			if !assert.NoError(t, err) {
				return false
			}

			t.Logf("provisioner job id:%s status:%s", pj.ID, pj.Status)

			if pj.Status != codersdk.ProvisionerJobFailed &&
				pj.Status != codersdk.ProvisionerJobCanceling &&
				pj.Status != codersdk.ProvisionerJobCanceled {
				return false
			}

			return true
		}, testutil.WaitLong, testutil.IntervalSlow)
		cleanupCancel()
		<-done
		cleanupLogsStr := cleanupLogs.String()
		require.Contains(t, cleanupLogsStr, "canceling workspace build")
	})

	t.Run("NoCleanup", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)

		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionPlan: []*proto.Response{
				{
					Type: &proto.Response_Plan{
						Plan: &proto.PlanComplete{
							Parameters: testParameters,
						},
					},
				},
			},
			ProvisionApply: []*proto.Response{
				{
					Type: &proto.Response_Log{
						Log: &proto.Log{
							Level:  proto.LogLevel_INFO,
							Output: "hello from logs",
						},
					},
				},
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
											Name: "agent",
											Auth: &proto.Agent_Token{
												Token: authToken,
											},
											Apps: []*proto.App{},
										},
									},
								},
							},
						},
					},
				},
			},
		})

		version = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx := testutil.Context(t, testutil.WaitLong)
		closeCh := goEventuallyStartFakeAgent(ctx, t, client, authToken)

		const (
			username = "scaletest-user"
			email    = "scaletest@test.coder.com"
		)
		runner := createworkspaces.NewRunner(client, createworkspaces.Config{
			NoCleanup: true,
			User: createworkspaces.UserConfig{
				OrganizationID: user.OrganizationID,
				Username:       username,
				Email:          email,
			},
			Workspace: workspacebuild.Config{
				OrganizationID: user.OrganizationID,
				Request: codersdk.CreateWorkspaceRequest{
					TemplateID:          template.ID,
					RichParameterValues: testParameterValues,
				},
			},
			ReconnectingPTY: &reconnectingpty.Config{
				Init: workspacesdk.AgentReconnectingPTYInit{
					Height:  24,
					Width:   80,
					Command: "echo hello",
				},
				Timeout: httpapi.Duration(testutil.WaitLong),
			},
			AgentConn: &agentconn.Config{
				ConnectionMode: agentconn.ConnectionModeDerp,
				HoldDuration:   0,
			},
		})

		logs := bytes.NewBuffer(nil)
		err := runner.Run(ctx, "1", logs)
		logsStr := logs.String()
		t.Log("Runner logs:\n\n" + logsStr)
		require.NoError(t, err)

		// Wait for the agent to start.
		closer := <-closeCh
		t.Cleanup(func() { _ = closer.Close() })

		// Ensure a user and workspace were created.
		users, err := client.Users(ctx, codersdk.UsersRequest{})
		require.NoError(t, err)
		require.Len(t, users.Users, 2) // 1 user already exists
		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)

		// Ensure the correct build parameters were used.
		buildParams, err := client.WorkspaceBuildParameters(ctx, workspaces.Workspaces[0].LatestBuild.ID)
		require.NoError(t, err)
		require.Len(t, buildParams, 1)
		require.Equal(t, testParameterValues[0].Name, buildParams[0].Name)
		require.Equal(t, testParameterValues[0].Value, buildParams[0].Value)

		// Look for strings in the logs.
		require.Contains(t, logsStr, "Generating user password...")
		require.Contains(t, logsStr, "Creating user:")
		require.Contains(t, logsStr, "Org ID:   "+user.OrganizationID.String())
		require.Contains(t, logsStr, "Username: "+username)
		require.Contains(t, logsStr, "Email:    "+email)
		require.Contains(t, logsStr, "Logging in as new user...")
		require.Contains(t, logsStr, "Creating workspace...")
		require.Contains(t, logsStr, `"agent" is connected`)
		require.Contains(t, logsStr, "Opening reconnecting PTY connection to agent")
		require.Contains(t, logsStr, "Opening connection to workspace agent")

		cleanupLogs := bytes.NewBuffer(nil)
		err = runner.Cleanup(ctx, "1", cleanupLogs)
		require.NoError(t, err)

		// Ensure the user and workspace were not deleted.
		users, err = client.Users(ctx, codersdk.UsersRequest{})
		require.NoError(t, err)
		require.Len(t, users.Users, 2)
		workspaces, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)
	})

	t.Run("FailedBuild", func(t *testing.T) {
		t.Parallel()

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			Logger:                   &logger,
		})
		user := coderdtest.CreateFirstUser(t, client)

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionPlan: []*proto.Response{
				{
					Type: &proto.Response_Plan{
						Plan: &proto.PlanComplete{
							Parameters: testParameters,
						},
					},
				},
			},
			ProvisionApply: []*proto.Response{
				{
					Type: &proto.Response_Apply{
						Apply: &proto.ApplyComplete{
							Error: "test error",
						},
					},
				},
			},
		})

		version = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		runner := createworkspaces.NewRunner(client, createworkspaces.Config{
			User: createworkspaces.UserConfig{
				OrganizationID: user.OrganizationID,
				Username:       "scaletest-user",
				Email:          "scaletest@test.coder.com",
			},
			Workspace: workspacebuild.Config{
				OrganizationID: user.OrganizationID,
				Request: codersdk.CreateWorkspaceRequest{
					TemplateID:          template.ID,
					RichParameterValues: testParameterValues,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)

		logs := bytes.NewBuffer(nil)
		err := runner.Run(ctx, "1", logs)
		logsStr := logs.String()
		t.Log("Runner logs:\n\n" + logsStr)
		require.Error(t, err)
		require.ErrorContains(t, err, "test error")
	})
}

// Since the runner creates the workspace on it's own, we have to keep
// listing workspaces until we find it, then wait for the build to
// finish, then start the agents. It is the caller's responsibility to
// call the returned function to stop the agents.
func goEventuallyStartFakeAgent(ctx context.Context, t *testing.T, client *codersdk.Client, agentToken string) chan io.Closer {
	t.Helper()
	ch := make(chan io.Closer, 1) // Don't block.
	go func() {
		defer close(ch)
		var workspace codersdk.Workspace
		for {
			res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
			if !assert.NoError(t, err) {
				return
			}
			workspaces := res.Workspaces

			if len(workspaces) == 1 {
				workspace = workspaces[0]
				break
			}

			time.Sleep(testutil.IntervalMedium)
		}

		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(agentToken)
		agentCloser := agent.New(agent.Options{
			Client: agentClient,
			Logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
				Named("agent").Leveled(slog.LevelWarn),
		})
		resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
		assert.GreaterOrEqual(t, len(resources), 1, "workspace %s has no resources", workspace.ID.String())
		assert.NotEmpty(t, resources[0].Agents, "workspace %s has no agents", workspace.ID.String())
		agentID := resources[0].Agents[0].ID
		t.Logf("agent %s is running for workspace %s", agentID.String(), workspace.ID.String())
		ch <- agentCloser
	}()
	return ch
}
