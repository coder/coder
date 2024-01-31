package workspacebuild_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
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

		authToken1 := uuid.NewString()
		authToken2 := uuid.NewString()
		authToken3 := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:         echo.ParseComplete,
			ProvisionPlan: echo.PlanComplete,
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
									Name: "example1",
									Type: "aws_instance",
									Agents: []*proto.Agent{
										{
											Id:   uuid.NewString(),
											Name: "agent1",
											Auth: &proto.Agent_Token{
												Token: authToken1,
											},
											Apps: []*proto.App{},
										},
										{
											Id:   uuid.NewString(),
											Name: "agent2",
											Auth: &proto.Agent_Token{
												Token: authToken2,
											},
											Apps: []*proto.App{},
										},
									},
								},
								{
									Name: "example2",
									Type: "aws_instance",
									Agents: []*proto.Agent{
										{
											Id:   uuid.NewString(),
											Name: "agent3",
											Auth: &proto.Agent_Token{
												Token: authToken3,
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
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		// Since the runner creates the workspace on it's own, we have to keep
		// listing workspaces until we find it, then wait for the build to
		// finish, then start the agents.
		go func() {
			var workspace codersdk.Workspace
			for {
				res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
					Owner: codersdk.Me,
				})
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

			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

			// Start the three agents.
			for i, authToken := range []string{authToken1, authToken2, authToken3} {
				i := i + 1

				agentClient := agentsdk.New(client.URL)
				agentClient.SetSessionToken(authToken)
				agentCloser := agent.New(agent.Options{
					Client: agentClient,
					Logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
						Named(fmt.Sprintf("agent%d", i)).
						Leveled(slog.LevelWarn),
				})
				t.Cleanup(func() {
					_ = agentCloser.Close()
				})
			}

			coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
		}()

		runner := workspacebuild.NewRunner(client, workspacebuild.Config{
			OrganizationID: user.OrganizationID,
			UserID:         codersdk.Me,
			Request: codersdk.CreateWorkspaceRequest{
				TemplateID: template.ID,
			},
		})

		logs := bytes.NewBuffer(nil)
		err := runner.Run(ctx, "1", logs)
		logsStr := logs.String()
		t.Log("Runner logs:\n\n" + logsStr)
		require.NoError(t, err)

		// Look for strings in the logs.
		require.Contains(t, logsStr, "hello from logs")
		require.Contains(t, logsStr, `"agent1" is connected`)
		require.Contains(t, logsStr, `"agent2" is connected`)
		require.Contains(t, logsStr, `"agent3" is connected`)

		// Find the workspace.
		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Owner: codersdk.Me,
		})
		require.NoError(t, err)
		workspaces := res.Workspaces
		require.Len(t, workspaces, 1)

		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspaces[0].LatestBuild.ID)
		coderdtest.AwaitWorkspaceAgents(t, client, workspaces[0].ID)

		cleanupLogs := bytes.NewBuffer(nil)
		err = runner.Cleanup(ctx, "1", cleanupLogs)
		require.NoError(t, err)
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
			ProvisionPlan: echo.PlanComplete,
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

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		runner := workspacebuild.NewRunner(client, workspacebuild.Config{
			OrganizationID: user.OrganizationID,
			UserID:         codersdk.Me,
			Request: codersdk.CreateWorkspaceRequest{
				TemplateID: template.ID,
			},
		})

		logs := bytes.NewBuffer(nil)
		err := runner.Run(ctx, "1", logs)
		logsStr := logs.String()
		t.Log("Runner logs:\n\n" + logsStr)
		require.Error(t, err)
		require.ErrorContains(t, err, "test error")
	})

	t.Run("RetryBuild", func(t *testing.T) {
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
			ProvisionPlan: echo.PlanComplete,
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

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		runner := workspacebuild.NewRunner(client, workspacebuild.Config{
			OrganizationID: user.OrganizationID,
			UserID:         codersdk.Me,
			Request: codersdk.CreateWorkspaceRequest{
				TemplateID: template.ID,
			},
			Retry: 1,
		})

		logs := bytes.NewBuffer(nil)
		err := runner.Run(ctx, "1", logs)
		logsStr := logs.String()
		t.Log("Runner logs:\n\n" + logsStr)
		require.Error(t, err)
		require.ErrorContains(t, err, "test error")
		require.Equal(t, 1, strings.Count(logsStr, "Retrying build"))
		split := strings.Split(logsStr, "Retrying build")
		// Ensure the error is present both before and after the retry.
		for _, s := range split {
			require.Contains(t, s, "test error")
		}
	})
}
