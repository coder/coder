package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestProvisionerJobLogs(t *testing.T) {
	t.Parallel()
	t.Run("StreamAfterComplete", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: "log-output",
					},
				},
			}, {
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{},
				},
			}},
		})
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitProjectVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		before := time.Now().UTC()
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		ctx, cancelFunc := context.WithCancel(context.Background())
		t.Cleanup(cancelFunc)
		logs, err := client.WorkspaceBuildLogsAfter(ctx, workspace.LatestBuild.ID, before)
		require.NoError(t, err)
		log, ok := <-logs
		require.True(t, ok)
		require.Equal(t, "log-output", log.Output)
		// Make sure the channel automatically closes!
		_, ok = <-logs
		require.False(t, ok)
	})

	t.Run("StreamWhileRunning", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: "log-output",
					},
				},
			}, {
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{},
				},
			}},
		})
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitProjectVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		before := database.Now()
		ctx, cancelFunc := context.WithCancel(context.Background())
		t.Cleanup(cancelFunc)
		logs, err := client.WorkspaceBuildLogsAfter(ctx, workspace.LatestBuild.ID, before)
		require.NoError(t, err)
		log := <-logs
		require.Equal(t, "log-output", log.Output)
		_, ok := <-logs
		require.False(t, ok)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: "log-output",
					},
				},
			}, {
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{},
				},
			}},
		})
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitProjectVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		logs, err := client.WorkspaceBuildLogsBefore(context.Background(), workspace.LatestBuild.ID, time.Now())
		require.NoError(t, err)
		require.Len(t, logs, 1)
	})
}
