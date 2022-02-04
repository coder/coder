package codersdk_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/database"
)

func TestWorkspaces(t *testing.T) {
	t.Parallel()
	t.Run("ListError", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_, err := server.Client.WorkspacesByUser(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("ListNoOwner", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_, err := server.Client.WorkspacesByUser(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("ListByUser", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "tomato",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		_, err = server.Client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			Name:      "wooow",
			ProjectID: project.ID,
		})
		require.NoError(t, err)
		_, err = server.Client.WorkspacesByUser(context.Background(), "me")
		require.NoError(t, err)
	})

	t.Run("ListByProject", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "tomato",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		_, err = server.Client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			Name:      "wooow",
			ProjectID: project.ID,
		})
		require.NoError(t, err)
		_, err = server.Client.WorkspacesByProject(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
	})

	t.Run("ListByProjectError", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_, err := server.Client.WorkspacesByProject(context.Background(), "", "")
		require.Error(t, err)
	})

	t.Run("CreateError", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_, err := server.Client.CreateWorkspace(context.Background(), "no", coderd.CreateWorkspaceRequest{})
		require.Error(t, err)
	})

	t.Run("Single", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "tomato",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		workspace, err := server.Client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			Name:      "wooow",
			ProjectID: project.ID,
		})
		require.NoError(t, err)
		_, err = server.Client.Workspace(context.Background(), "", workspace.Name)
		require.NoError(t, err)
	})

	t.Run("SingleError", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_, err := server.Client.Workspace(context.Background(), "", "blob")
		require.Error(t, err)
	})

	t.Run("History", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "tomato",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		workspace, err := server.Client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			Name:      "wooow",
			ProjectID: project.ID,
		})
		require.NoError(t, err)
		_, err = server.Client.ListWorkspaceHistory(context.Background(), "", workspace.Name)
		require.NoError(t, err)
	})

	t.Run("HistoryError", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_, err := server.Client.ListWorkspaceHistory(context.Background(), "", "blob")
		require.Error(t, err)
	})

	t.Run("LatestHistory", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "tomato",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		workspace, err := server.Client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			Name:      "wooow",
			ProjectID: project.ID,
		})
		require.NoError(t, err)
		_, err = server.Client.WorkspaceHistory(context.Background(), "", workspace.Name, "")
		require.Error(t, err)
	})

	t.Run("CreateHistory", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "tomato",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		workspace, err := server.Client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{
			Name:      "wooow",
			ProjectID: project.ID,
		})
		require.NoError(t, err)
		_, err = server.Client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectHistoryID: uuid.New(),
			Transition:       database.WorkspaceTransitionCreate,
		})
		require.Error(t, err)
	})
}
