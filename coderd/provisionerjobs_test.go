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
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		before := time.Now()
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, &echo.Responses{
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
		logs, err := client.ProjectImportJobLogsAfter(context.Background(), user.Organization, job.ID, before)
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
		client := coderdtest.New(t, nil)
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
		job, err := client.CreateProjectImportJob(context.Background(), user.Organization, coderd.CreateProjectImportJobRequest{
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
		job = coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		values, err := client.ProjectImportJobParameters(context.Background(), user.Organization, job.ID)
		require.NoError(t, err)
		require.Equal(t, "somevalue", values[0].SourceValue)
	})
}

func TestProvisionerJobParametersByID(t *testing.T) {
	t.Parallel()
	t.Run("NotImported", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, nil)
		_, err := client.ProjectImportJobParameters(context.Background(), user.Organization, job.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, &echo.Responses{
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
		job = coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		params, err := client.ProjectImportJobParameters(context.Background(), user.Organization, job.ID)
		require.NoError(t, err)
		require.Len(t, params, 1)
	})

	t.Run("ListNoRedisplay", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, &echo.Responses{
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
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		params, err := client.ProjectImportJobParameters(context.Background(), user.Organization, job.ID)
		require.NoError(t, err)
		require.Len(t, params, 1)
		require.NotNil(t, params[0])
		require.Equal(t, params[0].SourceValue, "")
	})
}

func TestProvisionerJobResourcesByID(t *testing.T) {
	t.Parallel()
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		_ = coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "hello",
							Type: "ec2_instance",
						}},
					},
				},
			}},
		})
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		resources, err := client.ProjectImportJobResources(context.Background(), user.Organization, job.ID)
		require.NoError(t, err)
		// One for start, and one for stop!
		require.Len(t, resources, 2)
	})
}

func TestProvisionerJobLogsByName(t *testing.T) {
	t.Parallel()
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: "log-output",
					},
				},
			}, {
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{},
				},
			}},
		})
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		history, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, history.ProvisionJobID)
		// Return the log after completion!
		logs, err := client.WorkspaceProvisionJobLogsBefore(context.Background(), user.Organization, history.ProvisionJobID, time.Time{})
		require.NoError(t, err)
		require.NotNil(t, logs)
		require.Len(t, logs, 1)
	})

	t.Run("StreamAfterComplete", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: "log-output",
					},
				},
			}, {
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{},
				},
			}},
		})
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		before := time.Now().UTC()
		history, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, history.ProvisionJobID)

		logs, err := client.WorkspaceProvisionJobLogsAfter(context.Background(), user.Organization, history.ProvisionJobID, before)
		require.NoError(t, err)
		log, ok := <-logs
		require.True(t, ok)
		require.Equal(t, "log-output", log.Output)
		// Make sure the channel automatically closes!
		_, ok = <-logs
		require.False(t, ok)
	})

	t.Run("StreamWhileRunning", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateInitialUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		job := coderdtest.CreateProjectImportJob(t, client, user.Organization, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: "log-output",
					},
				},
			}, {
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{},
				},
			}},
		})
		project := coderdtest.CreateProject(t, client, user.Organization, job.ID)
		coderdtest.AwaitProjectImportJob(t, client, user.Organization, job.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		before := database.Now()
		history, err := client.CreateWorkspaceHistory(context.Background(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		logs, err := client.WorkspaceProvisionJobLogsAfter(context.Background(), user.Organization, history.ProvisionJobID, before)
		require.NoError(t, err)
		log := <-logs
		require.Equal(t, "log-output", log.Output)
		// Make sure the channel automatically closes!
		_, ok := <-logs
		require.False(t, ok)
	})
}
