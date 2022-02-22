package coderd_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestWorkspaceAgent(t *testing.T) {
	t.Parallel()
	t.Run("Connect", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		signedKey, keyID, privateKey := createSignedToken(t, instanceID, nil)
		validator := createValidator(t, keyID, privateKey)
		client := coderdtest.New(t, &coderdtest.Options{
			GoogleTokenValidator: validator,
		})
		user := coderdtest.CreateInitialUser(t, client)
		closeDaemon := coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, &echo.Responses{
			Parse: echo.ParseComplete,
			Plan:  echo.PlanComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.ProvisionedResource{{
							Name:       "somename",
							Type:       "someinstance",
							InstanceId: instanceID,
						}},
					},
				},
			}},
		})
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		firstHistory, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceProvisionJob(t, client, user.Organization, firstHistory.ProvisionJobID)

		token, err := client.AuthenticateWorkspaceAgentUsingGoogleCloudIdentity(context.Background(), "", createMetadataClient(signedKey))
		require.NoError(t, err)

		otherClient := codersdk.New(client.URL)
		otherClient.SessionToken = token.SessionToken
		closer := agent.New(otherClient.WorkspaceAgentClient, &agent.Options{
			Logger: slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		})
		t.Cleanup(func() {
			_ = closer.Close()
		})
		closeDaemon.Close()

		var resources []coderd.WorkspaceResource
		require.Eventually(t, func() bool {
			resources, err = client.WorkspaceHistoryResources(context.Background(), "", workspace.Name, "")
			require.NoError(t, err)
			fmt.Printf("%+v\n", resources[0].Agent)
			return !resources[0].Agent.UpdatedAt.IsZero()
		}, 5*time.Second, 25*time.Millisecond)

		conn, err := client.WorkspaceAgentConnect(context.Background(), "", workspace.Name, "", resources[0].ID.String())
		require.NoError(t, err)

		for i := 0; i < 5; i++ {
			latency, err := conn.Ping()
			require.NoError(t, err)
			fmt.Printf("Latency: %d\n", latency.Microseconds())
		}

	})
}
