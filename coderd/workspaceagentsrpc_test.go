package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// Ported to RPC API from coderd/workspaceagents_test.go
func TestWorkspaceAgentReportStats(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		apiKeyScope rbac.ScopeName
	}{
		{
			name:        "empty (backwards compat)",
			apiKeyScope: "",
		},
		{
			name:        "all",
			apiKeyScope: rbac.ScopeAll,
		},
		{
			name:        "no_user_data",
			apiKeyScope: rbac.ScopeNoUserData,
		},
		{
			name:        "application_connect",
			apiKeyScope: rbac.ScopeApplicationConnect,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tickCh := make(chan time.Time)
			flushCh := make(chan int, 1)
			client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
				WorkspaceUsageTrackerFlush: flushCh,
				WorkspaceUsageTrackerTick:  tickCh,
			})
			user := coderdtest.CreateFirstUser(t, client)
			r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OrganizationID: user.OrganizationID,
				OwnerID:        user.UserID,
				LastUsedAt:     dbtime.Now().Add(-time.Minute),
			}).WithAgent(
				func(agent []*proto.Agent) []*proto.Agent {
					for _, a := range agent {
						a.ApiKeyScope = string(tc.apiKeyScope)
					}

					return agent
				},
			).Do()

			ac := agentsdk.New(client.URL, agentsdk.WithFixedToken(r.AgentToken))
			conn, err := ac.ConnectRPC(context.Background())
			require.NoError(t, err)
			defer func() {
				_ = conn.Close()
			}()
			agentAPI := agentproto.NewDRPCAgentClient(conn)

			_, err = agentAPI.UpdateStats(context.Background(), &agentproto.UpdateStatsRequest{
				Stats: &agentproto.Stats{
					ConnectionsByProto:          map[string]int64{"TCP": 1},
					ConnectionCount:             1,
					RxPackets:                   1,
					RxBytes:                     1,
					TxPackets:                   1,
					TxBytes:                     1,
					SessionCountVscode:          1,
					SessionCountJetbrains:       0,
					SessionCountReconnectingPty: 0,
					SessionCountSsh:             0,
					ConnectionMedianLatencyMs:   10,
				},
			})
			require.NoError(t, err)

			tickCh <- dbtime.Now()
			count := <-flushCh
			require.Equal(t, 1, count, "expected one flush with one id")

			newWorkspace, err := client.Workspace(context.Background(), r.Workspace.ID)
			require.NoError(t, err)

			assert.True(t,
				newWorkspace.LastUsedAt.After(r.Workspace.LastUsedAt),
				"%s is not after %s", newWorkspace.LastUsedAt, r.Workspace.LastUsedAt,
			)
		})
	}
}

func TestAgentAPI_LargeManifest(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		apiKeyScope rbac.ScopeName
	}{
		{
			name:        "empty (backwards compat)",
			apiKeyScope: "",
		},
		{
			name:        "all",
			apiKeyScope: rbac.ScopeAll,
		},
		{
			name:        "no_user_data",
			apiKeyScope: rbac.ScopeNoUserData,
		},
		{
			name:        "application_connect",
			apiKeyScope: rbac.ScopeApplicationConnect,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			client, store := coderdtest.NewWithDatabase(t, nil)
			adminUser := coderdtest.CreateFirstUser(t, client)
			n := 512000
			longScript := make([]byte, n)
			for i := range longScript {
				longScript[i] = 'q'
			}
			r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
				OrganizationID: adminUser.OrganizationID,
				OwnerID:        adminUser.UserID,
			}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
				agents[0].Scripts = []*proto.Script{
					{
						Script: string(longScript),
					},
				}
				agents[0].ApiKeyScope = string(tc.apiKeyScope)
				return agents
			}).Do()
			ac := agentsdk.New(client.URL, agentsdk.WithFixedToken(r.AgentToken))
			conn, err := ac.ConnectRPC(ctx)
			defer func() {
				_ = conn.Close()
			}()
			require.NoError(t, err)
			agentAPI := agentproto.NewDRPCAgentClient(conn)
			manifest, err := agentAPI.GetManifest(ctx, &agentproto.GetManifestRequest{})
			require.NoError(t, err)
			require.Len(t, manifest.Scripts, 1)
			require.Len(t, manifest.Scripts[0].Script, n)
		})
	}
}

func TestWorkspaceAgentRPCRole(t *testing.T) {
	t.Parallel()

	t.Run("AgentRoleMonitorsConnection", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()

		// Connect with role=agent using ConnectRPCWithRole. This is
		// how the real workspace agent connects.
		ac := agentsdk.New(client.URL, agentsdk.WithFixedToken(r.AgentToken))
		conn, err := ac.ConnectRPCWithRole(ctx, "agent")
		require.NoError(t, err)
		defer func() {
			_ = conn.Close()
		}()

		// The connection monitor updates the database asynchronously,
		// so we need to wait for first_connected_at to be set.
		var agent database.WorkspaceAgent
		require.Eventually(t, func() bool {
			agent, err = db.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), r.Agents[0].ID)
			if err != nil {
				return false
			}
			return agent.FirstConnectedAt.Valid
		}, testutil.WaitShort, testutil.IntervalFast)
		assert.True(t, agent.LastConnectedAt.Valid,
			"last_connected_at should be set for agent role")
	})

	t.Run("NonAgentRoleSkipsMonitoring", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()

		// Connect with a non-agent role using ConnectRPCWithRole.
		// This is how coder-logstream-kube should connect.
		ac := agentsdk.New(client.URL, agentsdk.WithFixedToken(r.AgentToken))
		conn, err := ac.ConnectRPCWithRole(ctx, "logstream-kube")
		require.NoError(t, err)

		// Send a log to confirm the RPC connection is functional.
		agentAPI := agentproto.NewDRPCAgentClient(conn)
		_, err = agentAPI.BatchCreateLogs(ctx, &agentproto.BatchCreateLogsRequest{
			LogSourceId: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		})
		// We don't care about the log source error, just that the
		// RPC is functional.
		_ = err

		// Close the connection and give the server time to process.
		_ = conn.Close()
		time.Sleep(100 * time.Millisecond)

		// Verify that connectivity timestamps were never set.
		agent, err := db.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), r.Agents[0].ID)
		require.NoError(t, err)
		assert.False(t, agent.FirstConnectedAt.Valid,
			"first_connected_at should NOT be set for non-agent role")
		assert.False(t, agent.LastConnectedAt.Valid,
			"last_connected_at should NOT be set for non-agent role")
		assert.False(t, agent.DisconnectedAt.Valid,
			"disconnected_at should NOT be set for non-agent role")
	})

	// NOTE: Backward compatibility (empty role) is implicitly tested by
	// existing tests like TestWorkspaceAgentReportStats which use
	// ConnectRPC() (no role). The server defaults to monitoring when
	// the role query parameter is omitted.
}
