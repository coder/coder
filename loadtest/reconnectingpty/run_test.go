package reconnectingpty_test

import (
	"bytes"
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/loadtest/reconnectingpty"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

func Test_Runner(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("PTY is flakey on Windows")
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, agentID := setupRunnerTest(t)

		runner := reconnectingpty.NewRunner(client, reconnectingpty.Config{
			AgentID: agentID,
			Init: codersdk.ReconnectingPTYInit{
				Command: "echo 'hello world' && sleep 1",
			},
			LogOutput: true,
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		logs := bytes.NewBuffer(nil)
		err := runner.Run(ctx, "1", logs)
		logStr := logs.String()
		t.Log("Runner logs:\n\n" + logStr)
		require.NoError(t, err)

		require.Contains(t, logStr, "Output:")
		require.Contains(t, logStr, "\thello world")
	})

	t.Run("NoLogOutput", func(t *testing.T) {
		t.Parallel()

		client, agentID := setupRunnerTest(t)

		runner := reconnectingpty.NewRunner(client, reconnectingpty.Config{
			AgentID: agentID,
			Init: codersdk.ReconnectingPTYInit{
				Command: "echo 'hello world'",
			},
			LogOutput: false,
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		logs := bytes.NewBuffer(nil)
		err := runner.Run(ctx, "1", logs)
		logStr := logs.String()
		t.Log("Runner logs:\n\n" + logStr)
		require.NoError(t, err)

		require.NotContains(t, logStr, "Output:")
		require.NotContains(t, logStr, "\thello world")
	})

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()

		t.Run("NoTimeout", func(t *testing.T) {
			t.Parallel()

			client, agentID := setupRunnerTest(t)

			runner := reconnectingpty.NewRunner(client, reconnectingpty.Config{
				AgentID: agentID,
				Init: codersdk.ReconnectingPTYInit{
					Command: "echo 'hello world'",
				},
				Timeout:   httpapi.Duration(5 * time.Second),
				LogOutput: true,
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()

			logs := bytes.NewBuffer(nil)
			err := runner.Run(ctx, "1", logs)
			logStr := logs.String()
			t.Log("Runner logs:\n\n" + logStr)
			require.NoError(t, err)
		})

		t.Run("Timeout", func(t *testing.T) {
			t.Parallel()

			client, agentID := setupRunnerTest(t)

			runner := reconnectingpty.NewRunner(client, reconnectingpty.Config{
				AgentID: agentID,
				Init: codersdk.ReconnectingPTYInit{
					Command: "sleep 5",
				},
				Timeout:   httpapi.Duration(2 * time.Second),
				LogOutput: true,
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()

			logs := bytes.NewBuffer(nil)
			err := runner.Run(ctx, "1", logs)
			logStr := logs.String()
			t.Log("Runner logs:\n\n" + logStr)
			require.Error(t, err)
			require.ErrorIs(t, err, context.DeadlineExceeded)
		})
	})

	t.Run("ExpectTimeout", func(t *testing.T) {
		t.Parallel()

		t.Run("Timeout", func(t *testing.T) {
			t.Parallel()

			client, agentID := setupRunnerTest(t)

			runner := reconnectingpty.NewRunner(client, reconnectingpty.Config{
				AgentID: agentID,
				Init: codersdk.ReconnectingPTYInit{
					Command: "sleep 5",
				},
				Timeout:       httpapi.Duration(2 * time.Second),
				ExpectTimeout: true,
				LogOutput:     true,
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()

			logs := bytes.NewBuffer(nil)
			err := runner.Run(ctx, "1", logs)
			logStr := logs.String()
			t.Log("Runner logs:\n\n" + logStr)
			require.NoError(t, err)
		})

		t.Run("NoTimeout", func(t *testing.T) {
			t.Parallel()

			client, agentID := setupRunnerTest(t)

			runner := reconnectingpty.NewRunner(client, reconnectingpty.Config{
				AgentID: agentID,
				Init: codersdk.ReconnectingPTYInit{
					Command: "echo 'hello world'",
				},
				Timeout:       httpapi.Duration(5 * time.Second),
				ExpectTimeout: true,
				LogOutput:     true,
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()

			logs := bytes.NewBuffer(nil)
			err := runner.Run(ctx, "1", logs)
			logStr := logs.String()
			t.Log("Runner logs:\n\n" + logStr)
			require.Error(t, err)
			require.ErrorContains(t, err, "expected timeout")
		})
	})

	t.Run("ExpectOutput", func(t *testing.T) {
		t.Parallel()

		t.Run("Matches", func(t *testing.T) {
			t.Parallel()

			client, agentID := setupRunnerTest(t)

			runner := reconnectingpty.NewRunner(client, reconnectingpty.Config{
				AgentID: agentID,
				Init: codersdk.ReconnectingPTYInit{
					Command: "echo 'hello world' && sleep 1",
				},
				ExpectOutput: "hello world",
				LogOutput:    false,
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()

			logs := bytes.NewBuffer(nil)
			err := runner.Run(ctx, "1", logs)
			logStr := logs.String()
			t.Log("Runner logs:\n\n" + logStr)
			require.NoError(t, err)
		})

		t.Run("NotMatches", func(t *testing.T) {
			t.Parallel()

			client, agentID := setupRunnerTest(t)

			runner := reconnectingpty.NewRunner(client, reconnectingpty.Config{
				AgentID: agentID,
				Init: codersdk.ReconnectingPTYInit{
					Command: "echo 'hello world' && sleep 1",
				},
				ExpectOutput: "bello borld",
				LogOutput:    false,
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()

			logs := bytes.NewBuffer(nil)
			err := runner.Run(ctx, "1", logs)
			logStr := logs.String()
			t.Log("Runner logs:\n\n" + logStr)
			require.Error(t, err)
			require.ErrorContains(t, err, `expected string "bello borld" not found`)
		})
	})
}

func setupRunnerTest(t *testing.T) (client *codersdk.Client, agentID uuid.UUID) {
	t.Helper()

	client = coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)

	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.ProvisionComplete,
		ProvisionApply: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id:   uuid.NewString(),
							Name: "agent",
							Auth: &proto.Agent_Token{
								Token: authToken,
							},
							Apps: []*proto.App{},
						}},
					}},
				},
			},
		}},
	})

	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	agentClient := codersdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	agentCloser := agent.New(agent.Options{
		Client: agentClient,
		Logger: slogtest.Make(t, nil).Named("agent"),
	})
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})

	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
	return client, resources[0].Agents[0].ID
}
