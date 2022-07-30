package coderd_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestTemplateVersion(t *testing.T) {
	t.Parallel()
	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_, err := client.TemplateVersion(context.Background(), version.ID)
		require.NoError(t, err)
	})
}

func TestPostTemplateVersionsByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("InvalidTemplate", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		templateID := uuid.New()
		_, err := client.CreateTemplateVersion(context.Background(), user.OrganizationID, codersdk.CreateTemplateVersionRequest{
			TemplateID:    templateID,
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			StorageSource: "hash",
			Provisioner:   codersdk.ProvisionerTypeEcho,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("FileNotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, err := client.CreateTemplateVersion(context.Background(), user.OrganizationID, codersdk.CreateTemplateVersionRequest{
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			StorageSource: "hash",
			Provisioner:   codersdk.ProvisionerTypeEcho,
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
		_, err = client.CreateTemplateVersion(context.Background(), user.OrganizationID, codersdk.CreateTemplateVersionRequest{
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			StorageSource: file.Hash,
			Provisioner:   codersdk.ProvisionerTypeEcho,
			ParameterValues: []codersdk.CreateParameterRequest{{
				Name:              "example",
				SourceValue:       "value",
				SourceScheme:      codersdk.ParameterSourceSchemeData,
				DestinationScheme: codersdk.ParameterDestinationSchemeProvisionerVariable,
			}},
		})
		require.NoError(t, err)
	})
}

func TestPatchCancelTemplateVersion(t *testing.T) {
	t.Parallel()
	t.Run("AlreadyCompleted", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		err := client.CancelTemplateVersion(context.Background(), version.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})
	t.Run("AlreadyCanceled", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{},
				},
			}},
		})
		require.Eventually(t, func() bool {
			var err error
			version, err = client.TemplateVersion(context.Background(), version.ID)
			if !assert.NoError(t, err) {
				return false
			}
			t.Logf("Status: %s", version.Job.Status)
			return version.Job.Status == codersdk.ProvisionerJobRunning
		}, 5*time.Second, 25*time.Millisecond)
		err := client.CancelTemplateVersion(context.Background(), version.ID)
		require.NoError(t, err)
		err = client.CancelTemplateVersion(context.Background(), version.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
		require.Eventually(t, func() bool {
			var err error
			version, err = client.TemplateVersion(context.Background(), version.ID)
			return assert.NoError(t, err) && version.Job.Status == codersdk.ProvisionerJobFailed
		}, 5*time.Second, 25*time.Millisecond)
	})
	// TODO(Cian): until we are able to test cancellation properly, validating
	// Running -> Canceling is the best we can do for now.
	t.Run("Canceling", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{},
				},
			}},
		})
		require.Eventually(t, func() bool {
			var err error
			version, err = client.TemplateVersion(context.Background(), version.ID)
			if !assert.NoError(t, err) {
				return false
			}
			t.Logf("Status: %s", version.Job.Status)
			return version.Job.Status == codersdk.ProvisionerJobRunning
		}, 5*time.Second, 25*time.Millisecond)
		err := client.CancelTemplateVersion(context.Background(), version.ID)
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			var err error
			version, err = client.TemplateVersion(context.Background(), version.ID)
			return assert.NoError(t, err) &&
				// The job will never actually cancel successfully because it will never send a
				// provision complete response.
				assert.Empty(t, version.Job.Error) &&
				version.Job.Status == codersdk.ProvisionerJobCanceling
		}, 5*time.Second, 25*time.Millisecond)
	})
}

func TestTemplateVersionSchema(t *testing.T) {
	t.Parallel()
	t.Run("ListRunning", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_, err := client.TemplateVersionSchema(context.Background(), version.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
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
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		schemas, err := client.TemplateVersionSchema(context.Background(), version.ID)
		require.NoError(t, err)
		require.NotNil(t, schemas)
		require.Len(t, schemas, 1)
	})
	t.Run("ListContains", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: []*proto.Parse_Response{{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						ParameterSchemas: []*proto.ParameterSchema{{
							Name:                 "example",
							ValidationTypeSystem: proto.ParameterSchema_HCL,
							ValidationValueType:  "string",
							ValidationCondition:  `contains(["first", "second"], var.example)`,
							DefaultDestination: &proto.ParameterDestination{
								Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
							},
						}},
					},
				},
			}},
			Provision: echo.ProvisionComplete,
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		schemas, err := client.TemplateVersionSchema(context.Background(), version.ID)
		require.NoError(t, err)
		require.NotNil(t, schemas)
		require.Len(t, schemas, 1)
		require.Equal(t, []string{"first", "second"}, schemas[0].ValidationContains)
	})
}

