package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/database"
)

func TestProjects(t *testing.T) {
	t.Parallel()

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		_, err = server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.Error(t, err)
	})

	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_ = server.RandomInitialUser(t)
		projects, err := server.Client.Projects(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, projects, 0)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		// Ensure global query works.
		projects, err := server.Client.Projects(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, projects, 1)

		// Ensure specified query works.
		projects, err = server.Client.Projects(context.Background(), user.Organization)
		require.NoError(t, err)
		require.Len(t, projects, 1)
	})

	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)

		projects, err := server.Client.Projects(context.Background(), user.Organization)
		require.NoError(t, err)
		require.Len(t, projects, 0)
	})

	t.Run("Single", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		project, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "someproject",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
		_, err = server.Client.Project(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
	})
}
