package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestWorkspaceAgent(t *testing.T) {
	t.Parallel()
	t.Run("GoogleCloud", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		validator, metadata := coderdtest.NewGoogleInstanceIdentity(t, instanceID, false)
		client := coderdtest.New(t, &coderdtest.Options{
			GoogleTokenValidator: validator,
		})
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "somename",
							Type: "someinstance",
							Agent: &proto.Agent{
								Auth: &proto.Agent_InstanceId{
									InstanceId: instanceID,
								},
							},
						}},
					},
				},
			}},
		})
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitProjectVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		cmd, _ := clitest.New(t, "workspaces", "agent", "--auth", "google-instance-identity", "--url", client.URL.String())
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		go func() {
			// A linting error occurs for weakly typing the context value here,
			// but it seems reasonable for a one-off test.
			// nolint
			ctx = context.WithValue(ctx, "gcp-client", metadata)
			err := cmd.ExecuteContext(ctx)
			require.NoError(t, err)
		}()
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)
		resources, err := client.WorkspaceResourcesByBuild(ctx, workspace.LatestBuild.ID)
		require.NoError(t, err)
		dialer, err := client.DialWorkspaceAgent(ctx, resources[0].ID, nil, nil)
		require.NoError(t, err)
		defer dialer.Close()
		_, err = dialer.Ping()
		require.NoError(t, err)
		cancelFunc()
	})
}
