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

	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		_ = coderdtest.NewInitialUser(t, server.Client)
		projects, err := server.Client.Projects(context.Background(), "")
		require.NoError(t, err)
		require.NotNil(t, projects)
		require.Len(t, projects, 0)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, server.Client)
		_ = coderdtest.NewProject(t, server.Client, user.Organization)
		projects, err := server.Client.Projects(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, projects, 1)
	})
}

func TestProjectsByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, server.Client)
		projects, err := server.Client.Projects(context.Background(), user.Organization)
		require.NoError(t, err)
		require.NotNil(t, projects)
		require.Len(t, projects, 0)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, server.Client)
		_ = coderdtest.NewProject(t, server.Client, user.Organization)
		projects, err := server.Client.Projects(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, projects, 1)
	})
}

func TestPostProjectsByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, server.Client)
		_ = coderdtest.NewProject(t, server.Client, user.Organization)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, server.Client)
		project := coderdtest.NewProject(t, server.Client, user.Organization)
		_, err := server.Client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        project.Name,
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
	})
}

func TestProjectByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, server.Client)
		project := coderdtest.NewProject(t, server.Client, user.Organization)
		_, err := server.Client.Project(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
	})
}

func TestPostParametersByProject(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, server.Client)
		project := coderdtest.NewProject(t, server.Client, user.Organization)
		_, err := server.Client.CreateProjectParameter(context.Background(), user.Organization, project.Name, coderd.CreateParameterValueRequest{
			Name:              "somename",
			SourceValue:       "tomato",
			SourceScheme:      database.ParameterSourceSchemeData,
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			DestinationValue:  "moo",
		})
		require.NoError(t, err)
	})
}

func TestParametersByProject(t *testing.T) {
	t.Parallel()
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		server := coderdtest.New(t)
		user := coderdtest.NewInitialUser(t, server.Client)
		project := coderdtest.NewProject(t, server.Client, user.Organization)
		params, err := server.Client.ProjectParameters(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.NotNil(t, params)
	})
}
