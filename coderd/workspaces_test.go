package coderd_test

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

	t.Run("ListNone", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_ = server.RandomInitialUser(t)
		workspaces, err := server.Client.Workspaces(context.Background(), "", "")
		require.NoError(t, err)
		require.Len(t, workspaces, 0)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "banana",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		_, err = server.Client.CreateWorkspace(context.Background(), user.Organization, project.Name, coderd.CreateWorkspaceRequest{
			Name: "hiii",
		})
		require.NoError(t, err)
		workspaces, err := server.Client.Workspaces(context.Background(), "", "")
		require.NoError(t, err)
		require.Len(t, workspaces, 1)
	})

	t.Run("ListNoneForProject", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "banana",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		workspaces, err := server.Client.Workspaces(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.Len(t, workspaces, 0)
	})

	t.Run("ListForProject", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "banana",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		_, err = server.Client.CreateWorkspace(context.Background(), user.Organization, project.Name, coderd.CreateWorkspaceRequest{
			Name: "hiii",
		})
		require.NoError(t, err)
		workspaces, err := server.Client.Workspaces(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.Len(t, workspaces, 1)
	})

	t.Run("CreateAlreadyExists", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "banana",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		workspace, err := server.Client.CreateWorkspace(context.Background(), user.Organization, project.Name, coderd.CreateWorkspaceRequest{
			Name: "wooow",
		})
		require.NoError(t, err)
		_, err = server.Client.CreateWorkspace(context.Background(), user.Organization, project.Name, coderd.CreateWorkspaceRequest{
			Name: workspace.Name,
		})
		require.Error(t, err)
	})
}

// /workspaces/<organization>/<project>/<name>

// /workspaces/<organization>/<project>

// /workspaces/<organization>/<project>/<name>

// /workspaces/<organization>/<project>
// This leans well towards collaboration!

// /workspaces/<organization>
// This leans well towards collaboration too!
