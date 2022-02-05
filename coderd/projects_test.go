package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
)

func TestProjects(t *testing.T) {
	t.Parallel()

	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		_ = coderdtest.CreateInitialUser(t, client)
		projects, err := client.Projects(context.Background(), "")
		require.NoError(t, err)
		require.NotNil(t, projects)
		require.Len(t, projects, 0)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.CreateProject(t, client, user.Organization)
		projects, err := client.Projects(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, projects, 1)
	})
}

func TestProjectsByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		projects, err := client.Projects(context.Background(), user.Organization)
		require.NoError(t, err)
		require.NotNil(t, projects)
		require.Len(t, projects, 0)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.CreateProject(t, client, user.Organization)
		projects, err := client.Projects(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, projects, 1)
	})
}

func TestPostProjectsByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.CreateProject(t, client, user.Organization)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		_, err := client.CreateProject(context.Background(), user.Organization, coderd.CreateProjectRequest{
			Name:        project.Name,
			Provisioner: database.ProvisionerTypeEcho,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})
}

func TestProjectByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		_, err := client.Project(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
	})
}

func TestPostParametersByProject(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		_, err := client.CreateProjectParameter(context.Background(), user.Organization, project.Name, coderd.CreateParameterValueRequest{
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
	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		params, err := client.ProjectParameters(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.NotNil(t, params)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		_, err := client.CreateProjectParameter(context.Background(), user.Organization, project.Name, coderd.CreateParameterValueRequest{
			Name:              "example",
			SourceValue:       "source-value",
			SourceScheme:      database.ParameterSourceSchemeData,
			DestinationScheme: database.ParameterDestinationSchemeEnvironmentVariable,
			DestinationValue:  "destination-value",
		})
		require.NoError(t, err)
		params, err := client.ProjectParameters(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.NotNil(t, params)
		require.Len(t, params, 1)
	})
}
