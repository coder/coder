package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/pty/ptytest"
)

func TestWorkspaceAgent(t *testing.T) {
	t.Parallel()

	t.Run("LogDirectory", func(t *testing.T) {
		t.Parallel()

		authToken := uuid.NewString()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		logDir := t.TempDir()
		inv, _ := clitest.New(t,
			"agent",
			"--auth", "token",
			"--agent-token", authToken,
			"--agent-url", client.URL.String(),
			"--log-dir", logDir,
		)

		pty := ptytest.New(t).Attach(inv)

		clitest.Start(t, inv)
		pty.ExpectMatch("starting agent")

		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		info, err := os.Stat(filepath.Join(logDir, "coder-agent.log"))
		require.NoError(t, err)
		require.Greater(t, info.Size(), int64(0))
	})

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

		inv, _ := clitest.New(t, "agent", "--auth", "azure-instance-identity", "--agent-url", client.URL.String())
		inv = inv.WithContext(
			//nolint:revive,staticcheck
			context.WithValue(inv.Context(), "azure-client", metadataClient),
		)
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		clitest.Start(t, inv)
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

		inv, _ := clitest.New(t, "agent", "--auth", "aws-instance-identity", "--agent-url", client.URL.String())
		inv = inv.WithContext(
			//nolint:revive,staticcheck
			context.WithValue(inv.Context(), "aws-client", metadataClient),
		)
		clitest.Start(t, inv)
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
		workspace, err := client.Workspace(inv.Context(), workspace.ID)
		require.NoError(t, err)
		resources := workspace.LatestBuild.Resources
		if assert.NotEmpty(t, resources) && assert.NotEmpty(t, resources[0].Agents) {
			assert.NotEmpty(t, resources[0].Agents[0].Version)
		}
		dialer, err := client.DialWorkspaceAgent(inv.Context(), resources[0].Agents[0].ID, nil)
		require.NoError(t, err)
		defer dialer.Close()
		require.True(t, dialer.AwaitReachable(context.Background()))
	})

	t.Run("GoogleCloud", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		validator, metadataClient := coderdtest.NewGoogleInstanceIdentity(t, instanceID, false)
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

		inv, cfg := clitest.New(t, "agent", "--auth", "google-instance-identity", "--agent-url", client.URL.String())
		ptytest.New(t).Attach(inv)
		clitest.SetupConfig(t, client, cfg)
		clitest.Start(t,
			inv.WithContext(
				//nolint:revive,staticcheck
				context.WithValue(context.Background(), "gcp-client", metadataClient),
			),
		)

		ctx := inv.Context()

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
	})

	t.Run("PostStartup", func(t *testing.T) {
		t.Parallel()

		authToken := uuid.NewString()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		logDir := t.TempDir()
		inv, _ := clitest.New(t,
			"agent",
			"--auth", "token",
			"--agent-token", authToken,
			"--agent-url", client.URL.String(),
			"--log-dir", logDir,
		)
		// Set the subsystem for the agent.
		inv.Environ.Set(agent.EnvAgentSubsystem, string(codersdk.AgentSubsystemEnvbox))

		pty := ptytest.New(t).Attach(inv)

		clitest.Start(t, inv)
		pty.ExpectMatch("starting agent")

		resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
		require.Len(t, resources, 1)
		require.Len(t, resources[0].Agents, 1)
		require.Equal(t, codersdk.AgentSubsystemEnvbox, resources[0].Agents[0].Subsystem)
	})
}
