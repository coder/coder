package coderd_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

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
	r := dbfake.WorkspaceBuild(t, store, database.Workspace{
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
