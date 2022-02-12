package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/stretchr/testify/require"
)

func TestPostProjectImportByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("FileNotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_, err := client.CreateProjectImportJob(context.Background(), user.Organization, coderd.CreateProjectImportJobRequest{
			StorageMethod: database.ProvisionerStorageMethodFile,
			StorageSource: "bananas",
			Provisioner:   database.ProvisionerTypeEcho,
		})
		require.Error(t, err)
	})
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
	})
}

func TestProjectImportJobSchemasByID(t *testing.T) {
	t.Parallel()
	t.Run("ListRunning", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		_, err := client.ProjectImportJobSchemas(context.Background(), user.Organization, job.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, &echo.Responses{
			Parse: []*proto.Parse_Response{{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						ParameterSchemas: []*proto.ParameterSchema{{
							Name: "example",
							DefaultDestination: &proto.ParameterDestination{
								Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
							},
						}},
					},
				},
			}},
			Provision: echo.ProvisionComplete,
		})
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		schemas, err := client.ProjectImportJobSchemas(context.Background(), user.Organization, job.ID)
		require.NoError(t, err)
		require.NotNil(t, schemas)
		require.Len(t, schemas, 1)
	})
}

func TestProjectImportJobParametersByID(t *testing.T) {
	t.Parallel()
	t.Run("ListRunning", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		_, err := client.ProjectImportJobSchemas(context.Background(), user.Organization, job.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, &echo.Responses{
			Parse: []*proto.Parse_Response{{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						ParameterSchemas: []*proto.ParameterSchema{{
							Name:           "example",
							RedisplayValue: true,
							DefaultSource: &proto.ParameterSource{
								Scheme: proto.ParameterSource_DATA,
								Value:  "hello",
							},
							DefaultDestination: &proto.ParameterDestination{
								Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
							},
						}},
					},
				},
			}},
			Provision: echo.ProvisionComplete,
		})
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		params, err := client.ProjectImportJobParameters(context.Background(), user.Organization, job.ID)
		require.NoError(t, err)
		require.NotNil(t, params)
		require.Len(t, params, 1)
		require.Equal(t, "hello", params[0].SourceValue)
	})
}

func TestProjectImportJobResourcesByID(t *testing.T) {
	t.Parallel()
	t.Run("ListRunning", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		_, err := client.ProjectImportJobResources(context.Background(), user.Organization, job.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "some",
							Type: "example",
						}},
					},
				},
			}},
		})
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		resources, err := client.ProjectImportJobResources(context.Background(), user.Organization, job.ID)
		require.NoError(t, err)
		require.NotNil(t, resources)
		require.Len(t, resources, 2)
		require.Equal(t, "some", resources[0].Name)
		require.Equal(t, "example", resources[0].Type)
	})
}
