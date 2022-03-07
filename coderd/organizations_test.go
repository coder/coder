package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
)

func TestProvisionerDaemonsByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("NoAuth", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.ProvisionerDaemonsByOrganization(context.Background(), "someorg")
		require.Error(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, err := client.ProvisionerDaemonsByOrganization(context.Background(), user.OrganizationID)
		require.NoError(t, err)
	})
}

func TestPostProjectVersionsByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("InvalidProject", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		projectID := uuid.New()
		_, err := client.CreateProjectVersion(context.Background(), user.OrganizationID, coderd.CreateProjectVersionRequest{
			ProjectID:     &projectID,
			StorageMethod: database.ProvisionerStorageMethodFile,
			StorageSource: "hash",
			Provisioner:   database.ProvisionerTypeEcho,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("FileNotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, err := client.CreateProjectVersion(context.Background(), user.OrganizationID, coderd.CreateProjectVersionRequest{
			StorageMethod: database.ProvisionerStorageMethodFile,
			StorageSource: "hash",
			Provisioner:   database.ProvisionerTypeEcho,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("WithParameters", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		data, err := echo.Tar(&echo.Responses{
			Parse:           echo.ParseComplete,
			Provision:       echo.ProvisionComplete,
			ProvisionDryRun: echo.ProvisionComplete,
		})
		require.NoError(t, err)
		file, err := client.Upload(context.Background(), codersdk.ContentTypeTar, data)
		require.NoError(t, err)
		_, err = client.CreateProjectVersion(context.Background(), user.OrganizationID, coderd.CreateProjectVersionRequest{
			StorageMethod: database.ProvisionerStorageMethodFile,
			StorageSource: file.Hash,
			Provisioner:   database.ProvisionerTypeEcho,
			ParameterValues: []coderd.CreateParameterRequest{{
				Name:              "example",
				SourceValue:       "value",
				SourceScheme:      database.ParameterSourceSchemeData,
				DestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
			}},
		})
		require.NoError(t, err)
	})
}

func TestPostProjectsByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		_, err := client.CreateProject(context.Background(), user.OrganizationID, coderd.CreateProjectRequest{
			Name:      project.Name,
			VersionID: version.ID,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("NoVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, err := client.CreateProject(context.Background(), user.OrganizationID, coderd.CreateProjectRequest{
			Name:      "test",
			VersionID: uuid.New(),
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})
}

func TestProjectsByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		projects, err := client.ProjectsByOrganization(context.Background(), user.OrganizationID)
		require.NoError(t, err)
		require.NotNil(t, projects)
		require.Len(t, projects, 0)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		projects, err := client.ProjectsByOrganization(context.Background(), user.OrganizationID)
		require.NoError(t, err)
		require.Len(t, projects, 1)
	})
}

func TestProjectByOrganizationAndName(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, err := client.ProjectByName(context.Background(), user.OrganizationID, "something")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, nil)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		_, err := client.ProjectByName(context.Background(), user.OrganizationID, project.Name)
		require.NoError(t, err)
	})
}
