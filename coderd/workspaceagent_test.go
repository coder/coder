package coderd_test

import (
	"context"
	"testing"
	"time"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceAgentServe(t *testing.T) {
	t.Parallel()
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		daemonCloser := coderdtest.NewProvisionerDaemon(t, client)
		authToken := uuid.NewString()
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, &echo.Responses{
			Parse:           echo.ParseComplete,
			ProvisionDryRun: echo.ProvisionComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "example",
							Type: "aws_instance",
							Agent: &proto.Agent{
								Id: uuid.NewString(),
								Auth: &proto.Agent_Token{
									Token: authToken,
								},
							},
						}},
					},
				},
			}},
		})
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		history, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceProvisionJob(t, client, user.Organization, history.ProvisionJobID)
		daemonCloser.Close()

		agentClient := codersdk.New(client.URL)
		agentClient.SessionToken = authToken
		agentCloser := agent.New(agentClient.WorkspaceAgentServe, &peer.ConnOptions{
			Logger: slogtest.Make(t, nil),
		})

		var resources []coderd.ProvisionerJobResource
		require.Eventually(t, func() bool {
			resources, err = client.WorkspaceProvisionJobResources(context.Background(), user.Organization, history.ProvisionJobID)
			require.NoError(t, err)
			require.Len(t, resources, 1)
			return !resources[0].Agent.UpdatedAt.IsZero()
		}, 5*time.Second, 25*time.Millisecond)

		workspaceClient, err := client.WorkspaceAgentConnect(context.Background(), user.Organization, history.ProvisionJobID, resources[0].ID)
		require.NoError(t, err)
		stream, err := workspaceClient.NegotiateConnection(context.Background())
		require.NoError(t, err)
		conn, err := peerbroker.Dial(stream, nil, &peer.ConnOptions{
			Logger: slogtest.Make(t, nil).Named("client").Leveled(slog.LevelDebug),
		})
		require.NoError(t, err)
		_, err = conn.Ping()
		require.NoError(t, err)

		workspaceClient.DRPCConn().Close()
		conn.Close()
		stream.Close()
		agentCloser.Close()
	})
}