func TestTemplateVersionParameters(t *testing.T) {
	t.Parallel()
	t.Run("ListRunning", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_, err := client.TemplateVersionParameters(context.Background(), version.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: []*proto.Parse_Response{{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{
						ParameterSchemas: []*proto.ParameterSchema{
							{
								Name:           "example",
								RedisplayValue: true,
								DefaultSource: &proto.ParameterSource{
									Scheme: proto.ParameterSource_DATA,
									Value:  "hello",
								},
								DefaultDestination: &proto.ParameterDestination{
									Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
								},
							},
							{
								Name:           "abcd",
								RedisplayValue: true,
								DefaultSource: &proto.ParameterSource{
									Scheme: proto.ParameterSource_DATA,
									Value:  "world",
								},
								DefaultDestination: &proto.ParameterDestination{
									Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
								},
							},
						},
					},
				},
			}},
			Provision: echo.ProvisionComplete,
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		params, err := client.TemplateVersionParameters(context.Background(), version.ID)
		require.NoError(t, err)
		require.NotNil(t, params)
		require.Len(t, params, 2)
		require.Equal(t, "hello", params[0].SourceValue)
		require.Equal(t, "world", params[1].SourceValue)
	})
}

func TestTemplateVersionResources(t *testing.T) {
	t.Parallel()
	t.Run("ListRunning", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_, err := client.TemplateVersionResources(context.Background(), version.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "some",
							Type: "example",
							Agents: []*proto.Agent{{
								Id:   "something",
								Auth: &proto.Agent_Token{},
							}},
						}, {
							Name: "another",
							Type: "example",
						}},
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		resources, err := client.TemplateVersionResources(context.Background(), version.ID)
		require.NoError(t, err)
		require.NotNil(t, resources)
		require.Len(t, resources, 4)
		require.Equal(t, "some", resources[0].Name)
		require.Equal(t, "example", resources[0].Type)
		require.Len(t, resources[0].Agents, 1)
	})
}

func TestTemplateVersionLogs(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
	user := coderdtest.CreateFirstUser(t, client)
	before := time.Now()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:           echo.ParseComplete,
		ProvisionDryRun: echo.ProvisionComplete,
		Provision: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Log{
				Log: &proto.Log{
					Level:  proto.LogLevel_INFO,
					Output: "example",
				},
			},
		}, {
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "some",
						Type: "example",
						Agents: []*proto.Agent{{
							Id: "something",
							Auth: &proto.Agent_Token{
								Token: uuid.NewString(),
							},
						}},
					}, {
						Name: "another",
						Type: "example",
					}},
				},
			},
		}},
	})
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	logs, err := client.TemplateVersionLogsAfter(ctx, version.ID, before)
	require.NoError(t, err)
	for {
		_, ok := <-logs
		if !ok {
			return
		}
	}
}

func TestTemplateVersionsByTemplate(t *testing.T) {
	t.Parallel()
	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		versions, err := client.TemplateVersionsByTemplate(context.Background(), codersdk.TemplateVersionsByTemplateRequest{
			TemplateID: template.ID,
		})
		require.NoError(t, err)
		require.Len(t, versions, 1)
	})
}

func TestTemplateVersionByName(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		_, err := client.TemplateVersionByName(context.Background(), template.ID, "nothing")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		_, err := client.TemplateVersionByName(context.Background(), template.ID, version.Name)
		require.NoError(t, err)
	})
}

func TestPatchActiveTemplateVersion(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		err := client.UpdateActiveTemplateVersion(context.Background(), template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: uuid.New(),
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("DoesNotBelong", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		version = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		err := client.UpdateActiveTemplateVersion(context.Background(), template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: version.ID,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		err := client.UpdateActiveTemplateVersion(context.Background(), template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: version.ID,
		})
		require.NoError(t, err)
	})
}

