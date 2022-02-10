package coderd_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestPostProvisionerImportJobByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		before := time.Now()
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, &echo.Responses{
			Parse: []*proto.Parse_Response{{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						ParameterSchemas: []*proto.ParameterSchema{},
					},
				},
			}},
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "dev",
							Type: "ec2_instance",
						}},
					},
				},
			}},
		})
		logs, err := client.FollowProvisionerJobLogsAfter(context.Background(), user.Organization, job.ID, before)
		require.NoError(t, err)
		for {
			log, ok := <-logs
			if !ok {
				break
			}
			t.Log(log.Output)
		}
	})

	t.Run("CreateWithParameters", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		data, err := echo.Tar(&echo.Responses{
			Parse: []*proto.Parse_Response{{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						ParameterSchemas: []*proto.ParameterSchema{{
							Name:           "test",
							RedisplayValue: true,
						}},
					},
				},
			}},
			Provision: echo.ProvisionComplete,
		})
		require.NoError(t, err)
		file, err := client.UploadFile(context.Background(), codersdk.ContentTypeTar, data)
		require.NoError(t, err)
		job, err := client.CreateProjectVersionImportProvisionerJob(context.Background(), user.Organization, coderd.CreateProjectImportJobRequest{
			StorageSource: file.Hash,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Provisioner:   database.ProvisionerTypeEcho,
			ParameterValues: []coderd.CreateParameterValueRequest{{
				Name:              "test",
				SourceValue:       "somevalue",
				SourceScheme:      database.ParameterSourceSchemeData,
				DestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
			}},
		})
		require.NoError(t, err)
		job = coderdtest.AwaitProvisionerJob(t, client, user.Organization, job.ID)
		values, err := client.ProvisionerJobParameterValues(context.Background(), user.Organization, job.ID)
		require.NoError(t, err)
		require.Equal(t, "somevalue", values[0].SourceValue)
	})
}

func TestProvisionerJobParametersByID(t *testing.T) {
	t.Parallel()
	t.Run("NotImported", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportProvisionerJob(t, client, user.Organization, nil)
		_, err := client.ProvisionerJobParameterValues(context.Background(), user.Organization, job.ID)
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
		job = coderdtest.AwaitProvisionerJob(t, client, user.Organization, job.ID)
		params, err := client.ProvisionerJobParameterValues(context.Background(), user.Organization, job.ID)
		require.NoError(t, err)
		require.Len(t, params, 1)
	})

	t.Run("ListNoRedisplay", func(t *testing.T) {
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
							DefaultSource: &proto.ParameterSource{
								Scheme: proto.ParameterSource_DATA,
								Value:  "tomato",
							},
							DefaultDestination: &proto.ParameterDestination{
								Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
							},
							RedisplayValue: false,
						}},
					},
				},
			}},
			Provision: echo.ProvisionComplete,
		})
		coderdtest.AwaitProvisionerJob(t, client, user.Organization, job.ID)
		params, err := client.ProvisionerJobParameterValues(context.Background(), user.Organization, job.ID)
		require.NoError(t, err)
		require.Len(t, params, 1)
		require.NotNil(t, params[0])
		require.Equal(t, params[0].SourceValue, "")
	})
}
