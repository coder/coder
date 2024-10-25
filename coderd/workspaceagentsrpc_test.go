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
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// Ported to RPC API from coderd/workspaceagents_test.go
func TestWorkspaceAgentReportStats(t *testing.T) {
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
	}).WithAgent().Do()

	ac := agentsdk.New(client.URL)
	ac.SetSessionToken(r.AgentToken)
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
}

func TestAgentAPI_LargeManifest(t *testing.T) {
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
		return agents
	}).Do()
	ac := agentsdk.New(client.URL)
	ac.SetSessionToken(r.AgentToken)
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
}
