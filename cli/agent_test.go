package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestWorkspaceAgent(t *testing.T) {
	t.Parallel()
	t.Run("Azure", func(t *testing.T) {
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

		cmd, _ := clitest.New(t, "agent", "--auth", "azure-instance-identity", "--agent-url", api.Client.URL.String())
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		go func() {
			// A linting error occurs for weakly typing the context value here,
			// but it seems reasonable for a one-off test.
			// nolint
			ctx = context.WithValue(ctx, "azure-client", metadataClient)
			err := cmd.ExecuteContext(ctx)
			require.NoError(t, err)
		}()
		coderdtest.AwaitWorkspaceAgents(t, api.Client, workspace.LatestBuild.ID)
		resources, err := api.Client.WorkspaceResourcesByBuild(ctx, workspace.LatestBuild.ID)
		require.NoError(t, err)
		dialer, err := api.Client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, nil)
		require.NoError(t, err)
		defer dialer.Close()
		_, err = dialer.Ping()
		require.NoError(t, err)
		cancelFunc()
	})

	t.Run("AWS", func(t *testing.T) {
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

		cmd, _ := clitest.New(t, "agent", "--auth", "aws-instance-identity", "--agent-url", api.Client.URL.String())
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		go func() {
			// A linting error occurs for weakly typing the context value here,
			// but it seems reasonable for a one-off test.
			// nolint
			ctx = context.WithValue(ctx, "aws-client", metadataClient)
			err := cmd.ExecuteContext(ctx)
			require.NoError(t, err)
		}()
		coderdtest.AwaitWorkspaceAgents(t, api.Client, workspace.LatestBuild.ID)
		resources, err := api.Client.WorkspaceResourcesByBuild(ctx, workspace.LatestBuild.ID)
		require.NoError(t, err)
		dialer, err := api.Client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, nil)
		require.NoError(t, err)
		defer dialer.Close()
		_, err = dialer.Ping()
		require.NoError(t, err)
		cancelFunc()
	})

	t.Run("GoogleCloud", func(t *testing.T) {
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

		cmd, _ := clitest.New(t, "agent", "--auth", "google-instance-identity", "--agent-url", api.Client.URL.String())
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		go func() {
			// A linting error occurs for weakly typing the context value here,
			// but it seems reasonable for a one-off test.
			// nolint
			ctx = context.WithValue(ctx, "gcp-client", metadata)
			err := cmd.ExecuteContext(ctx)
			require.NoError(t, err)
		}()
		coderdtest.AwaitWorkspaceAgents(t, api.Client, workspace.LatestBuild.ID)
		resources, err := api.Client.WorkspaceResourcesByBuild(ctx, workspace.LatestBuild.ID)
		require.NoError(t, err)
		dialer, err := api.Client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, nil)
		require.NoError(t, err)
		defer dialer.Close()
		_, err = dialer.Ping()
		require.NoError(t, err)
		cancelFunc()
	})
}
