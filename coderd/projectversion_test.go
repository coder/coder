package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestProjectVersionsByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		versions, err := client.ProjectVersions(context.Background(), user.Organization, project.Name)
		require.NoError(t, err)
		require.NotNil(t, versions)
		require.Len(t, versions, 1)
	})
}

func TestProjectVersionByOrganizationAndName(t *testing.T) {
	t.Parallel()
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

func TestPostProjectVersionByOrganization(t *testing.T) {
	t.Parallel()
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

func TestProjectVersionParametersByOrganizationAndName(t *testing.T) {
	t.Parallel()
	t.Run("NotImported", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, nil)
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		_, err := client.ProjectVersionParameters(context.Background(), user.Organization, project.Name, project.ActiveVersionID.String())
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionRequired, apiErr.StatusCode())
	})

	t.Run("FailedImport", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, &echo.Responses{
			Provision: []*proto.Provision_Response{{}},
		})
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		coderdtest.AwaitProvisionerJob(t, client, user.Organization, job.ID)
		_, err := client.ProjectVersionParameters(context.Background(), user.Organization, project.Name, project.ActiveVersionID.String())
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, &echo.Responses{
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
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		coderdtest.AwaitProvisionerJob(t, client, user.Organization, job.ID)
		params, err := client.ProjectVersionParameters(context.Background(), user.Organization, project.Name, project.ActiveVersionID.String())
		require.NoError(t, err)
		require.Len(t, params, 1)
	})
}
