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
	t.Run("ListError", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_, err := server.Client.Workspaces(context.Background(), "", "")
		require.Error(t, err)
	})

	t.Run("ListNoOwner", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_, err := server.Client.Workspaces(context.Background(), "", "asd")
		require.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "tomato",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		_, err = server.Client.CreateWorkspace(context.Background(), user.Organization, project.Name, coderd.CreateWorkspaceRequest{
			Name: "wooow",
		})
		require.NoError(t, err)
		_, err = server.Client.Workspaces(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
	})

	t.Run("CreateError", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_, err := server.Client.CreateWorkspace(context.Background(), "no", "nothin", coderd.CreateWorkspaceRequest{})
		require.Error(t, err)
	})
}
