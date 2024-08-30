package cli_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
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
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceAgent(t *testing.T) {
	t.Parallel()

	t.Run("LogDirectory", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).
			WithAgent().
			Do()
		logDir := t.TempDir()
		inv, _ := clitest.New(t,
			"agent",
			"--auth", "token",
			"--agent-token", r.AgentToken,
			"--agent-url", client.URL.String(),
			"--log-dir", logDir,
		)

		clitest.Start(t, inv)

		coderdtest.AwaitWorkspaceAgents(t, client, r.Workspace.ID)

		require.Eventually(t, func() bool {
			info, err := os.Stat(filepath.Join(logDir, "coder-agent.log"))
			if err != nil {
				return false
			}
			return info.Size() > 0
		}, testutil.WaitLong, testutil.IntervalMedium)
	})

	t.Run("Azure", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		certificates, metadataClient := coderdtest.NewAzureInstanceIdentity(t, instanceID)
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			AzureCertificates: certificates,
		})
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Auth = &proto.Agent_InstanceId{InstanceId: instanceID}
			return agents
		}).Do()

		inv, _ := clitest.New(t, "agent", "--auth", "azure-instance-identity", "--agent-url", client.URL.String())
		inv = inv.WithContext(
			//nolint:revive,staticcheck
			context.WithValue(inv.Context(), "azure-client", metadataClient),
		)

		ctx := inv.Context()
		clitest.Start(t, inv)
		coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).
			MatchResources(matchAgentWithVersion).Wait()
		workspace, err := client.Workspace(ctx, r.Workspace.ID)
		require.NoError(t, err)
		resources := workspace.LatestBuild.Resources
		if assert.NotEmpty(t, workspace.LatestBuild.Resources) && assert.NotEmpty(t, resources[0].Agents) {
			assert.NotEmpty(t, resources[0].Agents[0].Version)
		}
		dialer, err := workspacesdk.New(client).
			DialAgent(ctx, resources[0].Agents[0].ID, nil)
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
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Auth = &proto.Agent_InstanceId{InstanceId: instanceID}
			return agents
		}).Do()

		inv, _ := clitest.New(t, "agent", "--auth", "aws-instance-identity", "--agent-url", client.URL.String())
		inv = inv.WithContext(
			//nolint:revive,staticcheck
			context.WithValue(inv.Context(), "aws-client", metadataClient),
		)

		clitest.Start(t, inv)
		ctx := inv.Context()
		coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).
			MatchResources(matchAgentWithVersion).
			Wait()
		workspace, err := client.Workspace(ctx, r.Workspace.ID)
		require.NoError(t, err)
		resources := workspace.LatestBuild.Resources
		if assert.NotEmpty(t, resources) && assert.NotEmpty(t, resources[0].Agents) {
			assert.NotEmpty(t, resources[0].Agents[0].Version)
		}
		dialer, err := workspacesdk.New(client).
			DialAgent(ctx, resources[0].Agents[0].ID, nil)
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
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: owner.OrganizationID,
			OwnerID:        memberUser.ID,
		}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Auth = &proto.Agent_InstanceId{InstanceId: instanceID}
			return agents
		}).Do()

		inv, cfg := clitest.New(t, "agent", "--auth", "google-instance-identity", "--agent-url", client.URL.String())
		clitest.SetupConfig(t, member, cfg)

		clitest.Start(t,
			inv.WithContext(
				//nolint:revive,staticcheck
				context.WithValue(inv.Context(), "gcp-client", metadataClient),
			),
		)

		ctx := inv.Context()
		coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).
			MatchResources(matchAgentWithVersion).
			Wait()
		workspace, err := client.Workspace(ctx, r.Workspace.ID)
		require.NoError(t, err)
		resources := workspace.LatestBuild.Resources
		if assert.NotEmpty(t, resources) && assert.NotEmpty(t, resources[0].Agents) {
			assert.NotEmpty(t, resources[0].Agents[0].Version)
		}
		dialer, err := workspacesdk.New(client).DialAgent(ctx, resources[0].Agents[0].ID, nil)
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
		r := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()

		logDir := t.TempDir()
		inv, _ := clitest.New(t,
			"agent",
			"--auth", "token",
			"--agent-token", r.AgentToken,
			"--agent-url", client.URL.String(),
			"--log-dir", logDir,
		)
		// Set the subsystems for the agent.
		inv.Environ.Set(agent.EnvAgentSubsystem, fmt.Sprintf("%s,%s", codersdk.AgentSubsystemExectrace, codersdk.AgentSubsystemEnvbox))

		clitest.Start(t, inv)

		resources := coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).
			MatchResources(matchAgentWithSubsystems).Wait()
		require.Len(t, resources, 1)
		require.Len(t, resources[0].Agents, 1)
		require.Len(t, resources[0].Agents[0].Subsystems, 2)
		// Sorted
		require.Equal(t, codersdk.AgentSubsystemEnvbox, resources[0].Agents[0].Subsystems[0])
		require.Equal(t, codersdk.AgentSubsystemExectrace, resources[0].Agents[0].Subsystems[1])
	})
	t.Run("Header", func(t *testing.T) {
		t.Parallel()

		var url string
		var called int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "wow", r.Header.Get("X-Testing"))
			assert.Equal(t, "Ethan was Here!", r.Header.Get("Cool-Header"))
			assert.Equal(t, "very-wow-"+url, r.Header.Get("X-Process-Testing"))
			assert.Equal(t, "more-wow", r.Header.Get("X-Process-Testing2"))
			atomic.AddInt64(&called, 1)
			w.WriteHeader(http.StatusGone)
		}))
		defer srv.Close()
		url = srv.URL
		coderURLEnv := "$CODER_URL"
		if runtime.GOOS == "windows" {
			coderURLEnv = "%CODER_URL%"
		}

		logDir := t.TempDir()
		inv, _ := clitest.New(t,
			"agent",
			"--auth", "token",
			"--agent-token", "fake-token",
			"--agent-url", srv.URL,
			"--log-dir", logDir,
			"--agent-header", "X-Testing=wow",
			"--agent-header", "Cool-Header=Ethan was Here!",
			"--agent-header-command", "printf X-Process-Testing=very-wow-"+coderURLEnv+"'\\r\\n'X-Process-Testing2=more-wow",
		)

		clitest.Start(t, inv)
		require.Eventually(t, func() bool {
			return atomic.LoadInt64(&called) > 0
		}, testutil.WaitShort, testutil.IntervalFast)
	})
}

func matchAgentWithVersion(rs []codersdk.WorkspaceResource) bool {
	if len(rs) < 1 {
		return false
	}
	if len(rs[0].Agents) < 1 {
		return false
	}
	if rs[0].Agents[0].Version == "" {
		return false
	}
	return true
}

func matchAgentWithSubsystems(rs []codersdk.WorkspaceResource) bool {
	if len(rs) < 1 {
		return false
	}
	if len(rs[0].Agents) < 1 {
		return false
	}
	if len(rs[0].Agents[0].Subsystems) < 1 {
		return false
	}
	return true
}
