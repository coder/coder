package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestPostWorkspaceAuthAzureInstanceIdentity(t *testing.T) {
	t.Parallel()
	instanceID := "instanceidentifier"
	certificates, metadataClient := coderdtest.NewAzureInstanceIdentity(t, instanceID)
	api := coderdtest.New(t, &coderdtest.Options{
		AzureCertificates: certificates,
	})
	user := coderdtest.CreateFirstUser(t, api.Client)
	coderdtest.NewProvisionerDaemon(t, api.Client)
	version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		Provision: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "somename",
						Type: "someinstance",
						Agents: []*proto.Agent{{
							Auth: &proto.Agent_InstanceId{
								InstanceId: instanceID,
							},
						}},
					}},
				},
			},
		}},
	})
	template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, api.Client, workspace.LatestBuild.ID)

	api.Client.HTTPClient = metadataClient
	_, err := api.Client.AuthWorkspaceAzureInstanceIdentity(context.Background())
	require.NoError(t, err)
}

func TestPostWorkspaceAuthAWSInstanceIdentity(t *testing.T) {
	t.Parallel()
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		certificates, metadataClient := coderdtest.NewAWSInstanceIdentity(t, instanceID)
		api := coderdtest.New(t, &coderdtest.Options{
			AWSCertificates: certificates,
		})
		user := coderdtest.CreateFirstUser(t, api.Client)
		coderdtest.NewProvisionerDaemon(t, api.Client)
		version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "somename",
							Type: "someinstance",
							Agents: []*proto.Agent{{
								Auth: &proto.Agent_InstanceId{
									InstanceId: instanceID,
								},
							}},
						}},
					},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, api.Client, workspace.LatestBuild.ID)

		api.Client.HTTPClient = metadataClient
		_, err := api.Client.AuthWorkspaceAWSInstanceIdentity(context.Background())
		require.NoError(t, err)
	})
}

func TestPostWorkspaceAuthGoogleInstanceIdentity(t *testing.T) {
	t.Parallel()
	t.Run("Expired", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		validator, metadata := coderdtest.NewGoogleInstanceIdentity(t, instanceID, true)
		api := coderdtest.New(t, &coderdtest.Options{
			GoogleTokenValidator: validator,
		})
		_, err := api.Client.AuthWorkspaceGoogleInstanceIdentity(context.Background(), "", metadata)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("InstanceNotFound", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		validator, metadata := coderdtest.NewGoogleInstanceIdentity(t, instanceID, false)
		api := coderdtest.New(t, &coderdtest.Options{
			GoogleTokenValidator: validator,
		})
		_, err := api.Client.AuthWorkspaceGoogleInstanceIdentity(context.Background(), "", metadata)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		validator, metadata := coderdtest.NewGoogleInstanceIdentity(t, instanceID, false)
		api := coderdtest.New(t, &coderdtest.Options{
			GoogleTokenValidator: validator,
		})
		user := coderdtest.CreateFirstUser(t, api.Client)
		coderdtest.NewProvisionerDaemon(t, api.Client)
		version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "somename",
							Type: "someinstance",
							Agents: []*proto.Agent{{
								Auth: &proto.Agent_InstanceId{
									InstanceId: instanceID,
								},
							}},
						}},
					},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, api.Client, workspace.LatestBuild.ID)

		_, err := api.Client.AuthWorkspaceGoogleInstanceIdentity(context.Background(), "", metadata)
		require.NoError(t, err)
	})
}
