package createworkspaces_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/scaletest/agentconn"
	"github.com/coder/coder/scaletest/createworkspaces"
	"github.com/coder/coder/scaletest/reconnectingpty"
	"github.com/coder/coder/scaletest/workspacebuild"
	"github.com/coder/coder/testutil"
)

func Test_Runner(t *testing.T) {
	t.Parallel()
	t.Skip("Flake seen here: https://github.com/coder/coder/actions/runs/3436164958/jobs/5729513320")

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

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		// Since the runner creates the workspace on it's own, we have to keep
		// listing workspaces until we find it, then wait for the build to
		// finish, then start the agents.
		go func() {
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
			agentClient.SetSessionToken(authToken)
			agentCloser := agent.New(agent.Options{
				Client: agentClient,
				Logger: slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelWarn),
			})
			t.Cleanup(func() {
				_ = agentCloser.Close()
			})

			coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
		}()

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

	t.Run("FailedBuild", func(t *testing.T) {
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
					Type: &proto.Provision_Response_Complete{
						Complete: &proto.Provision_Complete{
							Error: "test error",
						},
					},
				},
			},
		})

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

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
