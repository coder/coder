package codersdk_test

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

func TestWorkspaces(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.Workspaces(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_ = coderdtest.CreateInitialUser(t, client)
		_, err := client.Workspaces(context.Background(), "")
		require.NoError(t, err)
	})
}

func TestWorkspacesByProject(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.WorkspacesByProject(context.Background(), "", "")
		require.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		_, err := client.WorkspacesByProject(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
	})
}

func TestWorkspace(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.Workspace(context.Background(), "", "")
		require.Error(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
		_, err := client.Workspace(context.Background(), "", workspace.Name)
		require.NoError(t, err)
	})
}

func TestListWorkspaceHistory(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.ListWorkspaceHistory(context.Background(), "", "")
		require.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
		_, err := client.ListWorkspaceHistory(context.Background(), "", workspace.Name)
		require.NoError(t, err)
	})
}

func TestWorkspaceHistory(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.WorkspaceHistory(context.Background(), "", "", "")
		require.Error(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, nil)
		coderdtest.AwaitProjectVersionImported(t, client, user.Organization, project.Name, version.Name)
		workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
		_, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: version.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)
	})
}

func TestCreateWorkspace(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{})
		require.Error(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		_ = coderdtest.CreateWorkspace(t, client, "", project.ID)
	})
}

func TestCreateWorkspaceHistory(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.CreateWorkspaceHistory(context.Background(), "", "", coderd.CreateWorkspaceHistoryRequest{})
		require.Error(t, err)
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, nil)
		coderdtest.AwaitProjectVersionImported(t, client, user.Organization, project.Name, version.Name)
		workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
		_, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: version.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)
	})
}

func TestWorkspaceHistoryLogs(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.WorkspaceHistoryLogs(context.Background(), "", "", "")
		require.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, nil)
		coderdtest.AwaitProjectVersionImported(t, client, user.Organization, project.Name, version.Name)
		workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
		history, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: version.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)
		_, err = client.WorkspaceHistoryLogs(context.Background(), "", workspace.Name, history.Name)
		require.NoError(t, err)
	})
}

func TestFollowWorkspaceHistoryLogsAfter(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.FollowWorkspaceHistoryLogsAfter(context.Background(), "", "", "", time.Time{})
		require.Error(t, err)
	})

	t.Run("Stream", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Output: "hello",
					},
				},
			}, {
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{},
				},
			}},
		})
		coderdtest.AwaitProjectVersionImported(t, client, user.Organization, project.Name, version.Name)
		workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
		after := database.Now()
		history, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: version.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)
		logs, err := client.FollowWorkspaceHistoryLogsAfter(context.Background(), "", workspace.Name, history.Name, after)
		require.NoError(t, err)
		_, ok := <-logs
		require.True(t, ok)
		_, ok = <-logs
		require.False(t, ok)
	})
}
