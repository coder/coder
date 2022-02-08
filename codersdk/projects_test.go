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
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.Projects(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_ = coderdtest.CreateInitialUser(t, client)
		_, err := client.Projects(context.Background(), "")
		require.NoError(t, err)
	})
}

func TestProject(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.Project(context.Background(), "", "")
		require.Error(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		_, err := client.Project(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
	})
}

func TestCreateProject(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.CreateProject(context.Background(), "org", coderd.CreateProjectRequest{
			Name:        "something",
			Provisioner: database.ProvisionerTypeEcho,
		})
		require.Error(t, err)
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.CreateProject(t, client, user.Organization)
	})
}

func TestProjectVersions(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.ProjectVersions(context.Background(), "some", "project")
		require.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)

		_, err := client.ProjectVersions(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
	})
}

func TestProjectVersion(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.ProjectVersion(context.Background(), "some", "project", "version")
		require.Error(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, nil)
		_, err := client.ProjectVersion(context.Background(), user.Organization, project.Name, version.Name)
		require.NoError(t, err)
	})
}

func TestCreateProjectVersion(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.CreateProjectVersion(context.Background(), "some", "project", coderd.CreateProjectVersionRequest{})
		require.Error(t, err)
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		_ = coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, nil)
	})
}

func TestProjectVersionParameters(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.ProjectVersionParameters(context.Background(), "some", "project", "version")
		require.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, nil)
		coderdtest.AwaitProjectVersionImported(t, client, user.Organization, project.Name, version.Name)
		_, err := client.ProjectVersionParameters(context.Background(), user.Organization, project.Name, version.Name)
		require.NoError(t, err)
	})
}

func TestProjectParameters(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.ProjectParameters(context.Background(), "some", "project")
		require.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		_, err := client.ProjectParameters(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
	})
}

func TestCreateProjectParameter(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_, err := client.CreateProjectParameter(context.Background(), "some", "project", coderd.CreateParameterValueRequest{})
		require.Error(t, err)
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		_, err := client.CreateProjectParameter(context.Background(), user.Organization, project.Name, coderd.CreateParameterValueRequest{
			Name:              "example",
			SourceValue:       "source-value",
			SourceScheme:      database.ParameterSourceSchemeData,
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			DestinationValue:  "destination-value",
		})
		require.NoError(t, err)
	})
}
