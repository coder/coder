package codersdk_test

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

	t.Run("UnauthenticatedList", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_, err := server.Client.Projects(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, err := server.Client.Projects(context.Background(), "")
		require.NoError(t, err)
		_, err = server.Client.Projects(context.Background(), user.Organization)
		require.NoError(t, err)
	})

	t.Run("UnauthenticatedCreate", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_, err := server.Client.CreateProject(context.Background(), "", coderd.CreateProjectRequest{})
		require.Error(t, err)
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := server.RandomInitialUser(t)
		_, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        "bananas",
			Provisioner: database.ProvisionerTypeTerraform,
		})
		require.NoError(t, err)
	})
}
