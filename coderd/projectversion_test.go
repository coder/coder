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
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestProjectVersionsByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		versions, err := client.ProjectVersions(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.NotNil(t, versions)
		require.Len(t, versions, 0)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		_ = coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, nil)
		versions, err := client.ProjectVersions(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.Len(t, versions, 1)
	})
}

func TestProjectVersionByOrganizationAndName(t *testing.T) {
	t.Parallel()
	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, nil)
		require.Equal(t, version.Import.Status, coderd.ProvisionerJobStatusPending)
	})
}

func TestPostProjectVersionByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		_ = coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, nil)
	})

	t.Run("InvalidStorage", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		_, err := client.CreateProjectVersion(context.Background(), user.Organization, project.Name, coderd.CreateProjectVersionRequest{
			StorageMethod: database.ProjectStorageMethod("invalid"),
			StorageSource: []byte{},
		})
		require.Error(t, err)
	})
}

func TestProjectVersionParametersByOrganizationAndName(t *testing.T) {
	t.Parallel()
	t.Run("NotImported", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, nil)
		_, err := client.ProjectVersionParameters(context.Background(), user.Organization, project.Name, version.Name)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionRequired, apiErr.StatusCode())
	})

	t.Run("FailedImport", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, &echo.Responses{
			Provision: []*proto.Provision_Response{{}},
		})
		coderdtest.AwaitProjectVersionImported(t, client, user.Organization, project.Name, version.Name)
		_, err := client.ProjectVersionParameters(context.Background(), user.Organization, project.Name, version.Name)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		project := coderdtest.CreateProject(t, client, user.Organization)
		version := coderdtest.CreateProjectVersion(t, client, user.Organization, project.Name, &echo.Responses{
			Parse: []*proto.Parse_Response{{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						ParameterSchemas: []*proto.ParameterSchema{{
							Name: "example",
						}},
					},
				},
			}},
			Provision: echo.ProvisionComplete,
		})
		coderdtest.AwaitProjectVersionImported(t, client, user.Organization, project.Name, version.Name)
		params, err := client.ProjectVersionParameters(context.Background(), user.Organization, project.Name, version.Name)
		require.NoError(t, err)
		require.Len(t, params, 1)
	})
}
