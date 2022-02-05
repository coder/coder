package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
)

func TestPostWorkspaceHistoryByUser(t *testing.T) {
	t.Run("NoVersion", func(t *testing.T) {
		client := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, client)
		project := coderdtest.NewProject(t, client, user.Organization)
		workspace := coderdtest.NewWorkspace(t, client, "me", project.ID)
		_, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: uuid.New(),
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.Error(t, err)
	})

	t.Run("Create", func(t *testing.T) {

	})
}

func TestWorkspaceHistory(t *testing.T) {
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

	setupProjectVersion := func(t *testing.T, client *codersdk.Client, user coderd.CreateInitialUserRequest, project coderd.Project, data []byte) coderd.ProjectVersion {
		projectVersion, err := client.CreateProjectVersion(context.Background(), user.Organization, project.Name, coderd.CreateProjectVersionRequest{
			StorageMethod: database.ProjectStorageMethodInlineArchive,
			StorageSource: data,
		})
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			version, err := client.ProjectVersion(context.Background(), user.Organization, project.Name, projectVersion.Name)
			require.NoError(t, err)
			t.Logf("Import status: %s\n", version.Import.Status)
			return version.Import.Status.Completed()
		}, 15*time.Second, 50*time.Millisecond)
		return projectVersion
	}

	t.Run("AllHistory", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		project, workspace := setupProjectAndWorkspace(t, client, user)
		history, err := client.ListWorkspaceHistory(context.Background(), "", workspace.Name)
		require.NoError(t, err)
		require.Len(t, history, 0)
		data, err := echo.Tar(nil)
		require.NoError(t, err)
		projectVersion := setupProjectVersion(t, client, user, project, data)
		_, err = client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: projectVersion.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)
		history, err = client.ListWorkspaceHistory(context.Background(), "", workspace.Name)
		require.NoError(t, err)
		require.Len(t, history, 1)
	})

	t.Run("LatestHistory", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		project, workspace := setupProjectAndWorkspace(t, client, user)
		_, err := client.WorkspaceHistory(context.Background(), "", workspace.Name, "")
		require.Error(t, err)
		data, err := echo.Tar(nil)
		require.NoError(t, err)
		projectVersion := setupProjectVersion(t, client, user, project, data)
		_, err = client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: projectVersion.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)
		_, err = client.WorkspaceHistory(context.Background(), "", workspace.Name, "")
		require.NoError(t, err)
	})

	t.Run("CreateHistory", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		project, workspace := setupProjectAndWorkspace(t, client, user)
		data, err := echo.Tar(nil)
		require.NoError(t, err)
		projectVersion := setupProjectVersion(t, client, user, project, data)
		_, err = client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: projectVersion.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)

		var workspaceHistory coderd.WorkspaceHistory
		require.Eventually(t, func() bool {
			workspaceHistory, err = client.WorkspaceHistory(context.Background(), "", workspace.Name, "")
			require.NoError(t, err)
			return workspaceHistory.Provision.Status.Completed()
		}, 15*time.Second, 50*time.Millisecond)
		require.Equal(t, "", workspaceHistory.Provision.Error)
		require.Equal(t, coderd.ProvisionerJobStatusSucceeded, workspaceHistory.Provision.Status)
	})

	t.Run("CreateHistoryAlreadyInProgress", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		project, workspace := setupProjectAndWorkspace(t, client, user)
		data, err := echo.Tar(nil)
		require.NoError(t, err)
		projectVersion := setupProjectVersion(t, client, user, project, data)

		_, err = client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: projectVersion.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)

		_, err = client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: projectVersion.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.Error(t, err)
	})

	t.Run("CreateHistoryInvalidProjectVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		_, workspace := setupProjectAndWorkspace(t, client, user)

		_, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: uuid.New(),
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.Error(t, err)
	})
}
