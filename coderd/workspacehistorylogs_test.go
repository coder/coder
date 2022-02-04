package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestWorkspaceHistoryLogs(t *testing.T) {
	t.Parallel()

	setupProjectAndWorkspace := func(t *testing.T, client *codersdk.Client, user coderd.CreateInitialUserRequest) (coderd.Project, coderd.Workspace) {
		project, err := client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "banana",
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
		workspace, err := client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			Name:      "example",
			ProjectID: project.ID,
		})
		require.NoError(t, err)
		return project, workspace
	}

	setupProjectHistory := func(t *testing.T, client *codersdk.Client, user coderd.CreateInitialUserRequest, project coderd.Project, data []byte) coderd.ProjectHistory {
		projectHistory, err := client.CreateProjectHistory(context.Background(), user.Organization, project.Name, coderd.CreateProjectHistoryRequest{
			StorageMethod: database.ProjectStorageMethodInlineArchive,
			StorageSource: data,
		})
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			hist, err := client.ProjectHistory(context.Background(), user.Organization, project.Name, projectHistory.Name)
			require.NoError(t, err)
			return hist.Import.Status.Completed()
		}, 15*time.Second, 50*time.Millisecond)
		return projectHistory
	}

	server := coderdtest.New(t)
	user := server.RandomInitialUser(t)
	_ = server.AddProvisionerd(t)
	project, workspace := setupProjectAndWorkspace(t, server.Client, user)
	data, err := echo.Tar(echo.ParseComplete, []*proto.Provision_Response{{
		Type: &proto.Provision_Response_Log{
			Log: &proto.Log{
				Output: "test",
			},
		},
	}, {
		Type: &proto.Provision_Response_Complete{
			Complete: &proto.Provision_Complete{},
		},
	}})
	require.NoError(t, err)
	projectHistory := setupProjectHistory(t, server.Client, user, project, data)

	workspaceHistory, err := server.Client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
		ProjectHistoryID: projectHistory.ID,
		Transition:       database.WorkspaceTransitionCreate,
	})
	require.NoError(t, err)

	now := database.Now()
	logChan, err := server.Client.FollowWorkspaceHistoryLogsAfter(context.Background(), "", workspace.Name, workspaceHistory.Name, now)
	require.NoError(t, err)

	for {
		log, more := <-logChan
		if !more {
			break
		}
		t.Logf("Output: %s", log.Output)
	}

	t.Run("ReturnAll", func(t *testing.T) {
		t.Parallel()

		_, err = server.Client.WorkspaceHistoryLogs(context.Background(), "", workspace.Name, workspaceHistory.Name)
		require.NoError(t, err)
	})

	t.Run("Between", func(t *testing.T) {
		t.Parallel()

		_, err = server.Client.WorkspaceHistoryLogsBetween(context.Background(), "", workspace.Name, workspaceHistory.Name, time.Time{}, database.Now())
		require.NoError(t, err)
	})
}
