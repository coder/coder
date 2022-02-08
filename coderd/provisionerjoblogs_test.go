package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestProvisionerJobLogsByName(t *testing.T) {
	t.Parallel()
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, &echo.Responses{
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
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		coderdtest.AwaitProvisionerJob(t, client, user.Organization, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		history, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		coderdtest.AwaitProvisionerJob(t, client, user.Organization, history.ProvisionJobID)
		// Return the log after completion!
		logs, err := client.ProvisionerJobLogs(context.Background(), user.Organization, history.ProvisionJobID)
		require.NoError(t, err)
		require.NotNil(t, logs)
		require.Len(t, logs, 1)
	})

	t.Run("StreamAfterComplete", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, &echo.Responses{
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
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		coderdtest.AwaitProvisionerJob(t, client, user.Organization, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		before := time.Now().UTC()
		history, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		coderdtest.AwaitProvisionerJob(t, client, user.Organization, history.ProvisionJobID)

		logs, err := client.FollowProvisionerJobLogsAfter(context.Background(), user.Organization, history.ProvisionJobID, before)
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
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, &echo.Responses{
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
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		coderdtest.AwaitProvisionerJob(t, client, user.Organization, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		before := database.Now()
		history, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		logs, err := client.FollowProvisionerJobLogsAfter(context.Background(), user.Organization, history.ProvisionJobID, before)
		require.NoError(t, err)
		log := <-logs
		require.Equal(t, "log-output", log.Output)
		// Make sure the channel automatically closes!
		_, ok := <-logs
		require.False(t, ok)
	})
}