func TestTemplateVersionDryRun(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		resource := &proto.Resource{
			Name: "cool-resource",
			Type: "cool_resource_type",
		}

		client := coderdtest.New(t, &coderdtest.Options{APIRateLimit: -1, IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{
				{
					Type: &proto.Provision_Response_Log{
						Log: &proto.Log{},
					},
				},
				{
					Type: &proto.Provision_Response_Complete{
						Complete: &proto.Provision_Complete{
							Resources: []*proto.Resource{resource},
						},
					},
				},
			},
		})
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		// Create template version dry-run
		after := time.Now()
		job, err := client.CreateTemplateVersionDryRun(ctx, version.ID, codersdk.CreateTemplateVersionDryRunRequest{
			ParameterValues: []codersdk.CreateParameterRequest{},
		})
		require.NoError(t, err)

		// Fetch template version dry-run
		newJob, err := client.TemplateVersionDryRun(ctx, version.ID, job.ID)
		require.NoError(t, err)
		require.Equal(t, job.ID, newJob.ID)

		// Stream logs
		logs, err := client.TemplateVersionDryRunLogsAfter(ctx, version.ID, job.ID, after)
		require.NoError(t, err)

		logsDone := make(chan struct{})
		go func() {
			defer close(logsDone)

			logCount := 0
			for range logs {
				logCount++
			}
			assert.GreaterOrEqual(t, logCount, 1, "unexpected log count")
		}()

		// Wait for the job to complete
		require.Eventually(t, func() bool {
			job, err := client.TemplateVersionDryRun(ctx, version.ID, job.ID)
			return assert.NoError(t, err) && job.Status == codersdk.ProvisionerJobSucceeded
		}, 5*time.Second, 25*time.Millisecond)

		<-logsDone

		resources, err := client.TemplateVersionDryRunResources(ctx, version.ID, job.ID)
		require.NoError(t, err)
		require.Len(t, resources, 1)
		require.Equal(t, resource.Name, resources[0].Name)
		require.Equal(t, resource.Type, resources[0].Type)
	})

	t.Run("ImportNotFinished", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		// This import job will never finish
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{},
				},
			}},
		})

		_, err := client.CreateTemplateVersionDryRun(context.Background(), version.ID, codersdk.CreateTemplateVersionDryRunRequest{
			ParameterValues: []codersdk.CreateParameterRequest{},
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})

	t.Run("Cancel", func(t *testing.T) {
		t.Parallel()

		t.Run("OK", func(t *testing.T) {
			t.Parallel()
			client, closer := coderdtest.NewWithProvisionerCloser(t, nil)
			defer closer.Close()

			user := coderdtest.CreateFirstUser(t, client)

			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
				Parse: echo.ParseComplete,
				Provision: []*proto.Provision_Response{
					{
						Type: &proto.Provision_Response_Log{
							Log: &proto.Log{},
						},
					},
					{
						Type: &proto.Provision_Response_Complete{
							Complete: &proto.Provision_Complete{},
						},
					},
				},
			})

			version = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			require.Equal(t, codersdk.ProvisionerJobSucceeded, version.Job.Status)

			closer.Close()
			// Create the dry-run
			job, err := client.CreateTemplateVersionDryRun(context.Background(), version.ID, codersdk.CreateTemplateVersionDryRunRequest{
				ParameterValues: []codersdk.CreateParameterRequest{},
			})
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				job, err := client.TemplateVersionDryRun(context.Background(), version.ID, job.ID)
				if !assert.NoError(t, err) {
					return false
				}

				t.Logf("Status: %s", job.Status)
				return job.Status == codersdk.ProvisionerJobPending
			}, 5*time.Second, 25*time.Millisecond)

			err = client.CancelTemplateVersionDryRun(context.Background(), version.ID, job.ID)
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				job, err := client.TemplateVersionDryRun(context.Background(), version.ID, job.ID)
				if !assert.NoError(t, err) {
					return false
				}

				t.Logf("Status: %s", job.Status)
				return job.Status == codersdk.ProvisionerJobCanceling
			}, 5*time.Second, 25*time.Millisecond)
		})

		t.Run("AlreadyCompleted", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

			// Create the dry-run
			job, err := client.CreateTemplateVersionDryRun(context.Background(), version.ID, codersdk.CreateTemplateVersionDryRunRequest{
				ParameterValues: []codersdk.CreateParameterRequest{},
			})
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				job, err := client.TemplateVersionDryRun(context.Background(), version.ID, job.ID)
				if !assert.NoError(t, err) {
					return false
				}

				t.Logf("Status: %s", job.Status)
				return job.Status == codersdk.ProvisionerJobSucceeded
			}, 5*time.Second, 25*time.Millisecond)

			err = client.CancelTemplateVersionDryRun(context.Background(), version.ID, job.ID)
			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
		})

		t.Run("AlreadyCanceled", func(t *testing.T) {
			t.Parallel()
			client, closer := coderdtest.NewWithProvisionerCloser(t, nil)
			defer closer.Close()

			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
				Parse: echo.ParseComplete,
				Provision: []*proto.Provision_Response{
					{
						Type: &proto.Provision_Response_Log{
							Log: &proto.Log{},
						},
					},
					{
						Type: &proto.Provision_Response_Complete{
							Complete: &proto.Provision_Complete{},
						},
					},
				},
			})

			version = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			require.Equal(t, codersdk.ProvisionerJobSucceeded, version.Job.Status)

			closer.Close()

			// Create the dry-run
			job, err := client.CreateTemplateVersionDryRun(context.Background(), version.ID, codersdk.CreateTemplateVersionDryRunRequest{
				ParameterValues: []codersdk.CreateParameterRequest{},
			})
			require.NoError(t, err)

			err = client.CancelTemplateVersionDryRun(context.Background(), version.ID, job.ID)
			require.NoError(t, err)

			err = client.CancelTemplateVersionDryRun(context.Background(), version.ID, job.ID)
			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
		})
	})
}

