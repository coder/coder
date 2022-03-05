package codersdk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/database"
)

func TestWorkspaces(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.Workspaces(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateInitialUser(t, client)
		_, err := client.Workspaces(context.Background(), "")
		require.NoError(t, err)
	})
}

func TestWorkspacesByProject(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.WorkspacesByProject(context.Background(), "", "")
		require.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		_, err := client.WorkspacesByProject(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
	})
}

func TestWorkspace(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.Workspace(context.Background(), "", "")
		require.Error(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
		_, err := client.Workspace(context.Background(), "", workspace.Name)
		require.NoError(t, err)
	})
}

func TestListWorkspaceBuild(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.ListWorkspaceBuild(context.Background(), "", "")
		require.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
		_, err := client.ListWorkspaceBuild(context.Background(), "", workspace.Name)
		require.NoError(t, err)
	})
}

func TestWorkspaceBuild(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.WorkspaceBuild(context.Background(), "", "", "")
		require.Error(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
		_, err := client.CreateWorkspaceBuild(context.Background(), "", workspace.Name, coderd.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
	})
}

func TestCreateWorkspace(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.CreateWorkspace(context.Background(), "", coderd.CreateWorkspaceRequest{})
		require.Error(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		_ = coderdtest.CreateWorkspace(t, client, "", project.ID)
	})
}

func TestCreateWorkspaceBuild(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.CreateWorkspaceBuild(context.Background(), "", "", coderd.CreateWorkspaceBuildRequest{})
		require.Error(t, err)
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
		_, err := client.CreateWorkspaceBuild(context.Background(), "", workspace.Name, coderd.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
	})
}
