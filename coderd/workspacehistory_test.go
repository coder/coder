package coderd_test

import (
	"archive/tar"
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
)

func TestWorkspaceHistory(t *testing.T) {
	t.Parallel()

	setupProjectAndWorkspace := func(t *testing.T, client *codersdk.Client, user coderd.CreateInitialUserRequest) (coderd.Project, coderd.Workspace) {
		project, err := client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "banana",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		workspace, err := client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			Name:      "example",
			ProjectID: project.ID,
		})
		require.NoError(t, err)
		return project, workspace
	}

	setupProjectHistory := func(t *testing.T, client *codersdk.Client, user coderd.CreateInitialUserRequest, project coderd.Project, files map[string]string) coderd.ProjectHistory {
		var buffer bytes.Buffer
		writer := tar.NewWriter(&buffer)
		for path, content := range files {
			err := writer.WriteHeader(&tar.Header{
				Name: path,
				Size: int64(len(content)),
			})
			require.NoError(t, err)
			_, err = writer.Write([]byte(content))
			require.NoError(t, err)
		}
		err := writer.Flush()
		require.NoError(t, err)

		projectHistory, err := client.CreateProjectHistory(context.Background(), user.Organization, project.Name, coderd.CreateProjectHistoryRequest{
			StorageMethod: database.ProjectStorageMethodInlineArchive,
			StorageSource: buffer.Bytes(),
		})
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			hist, err := client.ProjectHistory(context.Background(), user.Organization, project.Name, projectHistory.Name)
			require.NoError(t, err)
			t.Logf("Import status: %s\n", hist.Import.Status)
			return hist.Import.Status.Completed()
		}, 15*time.Second, 50*time.Millisecond)
		return projectHistory
	}

	t.Run("AllHistory", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_ = server.AddProvisionerd(t)
		project, workspace := setupProjectAndWorkspace(t, server.Client, user)
		history, err := server.Client.ListWorkspaceHistory(context.Background(), "", workspace.Name)
		require.NoError(t, err)
		require.Len(t, history, 0)
		projectVersion := setupProjectHistory(t, server.Client, user, project, map[string]string{
			"example": "file",
		})
		_, err = server.Client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectHistoryID: projectVersion.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)
		history, err = server.Client.ListWorkspaceHistory(context.Background(), "", workspace.Name)
		require.NoError(t, err)
		require.Len(t, history, 1)
	})

	t.Run("LatestHistory", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_ = server.AddProvisionerd(t)
		project, workspace := setupProjectAndWorkspace(t, server.Client, user)
		_, err := server.Client.WorkspaceHistory(context.Background(), "", workspace.Name, "")
		require.Error(t, err)
		projectHistory := setupProjectHistory(t, server.Client, user, project, map[string]string{
			"some": "file",
		})
		_, err = server.Client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectHistoryID: projectHistory.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)
		_, err = server.Client.WorkspaceHistory(context.Background(), "", workspace.Name, "")
		require.NoError(t, err)
	})

	t.Run("CreateHistory", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_ = server.AddProvisionerd(t)
		project, workspace := setupProjectAndWorkspace(t, server.Client, user)
		projectHistory := setupProjectHistory(t, server.Client, user, project, map[string]string{
			"main.tf": `resource "null_resource" "example" {}`,
		})
		_, err := server.Client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectHistoryID: projectHistory.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)

		var workspaceHistory coderd.WorkspaceHistory
		require.Eventually(t, func() bool {
			workspaceHistory, err = server.Client.WorkspaceHistory(context.Background(), "", workspace.Name, "")
			require.NoError(t, err)
			return workspaceHistory.Provision.Status.Completed()
		}, 15*time.Second, 50*time.Millisecond)
		require.Equal(t, "", workspaceHistory.Provision.Error)
		require.Equal(t, coderd.ProvisionerJobStatusSucceeded, workspaceHistory.Provision.Status)
	})

	t.Run("CreateHistoryAlreadyInProgress", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_ = server.AddProvisionerd(t)
		project, workspace := setupProjectAndWorkspace(t, server.Client, user)
		projectHistory := setupProjectHistory(t, server.Client, user, project, map[string]string{
			"some": "content",
		})

		_, err := server.Client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectHistoryID: projectHistory.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.NoError(t, err)

		_, err = server.Client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectHistoryID: projectHistory.ID,
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.Error(t, err)
	})

	t.Run("CreateHistoryInvalidProjectVersion", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_ = server.AddProvisionerd(t)
		_, workspace := setupProjectAndWorkspace(t, server.Client, user)

		_, err := server.Client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectHistoryID: uuid.New(),
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.Error(t, err)
	})
}
