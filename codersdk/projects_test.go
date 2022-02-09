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
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
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
			Name:               "something",
			VersionImportJobID: uuid.New(),
		})
		require.Error(t, err)
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, nil)
		_ = coderdtest.CreateProject(t, client, user.Organization, job.ID)
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
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
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
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		_, err := client.ProjectVersion(context.Background(), user.Organization, project.Name, project.ActiveVersionID.String())
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
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		_, err := client.CreateProjectVersion(context.Background(), user.Organization, project.Name, coderd.CreateProjectVersionRequest{
			ImportJobID: job.ID,
		})
		require.NoError(t, err)
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
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		coderdtest.AwaitProvisionerJob(t, client, user.Organization, job.ID)
		_, err := client.ProjectVersionParameters(context.Background(), user.Organization, project.Name, project.ActiveVersionID.String())
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
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
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
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
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
