package cli_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty/ptytest"
)

func TestWorkspaceAgent(t *testing.T) {
	t.Parallel()

	t.Run("LogDirectory", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		ws, authToken := dbfake.WorkspaceWithAgent(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		})
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
		ctx := inv.Context()
		pty.ExpectMatchContext(ctx, "agent is starting now")

		coderdtest.AwaitWorkspaceAgents(t, client, ws.ID)

		info, err := os.Stat(filepath.Join(logDir, "coder-agent.log"))
		require.NoError(t, err)
		require.Greater(t, info.Size(), int64(0))
	})

	t.Run("Azure", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		certificates, metadataClient := coderdtest.NewAzureInstanceIdentity(t, instanceID)
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			AzureCertificates: certificates,
		})
		user := coderdtest.CreateFirstUser(t, client)
		ws := dbfake.Workspace(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		})
		dbfake.WorkspaceBuild(t, db, ws, database.WorkspaceBuild{}, &proto.Resource{
			Name: "somename",
			Type: "someinstance",
			Agents: []*proto.Agent{{
				Auth: &proto.Agent_InstanceId{
					InstanceId: instanceID,
				},
			}},
		})

		inv, _ := clitest.New(t, "agent", "--auth", "azure-instance-identity", "--agent-url", client.URL.String())
		inv = inv.WithContext(
			//nolint:revive,staticcheck
			context.WithValue(inv.Context(), "azure-client", metadataClient),
		)
		ctx := inv.Context()
		clitest.Start(t, inv)
		coderdtest.AwaitWorkspaceAgents(t, client, ws.ID)
		workspace, err := client.Workspace(ctx, ws.ID)
		require.NoError(t, err)
		resources := workspace.LatestBuild.Resources
		if assert.NotEmpty(t, workspace.LatestBuild.Resources) && assert.NotEmpty(t, resources[0].Agents) {
			assert.NotEmpty(t, resources[0].Agents[0].Version)
		}
		dialer, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, nil)
		require.NoError(t, err)
		defer dialer.Close()
		require.True(t, dialer.AwaitReachable(ctx))
	})

	t.Run("AWS", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		certificates, metadataClient := coderdtest.NewAWSInstanceIdentity(t, instanceID)
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			AWSCertificates: certificates,
		})
		user := coderdtest.CreateFirstUser(t, client)
		ws := dbfake.Workspace(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		})
		dbfake.WorkspaceBuild(t, db, ws, database.WorkspaceBuild{}, &proto.Resource{
			Name: "somename",
			Type: "someinstance",
			Agents: []*proto.Agent{{
				Auth: &proto.Agent_InstanceId{
					InstanceId: instanceID,
				},
			}},
		})

		inv, _ := clitest.New(t, "agent", "--auth", "aws-instance-identity", "--agent-url", client.URL.String())
		inv = inv.WithContext(
			//nolint:revive,staticcheck
			context.WithValue(inv.Context(), "aws-client", metadataClient),
		)
		clitest.Start(t, inv)
		ctx := inv.Context()
		coderdtest.AwaitWorkspaceAgents(t, client, ws.ID)
		workspace, err := client.Workspace(ctx, ws.ID)
		require.NoError(t, err)
		resources := workspace.LatestBuild.Resources
		if assert.NotEmpty(t, resources) && assert.NotEmpty(t, resources[0].Agents) {
			assert.NotEmpty(t, resources[0].Agents[0].Version)
		}
		dialer, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, nil)
		require.NoError(t, err)
		defer dialer.Close()
		require.True(t, dialer.AwaitReachable(ctx))
	})

	t.Run("GoogleCloud", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		validator, metadataClient := coderdtest.NewGoogleInstanceIdentity(t, instanceID, false)
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			GoogleTokenValidator: validator,
		})
		owner := coderdtest.CreateFirstUser(t, client)
		member, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		ws := dbfake.Workspace(t, db, database.Workspace{
			OrganizationID: owner.OrganizationID,
			OwnerID:        memberUser.ID,
		})
		dbfake.WorkspaceBuild(t, db, ws, database.WorkspaceBuild{}, &proto.Resource{
			Name: "somename",
			Type: "someinstance",
			Agents: []*proto.Agent{{
				Auth: &proto.Agent_InstanceId{
					InstanceId: instanceID,
				},
			}},
		})
		inv, cfg := clitest.New(t, "agent", "--auth", "google-instance-identity", "--agent-url", client.URL.String())
		ptytest.New(t).Attach(inv)
		clitest.SetupConfig(t, member, cfg)
		clitest.Start(t,
			inv.WithContext(
				//nolint:revive,staticcheck
				context.WithValue(inv.Context(), "gcp-client", metadataClient),
			),
		)

		ctx := inv.Context()
		coderdtest.AwaitWorkspaceAgents(t, client, ws.ID)
		workspace, err := client.Workspace(ctx, ws.ID)
		require.NoError(t, err)
		resources := workspace.LatestBuild.Resources
		if assert.NotEmpty(t, resources) && assert.NotEmpty(t, resources[0].Agents) {
			assert.NotEmpty(t, resources[0].Agents[0].Version)
		}
		dialer, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, nil)
		require.NoError(t, err)
		defer dialer.Close()
		require.True(t, dialer.AwaitReachable(ctx))
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

		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		ws, authToken := dbfake.WorkspaceWithAgent(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		})

		logDir := t.TempDir()
		inv, _ := clitest.New(t,
			"agent",
			"--auth", "token",
			"--agent-token", authToken,
			"--agent-url", client.URL.String(),
			"--log-dir", logDir,
		)
		// Set the subsystems for the agent.
		inv.Environ.Set(agent.EnvAgentSubsystem, fmt.Sprintf("%s,%s", codersdk.AgentSubsystemExectrace, codersdk.AgentSubsystemEnvbox))

		pty := ptytest.New(t).Attach(inv)

		clitest.Start(t, inv)
		pty.ExpectMatchContext(inv.Context(), "agent is starting now")

		resources := coderdtest.AwaitWorkspaceAgents(t, client, ws.ID)
		require.Len(t, resources, 1)
		require.Len(t, resources[0].Agents, 1)
		require.Len(t, resources[0].Agents[0].Subsystems, 2)
		// Sorted
		require.Equal(t, codersdk.AgentSubsystemEnvbox, resources[0].Agents[0].Subsystems[0])
		require.Equal(t, codersdk.AgentSubsystemExectrace, resources[0].Agents[0].Subsystems[1])
	})
}