// TestPaginatedTemplateVersions creates a list of template versions and paginate.
func TestPaginatedTemplateVersions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	client := coderdtest.New(t, &coderdtest.Options{APIRateLimit: -1, IncludeProvisionerD: true})
	// Prepare database.
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

	// Populate database with template versions.
	total := 9
	for i := 0; i < total; i++ {
		data, err := echo.Tar(nil)
		require.NoError(t, err)
		file, err := client.Upload(context.Background(), codersdk.ContentTypeTar, data)
		require.NoError(t, err)
		templateVersion, err := client.CreateTemplateVersion(ctx, user.OrganizationID, codersdk.CreateTemplateVersionRequest{
			TemplateID:    template.ID,
			StorageSource: file.Hash,
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			Provisioner:   codersdk.ProvisionerTypeEcho,
		})
		require.NoError(t, err)

		_ = coderdtest.AwaitTemplateVersionJob(t, client, templateVersion.ID)
	}

	templateVersions, err := client.TemplateVersionsByTemplate(ctx,
		codersdk.TemplateVersionsByTemplateRequest{
			TemplateID: template.ID,
		},
	)
	require.NoError(t, err)
	require.Len(t, templateVersions, 10, "wrong number of template versions created")

	type args struct {
		ctx        context.Context
		pagination codersdk.Pagination
	}
	tests := []struct {
		name          string
		args          args
		want          []codersdk.TemplateVersion
		expectedError string
	}{
		{
			name: "Single result",
			args: args{ctx: ctx, pagination: codersdk.Pagination{Limit: 1}},
			want: templateVersions[:1],
		},
		{
			name: "Single result, second page",
			args: args{ctx: ctx, pagination: codersdk.Pagination{Limit: 1, Offset: 1}},
			want: templateVersions[1:2],
		},
		{
			name: "Last two results",
			args: args{ctx: ctx, pagination: codersdk.Pagination{Limit: 2, Offset: 8}},
			want: templateVersions[8:10],
		},
		{
			name: "AfterID returns next two results",
			args: args{ctx: ctx, pagination: codersdk.Pagination{Limit: 2, AfterID: templateVersions[1].ID}},
			want: templateVersions[2:4],
		},
		{
			name: "No result after last AfterID",
			args: args{ctx: ctx, pagination: codersdk.Pagination{Limit: 2, AfterID: templateVersions[9].ID}},
			want: []codersdk.TemplateVersion{},
		},
		{
			name: "No result after last Offset",
			args: args{ctx: ctx, pagination: codersdk.Pagination{Limit: 2, Offset: 10}},
			want: []codersdk.TemplateVersion{},
		},
		{
			name:          "After_id does not exist",
			args:          args{ctx: ctx, pagination: codersdk.Pagination{AfterID: uuid.New()}},
			expectedError: "does not exist",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := client.TemplateVersionsByTemplate(tt.args.ctx, codersdk.TemplateVersionsByTemplateRequest{
				TemplateID: template.ID,
				Pagination: tt.args.pagination,
			})
			if tt.expectedError != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
