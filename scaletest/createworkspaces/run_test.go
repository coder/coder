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

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)

		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.ProvisionComplete,
			ProvisionApply: []*proto.Provision_Response{
				{
					Type: &proto.Provision_Response_Log{
						Log: &proto.Log{
							Level:  proto.LogLevel_INFO,
							Output: "hello from logs",
						},
					},
				},
				{
					Type: &proto.Provision_Response_Complete{
						Complete: &proto.Provision_Complete{
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

		version = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		closer := goEventuallyStartFakeAgent(ctx, t, client, authToken)
		t.Cleanup(closer)

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
					TemplateID: template.ID,
				},
			},
			ReconnectingPTY: &reconnectingpty.Config{
				Init: codersdk.WorkspaceAgentReconnectingPTYInit{
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

		// Ensure a user and workspace were created.
		users, err := client.Users(ctx, codersdk.UsersRequest{})
		require.NoError(t, err)
		require.Len(t, users.Users, 2) // 1 user already exists
		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)

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

		err = runner.Cleanup(ctx, "1")
		require.NoError(t, err)

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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.ProvisionComplete,
			ProvisionApply: []*proto.Provision_Response{
				{
					Type: &proto.Provision_Response_Log{Log: &proto.Log{}},
				},
			},
		})

		version = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
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
					TemplateID: template.ID,
				},
			},
		})

		cancelCtx, cancelFunc := context.WithCancel(ctx)
		done := make(chan struct{})
		logs := bytes.NewBuffer(nil)
		go func() {
			err := runner.Run(cancelCtx, "1", logs)
			logsStr := logs.String()
			t.Log("Runner logs:\n\n" + logsStr)
			require.ErrorIs(t, err, context.Canceled)
			close(done)
		}()

		require.Eventually(t, func() bool {
			workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
			if err != nil {
				return false
			}

			return len(workspaces.Workspaces) > 0
		}, testutil.WaitShort, testutil.IntervalFast)

		cancelFunc()
		<-done

		// When we run the cleanup, it should be canceled
		cancelCtx, cancelFunc = context.WithCancel(ctx)
		done = make(chan struct{})
		go func() {
			// This will return an error as the "delete" operation will never complete.
			_ = runner.Cleanup(cancelCtx, "1")
			close(done)
		}()

		// Ensure the job has been marked as deleted
		require.Eventually(t, func() bool {
			workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
			if err != nil {
				return false
			}

			if len(workspaces.Workspaces) == 0 {
				return false
			}

			// There should be two builds
			builds, err := client.WorkspaceBuilds(ctx, codersdk.WorkspaceBuildsRequest{
				WorkspaceID: workspaces.Workspaces[0].ID,
			})
			if err != nil {
				return false
			}
			for _, build := range builds {
				// One of the builds should be for creating the workspace,
				if build.Transition != codersdk.WorkspaceTransitionStart {
					continue
				}

				// And it should be either canceled or canceling
				if build.Job.Status == codersdk.ProvisionerJobCanceled || build.Job.Status == codersdk.ProvisionerJobCanceling {
					return true
				}
			}
			return false
		}, testutil.WaitShort, testutil.IntervalFast)
		cancelFunc()
		<-done
	})

	t.Run("NoCleanup", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)

		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.ProvisionComplete,
			ProvisionApply: []*proto.Provision_Response{
				{
					Type: &proto.Provision_Response_Log{
						Log: &proto.Log{
							Level:  proto.LogLevel_INFO,
							Output: "hello from logs",
						},
					},
				},
				{
					Type: &proto.Provision_Response_Complete{
						Complete: &proto.Provision_Complete{
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

		version = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		closer := goEventuallyStartFakeAgent(ctx, t, client, authToken)
		t.Cleanup(closer)

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
					TemplateID: template.ID,
				},
			},
			ReconnectingPTY: &reconnectingpty.Config{
				Init: codersdk.WorkspaceAgentReconnectingPTYInit{
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

		// Ensure a user and workspace were created.
		users, err := client.Users(ctx, codersdk.UsersRequest{})
		require.NoError(t, err)
		require.Len(t, users.Users, 2) // 1 user already exists
		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 1)

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

		err = runner.Cleanup(ctx, "1")
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			Logger:                   &logger,
		})
		user := coderdtest.CreateFirstUser(t, client)

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.ProvisionComplete,
			ProvisionApply: []*proto.Provision_Response{
				{
					Type: &proto.Provision_Response_Complete{
						Complete: &proto.Provision_Complete{
							Error: "test error",
						},
					},
				},
			},
		})

		version = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
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
					TemplateID: template.ID,
				},
			},
		})

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
func goEventuallyStartFakeAgent(ctx context.Context, t *testing.T, client *codersdk.Client, agentToken string) func() {
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

			time.Sleep(100 * time.Millisecond)
		}

		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(agentToken)
		agentCloser := agent.New(agent.Options{
			Client: agentClient,
			Logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
				Named("agent").Leveled(slog.LevelWarn),
		})
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
		ch <- agentCloser
	}()
	closeFunc := func() {
		if closer, ok := <-ch; ok {
			_ = closer.Close()
		}
	}
	return closeFunc
}
