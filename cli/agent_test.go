package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

func TestWorkspaceAgent(t *testing.T) {
	t.Parallel()
	t.Run("Azure", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		certificates, metadataClient := coderdtest.NewAzureInstanceIdentity(t, instanceID)
		client := coderdtest.New(t, &coderdtest.Options{
			AzureCertificates:        certificates,
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
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
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		cmd, _ := clitest.New(t, "agent", "--auth", "azure-instance-identity", "--agent-url", client.URL.String())
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		errC := make(chan error)
		go func() {
			// A linting error occurs for weakly typing the context value here.
			//nolint // The above seems reasonable for a one-off test.
			ctx := context.WithValue(ctx, "azure-client", metadataClient)
			errC <- cmd.ExecuteContext(ctx)
		}()
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
		workspace, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		resources := workspace.LatestBuild.Resources
		if assert.NotEmpty(t, workspace.LatestBuild.Resources) && assert.NotEmpty(t, resources[0].Agents) {
			assert.NotEmpty(t, resources[0].Agents[0].Version)
		}
		dialer, err := client.DialWorkspaceAgentTailnet(ctx, slog.Logger{}, resources[0].Agents[0].ID)
		require.NoError(t, err)
		defer dialer.Close()
		require.Eventually(t, func() bool {
			_, err := dialer.Ping()
			return err == nil
		}, testutil.WaitMedium, testutil.IntervalFast)
		cancelFunc()
		err = <-errC
		require.NoError(t, err)
	})

	t.Run("AWS", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		certificates, metadataClient := coderdtest.NewAWSInstanceIdentity(t, instanceID)
		client := coderdtest.New(t, &coderdtest.Options{
			AWSCertificates:          certificates,
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
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
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		cmd, _ := clitest.New(t, "agent", "--auth", "aws-instance-identity", "--agent-url", client.URL.String())
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		errC := make(chan error)
		go func() {
			// A linting error occurs for weakly typing the context value here.
			//nolint // The above seems reasonable for a one-off test.
			ctx := context.WithValue(ctx, "aws-client", metadataClient)
			errC <- cmd.ExecuteContext(ctx)
		}()
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
		workspace, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		resources := workspace.LatestBuild.Resources
		if assert.NotEmpty(t, resources) && assert.NotEmpty(t, resources[0].Agents) {
			assert.NotEmpty(t, resources[0].Agents[0].Version)
		}
		dialer, err := client.DialWorkspaceAgentTailnet(ctx, slog.Logger{}, resources[0].Agents[0].ID)
		require.NoError(t, err)
		defer dialer.Close()
		require.Eventually(t, func() bool {
			_, err := dialer.Ping()
			return err == nil
		}, testutil.WaitMedium, testutil.IntervalFast)
		cancelFunc()
		err = <-errC
		require.NoError(t, err)
	})

	t.Run("GoogleCloud", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		validator, metadata := coderdtest.NewGoogleInstanceIdentity(t, instanceID, false)
		client := coderdtest.New(t, &coderdtest.Options{
			GoogleTokenValidator:     validator,
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
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
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		cmd, _ := clitest.New(t, "agent", "--auth", "google-instance-identity", "--agent-url", client.URL.String())
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		errC := make(chan error)
		go func() {
			// A linting error occurs for weakly typing the context value here.
			//nolint // The above seems reasonable for a one-off test.
			ctx := context.WithValue(ctx, "gcp-client", metadata)
			errC <- cmd.ExecuteContext(ctx)
		}()
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
		workspace, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		resources := workspace.LatestBuild.Resources
		if assert.NotEmpty(t, resources) && assert.NotEmpty(t, resources[0].Agents) {
			assert.NotEmpty(t, resources[0].Agents[0].Version)
		}
		dialer, err := client.DialWorkspaceAgentTailnet(ctx, slog.Logger{}, resources[0].Agents[0].ID)
		require.NoError(t, err)
		defer dialer.Close()
		require.Eventually(t, func() bool {
			_, err := dialer.Ping()
			return err == nil
		}, testutil.WaitMedium, testutil.IntervalFast)
		cancelFunc()
		err = <-errC
		require.NoError(t, err)
	})
}
