package cli_test

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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
		client := coderdtest.New(t, &coderdtest.Options{
			AzureCertificates:        certificates,
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Provision_Response{{
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
		dialer, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, nil)
		require.NoError(t, err)
		defer dialer.Close()
		require.True(t, dialer.AwaitReachable(context.Background()))
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
			ProvisionApply: []*proto.Provision_Response{{
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
		dialer, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, nil)
		require.NoError(t, err)
		defer dialer.Close()
		require.True(t, dialer.AwaitReachable(context.Background()))
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
			ProvisionApply: []*proto.Provision_Response{{
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
		dialer, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, nil)
		require.NoError(t, err)
		defer dialer.Close()
		require.True(t, dialer.AwaitReachable(context.Background()))
		sshClient, err := dialer.SSHClient(ctx)
		require.NoError(t, err)
		defer sshClient.Close()
		session, err := sshClient.NewSession()
		require.NoError(t, err)
		defer session.Close()
		key := "CODER_AGENT_TOKEN"
		command := "sh -c 'echo $" + key + "'"
		if runtime.GOOS == "windows" {
			command = "cmd.exe /c echo %" + key + "%"
		}
		token, err := session.CombinedOutput(command)
		require.NoError(t, err)
		_, err = uuid.Parse(strings.TrimSpace(string(token)))
		require.NoError(t, err)

		cancelFunc()
		err = <-errC
		require.NoError(t, err)
	})
}
